# Tasks: invoice-warranty-asset-flow

All tasks are test-first: write failing tests, then implement until green. Pure unit tests must run without Docker or network; integration tests require `LM_TEST_DATABASE_URL` (PostgreSQL 18 via `docker compose up -d` in `procrastinator-backend/`) and otherwise skip explicitly.

**Normative test conventions (apply to every task):** tests in `<prod_file>_test.go`; table-driven (slice of case structs + `t.Run`); `t.Parallel()` in every independent test; **no env vars in tests** — construct `config.Config` literals directly (no `t.Setenv`/`os.Getenv` in `*_test.go`); the only exception is reading `LM_TEST_DATABASE_URL` (and the real-LLM gate) **once** in `TestMain`/a single helper. All paths below are relative to the repo root; the Go module root is `procrastinator-backend/`.

## 1. Move existing backend into `procrastinator-backend/` and restructure

Existing code at repo root (`go.mod`, `internal/config/`, `internal/store/`, `internal/llm/`, `migrations/`) is already implemented (old tasks 1, 2, 5, 11). Move it, reorganize into subpackages, and bring all moved tests under the normative conventions — no behavior changes.

- [x] 1.1 Move `go.mod` → `procrastinator-backend/go.mod` (module path `procrastinator-backend`); move `migrations/00001_init.sql` → `procrastinator-backend/migrations/00001_init.sql`; update all import paths. Remove the old root-level Go files/dirs.
- [x] 1.2 Move `internal/store/store.go` → `procrastinator-backend/internal/store/store.go` (pool/migrate/TruncateAll only) and `store_test.go` alongside it; refactor the test to read `LM_TEST_DATABASE_URL` once in `TestMain`/helper, make cases table-driven, add `t.Parallel()` where DB isolation allows.
- [x] 1.3 Refactor `internal/config`: export `Config` struct; `Load(src map[string]string) (*Config, error)` is the only env-touching function (nil source → real env). Move to `procrastinator-backend/internal/config/config.go`; rewrite `config_test.go` as table-driven + `t.Parallel()`, calling `Load(map)` / validating `Config` literals — zero env access.
- [x] 1.4 Move `internal/llm/client.go` → `procrastinator-backend/internal/llm/client.go` and its `client_test.go` alongside; move `llm_integration_test.go` + `testdata/` fixture. Apply conventions (table-driven, `t.Parallel()`, config injected as struct — no env reads outside the single gated integration helper).
- **Files:** `procrastinator-backend/go.mod`, `procrastinator-backend/migrations/00001_init.sql`, `procrastinator-backend/internal/store/store.go`, `procrastinator-backend/internal/store/store_test.go`, `procrastinator-backend/internal/config/config.go`, `procrastinator-backend/internal/config/config_test.go`, `procrastinator-backend/internal/llm/client.go`, `procrastinator-backend/internal/llm/client_test.go`, `procrastinator-backend/internal/llm/llm_integration_test.go`, `procrastinator-backend/internal/llm/testdata/invoice-fixture.png` (plus deletion of the old root-level `go.mod`, `internal/`, `migrations/`)
- **Verify:** `go build ./...` and `go test ./...` from `procrastinator-backend/` green (unit suite; integration tests skip cleanly without `LM_TEST_DATABASE_URL`, pass with it); old root paths gone.

## 2. Asset repository

- [x] 2.1 Write failing tests in `procrastinator-backend/internal/store/assets/assets_test.go` (table-driven, DB URL via shared helper): create/read round-trip of the full field set; partially-null asset; price `39999.99` round-trips exactly (numeric, no float); list returns stable deterministic order and `[]` when empty; get-by-id misses; find by `norm_serial`; find by `norm_brand`+`norm_model`; `INSERT ... ON CONFLICT (norm_serial)` re-select converges two concurrent inserts to one row; field-update persists merged values.
- [x] 2.2 Implement `procrastinator-backend/internal/store/assets/assets.go` (`Create`, `CreateOnConflictSerial`, `GetByID`, `List`, `FindBySerial`, `FindByBrandModel`, `UpdateFields`) with normalized columns maintained on write.
- **Files:** `procrastinator-backend/internal/store/assets/assets.go`, `procrastinator-backend/internal/store/assets/assets_test.go`
- **Depends on:** 1
- **Verify:** `go test ./internal/store/assets/`

