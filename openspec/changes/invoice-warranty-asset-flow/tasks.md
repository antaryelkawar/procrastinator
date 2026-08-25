# Tasks: invoice-warranty-asset-flow (redesign)

All tasks are test-first: write failing tests, then implement until green. This is a redesign of committed code: task 1 deletes the legacy tree, then each task builds its package to final state — the module stays green after every task because deleted code is unreferenced.

**Normative test conventions (apply to every task):** tests in `<prod_file>_test.go`; table-driven (slice of case structs + `t.Run`); `t.Parallel()` in every independent test; **no env vars in tests** — construct `config.Config` literals directly (no `t.Setenv`/`os.Getenv` in `*_test.go`); the only exceptions are reading `PROCRASTINATOR_TEST_DATABASE_URL` (and the real-LLM gate) **once** per package in `TestMain`/a single helper. Integration tests use **per-package private PostgreSQL schemas** (search_path DSN param) and explicit test tenants (`test-tenant`, `test-tenant-b`) so `go test ./...` stays race-free at default parallelism. All paths below are relative to the repo root; the Go module root is `procrastinator-backend/`.

## 1. Legacy teardown and module skeleton

Delete the old implementation wholesale, then lay the schema and ops skeleton. The module path (`procrastinator-backend`) and dependency pins in `go.mod` are unchanged.

- [x] 1.1 Delete `procrastinator-backend/cmd/` and `procrastinator-backend/internal/` entirely (all packages, tests, testdata).
- [x] 1.2 Rewrite `procrastinator-backend/migrations/00001_init.sql` per design D4: `tenant_id text NOT NULL` on all three tables; `assets` gains `doc_type text NOT NULL DEFAULT 'other' CHECK (doc_type IN ('invoice','warranty','amc','other'))` and `metadata jsonb NOT NULL DEFAULT '{}'`; `warranty_start` removed from `assets`; unique index becomes `(tenant_id, norm_serial) WHERE norm_serial IS NOT NULL`; `documents.doc_type` CHECK gains `'amc'`.
- [x] 1.3 Update `procrastinator-backend/docker-compose.yml`: `POSTGRES_USER`/`POSTGRES_PASSWORD`/`POSTGRES_DB` = `procrastinator`, healthcheck to match. Update `procrastinator-backend/.env.example` with all `PROCRASTINATOR_*` vars (design D3 table). Verify `.gitignore` still covers `.env`, `storage/`, `*.exe`.
- **Files:** `procrastinator-backend/migrations/00001_init.sql`, `procrastinator-backend/docker-compose.yml`, `procrastinator-backend/.env.example`, `.gitignore` (verify-only), plus deletion of `procrastinator-backend/cmd/`, `procrastinator-backend/internal/`
- **Verify:** old tree gone; `go build ./...` and `go test ./...` from `procrastinator-backend/` succeed (vacuous); `docker compose up -d` brings up PG 18 with database `procrastinator`; `grep -ri "lifemanager\|life-manager" procrastinator-backend/` finds nothing outside this note.

## 2. commons-data: entities, tenant context, shared helpers

- [x] 2.1 Write failing tests in `procrastinator-backend/commons-data/tenant_test.go` (round-trip of `WithTenant`/`TenantFrom`; `ErrNoTenant` when absent; rejects empty/invalid ids per `^[A-Za-z0-9_-]{1,64}$`), `normalize_test.go` (trim/collapse/case-fold, serial uppercase — carried over from archived identity tests), and `dates_test.go` (`ParseDate` accepts `2006-01-02`, RFC3339, `02-Jan-06`, `02-Jan-2006`, `02.01.2006`; rejects garbage → zero time).
- [x] 2.2 Implement `tenant.go` (`WithTenant`, `TenantFrom(ctx) (string, error)`, `ErrNoTenant`, validation), `normalize.go` (`NormalizeSerial`, `NormalizeName`, `CollapseWhitespace`), `dates.go` (`ParseDate`), and the entity files: `asset.go` (`Asset` with `TenantID`, `DocType`, `Metadata map[string]any`; `UpdateFields` incl. `DocType *string` + `Metadata map[string]any`; `DocType` constants `invoice|warranty|amc|other` + `Valid()`), `source.go`, `document.go` (`Document`, `DocumentWithSource`), `extraction.go` (`Extraction`: typed core per D4 + `Metadata map[string]any` + `RawPayload`).
- **Files:** `procrastinator-backend/commons-data/asset.go`, `source.go`, `document.go`, `extraction.go`, `tenant.go`, `tenant_test.go`, `normalize.go`, `normalize_test.go`, `dates.go`, `dates_test.go`
- **Depends on:** 1
- **Verify:** `go test ./commons-data/` green without Docker/network.

