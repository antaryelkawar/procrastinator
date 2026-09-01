# Life Manager — Technical Specification (Working Draft)

> This document captures technical decisions and recommendations for Life Manager. It is intentionally separate from the domain/design document. Technology choices should support the canonical model, not dictate it.

## 1. Current Technical Decisions

- **Deployment:** Docker Compose is the primary deployment target for the initial system.
- **Backend:** Go.
- **Frontend:** TypeScript + React.
- **Web experience:** Responsive web application, designed to work well on desktop and phones. A PWA-style experience is the initial direction; native mobile apps are not an initial requirement.
- **Architecture:** Modular monolith initially, with clear internal facet/core boundaries.
- **Async processing:** Background workers for ingestion, OCR, LLM processing, identity matching, reconciliation, and other long-running operations.
- **Database:** Not yet locked. MongoDB is already available for experimentation, but the canonical model should determine the final persistence choice.
- **LLM:** BYO local/cloud LLMs; provider abstraction is expected.

## 2. Recommended Baseline Stack

### 2.1 Backend

**Go + standard `net/http` + chi** is the preferred starting point.

Reasons:
- Keeps HTTP behavior close to the Go standard library.
- chi is lightweight, composable, and explicitly designed for maintainable REST services. citeturn817003search5
- Avoids taking on a large framework before the domain model stabilizes.

Fiber remains a viable alternative, but it is more opinionated and built around fasthttp. For Life Manager, maintainability and standard-library compatibility are more important than micro-optimizing HTTP throughput. citeturn817003search8

### 2.2 Frontend

**React + TypeScript + Vite 8.x**.

Vite 8 introduced a unified Rolldown-powered bundler, and Vite 8.1 shipped in June 2026. citeturn578739search8

Recommended frontend libraries:

- **TanStack Router** for type-safe routing and route-level data handling. Current documentation recommends current TypeScript releases and provides file-based and code-based routing. citeturn578739search1
- **TanStack Query** for server-state fetching, caching, mutations, retries, and synchronization. Its current React implementation is TypeScript-native. citeturn817003search3
- **shadcn/ui** for UI primitives, using **Base UI** as the current default foundation. shadcn/ui added first-class React Aria support in July 2026, so the component base should be selected deliberately rather than assuming Radix. citeturn578739search7turn578739search5
- **Tailwind CSS** for layout and responsive styling, subject to final version verification during implementation.
- **Zod** for runtime validation of API responses, form data, and external/LLM-derived data. Exact version should be pinned during implementation.

The frontend should not become a second domain implementation. Canonical domain rules remain server-side.

## 3. Persistence Direction

### 3.1 PostgreSQL should be the leading candidate

Although MongoDB is already available, PostgreSQL currently looks like the stronger fit for the core canonical system because Life Manager requires:

- strong relational integrity
- typed relationships
- transactions across multiple facets
- corrections and reconciliation
- financial semantics
- structured querying and aggregation
- ownership/scope constraints
- durable audit/provenance

PostgreSQL also gives us a path to vector search through **pgvector**, allowing embeddings to live beside canonical data rather than immediately introducing a second vector database. pgvector supports exact and approximate vector search, multiple vector representations, ACID/Postgres features, and current PostgreSQL versions including PostgreSQL 18. citeturn817003search2

This is a recommendation, not yet a final database decision.

### 3.2 Go database layer

If PostgreSQL is selected:

- **pgx v5** as the PostgreSQL driver/toolkit.
- Avoid a heavy ORM initially.
- Prefer explicit SQL and generated/type-safe query code where useful.

pgx is currently the stable v5 PostgreSQL driver/toolkit for Go and recommends its native interface when PostgreSQL is the sole target. citeturn578739search2

`sqlc` is a strong candidate for turning SQL into type-safe Go code, but the exact migration/query toolchain remains open.

## 4. Asynchronous Processing

Life Manager will need asynchronous work for:

- ingestion
- OCR/document extraction
- image processing
- LLM processing
- identity resolution
- reconciliation
- notifications
- re-indexing
- expensive derived-state rebuilding

### Initial recommendation: keep this simple