## 3. Source and Document repositories

- [x] 3.1 Write failing tests in `procrastinator-backend/internal/store/sources/sources_test.go` and `procrastinator-backend/internal/store/documents/documents_test.go` (one test file per prod file): Source insert retains filename/content-type/size/path/sha256/upload timestamp; Document insert links exactly one Source and one Asset with type check (`invoice`/`warranty`/`other`), `extracted_fields` and `raw_extraction` jsonb; list-by-asset returns ingestion order with source filename + upload timestamp joined; unknown asset id yields empty/miss.
- [x] 3.2 Implement `procrastinator-backend/internal/store/sources/sources.go` and `procrastinator-backend/internal/store/documents/documents.go`.
- **Files:** `procrastinator-backend/internal/store/sources/sources.go`, `procrastinator-backend/internal/store/sources/sources_test.go`, `procrastinator-backend/internal/store/documents/documents.go`, `procrastinator-backend/internal/store/documents/documents_test.go`
- **Depends on:** 1, 2
- **Verify:** `go test ./internal/store/sources/ ./internal/store/documents/`

## 4. Extraction parsing and validation (incl. `<thought>` stripping)

- [x] 4.1 Write failing tests in `procrastinator-backend/internal/llm/extraction/extraction_test.go` (table-driven, `t.Parallel()`): full valid payload populates all fields; **`<thought>...</thought>` blocks (single, multiple, surrounding whitespace) stripped before JSON parse — pure JSON and thought-prefixed JSON yield identical results**; classification `invoice`/`warranty` kept, unknown/missing degrades to `other`; ISO 8601 dates parse, garbage dates absent; price parses as exact decimal string, invalid price absent; currency must be 3-letter code else absent; null fields stay absent; raw payload preserved unmodified (thought blocks intact in raw).
- [x] 4.2 Implement `procrastinator-backend/internal/llm/extraction/extraction.go` (`ParseExtraction` → validated `Extraction` struct with string-typed price; thought-strip step first).
- **Files:** `procrastinator-backend/internal/llm/extraction/extraction.go`, `procrastinator-backend/internal/llm/extraction/extraction_test.go`
- **Depends on:** 1
- **Verify:** `go test ./internal/llm/extraction/`

## 5. Identity resolution and field merge

- [x] 5.1 Write failing table-driven parallel unit tests in `procrastinator-backend/internal/identity/normalize_test.go`: trim, whitespace-run collapse, serial uppercasing (`  sn-123  ABC ` ≡ `SN-123 abc`), brand/model case folding.
- [x] 5.2 Write failing tests in `procrastinator-backend/internal/identity/resolve_test.go`: serial match wins over conflicting brand+model match; brand+model match only when both present (brand alone → no match); no match + usable identity → create; no identity → `ErrNoIdentity`; merge — non-empty overwrites, absent never erases (price retained when extraction null; model upgraded; warranty dates added without touching other fields).
- [x] 5.3 Implement `procrastinator-backend/internal/identity/normalize.go` and `procrastinator-backend/internal/identity/resolve.go` (`Resolve(ctx, Extraction) (asset, created, error)` over a narrow store interface; transaction-scoped).
- **Files:** `procrastinator-backend/internal/identity/normalize.go`, `procrastinator-backend/internal/identity/normalize_test.go`, `procrastinator-backend/internal/identity/resolve.go`, `procrastinator-backend/internal/identity/resolve_test.go`
- **Depends on:** 2, 4
- **Verify:** `go test ./internal/identity/`

## 6. Ingestion service and file storage

- [x] 6.1 Write failing tests in `procrastinator-backend/internal/ingest/service_test.go` and `procrastinator-backend/internal/ingest/storage_test.go` (fake `Extractor`, real store via `LM_TEST_DATABASE_URL` helper, temp storage dir injected via config struct): accepted upload persists Source with all metadata and file bytes at recorded path; LLM failure → `ErrExtraction`, Source retained, zero Document/Asset writes; no-identity extraction → `ErrNoIdentity`, Source retained, no Document/Asset; successful invoice creates Asset + Document with raw payload; second upload matching serial links second Document to same Asset and merges fields; content sniffing rejects non-PDF/PNG/JPEG bytes (`ErrUnsupportedType`); oversize rejected (`ErrTooLarge`).
- [x] 6.2 Implement `procrastinator-backend/internal/ingest/storage.go` (write bytes to `<storage-dir>/<uuid><ext>`, SHA-256, content sniffing) and `procrastinator-backend/internal/ingest/service.go` (`Process` orchestration per design D6, transactional resolution+document insert).
- **Files:** `procrastinator-backend/internal/ingest/service.go`, `procrastinator-backend/internal/ingest/service_test.go`, `procrastinator-backend/internal/ingest/storage.go`, `procrastinator-backend/internal/ingest/storage_test.go`
- **Depends on:** 3, 4, 5
- **Verify:** `go test ./internal/ingest/`

