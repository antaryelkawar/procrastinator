# Life Manager — Research & Decision Backlog

> Living backlog for questions that require research, comparison, experiments, or design validation before implementation. This is intentionally non-sequential.

## Purpose

Life Manager is a self-hosted personal + household life-management system. This document tracks the unresolved technical, domain, security, UX, and implementation questions that should be investigated before decisions are locked.

Research should favor reusable libraries and components over adopting another complete self-hosted application.

---

## 1. Canonical Model — Highest Priority

### 1.1 Core semantic vocabulary
- [ ] Define exact meaning and boundaries of:
  - Source
  - Assertion
  - Entity
  - Event
  - Fact
  - Relationship
  - Impact
  - Derived State
- [ ] Decide whether Event is a distinct semantic category or simply a class of canonical object/occurrence.
- [ ] Define what is authoritative vs derived vs materialized.
- [ ] Define canonical identity and stable IDs.

### 1.2 Relationships
- [ ] Define relationship-type schema.
- [ ] Define source/target type constraints.
- [ ] Define relationship direction and inverse semantics.
- [ ] Define relationship metadata/provenance.
- [ ] Determine when generic relationships such as `contains` are sufficient versus when a specific relationship type is required.
- [ ] Define how new relationship types are added by hand-crafted Life Manager facets.

### 1.3 Assertions / provenance
- [ ] Define assertion lifecycle:
  `source → interpretation → assertion → validation → canonical data`.
- [ ] Define assertion versioning.
- [ ] Define confidence vs validation/approval status.
- [ ] Define how multiple assertions support the same fact or relationship.
- [ ] Define how conflicting assertions are represented.
- [ ] Define permanent retention requirements for internal assertions.

### 1.4 Identity resolution
- [ ] Define rules for matching evidence to an existing canonical object.
- [ ] Define create-new vs attach-to-existing outcomes.
- [ ] Research deterministic matching strategies.
- [ ] Research multi-model agreement strategies.
- [ ] Define default confidence policies.
- [ ] Define future global/facet-specific policy hierarchy.
- [ ] Test with SMS + receipt, duplicate emails, repeated photos, invoices, and bank transactions.

### 1.5 Corrections and reconciliation
- [ ] Define correction model without exposing users to low-level graph/state mutation.
- [ ] Research versioned facts/assertions.
- [ ] Define relationship reconciliation when source information changes.
- [ ] Define impact reconciliation when source information changes.
- [ ] Define what happens when evidence moves from object A to object B.
- [ ] Define rebuild/recovery semantics for derived state.
- [ ] Test amount corrections, relationship corrections, identity reassignment, deletion/reversal, and conflicting evidence.

---

## 2. Finance Domain — Dedicated Research

### 2.1 Minimal financial vocabulary
- [ ] Research and distill the minimum useful concepts from established open-source finance systems.
- [ ] Evaluate:
  - Financial account
  - Money movement
  - Allocation / attribution
  - Debt / receivable
  - Payment / repayment
  - Settlement
  - Balance
  - Asset
  - Liability
  - Investment holding
  - Valuation
  - Loan
  - Interest
  - Fees
  - Currency / exchange rate
- [ ] Decide what belongs in MVP vs later.

### 2.2 Existing open-source finance implementations
- [ ] Study Firefly III for transaction/account semantics and reconciliation.
- [ ] Study Sure / Maybe for net worth, investments, loans, assets and wealth concepts.
- [ ] Study Ghostfolio for investment/portfolio concepts.
- [ ] Evaluate whether any libraries/components can be reused rather than application code.
- [ ] Avoid importing a full finance application's architecture wholesale.

### 2.3 Net worth
- [ ] Define assets and liabilities needed for net worth.
- [ ] Define valuation snapshots vs transactions.
- [ ] Define treatment of property, vehicles, gold, investments, loans, receivables and personal assets.
- [ ] Define historical net-worth calculation.

### 2.4 Investments
- [ ] Determine minimum model for stocks, mutual funds, ETFs, fixed income, crypto, etc.
- [ ] Research market-data libraries/APIs.
- [ ] Separate security/asset definition from holding and valuation.
- [ ] Determine manual vs automated valuation in MVP.

