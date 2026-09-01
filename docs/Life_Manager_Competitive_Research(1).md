# Life Manager — Competitive & Design Research

> Deep comparative research of current self-hosted/open-source products relevant to a personal + household life manager. Research date: 21 August 2026.
>
> Purpose: identify design patterns worth reusing, gaps in our current requirements, traps to avoid, and areas where Life Manager's canonical-model approach is genuinely differentiated.

## 1. Executive Summary

Life Manager is entering a crowded space, but the competitors are fragmented by starting point:

- personal finance / wealth systems
- household ERP and pantry systems
- document archives
- personal relationship managers
- task/project systems
- recipe/meal systems
- generic personal operating systems / life dashboards
- home inventory / asset systems

The strongest existing systems are generally successful because they choose a narrow domain and model it deeply. The problem we are trying to solve is the boundary between those domains.

The most important conclusion from the research is therefore not that Life Manager needs more modules. It is that the shared model needs to solve several cross-cutting concerns better than ordinary modular applications:

1. source/evidence and provenance
2. identity resolution across inputs and domains
3. typed relationships
4. reconciliation when facts change
5. explicit impacts and derived state
6. scope, ownership, participation and access
7. time semantics
8. recurring commitments and schedules
9. queryability across heterogeneous data
10. safe automation with human decisions

The competitor research also exposes several requirements we had underweighted:

- a universal capture/inbox concept
- import preview, validation and rollback
- document classification and learned matching
- subscriptions/bills/recurring obligations
- locations and physical placement
- richer relationship memory
- offline/browser resilience
- saved views/queries
- external-reference and reconciliation identities
- state transitions for things such as warranties, tasks and debts
- source-specific sync cursors and idempotency
- deletion/retention/export semantics
- explicit uncertainty and conflict handling

These should become research/design backlog items. They do not all belong in MVP.

---

## 2. Competitor Landscape

### 2.1 Lifestack — closest architectural competitor

Repository: `sajankp/lifestack-api`

Lifestack is explicitly a personal/household operating system, currently centered on spending, investing, tasks, notifications, imports/exports and summaries. It uses a modular monolith with PostgreSQL, Redis and a React frontend; its engineering documentation emphasizes router → service → repository layering, application services for cross-module workflows, database-enforced workspace scoping, append-only audit logging, scheduled jobs and substantial E2E coverage.

Notable capabilities include:

- spending transactions and budgets
- investing with lot-accurate cost basis and corporate actions
- recurring transactions and recurring todos
- notifications and weekly summaries
- imports/exports with source metadata
- workspace isolation
- audit logging
- deterministic demo reset and E2E hooks
- planned documents, memory, health and AI access

**What Life Manager should learn:**

- clear module boundaries inside a modular monolith
- application-layer orchestration instead of modules calling each other directly
- database-enforced scope isolation
- append-only audit events
- deterministic imports and rollback hooks
- test fixtures that exercise cross-module workflows
- treating AI as a layer above deterministic business capabilities

**Where Life Manager differs:**

Lifestack is primarily module/application-workflow oriented. Life Manager is deliberately experimenting with a deeper canonical layer: source → assertion → canonical information → typed relationships → impacts → derived state.

**Risk:** our additional abstraction is only justified if it makes cross-domain problems materially easier. We should continually compare our model against Lifestack's simpler application-service approach.

Source: https://github.com/sajankp/lifestack-api

---

### 2.2 Homechart — closest household breadth competitor

Homechart positions itself as an all-in-one household organizer. Its feature surface includes budgeting/savings, calendar/events, contacts, cooking/meals, health/allergies, inventory/pantry, notes/wiki, planning/to-dos, rewards/gifts, secrets/passwords, shopping/grocery and household permissions. It supports self-hosting and uses a Go server.

Notable design elements:

- one household place for many data types
- a unified calendar across tasks, transactions, meals and events
- links between budgets, projects and shopping lists
- contacts including friends, family, businesses and stores
- pantry/inventory extending beyond groceries
- notes/wiki as a household knowledge layer
- secrets/passwords as a household responsibility domain
- granular household permissions
- support for complicated family structures

**New implications for Life Manager:**

- Calendar may be a cross-cutting primitive, not merely another facet.
- Locations/places such as stores, homes and service providers may be first-class references.
- A household knowledge layer may be important independently of documents.
- Secret/credential storage is a possible future facet but should remain separated from ordinary documents.
- Household access needs more than simple membership.

Source: https://homechart.app/about/

---

### 2.3 Grocy — strongest household inventory/food competitor

Grocy describes itself as an ERP beyond the fridge. It covers stock, products, shopping lists, recipes, meal planning, chores, tasks, calendar export and other household operations. Its model is notably more operational than a simple pantry list.

Important design patterns:

- products have quantity units and locations
- stock can be added, consumed and transferred
- recipes can check inventory fulfillment
- missing/expired products can flow to shopping lists
- chores have scheduled execution
- barcode and camera scanning are supported
- REST API exposes application behavior
- self-hosting is simple and mature

**What we should learn:**

- inventory should be operational, not merely catalog-based
- consumption and transfer are explicit domain movements
- product/unit/location relationships matter
- recipes should calculate requirements against inventory
- scanning can be implemented in the browser without native apps
- domain-specific APIs are valuable for deterministic automation

**New missed factor:** location and storage location should be investigated as first-class inventory context.

Source: https://github.com/grocy/grocy

---

### 2.4 Mealie — strongest recipe/meal workflow reference

Mealie focuses on recipe management, meal planning, shopping lists and family sharing. It supports URL recipe extraction, ingredient normalization, meal planning and supermarket-oriented shopping lists.

Important design patterns:

- import from URLs
- recipe definition separate from meal-plan occurrence
- ingredient requirements flow into shopping
- shopping lists can be structured around stores/sections
- REST API enables external automation
- family-oriented UI and deployment

**New implication:** recipe requirements, planned meals, shopping requirements, inventory and purchases form a chain that needs real semantic integration rather than independent recipe and grocery screens.

Source: https://github.com/mealie-recipes/mealie

---

### 2.5 Homebox — strongest asset/inventory reference

Homebox focuses on household inventory and organization. It supports images, documents, warranties, purchases, maintenance, locations, labels, custom fields, CSV import/export and responsive use across devices. The current continuation is written largely in Go.

Important patterns:

- location-based organization
- item images
- warranty tracking
- purchase price/date
- maintenance tracking
- custom fields
- QR labels
- CSV import/export
- mobile-friendly UI

**New implications:**

- physical placement/location is distinct from ownership
- QR/barcode identities can become practical source identifiers
- asset/document relationships need strong file categorization
- custom fields are valuable, but Life Manager should constrain them with the promotion model already adopted

Source: https://github.com/sysadminsmedia/homebox

---

### 2.6 Paperless-ngx — strongest document-processing reference

Paperless-ngx is a mature document archive with OCR, PDF/A archiving, tags, correspondents, document types, storage paths, custom fields, full-text search, saved views, automatic matching, learned matching and optional LLM/RAG features.

Important patterns:

- retain unaltered originals alongside normalized archives
- OCR only when necessary
- metadata classification before persistence
- automatic matching rules
- machine-learned classification from prior user behavior
- suggestions rather than always applying uncertain classifications
- custom fields with search/query support
- rich full-text search with relevance and highlighting
- retag/reprocess existing documents after classification rules change
- task-like inbox processing for documents requiring action

**Very important Life Manager lesson:** document processing needs a **reprocessable pipeline**. If our taxonomy, parser, extraction prompt or matching logic improves, existing source documents should be able to be reprocessed without losing history.

Also important: Paperless distinguishes document type, topic/tag and correspondent rather than dumping everything into one label system. This is a useful pattern for Life Manager's controlled extensibility.