## 7. HTTP API

- [x] 7.1 Write failing tests in `procrastinator-backend/internal/httpapi/handlers_test.go` (chi server over real store + fake LLM `httptest` server, config injected): `POST /api/documents` happy path → 201 + Asset JSON; missing `file` field → 400; unsupported type → 415; oversize → 413; LLM outage → 502; unidentifiable doc → 422; `GET /api/assets` → 200 array (`[]` empty); `GET /api/assets/{id}` → 200 / 404; `GET /api/assets/{id}/documents` → 200 ordered entries incl. type, source filename, upload timestamp / 404 unknown asset; price serialized as JSON string.
- [x] 7.2 Implement `procrastinator-backend/internal/httpapi/server.go` (router construction, `MaxBytesReader`, JSON error envelope) and `procrastinator-backend/internal/httpapi/handlers.go` (all four endpoints, error → status mapping per design D8).
- **Files:** `procrastinator-backend/internal/httpapi/server.go`, `procrastinator-backend/internal/httpapi/handlers.go`, `procrastinator-backend/internal/httpapi/handlers_test.go`
- **Depends on:** 6
- **Verify:** `go test ./internal/httpapi/`

## 8. Binary wiring, `.env`, and local infrastructure

- [x] 8.1 Implement `procrastinator-backend/cmd/life-manager-api/main.go` — the ONLY env-touching code: load `.env` (godotenv-style loader, missing file tolerated), `config.Load(nil)`, pool, goose migrate, ensure storage dir, chi serve, graceful shutdown.
- [x] 8.2 Write `procrastinator-backend/docker-compose.yml` (`postgres:18`, exposed port, healthcheck); create gitignored `procrastinator-backend/.env` with `LM_DATABASE_URL`, `LM_LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai/`, `LM_LLM_API_KEY`, `LM_LLM_MODEL=gemma-4-26b-a4b-it`, `LM_STORAGE_DIR`; ensure `.gitignore` covers `.env`; add a committed `.env.example` with placeholder values.
- **Files:** `procrastinator-backend/cmd/life-manager-api/main.go`, `procrastinator-backend/docker-compose.yml`, `procrastinator-backend/.env` (gitignored), `procrastinator-backend/.env.example`, `.gitignore`
- **Depends on:** 1, 7
- **Verify:** `go build ./...`; manual run against compose DB boots, migrates, serves; startup aborts naming the first missing required var; `git status` shows `.env` untracked/ignored.

## 9. Opt-in real Gemini integration test

- [x] 9.1 Update the moved `procrastinator-backend/internal/llm/llm_integration_test.go`: skips unless `LM_TEST_LLM_INTEGRATION=1` plus live `LM_LLM_*` credentials (read once in the gated helper); uploads the fixture invoice image through the real client + extraction parser against `gemma-4-26b-a4b-it` and asserts successful classification + at least one usable identity field — this is the end-to-end proof of `<thought>` stripping against the real endpoint. Excluded from default runs.
- **Files:** `procrastinator-backend/internal/llm/llm_integration_test.go`, `procrastinator-backend/internal/llm/testdata/invoice-fixture.png`
- **Depends on:** 4, 8
- **Verify:** default `go test ./internal/llm/` reports skip; gated run passes with real Gemini credentials.

## 10. Final gate

- [x] 10.1 `go vet ./...` and full `go test ./...` green from `procrastinator-backend/` (unit + integration with Docker PG up); every spec scenario mapped to a test; no env access in `*_test.go` outside sanctioned helpers (grep check); `openspec validate invoice-warranty-asset-flow --strict` passes.
- **Depends on:** all
