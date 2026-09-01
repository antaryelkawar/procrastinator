# Life Manager — Design Document

> Living design document. This records decisions we have explicitly made during the design discussion, along with open questions. It should evolve as the discussion continues.

## 1. Vision

Life Manager is a self-hosted system for managing a home and life as an interconnected whole.

Life contains multiple facets — finances, inventory, appliances, warranties, maintenance, reconciliation, tasks, recipes, documents, people and relationships, debts, and more. The central problem is that these facets affect one another, while conventional applications isolate them into separate systems.

Life Manager aims to reduce that friction by providing a coherent system with multiple deployable and configurable facets that operate over shared underlying data.

The initial goal is personal use. Business viability is a later consideration, not a constraint on the initial design.

## 2. Core Design Principle

**Everything is independently meaningful, but can participate in multiple systems.**

Life Manager should have a central model of real-world entities and events, with optional relationships and capabilities rather than one giant universal entity.

Examples:

### Grocery item

- Inventory quantity
- Purchase
- Financial transaction
- Recipe usage
- Consumption pattern
- Shopping requirement

### Appliance

- Asset
- Purchase
- Financial transaction
- Warranty
- Maintenance
- Documents
- Serial number / identifying information

### Restaurant meal

- Financial transaction
- Optional receipt/document
- Optional people involved
- No inventory or warranty requirement

### Person

- Identity/contact information
- Relationship to household members
- Debts owed to/from them
- Shared transactions
- Task assignment
- Other interactions as applicable

A facet should not necessarily own the underlying fact. Instead, facets should contribute capabilities, views, workflows, and relationships around shared data.

## 3. Source of Truth

The current direction is:

**Central canonical data model + typed, directional relationships.**

A transaction, purchase, appliance, person, document, inventory holding, debt, etc. should exist as coherent objects in the common Life Manager model where appropriate, rather than being independently owned by isolated applications.

The same underlying object can therefore participate in multiple facets.

## 4. Facts, Relationships, Impacts, and State

The current conceptual model is:

**Inputs → Processing → Facts → Relationships → Impacts**

An event is useful as a user-visible record of something that happened, but Life Manager should not assume that every domain needs full event sourcing. What matters is that the system can explain how a source fact led to downstream consequences.

Example:

`Receipt photo`
→ purchase fact
→ linked to rice inventory + financial transaction
→ impacts inventory quantity and spending

Current direction for state:

- Underlying facts and relationships are the authoritative information.
- Current state may initially be derived from those facts.
- Materialized/current state may be introduced where it improves performance or maintainability.
- Materialized state should be recoverable from the underlying facts and relationships where practical.

This is intentionally a **hybrid rather than a blanket event-sourcing requirement**. Maintainability is more important than architectural purity.

## 4.1 Events and Corrections

Users should be able to see meaningful events/source records and correct them without manipulating low-level relationships directly.

A correction should preserve audit/history and allow Life Manager to identify which relationships and impacts were produced by the corrected information so that downstream state can be reconciled.

The exact correction mechanism is unresolved.

## 5. People and Relationships

**People and relationships are a first-class facet of life.**

They are not mandatory for every object, but they should be available throughout the system wherever relevant.

Examples:

- A debt can be associated with a person.
- A shared purchase can involve multiple people.
- A task can be assigned to a person.
- A household can contain multiple people.
- A maintenance interaction can involve a contractor.
- A document can be related to a person.
- A transaction can involve other people.

The model should therefore support people and relationships as first-class entities without requiring every record to have a person attached to it.

## 6. Debts and Receivables

Life Manager must support money owed **to** and **from** other people, including friends and other personal relationships.

Initial conceptual model:

`Person → Debt → Transaction/Payment → Settlement`

The system should eventually be able to represent:

- Money someone owes me
- Money I owe someone
- Partial repayments
- Multiple repayments
- Shared expenses
- Settlements
- Reconciliation of debts against actual payments/transactions

Debts should integrate with the financial system rather than being a completely separate ledger.

## 7. Intended System Shape

Life Manager is **not intended to be a general community plugin marketplace/platform**.

The initial system should consist of carefully designed, hand-crafted facets that share the Life Manager data model and infrastructure.

Facets should be deployable/configurable as needed, while remaining coherent with the rest of the system.

Potential facets include:

- Finance
- Inventory
- Grocery / shopping
- Recipes / food
- Appliances / assets
- Warranty
- Maintenance
- Tasks
- Reconciliation
- Documents
- People / relationships
- Debts
- Search
- Notifications

This list is exploratory, not a commitment to an MVP scope.

## 8. Data Ingestion Pipeline

The proposed ingestion flow is:

1. Ingest data from sources such as email, SMS, documents, photos, and other inputs.
2. Perform programmatic preprocessing wherever possible.
3. Optionally use a user-provided LLM for post-processing / interpretation.
4. Create manual actions for decisions the system/LLM cannot safely determine.
5. Convert accepted information into structured Life Manager data.

The system should avoid using an LLM for tasks that can be handled deterministically.

## 9. Data Types and Queryability

Life Manager will need to support multiple kinds of data, including at least:

- Documents
- Transactions
- Quantities
- Entities
- Events
- Relationships
- Tasks
- Images/photos
- Metadata

A single real-world input may result in multiple linked structured objects.

### Structured representation alongside raw data

Life Manager must not allow important life data to become effectively unqueryable simply because its original source is unstructured.

The system should therefore preserve both:

1. **Original source data** — the raw email, SMS, image, PDF, document, note, etc., retained for fidelity and provenance.
2. **Queryable representation** — extracted metadata, structured facts, entities, relationships, and other indexed information derived from that source.

The structured representation does not need to capture everything in the original source. The purpose is to expose the information that Life Manager can confidently understand and use. The original source remains available when more context is needed.

For example:

`Receipt photo`
→ raw document/image retained
→ extracted merchant, date, total, line items
→ linked purchase and transaction
→ searchable through structured fields and the original text/image-derived index

This allows Life Manager to progressively structure information without requiring every input to be perfectly understood at ingestion time.

### Hybrid canonical data model

Life Manager should use a **hybrid model**:

- Important concepts use explicit, typed canonical entities and relationships.
- Canonical entities may support controlled extension for information that does not yet justify a dedicated field/type.
- Unknown or not-yet-promoted information remains attached to the source and/or intermediate representation rather than being discarded.
- Information that Life Manager can reliably understand should be promoted into structured, queryable facts instead of remaining trapped in arbitrary blobs.

The goal is to keep the core model predictable and maintainable while allowing the ingestion system to handle reality that does not perfectly fit today's schema.

Queryability should therefore be treated as a first-class requirement for both structured and unstructured information. The eventual search design may combine structured queries, full-text search, semantic search, and LLM-assisted interpretation, but the underlying data model should preserve enough metadata/provenance to support those approaches.

## 10. Photos and Visual Ingestion

The application should support taking/uploading a photo and allowing the ingestion pipeline to interpret it later.

Examples:

- Photo of appliance serial number
- Photo of fruit/vegetables for inventory identification
- Receipt photo
- Product label
- Document/photo containing useful life information

The initial upload should not require the user to manually classify everything. Processing can happen asynchronously through deterministic preprocessing and optionally an LLM.

## 11. Authentication and Access

The system should support pluggable authentication mechanisms.

Potential options include:

- Basic/local authentication
- OAuth / external identity providers

The architectural choice between independently deployed services and plugins/modules is still open.

If independently deployed services are used, JWT or an equivalent mechanism may be considered for service-to-service authentication.

## 12. Ownership, Participation, Identity, and Access

Life Manager should distinguish between **ownership**, **participation**, and **user accounts**.

