# Design: invoice-warranty-asset-flow (redesign)

## Context

Full redesign of the archived first slice (`archive/2026-08-24-invoice-warranty-asset-flow`), driven by 9 user requirements: clean architecture with DI, interface guards, `procrastinator-*` naming, structured+metadata Asset model, reuse, hard multitenancy, thinking-free LLM integration, a layered directory layout, and a stronger extraction prompt informed by the two real sample documents in `sample-data/` (LG AMC invoice `INVNAG2302754.pdf`, IFB AMC contract `Annual Maintenance Contract.pdf`).

The existing implementation in `procrastinator-backend/` (single module `procrastinator-backend`, Go 1.27.0, chi v5.3.2, pgx v5.10.0, goose v3.27.3, PostgreSQL 18) is treated as a **greenfield rewrite within the same module**: the old `cmd/` and `internal/` trees are deleted first, then the new tree is built package by package. The dev database is disposable; `migrations/00001_init.sql` is rewritten in place (no data migration). Stack, endpoints, and the Gemini endpoint (`gemma-4-26b-a4b-it` at `https://generativelanguage.googleapis.com/v1beta/openai/`) are unchanged.

## Goals / Non-Goals

**Goals** — everything in the archived design, plus: tenant isolation on every row and request; Asset with typed core + open `metadata` JSONB; `amc` document type; a proven no-thinking system prompt with parser-level `<thought>` stripping as safety net; interfaces in `commons-data` with compile-time guards; no duplicated SQL (single repo implementation, transaction-bound via factory); ingest/identity testable without Docker.

**Non-Goals** — no authn/authz (tenant header is trusted as-is), no per-tenant encryption or DB-level row security policies (application-level scoping only), no pagination, no async pipeline, no object storage, no frontend, no data migration from the old `lifemanager` database.

## Decisions

### D1: Architecture — clean layers, DI, one dependency rule

Layers mapped to top-level package roots inside the single module:

| Layer | Package root | Contents | May import |
|---|---|---|---|
| shared kernel | `commons-data` | entities (Asset, Source, Document, Extraction), boundary interfaces, tenant context helpers, pure helpers (normalize, dates, extraction parser) | stdlib only |
| platform | `commons-server` | `config` (Config + Load), `httpx` (JSON envelope, error helpers, tenant middleware) | stdlib + `commons-data` (+ chi for middleware) |
| domain/application | `procrastinator-core` | `identity` (resolution + merge), `ingest` (orchestration) | stdlib + `commons-data` only |
| infrastructure | `procrastinator-infra` | `postgres` (pool, migrations, repos, repo factory), `llm` (chat client, extractor adapter, prompt), `filestorage` | stdlib + `commons-data` + pgx/goose (postgres) |
| delivery | `procrastinator-api` | chi router + handlers (package `api`), `cmd/procrastinator-api/main.go` (package main) | all of the above |

Hard rules:
- `procrastinator-core` MUST NOT import `net/http`, `pgx`, or any `procrastinator-infra` package. The extraction parser moves from `internal/llm/extraction` into `commons-data` precisely so `core/ingest` can parse LLM output without touching infrastructure.
- No `procrastinator-infra` package imports another `procrastinator-infra` package.
- `main` is the only composition root: it builds concrete infra implementations and injects them as `commons-data` interfaces into services and the API server.
- Verified mechanically in the final gate via `go list -deps` assertions (no `pgx`/`net/http` under `procrastinator-core/...`).

### D2: Directory layout — single module, five package roots

Single Go module (`module procrastinator-backend`, `go.mod` unchanged). Multi-module workspace rejected: one consumer (this repo), atomic refactors matter more than independent versioning, and `go.work`/`replace` plumbing adds failure modes with no payoff.