## 3. commons-data: boundary interfaces and extraction parser

- [x] 3.1 Write failing tests in `parse_test.go` (table-driven, `t.Parallel()`): full core+metadata payload; `<thought>` stripping safety net (single/multiple blocks — pure JSON and thought-prefixed JSON yield identical results); classification incl. `amc`, unknown → `other`; Indian date formats parse; exact-decimal price; currency 3-letter; null fields absent; **metadata capture** — snake_case keys kept verbatim, non-snake_case keys dropped, null values dropped, oversize metadata rejected; `RawPayload` preserved verbatim.
- [x] 3.2 Implement `repository.go` (`AssetRepository` 7 methods per D7, `SourceRepository`, `DocumentRepository`, `ErrNotFound`), `services.go` (`Extractor`, `FileStorage`, `StoredFile{Path, ContentType, ByteSize, SHA256}`), `tx.go` (`RepoFactory` with `InTransaction`), and `parse.go` (`ParseExtraction` per D6: strip-first, `{`-guard, `UseNumber`, stream exhaustion, core validation via `dates.go`, metadata key validation `^[a-z][a-z0-9_]{0,63}$` + size cap ~8 KiB).
- **Files:** `procrastinator-backend/commons-data/repository.go`, `services.go`, `tx.go`, `parse.go`, `parse_test.go`
- **Depends on:** 2
- **Verify:** `go test ./commons-data/` green; interfaces compile against hand-written fakes in tests.

## 4. commons-server: config and HTTP utilities

- [x] 4.1 Write failing tests in `config/config_test.go` (map-injected `Load`, zero env access: required-var aborts naming each `PROCRASTINATOR_*` var in order; defaults for addr/upload-bytes/timeout; invalid duration/int errors) and `httpx/tenant_test.go` + `httpx/json_test.go` (middleware: missing/invalid `X-Tenant-ID` → 400 JSON envelope before handler; valid → handler sees tenant via `TenantFrom`; `WriteJSON`/`WriteError` shape and content-type).
- [x] 4.2 Implement `commons-server/config/config.go` (`Config` struct + `Load(src map[string]string)` per D3 — only function that may read env, and only when src is nil), `commons-server/httpx/json.go`, `commons-server/httpx/tenant.go` (`TenantMiddleware`).
- **Files:** `procrastinator-backend/commons-server/config/config.go`, `config_test.go`, `procrastinator-backend/commons-server/httpx/json.go`, `httpx/tenant.go`, `json_test.go`, `tenant_test.go`
- **Depends on:** 2
- **Verify:** `go test ./commons-server/...` green without Docker/network.

## 5. postgres: store, Querier, asset repository