### 2.5 Loans and liabilities
- [ ] Research principal, interest, repayment, fees and outstanding balance semantics.
- [ ] Determine whether bank/loan statements should create money movements or loan-state facts.

### 2.6 Reconciliation
- [ ] Define imported statement → asserted transactions → canonical money movements → reconciliation workflow.
- [ ] Research CSV/OFX/QIF formats and useful Go libraries.
- [ ] Define duplicate transaction matching.

---

## 3. Inventory / Physical World

### 3.1 Product / acquisition / holding model
- [ ] Validate current separation of:
  - Product
  - Purchase
  - Purchase Line
  - Inventory Holding
  - Physical Asset
  - Consumption / Usage
  - Adjustment / Wastage / Transfer
- [ ] Test with groceries, consumables, medicines/household supplies, electronics and appliances.

### 3.2 Quantity model
- [ ] Define quantity + unit representation.
- [ ] Research unit libraries/conversion libraries.
- [ ] Define decimal precision and rounding.
- [ ] Define packages vs units vs weight vs volume.
- [ ] Define partial consumption.
- [ ] Define spoilage/wastage.

### 3.3 Asset model
- [ ] Define physical asset identity.
- [ ] Serial numbers, model numbers, purchase identity, ownership, lifecycle.
- [ ] Warranty and maintenance relationships.

### 3.4 Inventory identity resolution
- [ ] Match receipt lines/photos/manual entries to existing products and holdings.
- [ ] Define ambiguous product handling.

---

## 4. Documents and Media Processing

### 4.1 Document ingestion
- [ ] Evaluate Docling for PDF/Office/document understanding.
- [ ] Evaluate Go-native wrappers/integration strategy for Python-based processors.
- [ ] Research pdfcpu for low-level Go PDF operations.
- [ ] Evaluate OCR choices including RapidOCR/PaddleOCR and alternatives.
- [ ] Research image preprocessing libraries.
- [ ] Define document conversion pipeline and failure handling.

### 4.2 Document types
- [ ] PDF
- [ ] DOCX/XLSX/PPTX
- [ ] HTML
- [ ] Email / EML
- [ ] Images
- [ ] Scanned documents
- [ ] Receipts/invoices
- [ ] Statements
- [ ] Warranty documents

### 4.3 Document storage
- [ ] Compare filesystem, S3-compatible object storage, and Postgres large-object/blob approaches.
- [ ] Decide content hashing and deduplication.
- [ ] Research image thumbnail/preview generation.
- [ ] Research malware/safety scanning options for uploaded files.

### 4.4 Search/indexing of documents
- [ ] Full-text extraction and indexing.
- [ ] PDF/image text indexing.
- [ ] Metadata indexing.
- [ ] Semantic indexing.

---

## 5. Email Integration

### 5.1 Source connectors
- [ ] IMAP support/library options.
- [ ] Gmail OAuth / API integration.
- [ ] Microsoft Graph / Outlook integration.
- [ ] Manual `.eml` import.
- [ ] Determine MVP connector order.

### 5.2 Email source model
- [ ] Message identity and provider IDs.
- [ ] Thread/conversation identity.
- [ ] Headers, body, MIME structure and attachments.
- [ ] Deduplication across connectors/imports.
- [ ] Cursor/incremental sync strategy.
- [ ] Folder/label selection.

### 5.3 Email security
- [ ] OAuth token storage/encryption.
- [ ] Refresh-token lifecycle.
- [ ] Minimum scopes.
- [ ] Revocation/disconnect behavior.
- [ ] Multi-account support.

### 5.4 Email processing
- [ ] Deterministic MIME parsing.
- [ ] HTML → text.
- [ ] Attachment extraction.
- [ ] Receipt/invoice/order email identification.
- [ ] Entity matching to existing Life Manager objects.
- [ ] Thread-level vs message-level assertions.

### 5.5 Operational constraints
- [ ] Large mailbox handling.
- [ ] Rate limits.
- [ ] Backfill strategy.
- [ ] Retry/error handling.
- [ ] Privacy boundaries and per-account scopes.

---

## 6. SMS Integration

### 6.1 MVP strategy
- [ ] Keep SMS a generic source type.
- [ ] Support manual paste/import first.
- [ ] Support test fixtures for development.
- [ ] Defer native Android connector.