```
procrastinator-backend/
  go.mod                                  # module procrastinator-backend (unchanged)
  docker-compose.yml                      # postgres:18, db/user procrastinator
  .env / .env.example                     # PROCRASTINATOR_* vars (.env gitignored)
  migrations/00001_init.sql               # rewritten schema (tenant_id, metadata, amc)
  commons-data/
    asset.go                              # Asset, UpdateFields, DocType
    source.go                             # Source
    document.go                           # Document, DocumentWithSource
    extraction.go                         # Extraction (typed core + Metadata map)
    parse.go                              # ParseExtraction: thought-strip, validate, metadata capture
    repository.go                         # AssetRepository, SourceRepository, DocumentRepository, ErrNotFound
    services.go                           # Extractor, FileStorage, StoredFile
    tx.go                                 # RepoFactory (+ InTransaction)
    tenant.go                             # WithTenant/TenantFrom context helpers, ErrNoTenant
    normalize.go                          # NormalizeSerial, NormalizeName, CollapseWhitespace
    dates.go                              # ParseDate: ISO + DD-MMM-YY + DD.MM.YYYY + DD-MMM-YYYY
  commons-server/
    config/config.go                      # Config + Load(src) — PROCRASTINATOR_* vars
    httpx/json.go                         # WriteJSON, WriteError
    httpx/tenant.go                       # TenantMiddleware (X-Tenant-ID → ctx)
  procrastinator-core/
    identity/resolve.go                   # Resolve(ctx, AssetRepository, Extraction) — pure
    ingest/service.go                     # Process orchestration over interfaces only
  procrastinator-infra/
    postgres/store.go                     # Open/Migrate/TruncateAll, pgRepoFactory, InTransaction
    postgres/assets.go                    # pgAssetRepo  (+ guard)
    postgres/sources.go                   # pgSourceRepo (+ guard)
    postgres/documents.go                 # pgDocumentRepo (+ guard)
    llm/client.go                         # generic OpenAI-compatible chat client
    llm/extractor.go                      # Extractor adapter (+ guard)
    llm/prompt.go                         # document-extraction system prompt
    filestorage/storage.go                # local FS storage (+ guard)
  procrastinator-api/
    server.go                             # package api — chi router, middleware chain
    handlers.go                           # package api — 4 handlers + DTOs
    cmd/procrastinator-api/main.go        # package main — ONLY env reader / composition root
```

### D3: Naming — `procrastinator-*` everywhere

- Binary: `procrastinator-api` (from `procrastinator-api/cmd/procrastinator-api`).
- Config struct: `Config` in `commons-server/config`; `Load(src map[string]string)` unchanged in shape (nil → real env, map → testable).
- Env vars (all renamed; only `main` reads real env):

| Var | Required | Default |
|---|---|---|
| `PROCRASTINATOR_DATABASE_URL` | yes | — |
| `PROCRASTINATOR_LLM_BASE_URL` | yes | Gemini OpenAI-compatible URL in `.env` |
| `PROCRASTINATOR_LLM_API_KEY` | yes | `.env` (gitignored) |
| `PROCRASTINATOR_LLM_MODEL` | yes | `gemma-4-26b-a4b-it` in `.env` |
| `PROCRASTINATOR_STORAGE_DIR` | yes | — |
| `PROCRASTINATOR_HTTP_ADDR` | no | `:8080` |
| `PROCRASTINATOR_MAX_UPLOAD_BYTES` | no | `20971520` |
| `PROCRASTINATOR_LLM_TIMEOUT` | no | `60s` |
| `PROCRASTINATOR_TEST_DATABASE_URL` | integration tests, read once in TestMain/helper | — |
| `PROCRASTINATOR_TEST_LLM_INTEGRATION` | real-LLM gate | unset |

- `docker-compose.yml`: `POSTGRES_USER/POSTGRES_PASSWORD/POSTGRES_DB=procrastinator`, healthcheck updated. Test database: `procrastinator_test`.
- Final gate greps the backend tree for `life-manager`, `lifemanager`, `LM_` — zero matches (archived openspec artifacts and user research docs excluded).

### D4: Asset model — structured core + generic metadata

`migrations/00001_init.sql` (rewritten):