- [x] 5.1 Write failing integration tests in `postgres/assets_test.go` (private schema `p_assets`, single `PROCRASTINATOR_TEST_DATABASE_URL` read in `TestMain`): create/read full field set incl. `doc_type`+`metadata`; partial asset; price `39999.99` exact round-trip; list stable order + `[]`; get-by-id miss → `ErrNotFound`; find by serial / brand+model; `ON CONFLICT (tenant_id, norm_serial)` converges concurrent inserts; **`UpdateFields` metadata shallow-merge (`metadata || $n::jsonb`) retains absent keys and overwrites present ones**; doc_type overwrite; **every method errors with `ErrNoTenant` on tenant-less ctx**; **all queries tenant-scoped** (foreign-tenant rows invisible; same serial in two tenants coexists).
- [x] 5.2 Implement `postgres/store.go` (`Open`, `Migrate`, `TruncateAll`, `Pool`, `Querier` interface satisfied by `*pgxpool.Pool` and `pgx.Tx`) and `postgres/assets.go` (`pgAssetRepo` over `Querier`, norm columns derived on write, tenant from ctx on every method) with `var _ data.AssetRepository = (*pgAssetRepo)(nil)`.
- **Files:** `procrastinator-backend/procrastinator-infra/postgres/store.go`, `store_test.go`, `assets.go`, `assets_test.go`
- **Depends on:** 1, 3
- **Verify:** `go test ./procrastinator-infra/postgres/ -run 'TestStore|TestAssets'` green against compose PG (create `procrastinator_test` DB once); skips explicitly without env.

## 6. postgres: source and document repositories

- [x] 6.1 Write failing integration tests in `postgres/sources_test.go` + `postgres/documents_test.go` (private schemas): Source create/get incl. `tenant_id`; Document create with `amc` doc_type, `extracted_fields` (incl. metadata) + `raw_extraction` jsonb, source-link uniqueness violation → typed error; `ListByAsset` join ordered by `created_at, id` with source filename/upload timestamp; unknown asset → empty; tenant-less ctx → `ErrNoTenant`; foreign-tenant rows invisible.
- [x] 6.2 Implement `postgres/sources.go` and `postgres/documents.go` over `Querier`, with interface guards.
- **Files:** `procrastinator-backend/procrastinator-infra/postgres/sources.go`, `sources_test.go`, `documents.go`, `documents_test.go`
- **Depends on:** 5
- **Verify:** `go test ./procrastinator-infra/postgres/` green; default-parallel runs stable (`-count=5`).

## 7. postgres: RepoFactory and transactions

- [x] 7.1 Write failing integration tests in `postgres/factory_test.go`: `pgRepoFactory` exposes the three repos; `InTransaction` commits on success (asset + document visible after), rolls back on error (neither visible); tx-bound repos are tenant-scoped and see uncommitted writes inside `fn`; nested/parallel transactions on separate tenants do not interfere.
- [x] 7.2 Implement `postgres/factory.go` (`pgRepoFactory` over pool; `InTransaction` begins pgx tx, passes a tx-bound factory whose repos share the tx `Querier`) with `var _ data.RepoFactory = (*pgRepoFactory)(nil)`.
- **Files:** `procrastinator-backend/procrastinator-infra/postgres/factory.go`, `factory_test.go`
- **Depends on:** 5, 6
- **Verify:** `go test ./procrastinator-infra/postgres/` green; no SQL duplicated outside repo files (the archived `txAssetStore` pattern is gone).

## 8. core: identity resolution

- [x] 8.1 Write failing pure unit tests in `procrastinator-core/identity/resolve_test.go` (mutex-protected fake `AssetRepository` with call recording + race-window fault injection; fully parallel): serial precedence over conflicting brand+model; brand+model only when both present; no identity → `ErrNoIdentity`; create on miss; conflict path merges into winner; merge — non-empty overwrites, absent never erases, **metadata overlay accumulated**, **doc_type set from latest classification**.
- [x] 8.2 Implement `procrastinator-core/identity/resolve.go`: `Resolve(ctx, data.AssetRepository, data.Extraction) (data.Asset, bool, error)` — same precedence algorithm as archived, extended with metadata-merge and doc_type in `UpdateFields`. No imports beyond stdlib + `commons-data`.
- **Files:** `procrastinator-backend/procrastinator-core/identity/resolve.go`, `resolve_test.go`
- **Depends on:** 3
- **Verify:** `go test ./procrastinator-core/identity/` green without Docker/network; `-count=5` stable.

## 9. infra: LLM client, extractor adapter, extraction prompt

