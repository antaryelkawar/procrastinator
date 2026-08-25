# Spec: llm-extraction

Delta for change `invoice-warranty-asset-flow`. Modifies the archived baseline in `openspec/specs/llm-extraction`.

## MODIFIED Requirements

### Requirement: BYO LLM configuration

The LLM client SHALL be configured exclusively through environment variables: `PROCRASTINATOR_LLM_BASE_URL` (base URL of an OpenAI-compatible endpoint), `PROCRASTINATOR_LLM_API_KEY`, and `PROCRASTINATOR_LLM_MODEL`. The application SHALL fail fast at startup with a clear error if any of these variables is unset or empty. No LLM endpoint, model, or credential SHALL be hardcoded.

#### Scenario: Startup fails without LLM configuration

- **WHEN** the application starts with `PROCRASTINATOR_LLM_BASE_URL` unset
- **THEN** startup aborts with an error naming the missing variable

#### Scenario: Client targets the configured endpoint

- **WHEN** the application is configured with `PROCRASTINATOR_LLM_BASE_URL` pointing at a given HTTP server
- **THEN** all extraction requests are sent to that server, with the configured model name and an `Authorization: Bearer` header carrying the configured API key

### Requirement: Document classification

The extraction result SHALL classify the document as exactly one of `invoice`, `warranty`, `amc`, or `other`. If the LLM returns a classification outside this enumeration or omits it, the system SHALL treat the document as `other`. Classification SHALL be document-type-agnostic: the same prompt handles invoices, warranty cards, AMC contracts, and unrecognized documents.

#### Scenario: AMC contract is classified

- **WHEN** the LLM returns classification `amc` for an Annual Maintenance Contract document
- **THEN** the resulting Document is typed `amc`

#### Scenario: Unknown classification degrades to other

- **WHEN** the LLM returns classification `receipt` (not in the enumeration)
- **THEN** the document is treated as type `other`

### Requirement: Structured field extraction

The extraction result SHALL support the structured core fields: `document_type`, `brand`, `model`, `serial_number`, `purchase_date`, `warranty_end`, `price`, and `currency`. Any core field the LLM cannot determine SHALL be null/absent rather than guessed. The LLM SHALL be instructed to emit dates in ISO 8601 (`YYYY-MM-DD`), normalizing source formats such as `DD-MMM-YY`, `DD.MM.YYYY`, and `DD-MMM-YYYY`; the parser SHALL additionally accept those source formats as a safety net. Extracted price SHALL parse as an exact decimal with an ISO 4217 currency code (e.g., `INR`). Fields that fail validation SHALL be treated as absent.

#### Scenario: Indian invoice date formats are normalized

- **WHEN** a document shows the purchase date as `12-Jan-24` or `12.01.2024`
- **THEN** the extraction yields a `purchase_date` that parses to 2024-01-12

#### Scenario: Unparseable field is treated as absent

- **WHEN** the LLM returns `purchase_date` as `"sometime last year"`
- **THEN** the `purchase_date` field is treated as absent while other valid fields are still used

## ADDED Requirements

### Requirement: Thinking-free LLM responses

Every extraction request SHALL use a system prompt that instructs the model to act as a strict JSON extraction engine outputting only a single valid JSON object — explicitly forbidding thinking, `<thought>` tags, and explanations. As a safety net, the extraction parser SHALL strip any `<thought>...</thought>` blocks that still appear before JSON parsing. No provider API parameter SHALL be relied upon to disable thinking.

#### Scenario: System prompt forbids thought output

- **WHEN** the system sends an extraction request
- **THEN** the system message instructs the model to output only valid JSON with no thinking and no `<thought>` tags

#### Scenario: Thought blocks are still stripped if emitted

- **WHEN** the LLM nonetheless returns `<thought>reasoning</thought>` followed by the JSON object
- **THEN** the parser strips the thought block and parses the JSON successfully

### Requirement: Generic metadata extraction

In addition to the structured core, the LLM SHALL populate a `metadata` object with any other useful document data as snake_case keys mapped to JSON values — for example `invoice_number`, `amc_card_number`, `amc_type`, `amc_start`, `icr_number`, `customer_name`, `customer_phone`, `service_branch`, `cgst_rate`, `cgst_amount`, `sgst_rate`, `sgst_amount`, `igst_rate`, `cess`, `total_tax`. The system SHALL retain metadata keys as provided (snake_case validated), SHALL NOT interpret metadata for identity resolution, and SHALL store the metadata object on both the Document's extracted fields and the owning Asset.

#### Scenario: AMC fields land in metadata

- **WHEN** the LLM processes an AMC contract showing `AMC Card No: LG2401` and `ICR No: 9982`
- **THEN** the extraction's metadata contains `amc_card_number` and `icr_number`

#### Scenario: Indian tax breakup lands in metadata

- **WHEN** the LLM processes an invoice showing CGST 9% ₹36.00 and SGST 9% ₹36.00
- **THEN** the extraction's metadata contains the CGST and SGST rates and amounts as snake_case keys

#### Scenario: Malformed metadata keys are dropped

- **WHEN** the LLM returns a metadata key that is not snake_case (e.g., `"AMC Card No"`)
- **THEN** that entry is dropped while valid snake_case entries are retained