```sql
sources(id uuid PK, tenant_id text NOT NULL, filename, content_type, byte_size,
        storage_path, sha256, uploaded_at)
assets(id uuid PK, tenant_id text NOT NULL, brand, model, serial_number,
       norm_serial, norm_brand, norm_model, purchase_date date, warranty_end date,
       price numeric, currency char(3),
       doc_type text NOT NULL DEFAULT 'other' CHECK (doc_type IN ('invoice','warranty','amc','other')),
       metadata jsonb NOT NULL DEFAULT '{}', created_at, updated_at)
  UNIQUE (tenant_id, norm_serial) WHERE norm_serial IS NOT NULL
documents(id uuid PK, tenant_id text NOT NULL, asset_id REFERENCES assets(id),
          source_id UNIQUE REFERENCES sources(id),
          doc_type CHECK (doc_type IN ('invoice','warranty','amc','other')),
          extracted_fields jsonb, raw_extraction jsonb, created_at)
```

- **Structured core** = the typed columns the system acts on (identity, uniqueness, future warranty-expiry queries). `warranty_start` leaves the core (user-specified core list); start dates remain available via metadata (`amc_start`, `warranty_start`).
- **`doc_type` on Asset** = classification of the most recently ingested Document (last write wins) — reflects the asset's current document context. On `Document` it is immutable per row.
- **`metadata`** = `map[string]any` in Go, `jsonb` in PG. Untyped bag the LLM fills (`amc_card_number`, `icr_number`, `cgst_rate`, `customer_name`, `service_branch`, …). Never used for identity. Keys validated snake_case (`^[a-z][a-z0-9_]{0,63}$`); values any JSON; total payload sanity-capped (~8 KiB) at parse time.
- **Metadata merge**: shallow per-key overlay — SQL `metadata = assets.metadata || $n::jsonb`; absent keys retained, present keys overwritten. Null values in incoming metadata are dropped at parse time (never stored, never erase).
- Identity resolution (norm columns, precedence, race-safe insert) is unchanged in logic, extended with `tenant_id` predicates; uniqueness index becomes `(tenant_id, norm_serial)`.

### D5: Multitenancy — header → context → scoped queries

- **Tenant id**: opaque string, `^[A-Za-z0-9_-]{1,64}$`. No tenant registry table in this slice.
- **Middleware** (`commons-server/httpx/tenant.go`): reads `X-Tenant-ID`; missing/invalid → `400 {"error":"missing or invalid X-Tenant-ID header"}`; valid → `commons-data.WithTenant(ctx, id)`. Mounted on all routes.
- **Context helpers** (`commons-data/tenant.go`): `WithTenant`, `TenantFrom(ctx) (string, error)` returning `ErrNoTenant` when absent.
- **Repositories**: every method starts with `tenant, err := TenantFrom(ctx)` — fail-closed, no cross-tenant fallback. All SQL gains `WHERE tenant_id = $n`; inserts stamp `tenant_id`. FK safety across tenants is application-level (asset lookup and document insert happen in one tenant-scoped transaction); composite FKs rejected as needless complexity.
- **Tests**: explicit tenants (`test-tenant`, `test-tenant-b`); integration suite includes cross-tenant isolation cases (same serial in two tenants → two assets; foreign asset id → 404).

### D6: LLM prompt — thinking-free, classify-then-extract, metadata-aware

Research finding (verified against the live endpoint): no API parameter disables thinking (`thinking_config`/`thinking` → 400); a strong system prompt eliminates thought blocks entirely (≈6 output tokens vs 450+). Design: **prompt first, stripping as safety net**.