Source: https://github.com/paperless-ngx/paperless-ngx

---

### 2.7 Monica / Bonds — strongest relationship-memory reference

Monica is a personal relationship manager covering contacts, relationships, reminders, birthdays, interactions, notes, tasks, addresses, pets, gifts, debts, journal and documents. A newer Go + React project called Bonds takes inspiration from Monica and adds vault isolation, recurring reminders, full-text search, CardDAV/CalDAV, API tokens and MCP.

Important patterns:

- relationship types are explicit
- interaction history matters
- contact data is more than name/email/phone
- reminders derive from relationships
- birthdays and life events become structured temporal data
- gifts and debts relate to people
- people can have pets, households and other context
- data freshness / verification status is useful
- calendar/contact sync is practical

**New Life Manager requirements:**

- relationship history should be considered a facet of person state
- contact freshness/verification may be worth modeling later
- contact/address/calendar interoperability should be investigated
- a person record can have multiple types of contextual information without becoming a generic notes bucket

Sources:
- https://github.com/monicahq/monica
- https://github.com/naiba/bonds

---

### 2.8 Firefly III — strongest bookkeeping semantics reference

Firefly III provides a deep personal finance model including double-entry accounting, recurring transactions, rules, budgets, goals, attachments, reports, multiple currencies and a broad REST API.

Important patterns:

- explicit financial accounts
- double-entry ledger semantics
- recurring transactions
- transaction rules
- attachments and metadata
- budgets and goals
- categories/tags
- reconciliation concepts

**Life Manager lesson:** money should not be represented as a generic "transaction amount" field. Financial accounts, movements, allocations, reconciliation and derived balances need explicit semantics.

Source: https://github.com/firefly-iii/firefly-iii

---

### 2.9 Maybe / Sure — strongest wealth-management reference

Maybe's vision explicitly separates a finance core from optional finance applications. Core concepts include checking, savings, credit cards, loans, equity, crypto and transactions; applications include net worth, budgeting, goals, retirement and investment tracking. The current community fork is Sure.

Important patterns:

- accounts as a central abstraction
- family support
- loans/liabilities/equity/crypto as distinct account/asset categories
- net worth as an output across assets and liabilities
- transaction rules
- budgeting and goals as higher-level views over finance

**New Life Manager financial requirements:**

- liabilities and loans are not just debts between people
- asset classes need a distinction between the underlying asset and the financial account holding it
- net worth is a derived cross-domain view
- goals/planning belong above basic movements

Sources:
- https://github.com/maybe-finance/maybe
- https://github.com/AllSage/sure
- https://github.com/maybe-finance/maybe/wiki/vision

---

### 2.10 Ghostfolio — strongest investment reference

Ghostfolio focuses on stocks, ETFs and cryptocurrencies with multi-account tracking, portfolio performance, imports/exports, risk analysis and PWA/mobile-first behavior. It uses PostgreSQL and Redis and exposes a public API.

Important patterns:

- investment activity types such as buy, sell, dividend, fee and interest
- account-backed holdings
- performance windows
- portfolio analytics
- risk/exposure analysis
- multi-account investment tracking
- mobile-first PWA

**Life Manager lesson:** investments are not just another transaction category. They have holdings, quantities, valuation, corporate actions, market prices and performance calculations.

Source: https://github.com/ghostfolio/ghostfolio

---

### 2.11 Actual Budget — budgeting and synchronization reference

Actual Budget is an important reference for envelope-style budgeting, local-first behavior and synchronization. It is useful less as a canonical financial model and more as a UX/data-ownership model.

Patterns worth studying:

- local-first interactions
- sync and conflict resolution
- budgeting as a planning layer rather than transaction categorization alone
- import/export as a fundamental part of user trust

Research target: exact current synchronization and budgeting semantics before adopting any model.

---

