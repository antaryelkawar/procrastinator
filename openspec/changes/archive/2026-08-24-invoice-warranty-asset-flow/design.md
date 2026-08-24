# Design: invoice-warranty-asset-flow

## Context

First vertical slice of Life Manager: document upload → LLM interpretation → canonical Asset. The repo is a **monorepo**; all backend code lives in `procrastinator-backend/` at the repo root (Go module root). The tech spec (`docs/Life_Manager_Technical_Spec.md` §14) mandates a modular monolith with `cmd/` + `internal/` and one hard boundary: **domain code must not depend on HTTP, DB, or LLM implementation details**.

Pinned stack (verified 2026-08-23): Go 1.27.0, chi v5.3.2, pgx v5.10.0, goose v3.27.3, PostgreSQL 18 (`postgres:18` Docker image). LLM provider: **Google Gemini OpenAI-compatible endpoint**, model `gemma-4-26b-a4b-it`, reached via a **native Go HTTP client — no LiteLLM, no Python sidecar**.

Existing code at the repo root (`go.mod`, `internal/config/`, `internal/store/`, `internal/llm/`, `migrations/`) is **moved into `procrastinator-backend/` and reorganized into subpackages as the first implementation task** — behavior is preserved, only structure and test conventions change.

## Goals / Non-Goals

**Goals**
- Synchronous `POST /api/documents` ingestion loop returning the resulting Asset.
- Read APIs: `GET /api/assets`, `GET /api/assets/{id}`, `GET /api/assets/{id}/documents`.
- Gemini (OpenAI-compatible) LLM extraction, env-configured at bootstrap, mockable via plain HTTP.
- Deterministic identity resolution with race-safe serial uniqueness.
- TDD under strict conventions: table-driven `<prod>_test.go` files, `t.Parallel()` by default, **no env vars in tests** (config injected as a struct), Docker-PG integration tests reading `LM_TEST_DATABASE_URL` exactly once (TestMain/helper), fake-LLM `httptest` server, one env-gated real-Gemini test.

**Non-Goals** (deliberate deferrals)
- No frontend, no worker binary, no auth, no async pipeline, no object storage (local filesystem only), no pagination on list endpoints, no LiteLLM/Python sidecar.

## Decisions

### D1: Monorepo module layout with subpackages

Module root: `procrastinator-backend/` (module path `procrastinator-backend`; `go.mod` lives there). Repo root keeps only docs/specs/ops files.

```
procrastinator-backend/
  go.mod
  docker-compose.yml                    — local PostgreSQL 18 (dev + tests)
  .env                                  — gitignored; dev credentials (LM_* vars)
  migrations/00001_init.sql             — single goose migration
  cmd/life-manager-api/main.go          — ONLY place env vars are read: .env load → config.Load → pool → migrate → router → serve
  internal/
    config/                             — Config struct + Load() (bootstrap use only)
    store/                              — pgx pool, goose migrate, TruncateAll test helper (no repositories here)
    store/assets/                       — asset repository
    store/sources/                      — source repository
    store/documents/                    — document repository
    llm/                                — OpenAI-compatible HTTP client (Extractor impl)
    llm/extraction/                     — extraction parsing/validation (incl. <thought> stripping)
    identity/                           — normalization + create-vs-update resolution + field merge
    ingest/                             — orchestration service + filesystem storage
    httpapi/                            — chi router, handlers, error mapping
```

Dependency direction: `httpapi → ingest → {llm, identity, store/*}`. `identity` operates on plain domain structs + a narrow `AssetStore` interface it defines; it never imports pgx or net/http. `store/assets|sources|documents` implement repositories over the pool from `store`. LLM is behind an `Extractor` interface defined in `ingest`; tests inject fakes, `main.go` selects the real client.

### D2: Data model (PostgreSQL, goose)

Unchanged from prior design. One migration, `procrastinator-backend/migrations/00001_init.sql`:

- **`sources`**: `id uuid PK DEFAULT gen_random_uuid()`, `filename text NOT NULL`, `content_type text NOT NULL`, `byte_size bigint NOT NULL`, `storage_path text NOT NULL`, `sha256 text NOT NULL` (hex), `uploaded_at timestamptz NOT NULL DEFAULT now()`.
- **`assets`**: `id uuid PK DEFAULT gen_random_uuid()`, nullable `brand`, `model`, `serial_number`, `norm_serial text`, `norm_brand text`, `norm_model text`, `purchase_date date`, `price numeric`, `currency char(3)`, `warranty_start date`, `warranty_end date`, `created_at`/`updated_at timestamptz`. **Partial unique index** `ON assets(norm_serial) WHERE norm_serial IS NOT NULL` — race-safety mechanism for D4.
- **`documents`**: `id uuid PK`, `asset_id uuid NOT NULL REFERENCES assets`, `source_id uuid NOT NULL UNIQUE REFERENCES sources`, `doc_type text NOT NULL CHECK (doc_type IN ('invoice','warranty','other'))`, `extracted_fields jsonb NOT NULL`, `raw_extraction jsonb NOT NULL`, `created_at timestamptz NOT NULL DEFAULT now()`. Ingestion order = `ORDER BY created_at, id`.

IDs are opaque UUIDs (DB-generated, exposed as strings in JSON). `gen_random_uuid()` is built-in on PG 18.

### D3: Money and JSON representation

Unchanged. Price is `numeric` in PostgreSQL (pgx `pgtype.Numeric`), carried through domain/API as a **string** validated as an exact decimal (`^-?\d+(\.\d{1,4})?$` via `math/big.Rat` parse). JSON emits `"price": "39999.99"` — never a float. Currency validated as 3 uppercase ASCII letters (ISO 4217 shape).

### D4: Identity resolution & concurrency

Unchanged. Normalization: trim, collapse whitespace runs to one space; serial → uppercase; brand/model → case-folded (stored normalized columns hold the folded form).

Resolution order (single DB transaction per upload):
1. Serial present → match on `norm_serial`. **Serial wins** over conflicting brand+model.
2. Else brand+model both present → match on `norm_brand AND norm_model`.
3. Else → `422`, nothing created.
4. Match → field merge: non-empty extracted values overwrite; absent/null never erases.
5. No match → INSERT; serial-bearing inserts use `INSERT ... ON CONFLICT (norm_serial) DO NOTHING` then re-SELECT. Brand+model-only races accepted best-effort.

### D5: LLM client — Gemini via native Go HTTP (no LiteLLM)

- **Endpoint**: `POST {LM_LLM_BASE_URL}/chat/completions` with `LM_LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai/`, `Authorization: Bearer {LM_LLM_API_KEY}` — same envelope shape as OpenAI, plain Go `net/http` client. No Python sidecar, no LiteLLM proxy.
- **Model**: `gemma-4-26b-a4b-it` (`LM_LLM_MODEL`).
- **Request body**: `model`, `messages` (system prompt describing the extraction schema + user message carrying the document), `response_format: {"type":"json_object"}` (supported by the endpoint), `temperature: 0`.
- **Document content**: PNG/JPEG sent as `image_url` data-URI content parts (standard OpenAI multimodal format, supported); PDF sent as base64 content part with a filename hint.
- **CRITICAL — `<thought>` stripping**: `gemma-4-26b-a4b-it` emits `<thought>...</thought>` blocks BEFORE the JSON content in `choices[0].message.content`. The extraction parser in `internal/llm/extraction` MUST strip all `<thought>...</thought>` blocks (non-greedy, possibly multiple, before any JSON) before JSON parsing. Failure to strip = guaranteed parse failure on real responses.
- **Response**: stripped content parsed as extraction JSON `{document_type, brand, model, serial_number, purchase_date, price, currency, warranty_start, warranty_end}`.
- **Validation**: classification outside `{invoice, warranty, other}` → `other`; dates must parse ISO 8601; price must parse as exact decimal; invalid fields treated as absent (never guessed). Raw payload retained unmodified (with thought blocks) on the Document.
- **Failure**: `http.Client.Timeout` (default 60s, `LM_LLM_TIMEOUT`); unreachable/timeout/non-2xx/unparseable body → typed `ExtractionError` → HTTP 502. No fallback values.

### D6: Ingestion pipeline (synchronous, in `ingest.Service.Process`)

Unchanged flow:
1. Sniff content type from first 512 bytes (`http.DetectContentType` + `%PDF-` magic); only PDF/PNG/JPEG pass → else 415. Size limit via `http.MaxBytesReader` (default 20 MiB) → 413.
2. Persist bytes to `<storage-dir>/<source-uuid><ext>`; SHA-256; INSERT Source. **Source survives all downstream failures.**
3. LLM extraction → failure: 502, Source kept, no Document/Asset writes.
4. Identity resolution + create/merge + INSERT Document in one transaction. No identity → 422 (rollback, Source kept).
5. Return Asset JSON → 201.