Use a database-backed or lightweight queue rather than introducing a workflow platform immediately.

Two current candidates:

**Asynq + Redis**
- Mature Go task queue.
- Retries, scheduling, recovery and concurrency controls are built in. citeturn578739search0

**River + PostgreSQL**
- Worth evaluating if PostgreSQL becomes the chosen primary store, because it can reduce infrastructure count.

**Temporal** is powerful and current, including an actively maintained Go SDK, but it is probably too much infrastructure for the first Life Manager deployment. It should be considered later for workflows that genuinely need durable orchestration, long-running stateful processes, or complex retries. citeturn817003search0turn817003search6

### Working recommendation

Start with **PostgreSQL + a Postgres-backed job system if practical**; otherwise use Asynq + Redis. Do not introduce Temporal in MVP.

## 5. Object / Document Storage

Life Manager will store original:

- PDFs
- receipts
- photos
- screenshots
- scanned documents
- other attachments

These should not live primarily inside the relational database.

### Requirement

Use an **S3-compatible object store abstraction**.

The initial local deployment can use MinIO or another S3-compatible implementation; the application should depend on the S3 API rather than a vendor-specific storage model.

The technical decision between MinIO and other self-hosted S3 implementations remains open.

## 6. API

The initial API should be HTTP/JSON.

### Working direction

- REST-style resource APIs for normal CRUD/query operations.
- Explicit domain commands for operations that cause consequential changes.
- Avoid exposing raw database CRUD for canonical objects.
- Use optimistic concurrency/version checks for user corrections where appropriate.
- API schemas should be generated/documented from the Go contracts where practical.

### Important distinction

The API should expose **domain operations**, not storage primitives.

For example:

`POST /purchases/{id}/correct`

is preferable to exposing arbitrary:

`PATCH /relationships/...`

for ordinary users.

## 7. Frontend State Architecture

Keep state separated into:

- **Server state:** TanStack Query.
- **UI-local state:** React state/hooks.
- **Small cross-screen client state:** add a dedicated state library only if real need appears.

Do not introduce Redux/Zustand/etc. by default.

## 8. Authentication / Authorization

Current product requirement:

- local/basic authentication
- optional OAuth/OIDC later
- people may exist without user accounts
- Personal and Household are the intended scopes
- ownership, participation, access and user identity are separate

### Technical direction

Implement authentication behind an internal interface so the domain does not care whether the identity came from:

- local credentials
- OIDC/OAuth
- another provider

Do not commit to Keycloak/Ory/etc. until the access model is defined.

Authorization should be enforced server-side and should understand:

`User → Person → Scope → Object → Access`

rather than treating a user account as the universal identity.

## 9. LLM Integration

The system should not embed itself around a single LLM vendor.

### Requirements

- provider abstraction
- local OpenAI-compatible endpoints where practical
- cloud providers where configured
- structured output support
- explicit model/task selection
- token/cost tracking when applicable
- provenance for every LLM-derived assertion

### LLM boundary

```text
Source
  ↓
Programmatic preprocessing
  ↓
LLM proposal / interpretation
  ↓
Assertion
  ↓
Validation / policy / human decision
  ↓
Canonical data
```

The LLM must not become the canonical source of truth or invent canonical relationship types.

## 10. Query Layer

Search should eventually become a query layer, not only keyword search.

Potential execution paths:

```text
User question
    ↓
Intent/query interpretation
    ↓
Validated query plan
    ├── structured query
    ├── relationship traversal
    ├── aggregation
    ├── full-text retrieval
    ├── semantic retrieval
    └── derived-state query
    ↓
Result
    ↓
Optional natural-language explanation
```

The LLM should help construct/interpret queries, not answer important factual questions purely from vector similarity.

### Initial search implementation

Keep it simple. PostgreSQL full-text search plus structured queries should be the baseline if PostgreSQL is selected. Add pgvector for semantic retrieval when a concrete use case requires it. citeturn817003search2

A dedicated search engine should not be introduced until PostgreSQL search proves insufficient.

## 11. Observability

Use **OpenTelemetry** for traces and metrics, with structured logs.