### Owner

The owner is the person, household, or organization that owns/controls the Life Manager data. Ownership does not necessarily correspond to the person who performed an action or paid for something.

### Participant

Participants are people or entities involved in the underlying real-world event or object. Participation does not automatically grant data access.

### User

A user is an authenticated Life Manager account. A person in the Life Manager life model does not need to be a user. A person should be able to exist as a contact/participant/debtor/etc. and potentially become an authenticated user later without requiring the underlying identity to be recreated.

Example:

> Grocery purchase → Owner: Household; Participants: household members; Paid by: one person.

Life Manager should support:

- Multiple users
- Households
- Shared access
- Cross-access within a household
- Data owned by an individual or household
- Participants who may not have Life Manager accounts

The precise authorization model is still an open question.

## 13. Initial UI Direction

The UI should remain basic for the MVP.

The initial priority is correctness of the underlying system and workflows rather than a highly polished interface.

## 14. Documents

Document management is part of the intended system.

Documents may be connected to other entities and events, for example:

- Appliance → invoice
- Appliance → warranty document
- Transaction → receipt
- Person → agreement/document
- Maintenance event → service report

## 15. Notifications and Human Decisions

Life Manager should provide in-app notifications, including notifications that require the user to make a decision.

A processing workflow should be able to generate a dynamic question when the system cannot safely determine an outcome automatically.

Examples:

> Is this SMS the same purchase as this receipt?

The response mechanism should support at least:

- Yes / No
- Multiple choice
- Multiple selections where appropriate
- Custom/free-form answer

The decision should then become part of Life Manager's resulting data/history rather than being an ephemeral UI action.

Notification architecture, delivery channels, and reminder scheduling are still open questions.

## 16. Search

Life Manager should provide a way to search the user's accumulated life data and answer questions about it.

The search system should eventually be able to operate across facets rather than requiring the user to know which facet contains the information.

Examples of the desired direction:

- Where is the warranty document for the washing machine?
- What groceries are running low?
- How much did we spend on groceries last month?
- Who owes me money?
- When was the last maintenance visit for the AC?

The exact search architecture is still open.

## 17. Security and Privacy

Security and privacy are important design goals, including:

- Encryption in transit
- Encryption at rest
- Appropriate access control
- Safe handling of highly personal data

These do not need to block the earliest MVP, but the architecture should avoid making them unnecessarily difficult later.

## 18. Typed Relationships

Life Manager should use **defined, directional, reusable relationship types** rather than arbitrary graph edges or free-form relationship phrases.

A relationship type should have a defined semantic contract, including applicable source/target entity types and, where appropriate, a schema for relationship metadata. The relationship instance can carry structured metadata such as quantities, dates, confidence, provenance, context, or other domain-specific information.

Relationship semantics should be defined by Life Manager's hand-crafted domain facets/core model. LLMs may propose or populate relationships, but should not be able to invent new canonical relationship types.

Simple containment relationships may cover cases that might otherwise become facet-specific relations. For example:

- `Purchase → contains → Purchase Line`
- `Purchase Line → refers_to → Product`
- `Physical Asset → contains → Warranty`

Whether a relation uses `contains` or a more specific type should be driven by the semantics required by the domain model, not by the UI wording alone.

This model aims to provide interconnectedness while avoiding an unstructured generic graph.

## 19. Audit and Processing History

Automated processing and important user decisions should have an auditable history.

Life Manager should retain enough information to understand the progression from source data through preprocessing, optional LLM interpretation, human decisions, and resulting canonical data.

The exact audit mechanism, schema, retention policy, and user-facing audit UI are deliberately deferred.

## 20. LLM Strategy

The user has access to both local and cloud LLMs.

Life Manager should therefore support a **BYO-LLM model**, rather than requiring the application itself to provide a proprietary model.

LLMs should be optional and used where they add value, especially for:

- Understanding unstructured input
- Extracting structured information
- Linking information to existing entities
- Classifying ambiguous data
- Suggesting actions

Deterministic/programmatic processing should be preferred whenever it is sufficient.

The system should provide manual intervention when the model cannot safely make a decision.

## 21. Decisions, Preferences, Rules, and Patterns

Life Manager should eventually be able to represent higher-level concepts such as:

- Decisions and commitments
- User/household preferences
- Rules and policies
- Recurring patterns and expectations

These concepts are important because they can influence multiple facets at once. For example, a purchasing preference could affect inventory, shopping, recipes, and finances.

**These are explicitly not part of the MVP.** They should be captured as a future requirement so that the initial data model does not accidentally make them impossible to add later.

## 22. MVP Candidates

The following were identified as intended MVP or near-MVP capabilities:

- Basic tasks
- Basic UI
- Data ingestion
- Documents
- Core structured data model
- Search
- Multiple users / household concepts (scope to be determined)

Other facets can be added incrementally.

No final MVP scope has been agreed yet.

## 23. Open Architecture Questions

These are intentionally unresolved and should be discussed before implementation decisions are made.

### 23.1 Services vs plugins/modules

Should Life Manager be:

- One modular application with internal facets/modules?
- Multiple independently deployable services?
- A hybrid, with a central core and separately deployable facets?

### 23.2 Data model

The working direction is a hybrid canonical model: typed core entities/relationships with controlled extensibility and preserved raw/intermediate representations.

Remaining questions include:

How should the canonical model represent:

- Entities
- Events
- Relationships
- Ownership/scope
- Derived state
- Provenance
- Corrections/reversals

### 23.3 Identity and authorization

How should users, households, organizations, roles, and cross-user access interact?

### 23.4 Structured extraction from unstructured sources

The current direction is to preserve the original source while promoting understood information into typed/queryable canonical facts. The exact promotion threshold and extension mechanism remain open.

### 23.5 Ingestion and provenance

How should Life Manager retain the relationship between:

- Original source data
- Preprocessed information
- LLM interpretation
- Human decisions
- Final canonical objects

This is likely important for trust, auditability, and correction.

### 23.6 LLM safety / confidence

How should the system distinguish:

- Automatically accepted information
- Suggested information awaiting approval
- Ambiguous information requiring a manual decision

### 23.7 Event sourcing boundaries

Which parts genuinely benefit from event-based history and which should simply store current state?

### 23.8 Search architecture

Should search be:

- Traditional structured search
- Full-text search
- Semantic/vector search
- Hybrid search
- LLM-assisted query interpretation

### 23.9 Deployment model

How much of Life Manager should be independently deployable/configurable for a personal installation without introducing unnecessary operational complexity?

### 23.10 Corrections and manual mutation

The correct model for user corrections is currently unresolved.

The concern is that allowing users to directly mutate canonical historical data may break relationships/derived state across the interconnected model and make recovery difficult or confusing. The eventual experience should avoid exposing users to fragile low-level data manipulation.

The principle of preserving audit/history remains agreed, but whether corrections are represented as edits, reversals, compensating events, replacement records, or another mechanism is deliberately deferred.

## 24. Architecture Review Findings

A separate architecture review was performed against this design. It broadly validates the direction and recommends refining the canonical model before choosing database, service, or deployment architecture. fileciteturn1file0

### Agreed / incorporated refinements