- [x] 9.1 Write failing tests in `llm/client_test.go` (httptest fake OpenAI server, table-driven, `t.Parallel()`): request envelope — POST `{base}/chat/completions`, Bearer auth, `response_format: json_object`, `temperature: 0`, document as base64 `image_url` data-URI (incl. PDF); **system prompt contains the anti-thinking directive** ("strict JSON extraction engine", no `<thought>`, JSON only) and the classify-then-extract schema incl. `amc` and metadata examples; failure modes — timeout, non-2xx, malformed body, empty choices → `ExtractionError`.
- [x] 9.2 Implement `llm/client.go` (generic `Chat(ctx, ChatRequest)` — reusable, prompt-agnostic), `llm/prompt.go` (the D6 system prompt: anti-thinking header, 4-way classification with AMC definition, core schema with ISO-date instruction covering `DD-MMM-YY`/`DD.MM.YYYY`/`DD-MMM-YYYY`, metadata contract with snake_case examples incl. Indian tax keys, one worked AMC example), `llm/extractor.go` (binds prompt to client; `var _ data.Extractor = (*Extractor)(nil)`).
- **Files:** `procrastinator-backend/procrastinator-infra/llm/client.go`, `client_test.go`, `prompt.go`, `extractor.go`
- **Depends on:** 3
- **Verify:** `go test ./procrastinator-infra/llm/` green without Docker/network (gated test from task 14 skips).

## 10. infra: filesystem storage

- [x] 10.1 Write failing tests in `filestorage/storage_test.go` (temp dirs, table-driven, `t.Parallel()` — carried over from archived storage tests): sniff PDF/PNG/JPEG via `http.DetectContentType` + `%PDF-` magic; reject other types (`ErrUnsupportedType`); write to `<dir>/<uuid><ext>`; SHA-256 recorded.
- [x] 10.2 Implement `filestorage/storage.go` (`Storage.Put` satisfying `data.FileStorage`; `var _ data.FileStorage = (*Storage)(nil)`).
- **Files:** `procrastinator-backend/procrastinator-infra/filestorage/storage.go`, `storage_test.go`
- **Depends on:** 3
- **Verify:** `go test ./procrastinator-infra/filestorage/` green without Docker/network.

## 11. core: ingest orchestration service

- [x] 11.1 Write failing **pure unit** tests in `procrastinator-core/ingest/service_test.go` (in-memory fake `RepoFactory`/repos, fake `Extractor` returning canned payloads, fake `FileStorage`; no Docker): oversize → `ErrTooLarge` before any write; Source created before LLM call and survives extraction failure (`ErrExtraction`, zero Document/Asset); no-identity → `ErrNoIdentity`, Source retained; happy path — tx resolve + document insert with `extracted_fields` incl. metadata and verbatim `raw_extraction`; tenant flows from ctx into repos (fakes assert it); rollback path — document insert failure leaves no asset.
- [x] 11.2 Implement `procrastinator-core/ingest/service.go` (`Service` over `data.RepoFactory` + `data.Extractor` + `data.FileStorage`; `Process(ctx, filename, data)` per D6 flow; document insert inside `InTransaction`). Imports: stdlib + `commons-data` only.
- **Files:** `procrastinator-backend/procrastinator-core/ingest/service.go`, `service_test.go`
- **Depends on:** 3, 8
- **Verify:** `go test ./procrastinator-core/ingest/` green without Docker/network; `go list -deps ./procrastinator-core/...` contains no `pgx`, no `net/http`, no `procrastinator-infra`.

## 12. procrastinator-api: HTTP handlers and server