### 6.2 Future capture options
- [ ] Research Android connector requirements and Play policy when needed.
- [ ] Evaluate export formats such as SMS Backup & Restore.
- [ ] Determine whether any desktop/mobile companion is justified later.

### 6.3 SMS processing
- [ ] Sender normalization.
- [ ] Template/bank-message parsing.
- [ ] Amount/date/reference extraction.
- [ ] Merchant/entity normalization.
- [ ] Duplicate detection.
- [ ] Financial transaction matching.

---

## 7. LLM Architecture

- [ ] Define BYO-LLM provider interface.
- [ ] Local model support.
- [ ] Cloud model support.
- [ ] OpenAI-compatible endpoint support where useful.
- [ ] Define structured-output strategy.
- [ ] Define validation of LLM outputs.
- [ ] Define model/prompt provenance in assertions.
- [ ] Define model confidence vs policy approval.
- [ ] Define multi-LLM convergence strategy.
- [ ] Define tool/query access for LLMs.
- [ ] Restrict LLM from inventing canonical relationship/entity types.
- [ ] Research current Go libraries for OpenAI-compatible clients and structured output.

---

## 8. Query / Search Layer

- [ ] Define query-layer abstraction before choosing search technology.
- [ ] Structured query generation.
- [ ] Relationship traversal.
- [ ] Aggregation/querying of derived state.
- [ ] Full-text document search.
- [ ] Semantic/vector search.
- [ ] LLM-assisted query interpretation.
- [ ] Validate/sanitize generated queries.
- [ ] Determine whether pgvector is sufficient initially.
- [ ] Research PostgreSQL full-text search vs dedicated search engines.
- [ ] Research hybrid retrieval approaches.

---

## 9. Notifications and Human Decisions

- [ ] Define durable decision/request model.
- [ ] Dynamic response schemas:
  - Yes/no
  - Single choice
  - Multiple choice
  - Free-form
- [ ] Decision expiration.
- [ ] Retry/reminder behavior.
- [ ] Read/unread/acknowledged state.
- [ ] Web notifications.
- [ ] Web Push/PWA.
- [ ] Email notifications later.
- [ ] Future mobile notification connector.

---

## 10. Identity, Access and Household

- [ ] Define Personal vs Household scope semantics.
- [ ] Define Owner vs Participant vs User vs Actor.
- [ ] Define access independently of participation.
- [ ] Define invitation/membership model.
- [ ] Define future linking of a Person to a User account.
- [ ] Research OIDC/OAuth libraries for Go.
- [ ] Evaluate simple local auth for MVP.
- [ ] Research authorization options (custom policy vs Casbin/etc.).
- [ ] Define object-level authorization boundaries.

---

## 11. Time Semantics

- [ ] Define:
  - occurred_at
  - recorded_at
  - effective_at
  - valid_from / valid_until
  - expires_at
- [ ] Decide which timestamps are mandatory per primitive.
- [ ] Handle timezone/local date semantics.
- [ ] Research recurring schedules and calendar libraries where needed.

---

## 12. PostgreSQL Data Architecture

- [ ] Confirm PostgreSQL over MongoDB.
- [ ] Define schema strategy for typed canonical objects.
- [ ] Define relationship storage.
- [ ] Define assertions/versioning.
- [ ] Define provenance storage.
- [ ] Define impact storage.
- [ ] Define extension-field strategy without arbitrary JSON everywhere.
- [ ] Evaluate JSONB only where appropriate.
- [ ] Define indexes for cross-facet querying.
- [ ] Define transaction boundaries.
- [ ] Research pgvector.
- [ ] Research full-text search.
- [ ] Research partitioning only if needed.
- [ ] Research row-level security only if it materially helps the access model.

### PostgreSQL tooling
- [ ] pgx
- [ ] sqlc
- [ ] Goose (or alternative migration tool)
- [ ] shopspring/decimal
- [ ] Testcontainers / database integration testing options

---

## 13. Async Processing / Jobs

- [ ] Decide whether Postgres-backed jobs are sufficient.
- [ ] Evaluate Asynq + Redis vs Postgres-backed queue options.
- [ ] Define job idempotency.
- [ ] Define retries/backoff.
- [ ] Define dead-letter/error handling.
- [ ] Define long-running LLM processing.
- [ ] Define cancellation.
- [ ] Avoid Temporal unless workflow complexity actually requires it.