OpenTelemetry Go currently has stable traces and metrics, with logs listed as beta. citeturn817003search1

Minimum MVP observability:

- structured application logs
- request IDs / correlation IDs
- worker/job IDs
- processing duration
- LLM call metadata
- failure/retry counts
- basic health/readiness endpoints

## 12. Configuration / Secrets

- Environment variables for deployment configuration.
- Secrets must not be stored in Git.
- `.env.example` for discoverability.
- Configuration should distinguish required core settings from optional integrations.
- LLM provider credentials should be external configuration.

## 13. Testing

The architecture should emphasize domain-level tests before UI tests.

Priority:

1. Canonical model/domain invariants.
2. Relationship validation.
3. Assertion/provenance behavior.
4. Impact/reconciliation behavior.
5. Finance/inventory calculations.
6. API integration tests.
7. Ingestion fixtures.
8. Frontend component / flow tests.
9. End-to-end tests for a small number of critical workflows.

The canonical-model stress tests should become executable test fixtures once the model is defined.

## 14. Repository Structure

Recommended starting shape:

```text
life-manager/
├── apps/
│   └── web/
├── cmd/
│   ├── life-manager-api/
│   └── life-manager-worker/
├── internal/
│   ├── core/
│   ├── identity/
│   ├── ingestion/
│   ├── assertions/
│   ├── query/
│   ├── notifications/
│   ├── finance/
│   ├── inventory/
│   ├── documents/
│   ├── tasks/
│   └── people/
├── migrations/
├── packages/
│   └── shared-contracts/ (only if genuinely useful)
├── deployments/
│   └── docker/
├── docker-compose.yml
└── docs/
```

The exact repository layout is not final. The important boundary is that **domain/core code must not depend on HTTP, database, or LLM implementation details**.

## 15. Recommended Docker Compose Topology

Initial target:

```text
                    ┌───────────────┐
                    │   Web Browser │
                    └───────┬───────┘
                            │
                    ┌───────▼───────┐
                    │  Reverse Proxy│
                    └───────┬───────┘
                            │
              ┌─────────────▼─────────────┐
              │       Life Manager API       │
              │            Go             │
              └─────────────┬─────────────┘
                            │
          ┌─────────────────┼─────────────────┐
          │                 │                 │
 ┌────────▼────────┐ ┌──────▼───────┐ ┌──────▼─────────┐
 │   PostgreSQL    │ │ Job/Queue    │ │ Object Storage │
 │ + pgvector(opt) │ │              │ │   S3-compatible│
 └─────────────────┘ └──────────────┘ └────────────────┘
                            │
                    ┌───────▼────────┐
                    │ Worker(s)       │
                    │ Go              │
                    └───────┬────────┘
                            │
                 ┌──────────┴──────────┐
                 │ LLM / OCR / external│
                 │ integrations        │
                 └─────────────────────┘
```

The web app can be served as static assets behind the reverse proxy or from the API container depending on deployment simplicity.

## 16. What Still Needs a Technical Decision

The following should be discussed before implementation:

### High priority

- PostgreSQL vs MongoDB final selection.
- Object storage implementation.
- Job queue implementation.
- API contract strategy and code generation.
- Authentication implementation.
- Authorization model and scope enforcement.
- Canonical IDs / identity / versioning strategy.
- Migration/versioning strategy.
- Correction/reconciliation transaction boundaries.
- Exact domain module boundaries.

### Medium priority

- Full-text search implementation.
- Vector storage and embedding strategy.
- OCR engine(s).
- Document parsing pipeline.
- LLM provider abstraction.
- Notification delivery model.
- Background worker scaling.
- Secrets/configuration management.

### Later

- SSO / enterprise identity.
- External integrations.
- Service extraction / microservices.
- Kubernetes deployment.
- Mobile-native applications.
- Advanced workflow orchestration such as Temporal.
- Distributed tracing UI and advanced observability.

## 17. Initial Recommended Technology Set

