## ADDED Requirements

### Requirement: Intrinsic asset category
Every asset SHALL have an `asset_category` describing what the asset *is*, independent of any document type. The initial vocabulary SHALL include at least: `appliance`, `electronics`, `computing`, `furniture`, `vehicle`, `tool`, `clothing`, `document_only`, and `other`. Category assignment SHALL carry a `category_confidence` in [0.0, 1.0]. The document classification (invoice/warranty/amc/receipt/statement/other) SHALL NOT be stored as the asset's type.

#### Scenario: Cabinet purchase categorized as furniture or computing accessory
- **WHEN** a receipt for "CABINET COOLER MASTER CD600 BLACK" is processed
- **THEN** the created asset has an intrinsic `asset_category` (not "invoice") with a confidence value
- **AND** no field on the asset is named or behaves as a document type

#### Scenario: Microwave from AMC contract
- **WHEN** the IFB AMC contract for "MICRO WAVE OVEN CONVECTION 30BRC2" is processed
- **THEN** the asset category is `appliance`
- **AND** the AMC document is linked to the asset with document classification `amc`

### Requirement: Canonical product name separate from brand and model
Every asset SHALL carry a human-readable canonical `name` describing what the item is (e.g. "Microwave Oven", "Cabinet"), distinct from `brand` and `model`. When a document presents a combined product description string, the pipeline SHALL split it into its parts — brand (from the known-brand lexicon), canonical product name, and model identifier — rather than storing the whole string as the model. The asset detail UI SHALL display name, brand, and model as distinct fields.

#### Scenario: Microwave oven description is split
- **WHEN** a document contains "MICRO WAVE OVEN CONVECTION 30BRC2"
- **THEN** the resulting asset has canonical name "Microwave Oven" (or close equivalent), model "30BRC2"
- **AND** the model field does NOT contain the full description string

#### Scenario: Cabinet description is split into brand, name, and model
- **WHEN** a receipt contains "CABINET COOLER MASTER CD600 BLACK"
- **THEN** the resulting asset has brand "Cooler Master", canonical name "Cabinet", model "CD600"

#### Scenario: Asset detail shows what the item is
- **GIVEN** an asset created from "MICRO WAVE OVEN CONVECTION 30BRC2"
- **WHEN** the user opens the asset detail page
- **THEN** the page shows the item is a microwave oven (name), alongside brand and model as separate fields

### Requirement: Category inference and correction
Category SHALL be inferred during extraction consensus from brand, model, and document content. Users SHALL be able to correct an asset's category via the API/UI; user-corrected categories SHALL take precedence over inferred ones and SHALL NOT be overwritten by subsequent document processing.

#### Scenario: User corrects a wrong category
- **GIVEN** asset A was auto-categorized as `other`
- **WHEN** the user sets category to `electronics`
- **THEN** A keeps `electronics` even after later documents are linked to it

### Requirement: Document classification vocabulary
The document classification vocabulary SHALL be extended to at least `invoice`, `receipt`, `warranty`, `amc`, `statement`, and `other`. Classification remains a property of the document record and is shown alongside linked documents.

#### Scenario: Statement classified as statement
- **WHEN** a bank statement PDF is processed through the unified add flow
- **THEN** its document classification is `statement`
- **AND** it is routed to ledger import rather than asset creation