- **Assertion should be treated as a first-class conceptual primitive.** An assertion explains why Life Manager believes a fact or relationship to be true. It can retain source, extracted value, confidence, producing process/model, validation status, approval status, and provenance. This creates a clean bridge from raw evidence and LLM interpretation to canonical data and later correction. fileciteturn1file3
- **Impact should be a formal concept.** An accepted fact/event can produce one or more explicit impacts, while derived state represents the current calculated or materialized view of those impacts. fileciteturn1file3
- **Corrections remain a major design problem.** The review recommends versioned assertions/facts plus explicit impact reconciliation as a promising direction, without requiring full event sourcing. This remains a design direction rather than a final implementation decision. fileciteturn1file3
- **LLMs are interpreters/proposal generators, not canonical truth.** They may extract, classify, match, interpret, and suggest, but cannot invent canonical relationship types. Confidence and validation/approval are separate concerns. fileciteturn1file5
- **Controlled extensibility is required.** Information should move from raw source → extracted field → extension field → canonical field as it becomes stable, important, query-relevant, or useful across facets. This reinforces the hybrid model rather than arbitrary JSON everywhere. fileciteturn1file5
- **Search should be considered a query layer.** Natural-language questions may require structured queries, relationship traversal, full-text retrieval, semantic retrieval, derived-state queries, or LLM-assisted query interpretation. The LLM should translate the question into safe query operations rather than answer from embeddings alone. fileciteturn1file2

### Additional concepts to evaluate before implementation

The review recommends explicitly evaluating:

- **Definition vs instance:** e.g. product definition vs purchase line vs purchased physical unit/holding vs asset. fileciteturn1file3
- **Time as a core concern:** occurred, recorded, effective, validity, and expiry may differ. fileciteturn1file3
- **Actor:** the person/system/LLM/external service responsible for an action or assertion. fileciteturn1file3
- **Scope:** a generalized ownership/access boundary for personal, household, organizational, or shared data. Ownership, participation, access, and scope remain distinct. fileciteturn1file3

### Architecture direction noted by the review

The review recommends favoring **one modular application with clear internal facet boundaries** at the current stage, rather than premature microservices, because the shared canonical model is not yet mature and independently deployed services would add data-consistency and operational complexity. This is a recommendation, not yet a final decision. fileciteturn1file2

### Finance and inventory

The review reinforces the idea that finance and inventory should derive balances from underlying movements where practical, but finance remains intentionally underspecified and should be researched and distilled into a minimal MVP model before implementation. fileciteturn1file2

## 25. Design Philosophy

The project should optimize first for:

1. Usefulness to the creator
2. Coherence of the model
3. Low friction for real-life use
4. Data correctness and traceability
5. Practical implementation complexity
6. Extensibility where it provides real value

Potential commercialization should not drive early architectural complexity.

## 26. Conceptual System Map

The current conceptual model can be represented as:

```text
                              LIFE MANAGER
                                  │
             ┌────────────────────┼────────────────────┐
             │                    │                    │
        INPUTS / EVIDENCE     PEOPLE / ACCESS      FACETS
             │                    │                    │
     ┌───────┼────────┐      ┌────┼────┐       ┌──────┼───────────────┐
     │       │        │      │         │       │      │       │       │
    Email   SMS    Photo    Person  Household  Finance Inventory Tasks Documents
     │       │        │       │         │       │      │       │       │
     PDF   Manual   API      │         Org      Food  Assets  ...     ...
     │       │        │      │
     └───────┴────────┘      └───────────────────────────────┐
             │                                                │
             ▼                                                │
       PREPROCESSING                                          │
             │                                                │
             ▼                                                │
       OPTIONAL LLM                                           │
             │                                                │
             ▼                                                │
       HUMAN DECISIONS                                        │
             │                                                │
             └───────────────────┬────────────────────────────┘
                                 ▼
                         CANONICAL MODEL
                                 │
              ┌──────────────────┼──────────────────┐
              │                  │                  │
          ENTITIES            FACTS / EVENTS     RELATIONSHIPS
              │                  │                  │
      ┌───────┼────────┐         │          ┌───────┼──────────────┐
      │       │        │         │          │       │              │
    Person  Purchase  Product  changes      CONTAINS  OWES        FUNDS
    Asset   Task      Debt     evidence     HAS       ...         ...
    Document Warranty Recipe    provenance
              │                  │
              └──────────┬───────┘
                         ▼
                      IMPACTS
                         │
            ┌────────────┼────────────┐
            │            │            │
        Inventory     Finance      Tasks/Reminders
            │            │            │
            └────────────┼────────────┘
                         ▼
                    CURRENT STATE
                         │
                 ┌───────┼────────┐
                 │       │        │
               Search  Views   Notifications

Key principle:
Raw inputs are preserved; understood information becomes queryable canonical data;
relationships explain how entities connect; impacts/current state can be derived or
materialized as needed. Provenance/audit connects interpretations and impacts back
to their sources.
```

This is a conceptual map only. It does not yet prescribe database structure, service
boundaries, event-sourcing, or deployment architecture.

## 27. Canonical Model Stress Test: Receipt + SMS

### Scenario

A household purchases a washing machine for ₹40,000.

Life Manager receives two independent inputs:

```text
SMS:
  Reliance Digital
  Amount: ₹40,000
  Time: 14:32

Receipt photo:
  Reliance Digital
  Washing Machine
  Total: ₹40,000
  Time: 14:31
```

The intended processing flow is:

```text
SMS ------------------┐
                      ├──> Processing / Interpretation
Receipt photo --------┘              │
                                     ▼
                                Assertions
                                     │
                         Identity resolution / matching
                              /                \
                             /                  \
                    existing Purchase       new Purchase
                             │                  │
                             └────────┬─────────┘
                                      ▼
                              Canonical information
                                      │
                                      ▼
                                    Impacts
```

### Questions this scenario must answer

1. How does Life Manager determine whether the SMS and receipt describe the same Purchase?
2. What identifies a canonical Purchase independently of either source?
3. Can multiple assertions point to the same fact or relationship?
4. What happens when the evidence conflicts (for example, SMS = ₹40,000 and receipt = ₹39,500)?
5. What does the system persist when the match is uncertain?
6. How does a later user correction change the purchase, its relationships, and its impacts?

No implementation decision is implied yet. This stress test exists to force precise semantics before persistence and service design.

## 28. Canonical Model Stress Test: Product vs Inventory Holding

The Product/Inventory boundary was pressure-tested from three perspectives: domain semantics, maintainability, and ingestion/LLM behavior.

### 27.1 Domain semantics

Life Manager should distinguish at least these concepts:

- **Product definition** — what a kind of thing is, independent of a particular purchase or household holding. Example: "India Gate Basmati Rice 5 kg".
- **Purchase** — the occurrence in which something was acquired.
- **Purchase Line** — the specific line within a purchase describing what was acquired, including quantity, unit, price, and other line-specific information.
- **Inventory Holding** — the household/personal holding of a product or quantity-bearing material. It represents the thing being held; its current quantity may be derived or materialized rather than authoritative.
- **Physical Asset** — an individually identifiable persistent object, usually with an identity such as serial number. Example: a specific washing machine.
- **Consumption / Usage** — an occurrence that changes or uses a holding/asset.
- **Adjustment / Wastage / Transfer** — explicit domain occurrences that change holdings for reasons other than purchase or consumption.

The generic term **Item should not be a canonical domain primitive** because it conflates materially different concepts such as rice, a purchase line, an inventory holding, and a serial-numbered appliance.

### 27.2 Recommended relationships

A grocery example can therefore be represented as:

```text
Purchase P1
    │
    └── CONTAINS → Purchase Line L1
                       │
                       ├── REFERS_TO → Product: Rice
                       └── quantity = 5 kg, price = ₹450

Purchase Line L1
    ├──> produces/changes → Inventory Holding: Rice in Household Pantry
    └──> produces/changes → Money Movement: -₹450
```

An appliance example differs:

```text
Product: Washing Machine Model X
            │
            ▼
Purchase Line L2
            │
            ├── REFERS_TO → Product
            └── quantity = 1
                    │
                    ▼
              Physical Asset A1
              serial_number = XYZ
                    │
                    └── CONTAINS → Warranty W1
```

The Purchase Line is therefore the acquisition-specific bridge between a purchase and what was acquired. It should be a first-class canonical object rather than an anonymous nested property.

### 27.3 Maintainability perspective

The model should avoid putting household state directly on Product. A Product such as "5 kg rice" is not owned by the household simply because it is a known product definition.

Inventory Holding is where household/personal ownership and holding context belong. The current quantity can initially be derived from accepted inventory movements and later materialized for performance. A user should not directly edit a materialized quantity; a domain-level adjustment/consumption/purchase/etc. should explain the change.

For individually identifiable assets, the model should not force an aggregate inventory holding. A washing machine is a specific physical asset and should have its own identity, while a bulk consumable such as rice can be represented as a quantity-bearing holding.

### 27.4 Ingestion / LLM perspective

Ingestion should be allowed to stop short of creating a canonical Product when the product identity is uncertain.

For example:

```text
Receipt line: "Rice 5kg"
        │
        ▼
Assertion: purchased quantity = 5 kg
Assertion: product candidate = unknown/ambiguous
        │
        ├── existing Product confidently matched → attach Purchase Line
        └── no confident match → retain candidate/extracted information
```

The LLM may propose that a receipt line matches an existing Product, but the canonical Product identity remains controlled by Life Manager's validation and identity-resolution rules.

### 27.5 Working decision

For the current canonical-model exercise:

> **Product definition, Purchase, Purchase Line, Inventory Holding, and Physical Asset are distinct concepts.**

> **Item is not a canonical primitive.** It may remain an informal UI term where useful, but the domain model should use the more precise concepts.

> **Inventory quantity is derived/materialized state; Inventory Holding is the canonical object describing what is held and in what scope/context.**

This is a working semantic decision and will be tested again in the finance, recipe, consumption, and appliance scenarios.


## 28. Canonical Model: Working Semantic Vocabulary

The latest architecture review indicates that the next milestone is a precise canonical-model contract rather than implementation architecture. The following is the current working vocabulary.

### 28.1 Source

A **Source** is original evidence received by Life Manager.

Examples:
- Email
- SMS
- Receipt image/PDF
- Photo
- Manual entry
- API response

Sources are retained for fidelity and provenance. A source is not automatically canonical truth.

### 28.2 Assertion

An **Assertion** is an internal statement that Life Manager has extracted, inferred, proposed, accepted, or is evaluating about the world or another canonical object.

Examples:
- `Purchase P1.total = ₹40,000`
- `Purchase P1 contains Purchase Line L1`
- `Purchase Line L1 refers_to Product RICE-5KG`

Assertions are permanent internal provenance/audit records and are normally invisible in the everyday UI. They may be surfaced when reviewing, troubleshooting, or correcting data.

An assertion should be able to retain enough information to answer:
- What was asserted?
- From which source(s)?
- By which actor/process/model?
- With what confidence?
- Was it proposed, accepted, rejected, or superseded?
- What canonical fact/relationship did it support?

### 28.3 Canonical Object

Use **canonical object** as the broad term for a persisted, independently referenceable thing in the Life Manager model.

Canonical objects may represent persistent things or meaningful occurrences.

Examples:
- Person
- Household
- Product definition
- Document
- Purchase
- Purchase Line
- Inventory Holding
- Physical Asset
- Warranty
- Task
- Debt
- Recipe

We should not force `Entity` and `Event` to be treated as two unrelated persistence primitives. A Purchase can be understood as a canonical occurrence with event semantics, while a Person or Product is a persistent thing. The precise type taxonomy can be formalized later without requiring a graph database or blanket event sourcing.

### 28.4 Fact

A **Fact** is a structured assertion about a canonical object.

Examples:
- `Purchase.total = ₹40,000`
- `Appliance.serial_number = XYZ`
- `Inventory Holding.unit = kg`

A fact is canonical when Life Manager has accepted it as part of the model. Assertions explain why Life Manager believes the fact.

### 28.5 Relationship

A **Relationship** is a defined, directional, typed semantic association between canonical objects.

Relationship types are controlled by Life Manager's domain model. They are not arbitrary phrases and cannot be invented by an LLM.

A relationship type should define:
- allowed source/target types
- direction
- semantic meaning
- optional relationship-data schema
- inverse semantics where applicable

A relationship instance can carry structured metadata such as quantity, dates, confidence, provenance, allocation, or contextual notes.

### 28.6 Impact

An **Impact** is a meaningful domain consequence caused by accepted canonical information.

Good examples:
- Money movement
- Inventory movement
- Debt balance change
- Asset status change
- A domain task becoming required/created

Impacts must not become a generic application event bus. UI refreshes, search-index updates, and dashboard rendering are consequences of implementation, not domain impacts.

### 28.7 Derived State

**Derived State** is current calculated or materialized state obtained from authoritative canonical information and domain impacts.

Examples:
- Current rice quantity
- Outstanding debt
- Warranty status
- Current asset status

Materialized state may be mutable for performance, but it must never become an independent domain source of truth. It should be reconstructable from authoritative information where practical.

## 29. Identity Resolution

Identity resolution is a first-class cross-cutting concern.

Life Manager must distinguish between:

1. **Create a new canonical object**
2. **Attach evidence/assertions to an existing canonical object**

Multiple sources may describe the same object even when they disagree on some fields.

Example:

```text
SMS -------------------┐
                        ├──> Assertions ──> Identity Resolution ──> Purchase P1
Receipt ----------------┘
```

Identity resolution may use:
- deterministic matching
- source metadata
- timing
- merchant/account/card information
- amounts and quantities
- product identity
- LLM proposals
- agreement among multiple models
- prior known relationships

The decision to automatically accept a match versus ask the user should initially use built-in defaults. Configuration can later evolve from:

```text
built-in defaults
    → global configuration
    → facet-specific configuration
    → more specific overrides if ever needed
```

This policy system is not an immediate MVP design priority. The canonical model only needs to preserve enough evidence and confidence for policy-driven decisions later.

### Identity resolution safety principle

Confidence and validation are separate concepts.

A high-confidence proposal may still require human confirmation depending on policy and domain sensitivity. For example, incorrectly merging financial records can be more damaging than failing to merge two grocery records.

## 30. Corrections and Reconciliation: Working Direction

Correction should be designed together with:
- identity resolution
- assertion/version history
- relationship reconciliation
- impact reconciliation
- derived-state rebuilding

A user should correct the understandable source/canonical object, not manually edit downstream materialized state.

Example:

```text
Receipt
   ↓
Assertion v1: Purchase.total = ₹40,000
   ↓
Canonical Purchase P1
   ↓
Impacts
   ├── Money Movement -₹40,000
   └── Inventory / Asset consequences

User correction
   ↓
Assertion v2: Purchase.total = ₹39,500
   ↓
Reconcile affected impacts
   ↓
Rebuild/materialize current state
```

The original assertion remains part of internal history. The user-facing model should present the current accepted interpretation without forcing the user to understand assertion versions unless they inspect provenance/correction history.

### Important unresolved aspect

The exact correction mechanism remains open. Candidates include versioned facts/assertions, replacement interpretations, compensating domain adjustments, or combinations of these. The implementation must avoid direct mutation of derived state and must make downstream consequences recoverable.

## 31. Time Semantics

Time should be treated as a cross-cutting concern rather than a single timestamp field.