- **Reusable client** (`llm/client.go`): generic chat-completions call `Chat(ctx, ChatRequest{SystemPrompt, UserText, ContentType, Data}) (string, error)` — not tied to any prompt. Envelope unchanged: `response_format: {"type":"json_object"}`, `temperature: 0`, Bearer auth, document as base64 `image_url` data-URI (PDF included — the endpoint rejects `document` parts, verified in the archived apply phase).
- **Extractor adapter** (`llm/extractor.go`): binds the document-extraction prompt (`llm/prompt.go`) to the client; satisfies `commons-data.Extractor`. Other prompts later = new adapters, same client.
- **System prompt structure** (single system message, exact wording pinned by request-shape tests):
  1. Anti-thinking header: *"You are a strict JSON extraction engine. You output ONLY a valid JSON object. No thinking. No `<thought>` tags. No explanations. Just JSON."*
  2. Task: classify the document (`invoice` | `warranty` | `amc` | `other` — with one-line definitions incl. "amc = Annual Maintenance Contract invoice or contract") then extract.
  3. Core schema: the 8 typed keys with formats — dates always ISO `YYYY-MM-DD` ("convert `12-Jan-24`, `12.01.2024`, `12-Jan-2024`"); price as decimal string without symbols; currency as ISO 4217 (`INR`, `USD`).
  4. Metadata contract: everything else useful goes under `"metadata"` with snake_case keys; worked examples: `invoice_number`, `customer_po_ref`, `system_order_no`, `amc_card_number`, `amc_type` (e.g. `"Gold Plan 3Yr"`), `amc_start`, `amc_end`, `icr_number`, `customer_name`, `customer_address`, `customer_phone`, `service_branch`, `cgst_rate`, `cgst_amount`, `sgst_rate`, `sgst_amount`, `igst_rate`, `cess`, `total_tax`. Unknown/illegible → omit the key, never guess.
  5. One compact example input→output pair (AMC invoice) anchoring the shape.
- **Parser** (`commons-data/parse.go`): strip `(?s)<thought>.*?</thought>` (safety net), require `{`, decode with `UseNumber`, exhaust the stream, validate core fields as before (dates via `commons-data/dates.go`: ISO, RFC3339, `02-Jan-06`, `02.01.2006`, `02-Jan-2006`), degrade classification outside the 4-value enum to `other`, capture `metadata` (drop non-snake_case keys and null values, cap size). `RawPayload` still retained verbatim.
- **Failure mapping** unchanged: timeout/unreachable/non-2xx/unparseable → typed error → 502. No fabricated fallbacks.

### D7: Interfaces + guards — and killing the SQL duplication

All boundary interfaces live in `commons-data`:

```go
type AssetRepository interface {           // tenant from ctx; superset of old assets.Repo + identity.AssetStore
    FindBySerial(ctx, normSerial string) (Asset, bool, error)
    FindByBrandModel(ctx, normBrand, normModel string) (Asset, bool, error)
    Create(ctx, Asset) (Asset, error)
    CreateOnConflictSerial(ctx, Asset) (Asset, bool, error)
    UpdateFields(ctx, id string, f UpdateFields) (Asset, error)
    GetByID(ctx, id string) (Asset, error)
    List(ctx) ([]Asset, error)
}
type SourceRepository interface   { Create(ctx, Source) (Source, error); GetByID(ctx, id string) (Source, error) }
type DocumentRepository interface { Create(ctx, Document) (Document, error); GetByID(ctx, id string) (Document, error); ListByAsset(ctx, assetID string) ([]DocumentWithSource, error) }
type Extractor  interface { Extract(ctx, contentType string, data []byte) ([]byte, error) }
type FileStorage interface { Put(data []byte) (StoredFile, error) }   // sniffing inside Put
type RepoFactory interface {
    Assets() AssetRepository; Sources() SourceRepository; Documents() DocumentRepository
    InTransaction(ctx context.Context, fn func(ctx context.Context, tx RepoFactory) error) error
}
```

- Guards in every impl: `var _ data.AssetRepository = (*pgAssetRepo)(nil)` (same pattern for sources, documents, extractor, filestorage, factory).
- **Reuse fix**: one repository implementation per entity, parameterized by a `Querier` (satisfied by both `*pgxpool.Pool` and `pgx.Tx`). `pgRepoFactory.InTransaction` begins a pgx tx and hands `fn` a tx-bound factory — the archived design's hand-duplicated `txAssetStore` (170+ lines of mirrored SQL in `ingest`) disappears. `ingest.Service` depends only on `RepoFactory` + `Extractor` + `FileStorage`.
- `identity.Resolve` consumes `AssetRepository` directly (the old narrow `AssetStore` interface is subsumed); merge now also builds metadata-overlay + doc_type fields.