### D7: Configuration — env vars in bootstrap ONLY

`internal/config` exports a `Config` struct and `Load() (*Config, error)`. `Load()` is the **only** function that touches the environment; it accepts an optional `map[string]string` source (nil → real env) so it is testable without `os.Getenv`/`t.Setenv`. **`cmd/life-manager-api/main.go` is the only code that reads real env vars** (loading `.env` first via a loader such as godotenv, then calling `config.Load(nil)`); it constructs `Config` and passes it down. No other package reads env vars.

| Var | Required | Default |
|---|---|---|
| `LM_DATABASE_URL` | yes | — |
| `LM_LLM_BASE_URL` | yes | `https://generativelanguage.googleapis.com/v1beta/openai/` in `.env` |
| `LM_LLM_API_KEY` | yes | in `.env` (gitignored — never committed) |
| `LM_LLM_MODEL` | yes | `gemma-4-26b-a4b-it` in `.env` |
| `LM_STORAGE_DIR` | yes | — |
| `LM_HTTP_ADDR` | no | `:8080` |
| `LM_MAX_UPLOAD_BYTES` | no | `20971520` |
| `LM_LLM_TIMEOUT` | no | `60s` |
| `LM_TEST_DATABASE_URL` | integration tests only, read once in TestMain/helper | — |
| `LM_TEST_LLM_INTEGRATION` | real-Gemini test gate | unset |

Startup aborts naming the first missing/empty required var. Migrations run via goose at startup; failure to reach latest version = no serving. `.env` (gitignored) holds dev credentials; the Gemini API key lives only there — it is never written to committed files.

### D8: Error mapping (consistent JSON `{"error": "..."}`)

Unchanged: 400 missing `file` field · 413 oversize · 415 unsupported type · 422 no usable identity · 502 extraction failure · 404 unknown asset id · 500 anything else.

## Testing Strategy (normative conventions)

These conventions override any conflicting prior guidance and apply to ALL tasks, including the moved existing tests:

- **File pairing**: tests live in `<prod_file>_test.go` next to the production file (e.g., `config.go` → `config_test.go`, `assets.go` → `assets_test.go`). No multi-source test files.
- **Table-driven**: each test is a slice of case structs iterated with `t.Run(tc.name, ...)`.
- **Parallel by default**: every independent test calls `t.Parallel()`. Integration tests sharing DB tables isolate by truncating in setup (and only forgo parallel where the shared-DB helper genuinely forbids it).
- **No env vars in tests**: tests construct `config.Config` literals directly. No `t.Setenv`, no `os.Getenv` in `*_test.go`. Sole exception: integration tests read `LM_TEST_DATABASE_URL` **once** in `TestMain` or a single test helper and skip explicitly when unset; the real-Gemini test reads its gate/credentials once the same way.
- **Pure unit** (no Docker/network): config struct validation, extraction parsing (incl. `<thought>` stripping), classification degrade, normalization, merge semantics, LLM client request shape/failure modes against `httptest` fakes.
- **Integration** (require `LM_TEST_DATABASE_URL`, else explicit skip): migrations, repositories (serial uniqueness, NUMERIC round-trip, list ordering), resolution with real SQL, ingestion service, HTTP handlers end-to-end with fake LLM.
- **Real LLM**: exactly one test, skips unless `LM_TEST_LLM_INTEGRATION=1` plus live Gemini credentials; exercises the real `gemma-4-26b-a4b-it` endpoint (validates thought-stripping end-to-end). Excluded from default runs.
- Local PG for dev/tests: `procrastinator-backend/docker-compose.yml` with `postgres:18`.

## Risks / Trade-offs

- **Brand+model-only concurrent duplicates** — accepted: spec demands uniqueness only for serials.
- **`<thought>` block format drift** — stripping is centralized in `llm/extraction` and covered by table-driven parser tests plus the gated real-Gemini test, so drift is caught fast.
- **PDF-over-LLM wire format varies by provider** — isolated inside `llm.Client`; canned-response tests pin the request envelope, not provider quirks.
- **Module move invalidates nothing behaviorally** — move task is structural only; all moved tests must pass before/after with convention rewrites.
- **Synchronous ingestion latency** — accepted for the slice; worker/async split is a future change.

(End of file)