### 2.12 LifeOS projects — personal operating-system UX references

Several current LifeOS projects have converged on:

- dashboard/command center
- command palette / quick capture
- global search
- goals/tasks/habits/journal/finance
- PWA/mobile installation
- JSON/file-based interoperability
- AI-assisted inbox processing
- customizable modules
- daily/weekly reviews

One current LifeOS implementation uses a static dashboard over Markdown/JSON, while another uses interconnected typed nodes, tasks, notes, journals, habits, finances, contacts and goals.

**Life Manager lesson:** there is a recurring UX pattern of a **universal capture/inbox**. Life Manager currently has ingestion sources but has not explicitly defined the user-facing capture surface. It should.

Sources:
- https://github.com/siddath/lifeOS
- https://github.com/karim-coder/life-os
- https://github.com/lunanoir21/Life-os-project

---

### 2.13 HomeHub / Yuvomi — modern household UX references

HomeHub combines invoices, shopping, meals, maintenance, calendars, documents, contacts, inventory and household tasks in a self-hosted web application. Yuvomi similarly offers a privacy-first family planner covering tasks, calendar, budget, groceries, meals and health with a polished mobile-first PWA.

Important patterns:

- household dashboard
- recurring household chores
- calendar integration
- document/attachment handling
- shopping and meals connected
- mobile-first household workflows
- backups and exports

**Lesson:** a good household experience is likely to be much more calendar/attention oriented than a database browser.

Sources:
- https://github.com/ThomasRooyakkers/HomeHub
- https://github.com/ulsklyc/yuvomi

---

### 2.14 Vikunja — task-system reference

Vikunja and its clients provide a mature task model with projects, labels, recurring tasks, reminders, subtasks, relationships/dependencies, attachments, views and APIs. It also has a strong ecosystem of desktop/mobile clients.

**Life Manager lesson:** tasks should not be reduced to `title + due_date + completed`. We should anticipate recurrence, reminders, parent/child structure, relationships, attachments, assignment, and future dependencies.

The same source can create a task, but tasks should remain domain objects with explicit semantics.

Source: https://vikunja.io / https://github.com/go-vikunja/vikunja

---

## 3. Design Elements We Were Missing or Underweighting

### 3.1 Universal Capture / Inbox

Multiple systems converge on a capture-first UX.

Life Manager should have a generic capture concept capable of receiving:

- free text
- photo
- file
- pasted SMS
- pasted email
- forwarded content
- manual transaction
- quick task
- voice input later

Capture should not require the user to decide the destination facet first.

This fits the ingestion architecture better than separate “add expense”, “add task”, “upload document” flows as the only entry points.

**Research/decision:** define Capture/Inbox semantics and how items graduate into canonical information.

---

### 3.2 Reprocessing as a First-Class Capability

Documents, email, imports and LLM-generated interpretations should be reprocessable.

Examples:

- improved OCR
- better extraction prompt
- new relationship type
- new identity resolver
- corrected product catalog
- new classification rules
- changed canonical schema

A source should therefore have a processing lineage, not just a final extracted record.

This reinforces Assertions as permanent internal provenance.

---

### 3.3 Import Lifecycle

Imports should not be a single “upload CSV” operation.

The mature pattern is:

```text
Source
  ↓
Import batch
  ↓
Preview / parse
  ↓
Validation
  ↓
Identity matching
  ↓
User decisions if needed
  ↓
Commit
  ↓
Reconciliation
```

Important properties:

- idempotency
- external IDs
- duplicate detection
- row-level errors
- preview before commit
- rollback where possible
- provenance to import batch and source row
- resumability

This should apply to finance, email, documents and future connectors.

---

### 3.4 Recurring Commitments

Competitors repeatedly represent recurring transactions, bills, chores, reminders or contact follow-ups.

This suggests a future cross-domain concept around **commitments/recurrence**, distinct from the future “rules/preferences/patterns” concept already in the design.