Depending on the domain, Life Manager may need to distinguish:
- `occurred_at` — when something happened in the real world
- `recorded_at` — when Life Manager received/recorded it
- `effective_at` — when the information should affect domain state
- `valid_from / valid_until` — period in which a fact/relationship applies
- `expires_at` — when something such as a warranty or document validity expires

Not every object needs every timestamp. The model should not require artificial timestamps merely for consistency.

## 32. Actor Semantics

Life Manager should distinguish **Actor** from Person/User.

An Actor is the entity/process responsible for an action or assertion.

Possible actors:
- Person/user
- Life Manager system process
- LLM
- External integration
- Scheduled/background process

Example:

```text
Source receipt
   ↓
OCR actor extracts text
   ↓
LLM actor proposes Purchase.total
   ↓
User actor approves
```

This supports auditability without implying that the system's LLM is authoritative.

## 33. Scope, Ownership, Participation, Access

For the intended product, Life Manager has two principal scopes:

- **Personal** — information belonging to an individual
- **Household** — shared organizational boundary for the household

A generic business/organization model is not required initially.

These remain separate concepts:

```text
Scope       = where the data belongs
Owner       = who conceptually owns/controls it
Participant = who/what is involved
User        = authenticated Life Manager account
Access      = who may view/change it
Actor       = who/what performed an action
```

A person can exist without being a User and may become a User later.

Example:

```text
Purchase
  scope       = Household
  owner       = Household
  participants= You, Wife, Friend
  funded_by   = You
  actor       = Life Manager ingestion process
  access      = Household members
```

The exact authorization/role model remains open.

## 34. Finance: Minimal Conceptual Pass Required

Finance should not be designed as a full accounting system for the MVP.

However, the model must avoid collapsing distinct concepts that have different semantics. The minimum concepts that need to be researched and pressure-tested are:

- **Financial Account** — where money is held/recorded
- **Money Movement** — an actual monetary movement observed or accepted by Life Manager
- **Allocation / Attribution** — how a movement relates to a purchase, category, person, or shared expense
- **Debt / Receivable** — an amount owed between parties
- **Payment / Repayment** — a movement that satisfies or reduces an obligation
- **Settlement** — the state/operation of resolving an obligation
- **Derived Balance** — current calculated state

Example:

```text
Dinner
  ↓
Money Movement: -₹1,000 from Alice's account
  ↓
Allocation: Alice ₹250, Bob ₹250, Carol ₹250, Dave ₹250
  ↓
Debts: Bob owes Alice ₹250, etc.
  ↓
Later repayment movements reduce the debts
```

A dedicated finance modeling pass is required before finance implementation.

## 35. Inventory: Working Conceptual Model

Inventory should distinguish:

```text
Product definition
      │
      ▼
Purchase Line
      │
      ▼
Inventory Holding
      │
      ├── Consumption / Usage
      ├── Adjustment / Wastage
      └── Transfer
```

A Physical Asset is different:

```text
Product definition
      │
      ▼
Purchase Line
      │
      ▼
Physical Asset
      └── serial / identifying information
```

This avoids treating a bulk consumable and a serial-numbered appliance as the same type of inventory object.

## 36. Query Layer

Life Manager should think in terms of a **query layer**, not only search.

Questions can require different operations:
- entity lookup
- relationship traversal
- aggregation
- derived-state query
- full-text retrieval
- semantic retrieval
- cross-facet composition

An LLM may interpret a natural-language question, but the preferred flow is:

```text
User question
   ↓
LLM / parser interpretation
   ↓
Safe inspectable query representation
   ↓
Canonical/query layer
   ↓
Result
   ↓
Optional natural-language explanation
```

The LLM should not answer solely from embeddings when authoritative structured data exists.

## 37. Controlled Extensibility / Promotion Rule

Extensions are an accommodation for uncertainty, not a second permanent schema.

The preferred progression is:

```text
Raw source
   ↓
Extracted field
   ↓
Extension / intermediate representation
   ↓
Canonical field / relationship / object
```

Promotion should occur when information is sufficiently:
- stable
- semantically understood
- query-relevant
- important
- useful across facets

This prevents arbitrary JSON/custom attributes from becoming an unmaintainable parallel domain model.

## 38. Implementation Direction After Semantic Stabilization

The latest review recommends a **modular monolith** as the default implementation direction for the current stage:

```text
One deployable Life Manager application
    │
    ├── Core canonical model
    ├── Finance
    ├── Inventory
    ├── Documents
    ├── Tasks
    ├── Recipes/Food
    ├── People
    └── other facets

Asynchronous workers where useful:
    ├── ingestion
    ├── OCR / preprocessing
    └── LLM processing
```

Service extraction should occur only when an actual scaling, isolation, security, or ownership reason appears.

This remains a direction rather than a final implementation decision until the canonical model is stable.

## 39. Full-System Stress Tests Required Before Implementation

The following scenarios should be modeled end-to-end before database schema/API/service decomposition:

1. Receipt + SMS duplicate / conflict
2. Grocery purchase → inventory → consumption
3. Appliance purchase → physical asset → warranty → document → maintenance
4. Shared dinner → allocation → debt → repayment → settlement
5. Correction of purchase amount
6. Correction that moves evidence from Purchase P1 to Purchase P2
7. Warranty expiration → derived state → notification
8. Household access and personal-vs-household ownership
9. Ambiguous product matching during ingestion
10. LLM proposal rejected/corrected by user

The output of each stress test should be a concrete canonical-object representation and transition sequence.

## 40. Design Decision Maturity

### Decided
- Shared canonical model
- Hand-crafted facets rather than community plugins
- Typed directional relationships
- Raw source preservation
- Queryable structured representation alongside raw data
- Optional BYO-LLM
- Deterministic-first processing
- Assertions as permanent internal provenance/audit
- LLMs cannot invent canonical semantic types
- Personal + Household scopes
- Item is not a canonical primitive
- Product / Purchase / Purchase Line / Inventory Holding / Physical Asset are distinct concepts
- No blanket event sourcing
- Domain impacts are explicit and bounded

### Directionally decided
- Identity resolution as a first-class cross-cutting concern
- Policy-driven automatic vs manual identity decisions
- Hybrid derived/materialized state
- Modular-monolith implementation direction
- Query layer instead of search-only architecture
- Controlled extensibility with promotion into canonical fields
- Corrections through versioned assertions/facts plus impact reconciliation

### Open
- Exact canonical identity rules
- Exact Entity/Event/Fact taxonomy and naming
- Exact relationship vocabulary and schemas
- Correction/reconciliation implementation
- Finance primitives and semantics
- Exact inventory movement semantics
- Authorization model
- Time field requirements by object type
- Extension/intermediate representation implementation
- Persistence technology

### Deferred
- Polished UI
- Notification delivery channels
- Service decomposition/scaling strategy
- Exact search technology
- Strong encryption implementation details
- Broader rules/preferences/decision engine

## 41. Autonomous Review Conclusion

The architecture no longer appears to have a fundamental conceptual flaw. The major risk has shifted from choosing the wrong architecture to allowing semantic ambiguity during implementation.

The model should therefore be developed **scenario-first rather than schema-first**.

The next design milestone is not selecting PostgreSQL, Neo4j, microservices, or an ORM. It is producing a small canonical-model specification that can represent the stress tests above unambiguously and explain how corrections propagate.

## 28. Canonical Model Stress Test: Finance

Finance is intentionally being treated as a minimal domain model rather than a full accounting system. The goal is to represent the real-life financial relationships Life Manager needs while preserving enough structure for reconciliation and later expansion.

### 28.1 Problem

A single real-world activity can have several different financial meanings that should not be collapsed:

- A bank/card account records a money movement.
- A purchase or other activity explains what the money was for.
- An allocation explains who or what economically bears the amount.
- A debt describes an amount owed between people.
- A repayment/settlement changes that debt.
- A current balance is derived from underlying movements and accepted facts.

Example: one person pays ₹1,000 for dinner for four people.

```text
Dinner
  │
  ├── Purchase/Expense context: ₹1,000
  │
  └── Money Movement: -₹1,000 from payer's account
          │
          └── Allocation
               ├── payer: ₹250
               ├── person B: ₹250
               ├── person C: ₹250
               └── person D: ₹250
                         │
                         ├── Debt: B owes payer ₹250
                         ├── Debt: C owes payer ₹250
                         └── Debt: D owes payer ₹250
```

The economic allocation and the actual bank movement are different facts and must not be represented as one transaction object.

### 28.2 Minimal working vocabulary

The current working model is:

- **Financial Account** — a source or destination of money, such as a bank account, credit card, cash balance, or other supported account.
- **Money Movement** — an actual movement of money between accounts/parties, including an external counterparty where appropriate.
- **Allocation** — how a money movement or expense is attributed to a purchase, activity, person, household responsibility, category, or other domain object.
- **Debt** — an outstanding obligation between people or other supported parties.
- **Repayment / Settlement** — a later money movement that reduces or closes an obligation.
- **Derived Balance** — a calculated current balance; not an independent source of truth.

These are deliberately narrower than a general accounting model. Additional concepts should be introduced only when real Life Manager use cases require them.

### 28.3 Purchase and finance are related but distinct

A Purchase is the real-world acquisition event. A Money Movement is the movement of money resulting from it. They may be related, but they are not the same object.

```text
Purchase P1
   │
   ├── contains → Purchase Line(s)
   │
   └── impacts → Money Movement M1
                      │
                      └── from → Financial Account A1
```

The relationship between purchase and money movement must support cases where the mapping is not one-to-one. Examples include:

- one purchase paid in multiple movements
- one movement covering multiple purchases
- cash purchases where no conventional account exists initially
- refunds or adjustments occurring later
- a credit-card purchase and a later bank payment

The exact relationship semantics remain to be finalized during the finance model pass.

### 28.4 Debt is not the same thing as a transaction

A debt represents an obligation; a payment is a money movement that may settle that obligation.

```text
Person A
   │
   └── Debt → Person B
              amount = ₹1,000

Later:
Money Movement
   │
   └── settles/reduces → Debt
                         remaining = ₹400
```

A debt should therefore remain meaningful even when no payment has yet occurred, and multiple repayments should be possible.

### 28.5 Shared expense

Shared expenses require three separate concepts:

1. the actual money movement,
2. the allocation of the expense,
3. the resulting obligations between participants.

This allows Life Manager to distinguish:

> "I paid ₹3,000"

from:

> "The expense was shared ₹1,500 / ₹1,500"

and from:

> "My friend still owes me ₹1,500."

### 28.6 Credit cards and payment timing

The model should not assume that a purchase immediately represents a bank-account movement.

For example:

```text
Purchase
   ↓
Credit-card Money Movement
   ↓
Credit-card obligation/balance
   ↓
Later bank Money Movement
   ↓
Reduction of credit-card balance
```

This is one reason Purchase, Money Movement, and Financial Account must remain separate.

### 28.7 Corrections

If Life Manager initially interprets a receipt as ₹1,000 and later the user corrects it to ₹800, the correction should operate on the source interpretation/canonical fact and trigger reconciliation of affected allocations and money impacts.

The user should not directly edit a derived account balance.

```text
Source
  ↓
Assertion: amount = ₹1,000
  ↓
Accepted Purchase / Allocation
  ↓
Money Impact = -₹1,000

Correction
  ↓
New accepted interpretation: amount = ₹800
  ↓
Reconcile affected impacts
  ↓
Current derived state updated
```

### 28.8 Working decision

For the current canonical-model exercise:

> **Purchase, Money Movement, Allocation, Debt, Repayment/Settlement, Financial Account, and Derived Balance are distinct concepts.**

> **Finance should derive current balances from underlying movements and accepted information where practical.**

> **The MVP should use the smallest useful financial vocabulary and should not attempt to implement general accounting.**

> **The exact semantics of refunds, transfers, credit-card obligations, account reconciliation, categorization, and multi-currency behavior remain to be pressure-tested before implementation.**


## 28. Existing Open-Source Finance Systems to Evaluate

A dedicated investigation found several mature/open-source finance systems that may provide reusable domain ideas or, subject to licensing and architectural fit, implementation that Life Manager could potentially integrate rather than recreate. This is an evaluation input, not yet a decision to depend on any project.

### 28.1 Maybe / Sure

Maybe's original project explicitly aimed at a personal-finance OS and included accounts, loans, equity, crypto, net worth, investments, debt insights, and investment portfolio features. The original `maybe-finance/maybe` repository was archived by its owner on July 27, 2025 and is no longer maintained. A community fork, `we-promise/sure`, now exists to continue that codebase. The original project was AGPLv3.

Potential value to Life Manager:
- Wealth-oriented conceptual model
- Financial accounts and account types
- Loans / liabilities
- Investments and portfolio concepts
- Net-worth calculation
- Historical account balances

Potential concern:
- It is a large application with its own architecture and assumptions, so embedding it wholesale could fight Life Manager's canonical model rather than help it.

### 28.2 Firefly III

Firefly III is a mature, self-hosted personal finance manager with double-entry bookkeeping, recurring transactions, rules, budgets, categories, reporting, multi-currency support, and a REST API. It is AGPL-3.0.

Potential value to Life Manager:
- Proven bookkeeping semantics
- Accounts and money movements
- Transfer/transaction concepts
- Categorization and reconciliation
- Strong financial correctness model

Potential concern:
- Its domain model and terminology are designed for a standalone finance application. Integrating its canonical model directly may create a second source of truth or force Life Manager to adopt assumptions that do not fit the broader life model.

### 28.3 Ghostfolio

Ghostfolio is an open-source wealth-management application focused primarily on stocks, ETFs, cryptocurrencies, portfolio performance, holdings, and investment analytics. It is AGPL-3.0 and is implemented with NestJS/PostgreSQL/Prisma on the backend.

Potential value to Life Manager:
- Investment holdings and transactions
- Portfolio valuation/performance concepts
- Investment analytics

Potential concern:
- It is investment-centric rather than a general personal-finance domain, so it is more likely to be a source of domain concepts than the core finance engine.

### 28.4 Securo and other newer finance projects

Securo is another self-hosted open-source finance manager with accounts, transactions, assets, liabilities, goals, net worth, multi-currency, multi-user support, and optional AI features. It may be useful as a reference for a simpler modern architecture, but it is less established than Firefly III and should be evaluated separately before any dependency decision.

### 28.5 Working direction

Life Manager should **not** immediately implement all finance functionality itself. The next step is to compare existing projects against the canonical concepts Life Manager actually needs. The preferred reuse hierarchy is:

1. Reuse established domain concepts and lessons where they fit.
2. Reuse libraries/components where the boundaries are clean.
3. Integrate an external finance subsystem only if it can participate without becoming a competing source of truth.
4. Reimplement only the concepts that are genuinely specific to Life Manager.

The finance domain still needs a dedicated semantic pass covering:
- Financial accounts
- Money movements
- Allocations/attribution
- Debts and receivables
- Payments/repayments
- Settlements
- Loans/liabilities
- Investments/holdings
- Other assets
- Valuations
- Net worth
- Derived balances

Licensing and dependency boundaries must be considered before adopting code from AGPL projects.


## 41. Adversarial Design Review — Potential Failure Modes