| Area | Working choice | Confidence |
|---|---|---|
| Backend | Go | Decided |
| HTTP | net/http + chi | High |
| Frontend | React + TypeScript | Decided |
| Bundler | Vite 8.x | High |
| Routing | TanStack Router | High |
| Server state | TanStack Query | High |
| UI | shadcn/ui + Base UI | High |
| Validation | Zod | High |
| Styling | Tailwind CSS | High |
| Database | PostgreSQL | Recommended, not final |
| Go DB driver | pgx v5 | High if Postgres |
| Vector search | pgvector | Recommended if Postgres |
| Async jobs | Postgres-backed queue / Asynq | Open |
| Workflow engine | Temporal | Later, not MVP |
| Object store | S3-compatible | Recommended |
| Observability | OpenTelemetry | High |
| Deployment | Docker Compose | Decided |
| Mobile | Responsive Web/PWA direction | Decided for MVP |

## 18. Technical Principles

1. **The canonical domain model is the center of the system.**
2. **Technology should not leak into domain semantics.**
3. **Prefer fewer infrastructure components in the personal/self-hosted MVP.**
4. **Use mature boring technology for persistence and transactions.**
5. **Add specialized infrastructure only when a concrete requirement justifies it.**
6. **Background processing must be restartable and observable.**
7. **LLMs remain replaceable infrastructure, not domain dependencies.**
8. **The system should be easy to run from Docker Compose.**
9. **The web UI must be first-class on phones, not merely shrink desktop layouts.**
10. **Do not prematurely split the modular monolith into services.**

## 19. Document Processing

Life Manager will build its own ingestion/storage/workflow system and use libraries as internal components. It will not depend on a separately deployed document-management or document-processing product.

### Working direction

**Docling** is the primary document-understanding candidate. It supports PDF, DOCX, PPTX, XLSX, HTML, EPUB, images, email formats and more, and produces a structured document representation rather than only flat OCR text. Its current pipelines include layout understanding, reading order, table extraction, OCR and optional enrichment. citeturn240375search4turn240375search13

Docling is Python-based, so it should initially run as an internal worker/process behind the Go application rather than forcing Python into the main backend. The Go core owns source storage, processing jobs, provenance, assertions and canonicalization; the document worker returns structured intermediate results.

For PDF-specific manipulation inside Go, **pdfcpu** is a suitable supporting library for validation, optimization, splitting, merging, encryption, inspection and other PDF operations. citeturn240375search1turn240375search3

For OCR, Docling already supports multiple engines. Its current documentation lists RapidOCR, Nemotron-OCR, EasyOCR, Tesseract and others; RapidOCR currently supports PP-OCR v4/v5/v6 through multiple runtimes. citeturn240375search7

### Proposed document pipeline

```text
Source file/photo
      |
      v
Original object storage
      |
      +--> deterministic inspection / MIME / metadata
      |
      +--> PDF/document conversion
      |       |
      |       +--> text
      |       +--> layout
      |       +--> tables
      |       +--> images
      |       +--> OCR where required
      |
      v
Structured intermediate representation
      |
      v
Assertions / optional LLM interpretation
      |
      v
Canonical Life Manager data
```

The raw document remains authoritative evidence; Docling output is an intermediate representation and must not itself become canonical truth.

### Phone capture

For the future Android companion, Google's **ML Kit Document Scanner** is a useful client-side library for capturing clean document images. It provides automatic capture, edge detection, cropping, rotation and cleanup, and operates on-device; the scanner UI/models are delivered through Google Play services. citeturn240375search0turn240375search8

This is optional convenience functionality, not a requirement for the server-side document pipeline.

## 20. SMS Ingestion

Life Manager should **not require a native Android app for the initial product or development workflow**. The web/PWA is the primary UI and should remain fully usable from a development PC.

### Important Android constraint

A normal web/PWA application cannot directly read a phone\'s general SMS inbox. Browser SMS APIs such as WebOTP are designed around origin-bound one-time-password flows, not general SMS history/inbox access. citeturn247532search2turn247532search7

Android SMS access is also heavily restricted. Google Play currently limits SMS permissions such as `READ_SMS` to approved core use cases, generally requiring the app to be the default SMS/Assistant handler, with limited exceptions. SMS-based money-management is listed among the possible exception use cases but remains subject to Play review and policy requirements. citeturn247532search0turn247532search1