Examples:

- rent every month
- service AC every 12 months
- call parent every two weeks
- buy milk every 4 days
- insurance premium annually
- subscription renews monthly

This should be researched as a reusable temporal capability, while remaining out of MVP unless needed.

---

### 3.5 Location / Place

Inventory systems and household systems repeatedly need:

- home
- room
- cabinet
- pantry shelf
- store
- service provider location
- workplace

Places may also be useful in purchases and relationships.

A general location model could eventually support both physical location and semantic place references, but we should avoid prematurely creating a generic GIS abstraction.

---

### 3.6 External Identity / Reference

Systems that ingest statements, CSVs, emails and broker feeds need stable external identifiers.

Life Manager should distinguish:

- internal canonical ID
- source identifier
- source system
- source account
- source message/document ID
- import batch

Identity resolution needs both semantic matching and deterministic external-reference matching.

---

### 3.7 Conflict, Uncertainty and Contradiction

We already have confidence and human decisions, but the competitor research strengthens the case for a first-class distinction between:

- extracted evidence
- proposed assertion
- accepted assertion
- conflicting accepted assertions
- user correction
- unresolved conflict

The system should not force every field into one scalar value if two trusted sources disagree.

Example:

```text
SMS: ₹40,000
Receipt: ₹39,500
Bank statement: ₹40,000
```

The system needs to retain the disagreement rather than silently overwriting one source.

---

### 3.8 Saved Views / Query Definitions

Document systems and task systems strongly benefit from saved filtered views.

Life Manager's future query layer should support reusable queries such as:

- “warranties expiring within 60 days”
- “unpaid debts”
- “documents requiring attention”
- “groceries below threshold”
- “subscriptions renewing this month”
- “tasks awaiting someone else”

A saved view is not a canonical fact; it is a query artifact over canonical state.

---

### 3.9 Attention / Inbox State

Documents can need review, transactions can be uncategorized, identities can be unresolved, imports can fail, warranties can expire, and tasks can become overdue.

These are all different domain states, but the UX needs a common **attention surface**.

This likely belongs above facets:

```text
Canonical state
    ↓
Attention candidates
    ↓
Unified inbox / dashboard / notifications
```

This is different from treating Notification as the source of truth.

---

### 3.10 Data Freshness

Monica and other systems expose freshness/verification concepts. This matters for life data:

- Is this person's phone number still valid?
- Is this appliance still owned?
- Is this warranty document current?
- Was this bank account balance last synced yesterday?
- Is this market price current?

Life Manager should consider metadata such as:

- observed_at
- recorded_at
- source freshness
- last verified
- stale/unavailable status

---

### 3.11 Offline / Resilience UX

Current life-management products increasingly assume phones and intermittent connectivity.

Even if Life Manager remains a web/PWA MVP, the UI should be designed for graceful interruption:

- optimistic quick capture where safe
- retryable uploads
- queued captures where practical
- explicit pending/sync state
- no silent loss

This is another reason not to make a native Android app a prerequisite for the core experience.

---

### 3.12 Calendar as a Cross-Facet Surface

Calendar repeatedly appears as the place where tasks, meals, maintenance, bills, events and appointments meet.

We should investigate whether calendar deserves to be:

- a normal facet
- a shared projection over timed objects
- or both

The third option may be strongest: canonical timed objects + calendar projection.

---

### 3.13 Subscription / Recurring-Service Semantics

Home and finance products both encounter subscriptions and recurring services.

Examples:

- streaming services
- insurance
- SaaS
- gym
- maintenance contracts
- internet
- utilities

The domain is different from a one-time transaction because it has:

- provider
- recurring commitment
- expected charge
- renewal period
- cancellation state
- next expected occurrence
- actual observed charges

This should be explicitly researched.

---

### 3.14 Asset Lifecycle

Homebox and financial wealth systems show that “asset” is too broad.

We should eventually distinguish:

- physical asset
- financial asset
- investment holding
- consumable inventory
- digital license/subscription
- liability

Each has lifecycle states such as acquired, active, transferred, sold, disposed, expired or settled.

---

### 3.15 Documents vs Knowledge

A document archive and a knowledge base are not the same.

Paperless-style document management answers:

> “Where is the warranty PDF?”

A knowledge system answers:

> “What do we know about this appliance?”

Life Manager should preserve the raw document while promoting relevant facts into the canonical model. A future knowledge/query layer can synthesize those facts and source documents.

---

### 3.16 Notifications vs Tasks vs Attention

We should avoid conflating:

- a thing that needs action
- a task someone accepted
- a notification delivered through a channel
- an overdue condition

These should be separate semantic concepts even if the UI combines them.

---

## 4. Finance: Additional Factors to Research

The competitor survey strengthens the need to model at least:

- financial accounts
- account types
- money movements
- allocations / attribution
- transfers
- debts / receivables
- loans / liabilities
- repayments / settlements
- investments / holdings
- market valuations
- FX rates
- fees
- interest
- dividends
- corporate actions
- recurring transactions / bills
- budgets
- goals
- net worth
- reconciliation
- external source IDs
- imported statements
- attachment/document provenance

Potentially later:

- tax lots
- cost basis
- retirement accounts
- property
- vehicles
- insurance cash value
- private equity
- crypto
- employee equity

The MVP should not implement all of these. The research question is which minimal financial primitives allow the rest to be added without semantic breakage.

Primary references: Firefly III, Maybe/Sure, Ghostfolio, Lifestack, Actual Budget.

---

## 5. Inventory / Household Factors to Research

Required distinctions already identified:

- Product
- Purchase
- Purchase Line
- Inventory Holding
- Physical Asset
- Consumption/Usage
- Adjustment/Wastage/Transfer

Additional factors surfaced:

- location
- barcode/QR code
- expiry / best-before
- lot/batch
- unit conversions
- substitutions
- recipe fulfillment
- shopping list provenance
- reorder thresholds
- minimum/maximum stock
- preferred store
- price history
- package size vs normalized unit price
- household ownership vs personal ownership
- disposal/sale/transfer
- maintenance schedule

Grocy should be treated as the primary design reference for this domain.

---

## 6. People / Relationships Factors to Research

Beyond contact details:

- explicit relationship types
- interaction history
- reminders/follow-ups
- birthdays/life events
- household membership
- people who are not users
- businesses/service providers
- pets/dependents
- gifts
- debts
- items loaned/borrowed
- freshness/verification
- addresses and communication methods
- calendar/contact interoperability

Monica/Bonds are the primary references.

---

## 7. Document / Ingestion Factors to Research

The document pipeline should likely support:

```text
Source acquisition
  ↓
Normalization
  ↓
OCR / text extraction
  ↓
Document structure extraction
  ↓
Metadata classification
  ↓
Entity/relationship candidate extraction
  ↓
Assertions
  ↓
Identity resolution
  ↓
Human decision if needed
  ↓
Canonicalization
```

Cross-cutting requirements:

- original preservation
- content hashing
- duplicate detection
- versioning
- reprocessing
- searchable text
- attachment relationships
- classification rules
- custom/extension fields
- confidence
- source lineage
- deletion/retention
- export

Paperless-ngx is the primary workflow reference.

---

## 8. Email / SMS / Connector Factors to Research

All connectors should terminate in the same source model.

Email needs:

- IMAP
- Gmail OAuth
- Microsoft OAuth
- `.eml` import
- message/thread identity
- attachment extraction
- MIME normalization
- sender/recipient identities
- source cursor
- deduplication
- idempotent sync
- selective mailbox scope

SMS needs:

- pasted/manual messages for MVP
- import formats
- backup archive parsing
- optional future Android connector
- source message IDs where available
- sender normalization
- connector permissions