This section is intentionally skeptical. The purpose is to identify ways the current architecture could fail even if the individual concepts are reasonable. These are review findings, not automatically rejected design choices.

### 41.1 The canonical model may become an abstraction tax

**Risk: HIGH**

The shared model currently contains Source, Assertion, Canonical Object, Fact, Relationship, Impact, Derived State, Actor, Scope, and Identity Resolution. Each is defensible, but together they can become a second programming language that every feature must understand.

The strongest competitor evidence suggests that a conventional modular monolith with application-level orchestration can already solve substantial cross-module behavior. Lifestack uses this approach successfully, including explicit cross-module workflows, imports, audit, scheduling, and finance. The current Life Manager model is only justified if it demonstrably reduces cross-domain complexity rather than merely relocating it.

**Guardrail:** every new primitive must solve a concrete stress-test problem. If a concept cannot explain a real use case better than ordinary application data, do not add it.

### 41.2 Assertion vs Fact may become an unnecessary two-model system

**Risk: HIGH**

The current design says a Fact is accepted canonical information while Assertions explain why it is believed. This is useful for provenance, but it risks every field existing twice: once as the current value and again as a set of assertion records.

Potential failure modes:
- querying current values becomes expensive or confusing;
- conflicting assertions require a hidden precedence algorithm;
- developers accidentally use assertions as the canonical database and bypass the typed model;
- correction logic becomes an implicit version-control system.

**Guardrail:** define a strict rule for when an assertion becomes a canonical fact and how the current accepted value is represented. Do not require ordinary domain queries to traverse provenance unless the query explicitly asks for it.

### 41.3 Identity resolution can become the real core system

**Risk: CRITICAL**

Once email, SMS, receipts, documents, manual entries, and photos all feed the same model, deciding that two inputs refer to the same object becomes one of the most consequential operations in Life Manager.

A bad merge can corrupt an entire relationship neighborhood:

```text
wrong merge
    ↓
Purchase
 ├── Finance
 ├── Inventory
 ├── Asset
 ├── Warranty
 └── Documents
```

Undoing a merge is substantially harder than creating a duplicate.

**Guardrails:**
- make merge and link operations explicitly reversible;
- distinguish `same_as`, `possible_match`, and `related_to` semantics;
- preserve the original identities/evidence after a merge;
- never make irreversible high-impact merges solely from an LLM confidence score;
- test split/merge/unmerge as a first-class stress test.

### 41.4 Corrections may be harder than initial ingestion

**Risk: CRITICAL**

The architecture is optimized around processing inputs into consequences, but a correction can invalidate a large downstream subgraph.

Example:

```text
Receipt
  → Purchase P1
      → Purchase Line
          → Asset
              → Warranty
                  → Maintenance reminder
      → Money Movement
          → Debt allocation
```

Changing the receipt could affect all of these.

The current design says impacts should be reconciled, but does not yet define whether impacts are immutable records, replaceable projections, compensating changes, or recomputable outputs.

**Guardrail:** before implementation, specify one complete correction algorithm and test it against:
- scalar correction;
- relationship correction;
- identity split;
- identity merge;
- deletion of source evidence;
- correction after downstream manual action.

### 41.5 Manual decisions can become hidden domain data without lifecycle semantics

**Risk: HIGH**

A user answer such as "Yes, these are the same purchase" is currently described as becoming part of the resulting history. But a decision can become stale.

Example:

> User says a product candidate is "Rice".

Six months later a better Product identity is introduced. Does the old decision remain authoritative? Can it be superseded? Who/what can invalidate it?

**Guardrail:** decisions need subject, decision type, actor, time, status, and supersession semantics even if the UI remains simple.

### 41.6 Relationships can explode into an ontology maintenance problem

**Risk: HIGH**

Typed relationships are safer than arbitrary graph edges, but a large hand-crafted relationship vocabulary can become difficult to maintain.

There is a tension between:

```text
contains
```

and highly specific relations such as:

```text
purchase_line_acquires_product
warranty_covers_asset
payment_settles_debt
```

Too generic loses semantics. Too specific creates hundreds of types.

**Guardrail:** relationship types should be introduced only when their semantics affect validation, querying, authorization, or domain behavior. Prefer a small vocabulary plus structured relationship metadata where appropriate.

### 41.7 `Impact` could become a disguised event bus

**Risk: HIGH**

The design explicitly says this must not happen, but the architecture naturally pushes in that direction.

For example:

```text
Purchase
  → Impact: inventory changed
  → Impact: finance changed
  → Impact: notification needed
  → Impact: search index changed
  → Impact: dashboard changed
```

Only some of these are domain consequences. If every reaction becomes an Impact, the distinction collapses.

**Guardrail:** define domain impacts narrowly. Infrastructure reactions should use a separate mechanism such as jobs, projections, cache invalidation, or an outbox.

### 41.8 Derived state reconstruction may become impractical

**Risk: HIGH**

The design prefers reconstructable state, but a mature household may contain years of purchases, inventory movements, financial movements, documents, corrections, and relationships.

Full reconstruction on every correction may be too expensive or operationally fragile.

**Guardrail:** design for checkpoints/materialized projections from the beginning, even if MVP uses simple derivation. Define how projections are rebuilt and verified without making projections authoritative.

### 41.9 Temporal semantics are still underspecified

**Risk: HIGH**

Multiple timestamps are recognized, but this can become a subtle source of bugs.

Examples:
- a receipt is received today for a purchase last month;
- a warranty is purchased today but starts after installation;
- a bank transaction posts two days after a card purchase;
- a correction is entered today but changes a historical effective date;
- an inventory adjustment is discovered today but represents last week's wastage.

**Guardrail:** each canonical type should explicitly declare which temporal semantics it supports. Do not add all timestamp fields to every object.

### 41.10 Scope and ownership can produce authorization ambiguity

**Risk: HIGH**

Personal and Household scopes are clear conceptually, but ownership, scope, participant, actor, and access are independent dimensions.

A difficult case is:

> A household purchase is visible to the household, but the underlying payment account is personally owned and should not expose all account details to every household member.

Another:

> A person participates in a household event but has no Life Manager account.

Another:

> A user belongs to two households.

**Guardrail:** authorization must be evaluated against the object and relationship context, not inferred from participant/owner alone. Database-level scope enforcement should be considered as a defense-in-depth measure.

### 41.11 Person → User identity promotion is an identity-merge problem

**Risk: MEDIUM/HIGH**

The design correctly allows a Person to exist without a User account and later become a User. However, this creates another identity-resolution operation.

The system must avoid accidentally creating:

```text
Person: John
User: John
```

as two unrelated identities.

**Guardrail:** define explicit account-linking/claiming semantics, including what happens when an authenticated user claims a pre-existing Person record.

### 41.12 Deletion and privacy conflict with provenance

**Risk: CRITICAL**

Permanent internal assertions and retained source evidence conflict with real-world deletion requirements.

Examples:
- user deletes a receipt containing personal data;
- user deletes a Person;
- an email connector is disconnected;
- a source contains another person's private information;
- a document must be removed while derived financial facts remain useful.

"Permanent audit" cannot literally mean "never deletable under any circumstance."

**Guardrail:** design retention, redaction, cryptographic erasure, source deletion, and derived-data treatment before calling provenance permanent. Preserve integrity of history without guaranteeing indefinite retention of raw personal data.

### 41.13 Raw-source retention can become the largest storage/security problem

**Risk: HIGH**

Photos, PDFs, email attachments, message archives, and document versions can dwarf structured data.

This also creates a privacy boundary: the raw source may contain much more information than the extracted canonical facts require.