### Working decision

**Do not make Android SMS ingestion an MVP dependency.**

Instead, design the Life Manager ingestion layer so SMS can enter through multiple source paths:

```text
Development / desktop
  ├── manual SMS paste/import
  ├── uploaded/exported SMS data
  └── test fixtures

Future phone capture
  └── optional Life Manager Android connector
          │
          ▼
      Life Manager API
```

This keeps the entire ingestion and canonicalization pipeline testable locally on the development PC. An Android connector can be added later if the real-world value justifies the Android engineering and distribution constraints.

If a native connector is eventually built, it should be a **thin capture client**, not a second Life Manager application: it reads permitted SMS locally, normalizes/uploads source records, and leaves parsing, assertions, identity resolution, LLM processing, and canonicalization to Life Manager.

### SMS canonical flow

```text
SMS source
   │
   ├── desktop test/import
   └── future Android connector
            │
            ▼
       Authenticated API
            │
            ▼
        Source / SMS
            │
            ▼
    Deterministic parsing
            │
            ▼
         Assertions
            │
            ▼
 Identity resolution / LLM
            │
            ▼
 Canonical objects + impacts
```

The raw SMS should remain source evidence. The parsing and canonicalization pipeline must be testable without requiring Android hardware or an Android development loop.

## 21. Document and SMS Processing Boundary

The design deliberately separates **capture**, **processing**, and **canonicalization**:

- Capture components collect source material.
- Processing libraries convert source material into useful intermediate representations.
- Life Manager itself decides what the information means, what canonical objects it refers to, and what impacts it produces.

This preserves the project's central principle that external libraries and LLMs are interpreters/helpers, not sources of canonical truth.

## 22. Technical Decisions Added

| Area | Working choice | Status |
|---|---|---|
| Document processing | Docling as internal document-processing worker | Recommended |
| PDF manipulation | pdfcpu in Go where needed | Recommended |
| OCR | Docling-supported OCR; initially evaluate RapidOCR/PP-OCR | Recommended |
| Mobile document capture | Browser camera/upload initially; optional Android capture helper later | Optional |
| SMS capture | Desktop import/test fixtures initially; optional thin Android connector later | MVP does not depend on Android |
| SMS OTP APIs | Not suitable for general ingestion | Excluded |
| SMS processing | Life Manager-owned parsing/assertion pipeline | Recommended |

## 23. Reusable Libraries and Components

Life Manager should reuse focused open-source libraries where they solve a well-defined technical problem. Reusable libraries must remain implementation helpers: they do not own Life Manager's canonical semantics.

### 23.1 Backend / data access

- PostgreSQL is the current database choice.
- `pgx` remains the preferred PostgreSQL driver/toolkit.
- `sqlc` should be evaluated for generating type-safe Go data-access code from SQL rather than introducing a large ORM.
- `goose` is the current migration candidate because it supports embedded SQL/Go migrations and PostgreSQL.
- `shopspring/decimal` is a candidate for exact monetary/decimal arithmetic; money should not use binary floating-point.
- A sortable ID library such as KSUID may be evaluated, but IDs should not be chosen until canonical identity semantics are settled.

### 23.2 Authentication and authorization

- `golang.org/x/oauth2` and `coreos/go-oidc` can provide OAuth 2.0 / OpenID Connect plumbing.
- Apache Casbin is a candidate for application authorization because it supports ACL/RBAC/ABAC-style models and PostgreSQL-backed policy storage.
- Life Manager should own the semantic distinction between Person, User, Scope, Ownership, Participation and Access; an authorization library only evaluates policy.

### 23.3 Documents, OCR, and extraction

- Docling remains the preferred high-level document-understanding library/worker for PDFs, Office documents, images, layout, tables, OCR orchestration and structured export.
- `pdfcpu` is preferred for Go-native PDF inspection/manipulation where full document understanding is unnecessary.
- OCR engines should remain replaceable beneath the document-processing boundary. RapidOCR / PP-OCR are candidates for initial evaluation.
- Life Manager should preserve both the original artifact and processing output; parser/OCR output is evidence/intermediate data, not canonical truth.