---

## 14. API / Backend Architecture

- [ ] Go HTTP stack selection.
- [ ] API style: REST vs other approach.
- [ ] Error model.
- [ ] Validation strategy.
- [ ] Versioning strategy.
- [ ] OpenAPI generation/maintenance.
- [ ] TypeScript client generation.
- [ ] Authentication middleware.
- [ ] Authorization middleware.
- [ ] Idempotency support for ingestion APIs.
- [ ] File upload API design.

---

## 15. Frontend / PWA

- [ ] React + TypeScript.
- [ ] Vite.
- [ ] TanStack Router.
- [ ] TanStack Query.
- [ ] shadcn/ui + Base UI / accessibility strategy.
- [ ] Zod.
- [ ] Tailwind.
- [ ] PWA strategy.
- [ ] Offline behavior requirements.
- [ ] Mobile-first responsive behavior.
- [ ] Camera/file upload from browser.
- [ ] Installability on Android/iOS.
- [ ] Push notification support.
- [ ] Accessibility and keyboard navigation.

---

## 16. Local Development / Testing

- [ ] Entire system runnable with Docker Compose.
- [ ] Local development database.
- [ ] Seed data.
- [ ] Deterministic ingestion fixtures.
- [ ] Fixture library for receipts, PDFs, SMS, emails and conflicting evidence.
- [ ] End-to-end browser tests.
- [ ] Contract tests for connectors.
- [ ] Integration tests for canonical-model transitions.
- [ ] Correction/reconciliation regression suite.
- [ ] LLM mocked/fixed-response test mode.
- [ ] Optional local LLM development mode.

---

## 17. Observability

- [ ] OpenTelemetry.
- [ ] Structured logs.
- [ ] Request correlation IDs.
- [ ] Job tracing.
- [ ] LLM call tracing and cost/latency metrics.
- [ ] Ingestion throughput/errors.
- [ ] Reconciliation failures.
- [ ] Audit trail independent of operational logs.

---

## 18. Security / Privacy

- [ ] Encryption in transit.
- [ ] Encryption at rest.
- [ ] Secret/token storage.
- [ ] OAuth token encryption.
- [ ] File access controls.
- [ ] PII handling.
- [ ] Backup encryption.
- [ ] Database backup/recovery strategy.
- [ ] Data export.
- [ ] Data deletion semantics.
- [ ] Audit trail security.
- [ ] Prompt/model privacy controls.
- [ ] Define what data may leave the self-hosted instance when using cloud LLMs.

---

## 19. Deployment / Operations

- [ ] Docker Compose topology.
- [ ] One deployable application initially.
- [ ] Separate worker containers only where useful.
- [ ] Database container.
- [ ] Object storage choice.
- [ ] Reverse proxy/TLS strategy.
- [ ] Upgrade/migration process.
- [ ] Backup/restore.
- [ ] Health checks.
- [ ] Configuration/secrets management.
- [ ] Future extraction path for independently deployable services.

---

## 20. Reusable Libraries / Components to Evaluate

This is a research list, not an adoption list.

### Backend / data
- [ ] pgx
- [ ] sqlc
- [ ] Goose
- [ ] shopspring/decimal
- [ ] Casbin
- [ ] go-oidc / oauth2
- [ ] OpenTelemetry Go

### Documents / images
- [ ] Docling
- [ ] pdfcpu
- [ ] RapidOCR
- [ ] PaddleOCR
- [ ] Image processing / thumbnail libraries
- [ ] Content hashing / perceptual hashing

### Search / retrieval
- [ ] PostgreSQL full-text search
- [ ] pgvector
- [ ] Hybrid retrieval libraries

### Frontend
- [ ] React
- [ ] Vite
- [ ] TanStack Router
- [ ] TanStack Query
- [ ] shadcn/ui / Base UI
- [ ] Zod
- [ ] Tailwind
- [ ] PWA tooling

### Ingestion formats
- [ ] IMAP libraries
- [ ] Gmail API/OAuth libraries
- [ ] Microsoft Graph libraries
- [ ] EML/MIME parsers
- [ ] CSV/OFX/QIF parsers
- [ ] SMS Backup & Restore XML parsers