**Guardrail:** separate source/blob lifecycle from canonical-data lifecycle. Define content hashing, deduplication, retention, compression, storage tiers, access control, and export behavior.

### 41.14 Queryability may become too expensive to provide universally

**Risk: HIGH**

The design expects structured queries, relationship traversal, full text, semantic retrieval, temporal queries, provenance queries, and LLM interpretation.

Trying to make all of those first-class over the same PostgreSQL schema could create an extremely complex query layer.

**Guardrail:** define a small authoritative query contract first. Add indexes/projections/materialized views for known query patterns rather than building a universal query language immediately.

### 41.15 LLM reprocessing can create non-deterministic history

**Risk: HIGH**

Models change. Prompts change. Local and cloud models behave differently. Reprocessing the same source can produce a different interpretation.

This is especially dangerous when reprocessing changes canonical identity or downstream impacts.

**Guardrail:** every model-derived proposal should retain model/provider/version, prompt/config version, input reference, output, and validation result. Reprocessing should produce a new interpretation rather than silently rewriting history.

### 41.16 Multiple LLM agreement is not independent evidence by default

**Risk: MEDIUM/HIGH**

Two models agreeing does not necessarily mean two independent pieces of evidence. They may share training data, prompts, extraction errors, or the same OCR mistake.

**Guardrail:** treat model agreement as one confidence signal, not proof. Independent evidence should preferentially come from different source artifacts or deterministic checks.

### 41.17 Automation can create self-reinforcing errors

**Risk: HIGH**

A dangerous loop is possible:

```text
LLM interpretation
  → canonical fact
  → derived state
  → notification
  → user action
  → new source
  → LLM interpretation
```

A wrong initial interpretation can therefore generate more evidence that appears to support itself.

**Guardrail:** distinguish source evidence from Life Manager-generated consequences. Derived outputs and prior model interpretations must not automatically count as independent evidence for identity or truth.

### 41.18 Recurring concepts are under-modeled

**Risk: HIGH**

Subscriptions, bills, recurring purchases, recurring maintenance, warranties, scheduled tasks, and expected income all have a pattern/expectation dimension that is different from individual occurrences.

The current model mentions recurring commitments but does not yet define them.

**Guardrail:** eventually distinguish an expected/recurring pattern from its individual occurrences. Do not force recurring behavior into duplicated future events or ordinary tasks.

### 41.19 Location is likely a missing cross-cutting primitive

**Risk: MEDIUM/HIGH**

Competitor systems repeatedly model physical placement and addresses. Life Manager examples already imply this:

- where an appliance is installed;
- where inventory is stored;
- where a document relates to;
- where a person lives;
- where maintenance happened;
- where a purchase occurred.

A generic location/Place concept may become important across facets.

**Guardrail:** research a small Place/Location model before individual facets invent their own address/location structures.

### 41.20 External identity is distinct from Life Manager identity

**Risk: HIGH**

Email message IDs, bank transaction IDs, invoice numbers, serial numbers, order IDs, broker identifiers, retailer customer IDs, and document IDs all provide identity in external systems.

These cannot safely be treated as canonical IDs.

**Guardrail:** model external references explicitly, scoped by source/provider, and make them idempotency/reconciliation aids rather than canonical identity.

### 41.21 Connector synchronization has state and failure semantics

**Risk: HIGH**

Email and future connectors require cursors, incremental sync, retries, duplicate handling, provider rate limits, deleted/edited remote objects, and authentication expiry.

This is not merely an ingestion function.

**Guardrail:** define connector state separately from canonical data. Sync must be idempotent and resumable, and connector failure must never imply deletion of canonical data.

### 41.22 The "facet" boundary is still ambiguous

**Risk: MEDIUM/HIGH**

Some concepts naturally span multiple facets:

- Grocery vs Inventory vs Recipes
- Finance vs Debts
- Appliances vs Assets vs Documents vs Maintenance
- People vs Tasks vs Debts

If facets are treated as owners, the shared-model principle breaks. If everything is in Core, Core becomes enormous.

**Guardrail:** define facets as capability boundaries, not ownership boundaries. Core should own only truly cross-cutting semantics; domain modules should own domain rules.

### 41.23 The system may become too clever to operate manually

**Risk: HIGH**

The architecture assumes substantial automation, but users need a reliable escape hatch when automation is wrong.

If fixing an incorrect purchase requires understanding assertions, relationships, impacts, identity resolution, and projections, the system has failed its usability goal.

**Guardrail:** every automated workflow must have a simple user-facing correction path that operates at the user's conceptual level. Complexity should remain behind the UI.

### 41.24 The universal model can create UI overload

**Risk: MEDIUM**

A deeply interconnected system can expose too much context. A user looking at a grocery purchase should not need to see finance, inventory, recipe, relationship, provenance, and source machinery simultaneously.

**Guardrail:** the canonical model should be richer than the UI. Facets and contextual views should present only relevant projections of the underlying model.

### 41.25 MVP scope is still too broad

**Risk: CRITICAL**

The architecture currently names ingestion, documents, search, tasks, finance, inventory, people, household access, notifications, LLMs, relationships, and reconciliation as important concepts. This is enough to build a large system before validating the central hypothesis.

The central hypothesis is not "can we build a life dashboard?" It is:

> **Can a shared canonical model make cross-domain life workflows materially easier without becoming harder to maintain than separate modules?**

**Guardrail:** the first vertical slice should deliberately cross at least two domains and include ingestion + correction. A small number of deeply tested workflows is more valuable than many disconnected facets.

### 41.26 Competitor benchmark risk

**Risk: MEDIUM**

Life Manager is not competing only against other life managers. Users can assemble a very capable system from specialized tools such as finance managers, inventory systems, document archives, task systems, recipe systems, and personal OS projects.

Lifestack demonstrates a simpler modular approach, while newer LifeOS projects demonstrate schema-driven or timeline-centered approaches. These are useful falsification points for our canonical-model hypothesis.

**Guardrail:** maintain explicit benchmark scenarios and compare implementation complexity against at least one conventional modular design. Do not assume the canonical model wins merely because it is more unified.

## 42. Review Conclusions and Required Design Tests

The adversarial review does **not** invalidate the architecture. It identifies several areas where the current design is conceptually sound but operationally underspecified.

### Highest-risk items before implementation

1. Identity merge / split / unmerge semantics
2. Correction and impact reconciliation
3. Assertion vs canonical fact semantics
4. Provenance retention vs deletion/privacy
5. Scope/authorization rules
6. External identity and connector idempotency
7. Derived-state reconstruction/checkpointing
8. Recurring/expected-vs-actual semantics
9. LLM reprocessing/version lineage
10. MVP scope

### New mandatory stress tests

Add these to the existing stress-test set:

11. Same purchase merged from three sources, then unmerged
12. Incorrect identity merge affecting an appliance, warranty and finance record
13. Person becomes a User and must claim an existing Person identity
14. User deletes a source document while derived facts remain useful
15. Reprocessing the same source with a new LLM version produces a different answer
16. Email connector retries the same message repeatedly
17. A recurring bill produces one actual occurrence, then is corrected
18. A physical location changes for an appliance and inventory holding
19. Two users have different access to the same household object and related personal financial data
20. A derived projection is corrupted and rebuilt from authoritative information

### Review position

The architecture should **not** be simplified merely because these problems are difficult. They are inherent to the product's stated goal. However, the architecture should also not introduce abstractions until the stress tests prove they are needed.

The practical rule is:

> **Model the smallest semantics that make a cross-domain scenario correct, explainable, reversible, and queryable.**

This is the standard against which the canonical model should be judged going forward.