### 23.4 Email and MIME processing

Email ingestion should use existing Go MIME/email parsing libraries where practical, with Life Manager owning normalization and assertion generation. We should avoid building MIME parsing, attachment handling, or charset decoding from scratch.

### 23.5 SMS ingestion

The MVP should not depend on an Android application.

Initial SMS ingestion should support file/import-based development flows so the complete pipeline can be developed and tested on the desktop. Android can later be a thin capture connector.

For Android SMS backup/import formats, an existing Go parser such as `sms-backup-and-restore-parser` can be evaluated rather than writing the XML parser from scratch.

The canonical SMS pipeline remains Life Manager-owned:

`Imported SMS -> normalized source -> assertions -> identity resolution -> canonical data`

### 23.6 Images and duplicate detection

Image metadata, resizing, thumbnailing and hashing should use focused libraries rather than custom implementations. Perceptual hashing can be added later if duplicate/near-duplicate detection becomes useful for documents and photos.

### 23.7 Notifications

Life Manager should use browser/PWA Web Push rather than introducing a separate notification service for the MVP. A Go Web Push library can handle VAPID and encrypted push payloads while Life Manager owns notification semantics and persistence.

### 23.8 Policy / rules

Do not introduce a general policy engine early. Identity-resolution and automation policies should initially use application defaults. If policy complexity grows, Apache Casbin or OPA can be evaluated. The first implementation should not introduce a policy DSL merely because one exists.

### 23.9 Observability

OpenTelemetry remains the preferred instrumentation layer for traces and metrics. Life Manager should make processing pipelines observable without coupling the domain model to a specific monitoring backend.

## 24. Reuse Principle

Reuse libraries for:

- Parsing
- OCR
- Document understanding
- Database access
- Migrations
- Authentication protocols
- Authorization enforcement
- Decimal arithmetic
- Push delivery
- Telemetry

Do not reuse a separate application as Life Manager's domain core. Life Manager should remain a new system with its own canonical model, assertions, relationships, impacts, reconciliation and query semantics.

## 25. Technical Items Still to Decide

The major remaining technical decisions are:

1. Exact PostgreSQL schema strategy and how canonical typed relationships are persisted.
2. Object/file storage: local filesystem vs S3-compatible storage abstraction.
3. SQL access strategy: handwritten SQL, `sqlc`, or another approach.
4. Async job implementation and whether Redis is justified or PostgreSQL-backed jobs are sufficient.
5. Authentication/session implementation for local auth plus optional OIDC.
6. Authorization model for personal vs household scope.
7. API contract and validation strategy.
8. Search implementation: PostgreSQL full-text, pgvector, structured query layer, and LLM query translation.
9. Document/processing worker boundary between Go and Python-based libraries such as Docling.
10. LLM provider abstraction and model capability discovery.
11. File storage, upload limits, retention, and deduplication.
12. Backup/restore strategy.
13. Local development/test fixtures for email, SMS, receipts, PDFs, photos and ambiguous matches.
14. CI/CD, container build strategy, and dependency update policy.
15. Security hardening that is deferred beyond the MVP but must remain architecturally possible.

## Competitive Architecture Reference — LifeStack

A current GitHub review of `sajankp/lifestack-api` shows a close architectural neighbor. LifeStack uses a modular monolith, PostgreSQL, React/TypeScript/Vite, Docker Compose, scheduled jobs, audit logging, and a separate Playwright E2E repository. It already covers spending, investing, tasks, imports, notifications, and cross-module workflows.

Life Manager should use this project as a reference, not as a dependency or architectural template to copy wholesale.

The main architectural distinction Life Manager should preserve is its deeper canonical-data model:

`Source -> Assertion -> Canonical data -> Typed relationships -> Impacts -> Derived state`

with identity resolution, provenance, correction/reconciliation, and multi-source ingestion treated as core semantics.

LifeStack is evidence that a modular-monolith + PostgreSQL + React + Docker Compose shape is viable. It is not evidence that Life Manager should copy its domain boundaries, three-repository layout, mobile-companion roadmap, or finance-first product strategy.
