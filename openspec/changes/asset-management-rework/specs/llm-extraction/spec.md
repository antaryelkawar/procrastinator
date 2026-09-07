## MODIFIED Requirements

### Requirement: OpenAI-compatible extraction request

For each uploaded document the system SHALL issue extraction requests to `{LM_LLM_BASE_URL}/chat/completions` using the configured model(s), requesting a structured JSON response that contains the document classification and the extracted fields. The processing module SHALL issue at least two such requests in parallel (one per configured extraction worker/model) for the same document and SHALL combine them via consensus; each individual request conforms to the OpenAI chat-completions shape.

(Previously: "For each uploaded document the system SHALL issue one extraction request to `{LM_LLM_BASE_URL}/chat/completions` using the configured model, requesting a structured JSON response that contains the document classification and the extracted fields in a single response.")

#### Scenario: Request conforms to the OpenAI chat completions shape

- **WHEN** the system sends an extraction request
- **THEN** the request is an HTTP POST to `{base}/chat/completions` with a JSON body containing the configured `model`, the document content in the messages, and a structured-output/JSON response format directive

#### Scenario: Parallel workers each issue conforming requests

- **WHEN** a document is processed with two configured extraction workers
- **THEN** each worker issues its own POST to `{base}/chat/completions` and the pipeline merges their responses through consensus

#### Scenario: Parallel extraction is mandatory, not optional

- **WHEN** a document is processed under the default configuration
- **THEN** at least two independent extraction requests are issued for that document before any canonical write
- **AND** a single-request (non-parallel) processing path does not exist outside of an explicit degraded-mode configuration

### Requirement: Document classification

The extraction result SHALL classify the document as exactly one of `invoice`, `receipt`, `warranty`, `amc`, `statement`, or `other`. If the LLM returns a classification outside this enumeration or omits it, the system SHALL treat the document as `other`.

(Previously: "The extraction result SHALL classify the document as exactly one of `invoice`, `warranty`, or `other`.")

#### Scenario: Known classification is used

- **WHEN** the LLM returns classification `warranty`
- **THEN** the resulting Document is classified `warranty`

#### Scenario: Bank statement classified as statement

- **WHEN** an uploaded bank statement is classified
- **THEN** the classification is `statement` and the content is routed to the ledger import pipeline

#### Scenario: Unknown classification degrades to other

- **WHEN** the LLM returns classification `contract` (not in the enumeration)
- **THEN** the document is treated as classification `other`

### Requirement: Structured field extraction

The extraction result SHALL support the fields: `brand`, **`name` (canonical product name — what the item is, e.g. "Microwave Oven")**, `model`, `serial_number`, `purchase_date`, `price`, `currency`, `warranty_start`, `warranty_end`, **`warranty_duration` (a natural-language or ISO-8601 duration as written, e.g. "2 years"), and `asset_category`**. When a document contains a combined product description string, the extraction SHOULD return the split parts (brand, name, model) rather than the whole string as model. Any field the LLM cannot determine SHALL be null/absent rather than guessed. Extracted dates SHALL parse as ISO 8601 calendar dates; extracted price SHALL parse as an exact decimal with an ISO 4217 currency code. Fields that fail validation SHALL be treated as absent. When `warranty_duration` is present with a `purchase_date`, the pipeline SHALL compute `warranty_end` deterministically as `purchase_date + warranty_duration`; an explicitly extracted `warranty_end` takes precedence over the computed value.

(Previously: "The extraction result SHALL support the fields: `brand`, `model`, `serial_number`, `purchase_date`, `price`, `currency`, `warranty_start`, and `warranty_end`." — no `name`, no `warranty_duration`, no `asset_category`, and no deterministic duration computation; all date arithmetic was left to the model.)

#### Scenario: Full extraction populates all fields

- **WHEN** the LLM returns valid values for all supported fields
- **THEN** all fields are available to identity resolution and asset population with dates and price parsed into their typed forms

#### Scenario: Warranty duration is computed in code

- **WHEN** extraction yields `purchase_date` `2026-01-15` and `warranty_duration` `2 years` with no explicit `warranty_end`
- **THEN** the resulting Asset's `warranty_end` is `2028-01-15`

#### Scenario: Unparseable field is treated as absent

- **WHEN** the LLM returns `purchase_date` as `"sometime last year"`
- **THEN** the `purchase_date` field is treated as absent while other valid fields are still used