- [x] 12.1 Write failing integration tests in `procrastinator-api/handlers_test.go` (chi server + real PG in private schema + real tenant middleware + real `ingest.Service` over `pgRepoFactory` + fake-LLM httptest server, config injected): upload happy path → 201 with Asset JSON incl. `doc_type` + `metadata`; **missing/invalid `X-Tenant-ID` → 400 on every endpoint**; missing `file` → 400; unsupported type → 415; oversize → 413; LLM outage → 502; no identity → 422; list → `[]`/ordered array; get → 200/404; documents → ordered entries with type/source filename/upload timestamp; price as JSON string; **cross-tenant isolation** (foreign asset id → 404; same serial, two tenants → two assets; list shows only own tenant). NOTE: missing header → 401 (httpx.TenantMiddleware contract, task 4); see apply-12-api.md.
- [x] 12.2 Implement `procrastinator-api/server.go` (package `api`: chi router with `httpx.TenantMiddleware`, `Server` taking `*ingest.Service` + repo interfaces) and `handlers.go` (four endpoints, DTOs incl. `doc_type`+`metadata`, error→status mapping per design API table, `WriteJSON`/`WriteError` from `commons-server/httpx`).
- **Files:** `procrastinator-backend/procrastinator-api/server.go`, `handlers.go`, `handlers_test.go`
- **Depends on:** 4, 5, 6, 7, 9, 10, 11
- **Verify:** `go test ./procrastinator-api/` green against compose PG; default-parallel `-count=5` stable.

## 13. Binary wiring and local runtime

- [x] 13.1 Implement `procrastinator-backend/procrastinator-api/cmd/procrastinator-api/main.go` — the ONLY env-touching code: godotenv `.env` load (missing tolerated), `config.Load(nil)`, `postgres.Open`, goose `Migrate`, `os.MkdirAll(storage)`, wire `pgRepoFactory` + `llm.NewExtractor` + `filestorage.Storage` into `ingest.Service` and `api.Server`, chi serve with graceful shutdown.
- [x] 13.2 Update gitignored `procrastinator-backend/.env` to the `PROCRASTINATOR_*` names (values carried over: Gemini base URL, key, `gemma-4-26b-a4b-it`, new DB URL `postgres://procrastinator:procrastinator@localhost:5432/procrastinator?sslmode=disable`).
- **Files:** `procrastinator-backend/procrastinator-api/cmd/procrastinator-api/main.go`, `procrastinator-backend/.env` (gitignored)
- **Depends on:** 4, 5, 7, 9, 12
- **Verify:** `go build ./...` produces `procrastinator-api`; against compose PG + real `.env`: boots, migrates, serves; missing `PROCRASTINATOR_DATABASE_URL` aborts naming it (exit 1); `POST /api/documents` with `X-Tenant-ID` and a `sample-data/` PDF → 201; `git check-ignore .env` confirms ignored.

## 14. Gated real-LLM integration test on sample documents

- [x] 14.1 Write `procrastinator-backend/procrastinator-infra/llm/llm_integration_test.go`: skips unless `PROCRASTINATOR_TEST_LLM_INTEGRATION=1` plus live credentials (single gated helper, sole env access); sends **both** real PDFs from `sample-data/` (`INVNAG2302754.pdf`, `Annual Maintenance Contract.pdf`) through the real `Extractor` + `data.ParseExtraction`; asserts classification (`invoice` or `amc`), ≥1 usable identity field (serial or brand+model), non-empty metadata, and logs whether any `<thought>` blocks appeared (evidence for the D6 prompt).
- **Files:** `procrastinator-backend/procrastinator-infra/llm/llm_integration_test.go`
- **Depends on:** 9, 13
- **Verify:** default run reports SKIP without env; gated run passes against real `gemma-4-26b-a4b-it` for both PDFs.

## 15. Final gate

- [x] 15.1 `go vet ./...`, `gofmt -l`, `go build ./...`, full `go test ./... -count=1` green from `procrastinator-backend/` — with compose PG up (integration) and without (explicit skips); default-parallel runs stable; no env access in `*_test.go` outside sanctioned helpers (grep); **naming grep**: zero `life-manager`/`lifemanager`/`LM_` matches in `procrastinator-backend/`; **layering check**: `go list -deps ./procrastinator-core/...` free of `pgx`/`net/http`/`procrastinator-infra`, and no cross-imports between `procrastinator-infra` packages; interface guards compile (by construction); every spec scenario mapped to a test; `openspec validate invoice-warranty-asset-flow --strict` passes.
- **Depends on:** all