---

## 21. Explicitly Avoid for Now

- [ ] Separate self-hosted finance application as a dependency.
- [ ] Separate self-hosted document-management application.
- [ ] Separate self-hosted workflow/automation platform.
- [ ] Separate graph database solely because relationships exist.
- [ ] Microservices before the canonical model stabilizes.
- [ ] Native Android application for MVP SMS capture.
- [ ] Temporal unless workflow complexity proves it necessary.
- [ ] Dedicated search engine before PostgreSQL search is evaluated.
- [ ] Community plugin marketplace architecture.

---

## 22. Research Rules

1. Prefer current official documentation and active repositories.
2. Prefer libraries over complete applications.
3. Prefer components with permissive/open licenses compatible with Life Manager's intended use.
4. Evaluate maintenance activity, release cadence, issue health, security history, and ecosystem maturity.
5. Do not select a tool merely because it is the newest; reduce operational complexity where possible.
6. Validate libraries against Life Manager's actual scenarios before adoption.
7. Separate domain semantics from implementation technology.
8. Record rejected candidates and the reason for rejection for major infrastructure choices.

---

## 23. Candidate Decision Records to Create Later

- [ ] PostgreSQL persistence architecture
- [ ] Object/file storage
- [ ] Search/retrieval
- [ ] Email connector strategy
- [ ] Document pipeline
- [ ] SMS ingestion strategy
- [ ] LLM provider abstraction
- [ ] Authentication
- [ ] Authorization
- [ ] Async job system
- [ ] API contract
- [ ] Frontend component system
- [ ] PWA strategy
- [ ] Backup/restore
- [ ] Security model
- [ ] Canonical model implementation

---

## 24. Current Technical Decisions

- Backend: Go
- Frontend: TypeScript + React
- Primary deployment: Docker Compose
- UI: responsive web application, mobile-compatible
- MVP phone strategy: web/PWA; no native Android app required
- Database direction: PostgreSQL
- MongoDB: available for experimentation but not the selected foundation
- Architecture: modular application / modular monolith direction
- LLM: BYO, optional
- Processing principle: deterministic first, LLM where useful
- Raw source data: preserved
- Assertions: permanent internal provenance/audit, normally invisible to users
- Canonical relationships: typed, directional, hand-defined
- Search: query-layer concept rather than embeddings-only search
- Finance: minimal domain to be derived from research before implementation
- Documents/SMS/email: ingestion sources, not separate applications

---

## 25. Parking Lot — Future Requirements

- [ ] Decisions / commitments
- [ ] Preferences
- [ ] Rules / policies
- [ ] Recurring patterns
- [ ] Advanced investment analytics
- [ ] Advanced notification channels
- [ ] Native mobile connector
- [ ] Additional external integrations
- [ ] Service extraction when scale/ownership requires it


## Research Finding — LifeStack overlap

GitHub project reviewed: `sajankp/lifestack-api` (Lifestack), current as of 2026-08-21.

Lifestack is a close conceptual neighbor: an open-source personal/household operating system built around shared finance, investing, tasks, imports, notifications, dashboard, PostgreSQL, and a modular monolith. Its web client is React/TypeScript/Vite and it already has strong audit, auth, investment, import, scheduler, and E2E infrastructure.

Important conclusions for Life Manager:

- Treat LifeStack as a reference/competitor, not a dependency.
- Do not duplicate its finance implementation blindly; use it as a reference for finance/account/investment semantics and engineering practices.
- Life Manager's intended differentiation is deeper cross-facet canonical semantics: source -> assertion -> canonical data -> typed relationships -> explicit impacts -> derived state, with provenance and correction/reconciliation built into the model.
- Life Manager also intends ingestion-first workflows across email, SMS, documents and photos, rather than primarily finance-led capture.
- Life Manager should avoid inheriting LifeStack's three-repository/mobile-companion architecture unless a concrete need appears; local PC development and one deployable web app remain priorities.
- LifeStack demonstrates that a modular monolith with PostgreSQL, React, Docker Compose, audit logging, and Playwright is a practical shape for this class of product.

Further research to compare: LifeStack canonical/domain boundaries, finance model, import/source metadata, audit/versioning, scheduler/outbox approach, and cross-module orchestration.