Connector design should stay outside the canonical model.

---

## 9. AI / LLM Design Factors

The competitor review reinforces the current rule:

> LLM = interpreter/proposal generator, never canonical truth.

Research areas:

- structured output / JSON Schema
- local model routing
- provider abstraction
- multimodal inputs
- confidence calibration
- multi-model agreement
- deterministic validation
- safe query generation
- retrieval and provenance
- prompt/version lineage
- reprocessing
- privacy boundaries
- model-specific capabilities
- cost controls

An important additional principle:

**LLM proposals should be bounded by the canonical schema.**

For example, a document extractor should be allowed to select among known document types and relationship types, not invent new ontology terms.

---

## 10. Query / Search Design Factors

Do not call this simply “search.”

Life Manager will eventually need a query layer supporting:

- exact structured filtering
- aggregation
- relationship traversal
- full-text search
- semantic retrieval
- derived-state queries
- temporal queries
- source/provenance queries
- saved views
- natural-language interpretation

The LLM should translate user language into a safe query plan, then the query engine executes it against canonical/queryable data.

---

## 11. Architecture Lessons

The competitor landscape strongly supports:

### Keep

- modular monolith initially
- PostgreSQL
- strict internal boundaries
- application orchestration for cross-facet workflows
- source preservation
- append-only audit/provenance
- deterministic imports
- asynchronous processing
- Docker Compose
- responsive PWA
- explicit API

### Avoid initially

- microservices
- generic plugin marketplace
- arbitrary graph database
- separate search cluster
- workflow engine such as Temporal without a concrete requirement
- native mobile app as a prerequisite
- generic JSON ontology
- community-defined canonical relationship types

### Reuse as libraries

Use focused libraries for:

- MIME parsing
- PDF/document parsing
- OCR
- barcode/QR decoding
- image processing
- OAuth/OIDC
- exact decimal arithmetic
- database access/migrations
- web push
- OpenTelemetry
- query/vector extensions

Do not make another self-hosted application the domain core.

---

## 12. New Canonical Concepts to Consider

These are **candidates**, not final decisions.

### Capture
A raw user-initiated or connector-created inbox object before canonicalization.

### Source
The durable original evidence/artifact.

### Assertion
An internal statement about reality or another canonical object, permanently retained for provenance.

### Decision
A durable human response to an unresolved question or system proposal.

### Identity Link
The accepted relationship that says multiple sources/evidence refer to the same canonical object.

### Import Batch
A unit of external ingestion with preview, idempotency and rollback semantics.

### External Reference
A source-specific identity or identifier attached to a canonical object.

### Attention Candidate
A derived condition that may warrant display/action, but is not itself a notification or task.

### Commitment / Recurrence
A future candidate primitive for subscriptions, repeating bills, maintenance schedules and relationship follow-ups.

These should be evaluated before database design.

---

## 13. Stress Tests We Should Run Before Implementation

1. Receipt + bank SMS + email invoice all describe one purchase.
2. Receipt + bank SMS conflict on amount.
3. Same appliance is purchased, documented, warranted, serviced and eventually sold.
4. Grocery recipe creates a shopping requirement, but inventory already satisfies it.
5. Grocery consumption reduces inventory and changes the next expected purchase.
6. Friend pays for dinner; allocation creates a debt; partial repayment occurs later.
7. A credit-card transaction is imported while the purchase is already known from a receipt.
8. An investment buy creates a money movement and a holding; market value changes without a transaction.
9. A loan repayment changes liability balance and bank balance simultaneously.
10. A recurring bill is expected but the actual charge differs.
11. A warranty document is replaced by a new warranty extension document.
12. Two emails, one PDF and one SMS all point to the same subscription.
13. A person exists without being a user, then later becomes a user.
14. Household owns an asset but one individual pays for it.
15. A document is private to one person but describes a household asset.
16. One source is reprocessed after the ontology changes.
17. A user corrects a source so that it changes the canonical object identity, not just a field value.
18. Two trusted sources remain contradictory and cannot be automatically reconciled.
19. The database is restored from backup while asynchronous ingestion work is pending.
20. The user asks a natural-language question that requires aggregation + relationship traversal + document retrieval.