### D8: Testing strategy — conventions unchanged, isolation improved

Normative conventions carried over verbatim: `<prod>_test.go` pairing; table-driven; `t.Parallel()` by default; **no env vars in tests** except a single `TestMain`/helper read of `PROCRASTINATOR_TEST_DATABASE_URL` per package and the gated real-LLM helper; integration tests skip explicitly without the DB.

- **Pure unit** (no Docker/network): config (map-injected), normalize/dates, `ParseExtraction` (incl. thought-strip safety net, metadata key validation, Indian date formats), `identity.Resolve` (fake `AssetRepository` with race-window fault injection), `ingest.Service` (**now pure** — fake `RepoFactory`/`Extractor`/`FileStorage`), LLM client request shape incl. anti-thinking prompt assertions (httptest fake), tenant middleware (httptest).
- **Integration** (Docker PG 18, per-package private schemas to keep `go test ./...` default-parallel race-free — carried lesson from the archived apply phase): repos (tenant scoping, tenant-scoped serial uniqueness, metadata `||` merge, NUMERIC round-trip), migrations, API end-to-end with fake LLM and real middleware (incl. missing-header 400, cross-tenant 404, two-tenant same-serial).
- **Real LLM**: exactly one gated test (`PROCRASTINATOR_TEST_LLM_INTEGRATION=1` + creds) uploading the two `sample-data/` PDFs; asserts classification (`invoice`/`amc`), ≥1 identity field, non-empty metadata, and (logged) absence of `<thought>` in the raw payload.

## API surface

Unchanged paths; all require `X-Tenant-ID`:

| Endpoint | Success | Errors |
|---|---|---|
| `POST /api/documents` (multipart field `file`) | 201 Asset JSON (incl. `doc_type`, `metadata`) | 400 no/invalid tenant · 400 missing `file` · 413 oversize · 415 bad type · 422 no identity · 502 extraction failure |
| `GET /api/assets` | 200 array (tenant-scoped, `[]` when empty) | 400 no/invalid tenant |
| `GET /api/assets/{id}` | 200 Asset JSON | 400 no/invalid tenant · 404 unknown/foreign |
| `GET /api/assets/{id}/documents` | 200 ordered array incl. metadata | 400 no/invalid tenant · 404 unknown/foreign asset |

Error envelope unchanged: `{"error": "..."}`. Price stays a JSON string, exact decimal.

## Risks / Trade-offs

- **Prompt drift re-enabling thoughts** — mitigated by the parser safety net + request-shape tests pinning the anti-thinking header + the gated live test.
- **Metadata unbounded growth / junk keys** — capped size + snake_case key validation at parse; accepted that metadata is best-effort LLM output (never identity, never canonical truth).
- **Tenant id trust** — header accepted as-is (no auth in this slice); a future auth change swaps the middleware, repositories stay unchanged.
- **`doc_type` last-write-wins can flip** (`invoice` → `amc`) — intended: it tracks the latest document context; per-document truth remains on `documents`.
- **Brand+model-only concurrent duplicates per tenant** — unchanged accepted trade-off; uniqueness guarantee remains serial-only.
- **Rewriting `00001_init.sql`** destroys existing dev data — accepted; the old `lifemanager` DB is disposable, and the compose DB is recreated.

## Testing conventions (normative, applies to all tasks)

Tests live in `<prod_file>_test.go`; table-driven; `t.Parallel()` in every independent test; no env vars in tests except one `PROCRASTINATOR_TEST_DATABASE_URL` read per package (TestMain/helper) and the gated real-LLM helper; integration tests use per-package private schemas and explicit test tenants; config is injected as `Config` literals.
