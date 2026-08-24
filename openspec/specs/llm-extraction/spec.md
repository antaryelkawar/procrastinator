# llm-extraction

## Purpose

Define the interface and requirements for document classification and structured field extraction using an external LLM, ensuring robust configuration, standardized request/response formats, and graceful error handling.

## Requirements

### Requirement: BYO LLM configuration

The LLM client SHALL be configured exclusively through environment variables: `LM_LLM_BASE_URL` (base URL of an OpenAI-compatible endpoint), `LM_LLM_API_KEY`, and `LM_LLM_MODEL`. The application SHALL fail fast at startup with a clear error if any of these variables is unset or empty. No LLM endpoint, model, or credential SHALL be hardcoded.

#### Scenario: Startup fails without LLM configuration

- **WHEN** the application starts with `LM_LLM_BASE_URL` unset
- **THEN** startup aborts with an error naming the missing variable

#### Scenario: Client targets the configured endpoint

- **WHEN** the application is configured with `LM_LLM_BASE_URL` pointing at a given HTTP server
- **THEN** all extraction requests are sent to that server, with the configured model name and an `Authorization: Bearer` header carrying the configured API key

### Requirement: OpenAI-compatible extraction request

For each uploaded document the system SHALL issue one extraction request to `{LM_LLM_BASE_URL}/chat/completions` using the configured model, requesting a structured JSON response that contains the document classification and the extracted fields in a single response.

#### Scenario: Request conforms to the OpenAI chat completions shape

- **WHEN** the system sends an extraction request
- **THEN** the request is an HTTP POST to `{base}/chat/completions` with a JSON body containing the configured `model`, the document content in the messages, and a structured-output/JSON response format directive

### Requirement: Document classification

The extraction result SHALL classify the document as exactly one of `invoice`, `warranty`, or `other`. If the LLM returns a classification outside this enumeration or omits it, the system SHALL treat the document as `other`.

#### Scenario: Known classification is used

- **WHEN** the LLM returns classification `warranty`
- **THEN** the resulting Document is typed `warranty`

#### Scenario: Unknown classification degrades to other

- **WHEN** the LLM returns classification `receipt` (not in the enumeration)
- **THEN** the document is treated as type `other`

### Requirement: Structured field extraction

The extraction result SHALL support the fields: `brand`, `model`, `serial_number`, `purchase_date`, `price`, `currency`, `warranty_start`, and `warranty_end`. Any field the LLM cannot determine SHALL be null/absent rather than guessed. Extracted dates SHALL parse as ISO 8601 calendar dates; extracted price SHALL parse as an exact decimal with an ISO 4217 currency code. Fields that fail validation SHALL be treated as absent.

#### Scenario: Full extraction populates all fields

- **WHEN** the LLM returns valid values for all supported fields
- **THEN** all fields are available to identity resolution and asset population with dates and price parsed into their typed forms

#### Scenario: Unparseable field is treated as absent

- **WHEN** the LLM returns `purchase_date` as `"sometime last year"`
- **THEN** the `purchase_date` field is treated as absent while other valid fields are still used

### Requirement: Extraction failure handling

The system SHALL enforce a bounded timeout on every LLM request. An unreachable endpoint, timeout, non-2xx response, or a response body that does not contain parseable extraction JSON SHALL be surfaced as an extraction failure to the caller; the system SHALL NOT silently fall back to fabricated field values.

#### Scenario: Timeout is enforced

- **WHEN** the LLM endpoint does not respond within the configured timeout
- **THEN** the request is aborted and an extraction failure is reported

#### Scenario: Malformed LLM JSON is an extraction failure

- **WHEN** the LLM returns a 200 response whose body is not valid extraction JSON
- **THEN** an extraction failure is reported and no field values are produced

### Requirement: LLM is not canonical truth

The LLM SHALL function only as an interpreter of source documents. Only validated extraction fields SHALL be written to canonical Asset data, and the raw LLM payload SHALL be retained on the Document record for provenance.

#### Scenario: Raw payload retained for provenance

- **WHEN** an extraction succeeds
- **THEN** the unmodified LLM extraction payload is stored on the resulting Document record alongside the validated fields