---

## 14. Strategic Comparison

Life Manager should not attempt to beat every competitor inside every vertical.

Instead:

- Firefly III / Maybe / Ghostfolio prove that finance requires deep semantics.
- Grocy / Mealie prove that food and inventory require operational workflows.
- Paperless-ngx proves that document ingestion, classification and reprocessing deserve serious infrastructure.
- Monica/Bonds prove that people and relationship memory are their own substantial domain.
- Vikunja proves that task management has richer semantics than a todo CRUD screen.
- Homebox proves that physical asset/location/warranty management is useful and practical.
- Homechart/HomeHub/Yuvomi prove there is demand for household-wide aggregation.
- Lifestack proves that a small team can make a modular personal OS with finance, investing, tasks and notifications without microservices.
- LifeOS projects prove that capture, dashboard, daily review and mobile/PWA UX matter.

Life Manager's strongest differentiated hypothesis remains:

> **The system should model the connections and consequences between these domains as first-class semantics, while preserving source evidence and making automated interpretation reversible/reconcilable.**

That is the part that must be validated before we add more features.

---

## 15. Decision: What We Should NOT Copy

We should not copy a competitor's entire module structure merely because the feature exists.

In particular:

- Do not turn every domain into an isolated mini-application.
- Do not put everything into generic JSON/custom fields.
- Do not treat the dashboard as the canonical model.
- Do not let the LLM invent domain semantics.
- Do not make notifications the source of truth.
- Do not make materialized state user-editable without a domain operation.
- Do not use embeddings as the only query mechanism.
- Do not require native Android for core ingestion architecture.
- Do not choose a graph database because the domain has relationships.

---

## 16. Recommended Next Research Areas

Highest value:

1. Canonical identity + correction/reconciliation model
2. Minimal financial ontology
3. Minimal inventory ontology
4. Capture/inbox model
5. Import lifecycle and connector identity/idempotency
6. Time/recurrence semantics
7. Query layer design
8. Access/scope model
9. Document reprocessing pipeline
10. Unified attention model

Only after those:

- database schema
- API contract
- module decomposition
- worker architecture
- library selection beyond the current shortlist

---

## 17. Primary References

- Lifestack: https://github.com/sajankp/lifestack-api
- Homechart: https://homechart.app/about/
- Grocy: https://github.com/grocy/grocy
- Mealie: https://github.com/mealie-recipes/mealie
- Homebox: https://github.com/sysadminsmedia/homebox
- Paperless-ngx: https://github.com/paperless-ngx/paperless-ngx
- Monica: https://github.com/monicahq/monica
- Bonds: https://github.com/naiba/bonds
- Firefly III: https://github.com/firefly-iii/firefly-iii
- Maybe: https://github.com/maybe-finance/maybe
- Sure: https://github.com/AllSage/sure
- Ghostfolio: https://github.com/ghostfolio/ghostfolio
- LifeOS: https://github.com/siddath/lifeOS
- Life OS: https://github.com/karim-coder/life-os
- Life OS project: https://github.com/lunanoir21/Life-os-project
- HomeHub: https://github.com/ThomasRooyakkers/HomeHub
- Yuvomi: https://github.com/ulsklyc/yuvomi
- Vikunja: https://vikunja.io / https://github.com/go-vikunja/vikunja
- Actual Budget: https://github.com/actualbudget/actual

---

## 18. Research Caveat

This document intentionally compares current projects and publicly visible product/design information rather than claiming exhaustive coverage of every life-management product on the internet. The landscape changes continuously. The purpose is to identify representative, high-value references and design patterns before Life Manager implementation.
