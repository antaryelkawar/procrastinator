# Tasks: statement-ledger-ingestion

Backend-only (all tasks tagged `[backend]`). Builds on the committed flat architecture and
the in-flight `invoice-warranty-asset-flow` conventions: generic `repo.Repository[T]` +
functional `Option`s, `repo.Repos`/`repo.Factory` + `factory.InTx(ctx, fn(ctx, *Repos))`,
money as exact-decimal strings, `gen_random_uuid()` IDs, fail-closed `repo.Tenant(tid)`,
per-package private-schema integration tests. The module must build and test green after
every task.

**Normative test conventions (apply to every task):** tests in `<prod_file>_test.go`;
table-driven (case structs + `t.Run`); `t.Parallel()` in every independent test; **no env
vars in tests** except reading `PROCRASTINATOR_TEST_DATABASE_URL` **once** per package in
`TestMain`/a single helper (see `procrastinator-backend/infra/postgres/testutil_test.go`);
integration tests use **per-package private PostgreSQL schemas** and explicit test tenants
(`test-tenant`, `test-tenant-b`); pure `core/` tests use in-memory fakes of `*repo.Factory`
and the consumer-side interfaces (no Docker); config is injected as `Config` literals.
All paths are relative to the repo root; the Go module root is `procrastinator-backend/`.
All verify commands run from `procrastinator-backend/`.

**Shared files (declared up front):** `commons/repo/factory.go`, `infra/postgres/factory.go`,
`api/server.go`, `api/cmd/procrastinator/main.go`, `config/config.go`, `go.mod` are touched
by more than one task. Each touch is **additive and ordered**; `main.go`'s final form is
owned by task 10 (minimal wiring in 8 and 9 keeps `go build ./...` green, mirroring
`invoice-warranty-asset-flow`).

## 1. Migration: finance tables `[backend]`

One goose migration adding the four tenant-scoped finance tables exactly as in design D2.

- [x] 1.1 Write `procrastinator-backend/migrations/00002_finance.sql`: `financial_accounts`
  (NO `uq_account_tenant_name`), `money_movements` (with `chk_kind_accounts`, `norm_description`,
  import-provenance columns, link columns, `uq_movement_linked_doc` partial unique index,
  and the tenant/account/occurred/external-reference indexes), `import_batches` (state
  CHECK + per-status counts), `import_lines` (`uq_line_ref UNIQUE (batch_id, line_ref)`,
  `ON DELETE CASCADE` to batch). `-- +goose Up` creates; `-- +goose Down` drops in FK
  order (lines, batches, movements, accounts). Conventions: `uuid PRIMARY KEY DEFAULT
  gen_random_uuid()`, `tenant_id text NOT NULL`, `numeric` money, `char(3)` currency,
  `timestamptz ... DEFAULT now()`. Existing `sources`/`assets`/`documents` untouched.
- **Files:** `procrastinator-backend/migrations/00002_finance.sql`
- **Depends on:** â€”
- **Verify:** from `procrastinator-backend/` with compose PG up: `goose -dir migrations up`
  (or via the existing `store.Migrate`) applies cleanly on a fresh schema; `\d` shows all
  four tables + `chk_kind_accounts` + `uq_movement_linked_doc`; `goose ... down` drops them;
  `00001` tables unchanged; `go build ./...` green.

## 2. commons: finance entities + money helpers (pure, no DB) `[backend]`

- [x] 2.1 `procrastinator-backend/commons/entity/account.go`: `FinancialAccount` struct
  (`ID, TenantID, Name, Type, Currency string; Institution, ExternalDescriptor *string;
  CreatedAt, UpdatedAt time.Time`) + type constants (`bank`,`wallet`,`cash`,`credit_card`)
  + `ValidAccountType(string) bool` + non-empty-name rule. `account_test.go`: field-set
  round-trip; each valid type accepted; `investment`/empty rejected; two accounts may share
  a name (no uniqueness asserted here â€” that is the DB/service concern).
- [x] 2.2 `procrastinator-backend/commons/entity/movement.go`: `MoneyMovement` struct
  (`ID, TenantID, Kind, Amount, Currency string; OccurredOn, RecordedAt time.Time;
  Description, NormDescription string; Origin string; SourceAccountID,
  DestinationAccountID *string; ImportBatchID *string; ImportLine *int;
  ExternalReference *string; LinkedDocumentID *string; LinkCreator *string;
  LinkConflicting bool; CreatedAt, UpdatedAt time.Time`) + kind constants
  (`expense`,`income`,`transfer`)+`ValidKind`, origin constants (`manual`,`import`), link
  creator constants (`manual`,`auto`). `movement_test.go`: `ValidKind` for all three +
  rejects `debit`/`credit`/`""`; origin/link-creator constants.
- [x] 2.3 `procrastinator-backend/commons/entity/import_batch.go`: `ImportBatch` struct
  (`ID, TenantID, State, AccountID, SourceID, Filename, Format string; LineCountValid,
  LineCountDuplicate, LineCountPossibleDup, LineCountError int; CreatedAt, UpdatedAt
  time.Time`) + state constants (`preview`,`committed`,`discarded`)+`ValidState` + a pure
  transition guard `CanTransition(from, to bool)` encoding the one-way machine:
  `previewâ†’committed`, `previewâ†’discarded` allowed; everything else (incl. `committedâ†’
  discarded`, `committedâ†’committed`, `discardedâ†’*`) false (ST-001/003/004). `import_batch_test.go`:
  full transition table (2 allowed, all terminal/illegal rejected).
- [x] 2.4 `procrastinator-backend/commons/entity/import_line.go`: `ImportLine` struct
  (`ID, TenantID, BatchID string; LineRef int; RawLine string; OccurredOn *time.Time;
  Amount *string; Direction string; Description, NormDescription, ExternalReference
  *string; Status string; ErrorReason *string; CreatedAt time.Time`) + status constants
  (`valid`,`duplicate`,`possible-duplicate`,`error`)+`ValidStatus`. `import_line_test.go`:
  `ValidStatus` table.
- [x] 2.5 `procrastinator-backend/commons/money.go`: `IsValidAmount(s string) bool`
  (strictly positive, exact decimal; reject `""`,`0`,`-5`,`"12.5.3"`,`"1e3"`,`float`
  representations), `IsValidCurrency(s string) bool` (exactly 3 letters),
  `NormalizeDescription(s string) string` = `strings.ToLower(CollapseWhitespace(s))`
  (reuse `commons.CollapseWhitespace`, `commons/normalize.go`). `money_test.go`: table-driven
  valid/invalid amounts (incl. `19999.99` ok), currencies (`INR` ok, `IN`/`INR1` no), and
  description normalization (`"  Reliance   Digital "` â†’ `"reliance digital"`).
- **Files:** `procrastinator-backend/commons/entity/{account,movement,import_batch,import_line}.go(+_test)`, `procrastinator-backend/commons/money.go(+_test)`
- **Depends on:** â€” (pure; independent of the migration)
- **Verify:** `go build ./...` green; `go test ./commons/... -count=1` green without Docker/network.

## 3. commons/repo: extend `Repos`/`Factory` with finance repos (additive) `[backend]`

- [x] 3.1 In `procrastinator-backend/commons/repo/factory.go`, add four fields to **both**
  `Repos` and `Factory`: `Accounts Repository[entity.FinancialAccount]`, `Movements
  Repository[entity.MoneyMovement]`, `ImportBatches Repository[entity.ImportBatch]`,
  `ImportLines Repository[entity.ImportLine]`. Purely additive; existing fields and the
  `InTx` signature are unchanged. Import `entity` (already imported for `Asset`/`Source`/
  `Document`).
- **Files:** `procrastinator-backend/commons/repo/factory.go`
- **Depends on:** 2
- **Verify:** `go build ./...` green; `go test ./commons/... -count=1` green (new fields
  are nil; no consumer references them yet).

## 4. infra/postgres: finance repositories + aggregate queries `[backend]`

- [x] 4.1 `procrastinator-backend/infra/postgres/finance.go`: `NewAccountRepository`,
  `NewMovementRepository`, `NewImportBatchRepository`, `NewImportLineRepository` â€” each a
  `pgRepository[T]` with its table name + `scanX`/`xToMap` funcs. Money: `numeric` â†” exact
  decimal `string`. On write, derive `norm_description` via
  `commons.NormalizeDescription` when unset (mirror the `norm_brand` pattern in
  `repos.go`). `NewMovementRepository` returns the **concrete** `*MovementRepository`
  (embedding `*pgRepository[entity.MoneyMovement]`) so the composition root can use its
  query methods (4.2). Compile-time guards
  (`var _ repo.Repository[entity.FinancialAccount] = (*AccountRepository)(nil)`, etc.).
- [x] 4.2 `procrastinator-backend/infra/postgres/finance_queries.go` â€” concrete methods
  (NOT on the generic interface): `(*MovementRepository).BalanceForAccount(ctx,
  accountID string, opts ...repo.Option) (string, error)` (the D2 `COALESCE(SUM...) -
  COALESCE(SUM...)` as a decimal string, `0` when no movements); `(*MovementRepository).
  MovementsForAccount(ctx, accountID, opts...) ([]entity.MoneyMovement, error)`
  (`source_account_id = $a OR destination_account_id = $a`); `(*DocumentRepository).
  LinkCandidates(ctx, amount, currency string, opts ...repo.Option) ([]entity.Document, error)`
  (the D2 JSONB `extracted_fields->>'price'='$amount' AND ->>'currency'='$currency' AND
  NOT EXISTS (same-tenant movement with linked_document_id = doc.id)`). Tenant resolution
  follows the existing `resolveTenant` precedence.
- [x] 4.3 Wire the four finance repos into `procrastinator-backend/infra/postgres/factory.go`:
  populate them in `NewFactory` (pool-bound) and in the `InTx` callback (tx-bound)
  alongside the existing three. Additive; the `InTx` signature is unchanged.
- [x] 4.4 `procrastinator-backend/infra/postgres/finance_test.go` (integration, private
  schema e.g. `p_finance`, single env read in `TestMain`, explicit tenants): generic CRUD
  round-trip per finance entity (full field set; money `19999.99` exact round-trip;
  `norm_description` derived on write); `Get`/`Delete` miss â†’ `repo.ErrNotFound`; `List`
  stable order + non-nil `[]`; tenant scoping (foreign-tenant rows invisible; no tenant â†’
  `tenant.ErrNoTenant`); `BalanceForAccount` (income +/expense âˆ’/transfer in +/transfer
  out âˆ’/no movements â†’ `0`/cross-currency not representable is a data concern, not here);
  `MovementsForAccount` (source OR destination); `LinkCandidates` (exact match returns the
  doc, no match â†’ empty, an already-linked doc is excluded); `InTx` tx-bound finance repos
  see uncommitted writes and write nothing through the pool (the verify-2 regression
  pattern from `generic_test.go`).
- **Files:** `procrastinator-backend/infra/postgres/finance.go`, `finance_queries.go`, `finance_test.go`, `factory.go` (populate finance repos)
- **Depends on:** 3 (and 1 for the tables)
- **Verify:** `go test ./infra/postgres/ -count=1` green against compose PG (legacy + new
  suites); default-parallel `-count=5` stable; `go build ./...` green; `go list -deps
  ./infra/...` shows no cross-`infra/` imports.

## 5. infra adapters: statement source store + PDF text extractor `[backend]`

- [x] 5.1 `procrastinator-backend/infra/filestorage/statement.go`: `StatementStorage`
  (mirrors `Storage` but for statements) with `Store(ctx context.Context, originalName
  string, data []byte) (entity.Source, error)` â€” sniffs **CSV/PDF only** (unexported
  `sniffStatementType`: `%PDF-` â†’ `application/pdf`; a text/CSV heuristic â†’ `text/csv`;
  else an error `ErrStatementUnsupportedType`), writes the bytes to the storage dir under a
  UUID name, and returns a `Source` with **`Filename = originalName`** (the original, not
  the UUID), the sniffed `ContentType`, `Size`, `Path`, `SHA256`, `UploadedAt`. Does NOT
  modify `storage.go`.
- [x] 5.2 `procrastinator-backend/infra/pdftext/extractor.go`: `Extractor` with
  `ExtractText(data []byte) (string, error)` using `github.com/ledongthuc/pdf` (add the
  require to `go.mod`); returns the concatenated text layer; returns `""` (no error) when a
  PDF has no extractable text layer so the caller maps it to "no lines".
- [x] 5.3 Tests: `statement_test.go` â€” store a CSV payload â†’ `Source` with original
  filename + `text/csv` + correct sha256/size + bytes retrievable from `Path`; store a PDF
  â†’ `application/pdf`; store a PNG â†’ `ErrStatementUnsupportedType` and nothing written.
  `extractor_test.go` â€” `ExtractText` on a minimal in-memory text-layer PDF returns the
  text; on a byte slice with no text layer returns `""`.
- **Files:** `procrastinator-backend/infra/filestorage/statement.go(+_test)`, `procrastinator-backend/infra/pdftext/extractor.go(+_test)`, `procrastinator-backend/go.mod`
- **Depends on:** â€” (uses `entity.Source`; independent of the DB)
- **Verify:** `go test ./infra/filestorage/ ./infra/pdftext/ -count=1` green without
  Docker; `go build ./...` green.

## 6. core/ledger: accounts + movements + balance + links (pure fakes) `[backend]`

- [x] 6.1 `procrastinator-backend/core/ledger/service.go`: `Service` constructed as
  `New(factory *repo.Factory, balancer BalanceQuerier) *Service` where
  `BalanceQuerier` is a consumer-side interface
  (`BalanceForAccount(ctx, accountID string, opts ...repo.Option) (string, error)`).
  Methods (all resolve `tid` from ctx, fail-closed, pass `repo.Tenant(tid)`):
  - `CreateAccount(ctx, in) (entity.FinancialAccount, error)` â€” validate non-empty name,
    `ValidAccountType`, `IsValidCurrency`; reject a currency-change (there is no currency-
    update method â€” currency is immutable by construction); insert.
  - `ListAccounts(ctx) ([]entity.FinancialAccount, error)` â€” `ORDER BY created_at, id`.
  - `GetAccount(ctx, id) (entity.FinancialAccount, string, error)` â€” with derived balance
    via `balancer`; `repo.ErrNotFound` â†’ unknown.
  - `CreateManualMovement(ctx, in) (entity.MoneyMovement, error)` â€” validate `IsValidAmount`
    (>0), `IsValidCurrency`, non-blank description, kind shape (expenseâ†’source only,
    incomeâ†’destination only, transferâ†’both distinct), referenced accounts exist
    (`repo.ErrNotFound`) and their currency equals the movement currency; insert origin
    `manual`, `NormDescription` set.
  - `ListMovements(ctx, f MovementListFilter) ([]entity.MoneyMovement, error)` â€” optional
    `AccountID` (source OR destination) + `OccurredFrom`/`OccurredTo` (`occurred_on`
    range), `ORDER BY created_at, id`.
  - `GetMovement(ctx, id) (entity.MoneyMovement, error)`.
  - `PatchDescription(ctx, id, desc string) (entity.MoneyMovement, error)` â€” any origin;
    blank â†’ `ErrInvalid`; only `description`/`norm_description` change (core fields never
    touched).
  - `DeleteMovement(ctx, id) error` â€” origin `manual` only; origin `import` â†’ `ErrConflict`;
    unknown â†’ `repo.ErrNotFound`.
  - `Link(ctx, movementID, documentID, creator string) error` â€” D9 order: unknown movement
    / unknown document / cross-tenant document â†’ `repo.ErrNotFound`; already linked to a
    different doc â†’ `ErrConflict`; already linked to the same doc â†’ no-op; else set
    `linked_document_id` + `link_creator`, compute `link_conflicting` from the document's
    `extracted_fields` price/currency vs the movement's amount/currency (both present and
    either differs â†’ `true`; else `false`); retain both values (no overwrite).
  - `Unlink(ctx, movementID) error` â€” idempotent (clears link if present; no-op otherwise);
    unknown â†’ `repo.ErrNotFound`.
  Sentinel errors: `ErrInvalid`, `ErrConflict` (exported for `api/` mapping).
- [x] 6.2 `procrastinator-backend/core/ledger/movement.go`: pure
  `validateKindShape(kind, sourceID, destID string) error` and
  `currencyMatches(kind, accounts map[string]entity.FinancialAccount, currency string)
  error` used by `CreateManualMovement` (isolated for table-driven tests).
- [x] 6.3 `procrastinator-backend/core/ledger/service_test.go` (pure fakes of
  `*repo.Factory` + `BalanceQuerier`; no Docker; `t.Parallel()`): map every `financial-
  ledger` scenario â€” account create+read / invalid type / currency immutable (no update
  path) / duplicate names; movement expense persisted / transfer requires two distinct
  same-currency accounts / non-positive rejected / blank description rejected / income
  destination-only / unknown account â†’ `ErrNotFound` / manual origin; list filter by
  account / by occurred range; balance via fake `BalanceQuerier` (accumulates incomeâˆ’
  expense; transfer moves value; `0` when empty); provenance (imported movement exposes
  `import_batch_id`/`import_line`/`external_reference`; manual has none); link created+
  visible / idempotent same-link / already-linked movement â†’ `ErrConflict` / already-
  linked doc â†’ `ErrConflict` / cross-tenant doc â†’ `ErrNotFound`; unlink removes / idempotent
  no-op; conflict (disagreeing â†’ `link_conflicting` true + both retained; agreeing â†’ false);
  immutability (only `PatchDescription` mutates); description correction succeeds / blank
  rejected / manual delete / unknown delete â†’ `ErrNotFound`; imported movement delete â†’
  `ErrConflict`; tenant (same name two tenants â†’ two accounts; foreign id â†’ `ErrNotFound`;
  no tenant â†’ error).
- **Files:** `procrastinator-backend/core/ledger/service.go(+_test)`, `movement.go`
- **Depends on:** 2, 3
- **Verify:** `go test ./core/ledger/ -count=1` green without Docker/network, `-count=5`
  stable; `go build ./...` green; `go list -deps ./core/ledger/...` contains no `pgx`, no
  `net/http`, no `infra`.

## 7. core/statement: parse + classify + service (pure fakes) `[backend]`

- [x] 7.1 `procrastinator-backend/core/statement/parse.go`: `StatementFormat` enum
  (`csv`,`pdf`); `RawLine` (raw string); `ParsedLine` (`LineRef int`, `RawLine string`,
  `OccurredOn *time.Time`, `Amount *string`, `Direction string` (`in`/`out`), `Description
  *string`, `NormDescription *string`, `ExternalReference *string`, `Status string`,
  `ErrorReason *string`); `ParseCSV(data []byte) ([]string, error)` (stdlib `encoding/csv`;
  header-optional, one raw string per row); `ExtractFields(lineRef int, raw string)
  ParsedLine` (date via `commons.ParseDate`, amount + direction, description, optional
  external reference; if date, amount, or description is missing â†’ `Status=error` with a
  human-readable `ErrorReason`, does not affect other lines); `PDFTextExtractor` consumer-
  side interface (`ExtractText(data []byte) (string, error)`).
- [x] 7.2 `procrastinator-backend/core/statement/classify.go`: pure
  `Classify(lines []ParsedLine, existing []entity.MoneyMovement) []ParsedLine` implementing
  D7 priority (duplicate: external-ref match vs existing OR within-batch repeat; possible-
  duplicate: content fingerprint `(occurred_on, amount, norm_description)` vs existing any-
  origin; else valid). Deterministic; `existing` is already scoped to the account+tenant by
  the caller.
- [x] 7.3 `procrastinator-backend/core/statement/service.go`: `Service` constructed as
  `New(factory *repo.Factory, src StatementSourceStore, movs MovementsForAccountLister,
  linkCands LinkCandidateLister, pdf PDFTextExtractor, maxBytes int64, maxLines int)
  *Service` with consumer-side interfaces `StatementSourceStore` (`Store(ctx, originalName,
  data) (entity.Source, error)`), `MovementsForAccountLister`
  (`MovementsForAccount(ctx, accountID, opts...) ([]entity.MoneyMovement, error)`),
  `LinkCandidateLister` (`LinkCandidates(ctx, amount, currency, opts...) ([]entity.Document,
  error)`). Sentinel errors `ErrTooLarge`, `ErrUnsupportedType`, `ErrNoLines`,
  `ErrTooManyLines`, `ErrConflict`. Methods (resolve `tid` fail-closed):
  - `Upload(ctx, filename string, data []byte) (entity.ImportBatch, []entity.ImportLine,
    error)` â€” D8 upload flow: size check â†’ sniff (via `src`'s returned content type;
    non-CSV/PDF â†’ `ErrUnsupportedType`) â†’ store Source (retained even on later failure) â†’
    account exists (`repo.ErrNotFound`) â†’ parse (CSV via `ParseCSV`; PDF via `pdf.
    ExtractText` then split; zero lines â†’ `ErrNoLines`) â†’ `len(lines) > maxLines` â†’
    `ErrTooManyLines` â†’ `ExtractFields` each â†’ `existing = movs.MovementsForAccount(tid,
    account)` â†’ `Classify` â†’ `factory.InTx`: `ImportBatches.Create(preview)` +
    `ImportLines.Create(each)` (atomic). Returns the batch + lines.
  - `Commit(ctx, batchID) (CommitSummary, error)` â€” `CommitSummary{Created, Skipped int}`;
    `ImportBatches.Get` (`repo.ErrNotFound`); `discarded` â†’ `ErrConflict`; `committed` â†’
    recompute summary from the batch's movements + `applyAutoLinks(batch)` + return (200
    idempotent); `preview` â†’ `factory.InTx` (create one movement per `valid` line â€” origin
    `import`, kind by direction, currency = account.currency, provenance â€” then
    `ImportBatches.Update` to `committed` with counts) then `applyAutoLinks(batch)`;
    return summary.
  - `Discard(ctx, batchID) (entity.ImportBatch, error)` â€” `preview` â†’ `discarded` (200);
    `discarded` â†’ idempotent (200); `committed` â†’ `ErrConflict`.
  - `GetBatch(ctx, batchID) (entity.ImportBatch, []entity.ImportLine, error)`; `ListBatches(ctx)
    ([]entity.ImportBatch, error)` â€” `ORDER BY created_at, id`, `[]` when empty.
  - `applyAutoLinks(ctx, batch)` (unexported; idempotent) â€” for each import-origin movement
    of the batch not already linked: `cands = linkCands.LinkCandidates(tid, mv.Amount,
    mv.Currency)`; exactly one â†’ set `linked_document_id` + `link_creator="auto"` via
    `factory.Movements.Update` (pool-bound); zero/many â†’ no link.
- [x] 7.4 `procrastinator-backend/core/statement/parse_test.go` + `classify_test.go`
  (pure, no Docker): CSV parsing (rows, empty â†’ no lines, malformed row); `ExtractFields`
  (valid line; missing date â†’ error+reason; missing amount â†’ error; missing description â†’
  error; external reference present/absent); `Classify` (external-ref duplicate; content-
  fingerprint possible-duplicate; within-batch repeat â†’ first on merits, later duplicate;
  re-import of a committed set â†’ all duplicate; otherwise valid).
- [x] 7.5 `procrastinator-backend/core/statement/service_test.go` (pure fakes of
  `*repo.Factory`, `StatementSourceStore`, `MovementsForAccountLister`,
  `LinkCandidateLister`, `PDFTextExtractor`; no Docker): map every `statement-import`
  scenario â€” successful CSV â†’ preview batch with per-line statuses; unsupported type â†’
  `ErrUnsupportedType`; oversize â†’ `ErrTooLarge`; unknown account â†’ `ErrNotFound`; image-
  only PDF (extractor returns `""`) â†’ `ErrNoLines`; Source retained even on parse failure;
  preview creates no movements / unparseable line marked error / re-readable; commit
  creates movements for valid lines only (origin import, kind by direction, provenance) /
  atomic (injected mid-tx failure â†’ no movements, batch not committed) / re-commit
  idempotent (same summary, no new movements) / discarded â†’ `ErrConflict` / zero valid
  lines â†’ committed + 0 movements; discard preview â†’ discarded / re-discard idempotent /
  committed â†’ `ErrConflict`; auto-link exactly-one â†’ linked auto / multiple â†’ no link /
  already-linked â†’ not a candidate; list/read empty â†’ [] / batch read â†’ state+counts+
  lines / unknown â†’ `ErrNotFound`; bounds > maxLines â†’ `ErrTooManyLines` (Source retained,
  no batch); tenant (other tenant's batch â†’ `ErrNotFound`; duplicate detection ignores
  other tenants' movements; no tenant â†’ error).
- **Files:** `procrastinator-backend/core/statement/{parse,parse_test,classify,classify_test,service,service_test}.go`
- **Depends on:** 2, 3
- **Verify:** `go test ./core/statement/ -count=1` green without Docker/network, `-count=5`
  stable; `go build ./...` green; `go list -deps ./core/statement/...` contains no `pgx`,
  no `net/http`, no `infra`.

## 8. api: finance accounts + movements endpoints `[backend]`

- [x] 8.1 `procrastinator-backend/api/finance_dto.go`: `accountJSON` (incl. derived
  `balance` as a JSON string), `movementJSON` (incl. `origin`, import-provenance fields
  `import_batch_id`/`import_line`/`external_reference` when present, and the link:
  `linked_document_id`, `link_creator`, `link_conflicting`), and converters; money as JSON
  strings. (Import DTOs land in task 9.)
- [x] 8.2 `procrastinator-backend/api/finance_accounts.go`: handlers `createAccount`
  (`POST /api/finance/accounts` â†’ `201`; invalid â†’ `400`), `listAccounts`
  (`GET /api/finance/accounts` â†’ `200` []), `getAccount` (`GET /api/finance/accounts/{id}`
  â†’ `200` with balance / `404`).
- [x] 8.3 `procrastinator-backend/api/finance_movements.go`: handlers `createMovement`
  (`POST /api/finance/movements` â†’ `201`/`400`/`404`), `listMovements`
  (`GET /api/finance/movements` with `?account_id=` + `?from=`/`?to=` â†’ `200`),
  `getMovement` (`GET /api/finance/movements/{id}` â†’ `200`/`404`), `patchMovement`
  (`PATCH /api/finance/movements/{id}`, `description` only; any core field present â†’ `400`;
  blank â†’ `400`; unknown â†’ `404`), `deleteMovement` (`DELETE /api/finance/movements/{id}`
  â†’ `204`/`409` (import)/`404`), `linkMovement` (`POST /api/finance/movements/{id}/link`
  `document_id` â†’ `200`/`404`/`409`), `unlinkMovement` (`DELETE /api/finance/movements/{id}/
  link` â†’ `204`/`404`). Map `core/ledger` sentinels (`ErrInvalid`â†’400, `ErrConflict`â†’409)
  and `repo.ErrNotFound`â†’404, `tenant.ErrNoTenant`â†’401 (the `writeProcessError` pattern).
- [x] 8.4 `procrastinator-backend/api/server.go` (additive): add `ledger *ledger.Service`
  to `Server`; extend `New` to take it; register the account + movement routes in
  `Routes()` (`/api/finance/accounts`, `/api/finance/movements`, `/api/finance/movements/{id}`,
  `/api/finance/movements/{id}/link`). Reuse the existing `tenantFromCtx` helper.
- [x] 8.5 `procrastinator-backend/api/finance_accounts_test.go` +
  `finance_movements_test.go` (real PG, per-test private schemas, real tenant middleware â€”
  extend the `testEnv` pattern from `handlers_test.go` with a real `ledger.Service`): cover
  the ledger API scenarios â€” create+read (balance 0) / empty [] / unknown 404 / invalid
  type 400; create movement 201 + balance reflects / filter by account / filter by
  occurred range / unknown account 404 / invalid 400; get movement 200/404; PATCH
  description 200 / blank 400 / core-field 400; DELETE manual 204 / import 409 / unknown
  404; link 200 / idempotent 200 / 409 / cross-tenant 404; unlink 204 / 404; conflict
  flag; cross-tenant account 404; missing tenant 401.
- [x] 8.6 Minimal `procrastinator-backend/api/cmd/procrastinator/main.go` rewiring: build
  `ledgerSvc := ledger.New(factory, <concrete BalanceQuerier = the postgres movement
  repo>)` and pass it to `api.New(...)`; keep `go build ./...` green.
- **Files:** `procrastinator-backend/api/finance_dto.go`, `finance_accounts.go(+_test)`, `finance_movements.go(+_test)`, `server.go` (additive), `api/cmd/procrastinator/main.go` (minimal)
- **Depends on:** 4 (concrete `BalanceQuerier`), 6
- **Verify:** `go test ./api/ -count=1` green against compose PG (accounts + movements
  suites + existing document suites); `go build ./...` green; `go vet ./...` clean.

## 9. api: statement import endpoints + config bounds `[backend]`

- [x] 9.1 `procrastinator-backend/config/config.go` (additive): add `MaxStatementBytes`
  (env `PROCRASTINATOR_MAX_STATEMENT_BYTES`, default `52428800`) and `MaxStatementLines`
  (env `PROCRASTINATOR_MAX_STATEMENT_LINES`, default `100000`) to `Config` + `Load`
  (map-injected; `config_test.go` covers both defaults + overrides).
- [x] 9.2 `procrastinator-backend/api/finance_import.go`: handlers `createImportBatch`
  (`POST /api/finance/import-batches`, multipart `file` + `account_id`; `http.MaxBytesReader`
  â†’ `413`; missing/blank fields â†’ `400`; `ErrUnsupportedType` â†’ `415`; `ErrNoLines` â†’
  `422`; `ErrTooManyLines` â†’ `422`; unknown account â†’ `404`; success â†’ `201` with batch +
  lines+statuses), `listImportBatches` (`GET /api/finance/import-batches` â†’ `200` []),
  `getImportBatch` (`GET /api/finance/import-batches/{id}` â†’ `200` (state, per-status
  counts, Source reference, lines)/`404`), `commitImportBatch`
  (`POST /api/finance/import-batches/{id}/commit` â†’ `200` summary / `404` / `409`),
  `discardImportBatch` (`POST /api/finance/import-batches/{id}/discard` â†’ `200` / `404` /
  `409`). Map `core/statement` sentinels per the D11 table.
- [x] 9.3 `procrastinator-backend/api/finance_dto.go` (additive): `importBatchJSON` (state,
  per-status counts, `source` reference, `lines[]` with `line_ref`/fields/`status`/
  `error_reason`) + converters.
- [x] 9.4 `procrastinator-backend/api/server.go` (additive): add `statement *statement.
  Service` + `maxStatementBytes int64` to `Server`; extend `New`; register the import
  routes (`/api/finance/import-batches`, `/api/finance/import-batches/{id}`, `/commit`,
  `/discard`).
- [x] 9.5 `procrastinator-backend/api/finance_import_test.go` (real PG, private schemas,
  real `statement.Service` with a real `filestorage.StatementStorage` + `pdftext.Extractor`
  + real postgres `MovementsForAccount`/`LinkCandidates`): cover the import API scenarios â€”
  successful CSV â†’ `201` preview / unsupported `415` / oversize `413` / unknown account
  `404` / image-only PDF `422` / > line limit `422`; preview no movements / re-readable;
  commit `200` + movements created / re-commit idempotent / discarded `409`; discard `200` /
  re-discard `200` / committed `409`; list/read / empty [] / unknown `404`; cross-tenant
  batch `404`; auto-link observable on a committed movement. Use small CSV fixtures (and a
  minimal text-layer PDF fixture) under a package-local `testdata/`.
- [x] 9.6 Minimal `main.go` rewiring: build `statementSvc := statement.New(factory,
  filestorage.NewStatement(cfg.StorageDir), <movementsForAccount concrete>, <linkCandidates
  concrete>, pdftext.New(), cfg.MaxStatementBytes, cfg.MaxStatementLines)` and pass it (+
  `cfg.MaxStatementBytes`) to `api.New(...)`; keep `go build ./...` green.
- **Files:** `procrastinator-backend/api/finance_import.go(+_test)`, `finance_dto.go` (additive), `server.go` (additive), `api/cmd/procrastinator/main.go` (minimal), `config/config.go(+_test)`
- **Depends on:** 4, 5, 7
- **Verify:** `go test ./api/ ./config/ -count=1` green against compose PG (import suite +
  existing suites + config); `go build ./...` green; `go vet ./...` clean.

## 10. Composition root: final main.go + boot `[backend]`

- [x] 10.1 Finalize `procrastinator-backend/api/cmd/procrastinator/main.go`: construct the
  concrete adapters once (`postgres.NewMovementRepository(pool)` as both the ledger
  `BalanceQuerier` and the statement `MovementsForAccountLister`; the postgres
  `LinkCandidates` concrete; `filestorage.NewStatement(cfg.StorageDir)`; `pdftext.New()`),
  build `ledger.New(...)` and `statement.New(...)`, call the final `api.New(...)`; boot,
  migrate, serve with graceful shutdown (unchanged behavior). No new config here (added in
  task 9).
- **Files:** `procrastinator-backend/api/cmd/procrastinator/main.go`
- **Depends on:** 8, 9
- **Verify:** `go build ./...` green; against compose PG + real `.env`, the binary boots,
  migrates (`00001` + `00002`), and `POST /api/finance/accounts` with `X-Tenant-ID` â†’ `201`,
  `POST /api/finance/import-batches` with a CSV â†’ `201`.

## 11. Final gate `[backend]`

- [x] 11.1 From `procrastinator-backend/`: `go vet ./...`, `gofmt -l` (empty), `go build
  ./...`, full `go test ./... -count=1` green with compose PG up (integration) and without
  (explicit skips); default-parallel `-count=3` stable; env-grep confirms no env access in
  `*_test.go` outside the sanctioned per-package helpers; **layering**: `go list -deps
  ./core/...` free of `pgx`/`net/http`/`infra`, no cross-imports between `infra/` packages,
  `commons/` imports stdlib (+`entity`) only; interface guards compile; every `financial-
  ledger` and `statement-import` spec scenario is mapped to a test (see the scenario map
  below); `openspec validate statement-ledger-ingestion --strict` passes.
- **Depends on:** all

## Spec-scenario â†’ task traceability (CRIT-T-05 / SPX-005)

| Capability | Scenario(s) | Task(s) |
|---|---|---|
| financial-ledger | account model (persist/invalid type/currency immutable/dup names) | 2.1, 6.1, 6.3, 8.5 |
| financial-ledger | account API (create+read/empty []/404) | 8.2, 8.5 |
| financial-ledger | movement model (expense/transfer shape/non-positive/blank desc/income dest) | 2.2, 6.2, 6.3 |
| financial-ledger | money representation (exact round-trip) | 2.5, 4.4, 8.5 |
| financial-ledger | manual movement API (create/manual origin/filter account/filter range/unknown acct) | 6.1, 6.3, 8.3, 8.5 |
| financial-ledger | derived balance (accumulates/transfer/never set directly) | 4.2, 4.4, 6.3, 8.5 |
| financial-ledger | movement provenance (imported fields/manual none) | 2.2, 6.3, 8.5 |
| financial-ledger | link creation (manual/idempotent/409 both directions/cross-tenant 404) | 6.1, 6.3, 8.3, 8.5 |
| financial-ledger | link removal (unlink/idempotent 204/404) | 6.1, 6.3, 8.3, 8.5 |
| financial-ledger | link metadata + non-modification (auto creator/both sides unchanged) | 2.2, 6.1, 6.3, 8.5 |
| financial-ledger | link conflict retention (disagreeâ†’flagged/agreeâ†’no conflict) | 6.1, 6.3, 8.5 |
| financial-ledger | field immutability (core fields 400) | 6.1, 8.3, 8.5 |
| financial-ledger | manual lifecycle (desc correction/blank/manual delete/unknown 404) | 6.1, 6.3, 8.3, 8.5 |
| financial-ledger | imported lifecycle (no individual delete â†’ 409) | 6.1, 6.3, 8.3, 8.5 |
| financial-ledger | tenant scoping (two tenants/foreign id 404/fail-closed) | 4.4, 6.3, 8.5 |
| statement-import | batch model (preview with lines/terminal states) | 2.3, 7.3, 7.5 |
| statement-import | upload endpoint (CSV 201/415/413/404/422 image-only) | 5.1, 7.3, 7.5, 9.2, 9.5 |
| statement-import | source retention (survives parse failure/committed exposes Source) | 5.1, 7.3, 7.5, 9.5 |
| statement-import | parse + per-line preview (no movements/error line/re-readable) | 7.1, 7.4, 7.3, 7.5, 9.5 |
| statement-import | deterministic dedup (ext-ref/fingerprint/within-batch/re-import) | 7.2, 7.4, 7.5 |
| statement-import | commit (valid only/atomic/idempotent/409 discarded) | 7.3, 7.5, 9.5 |
| statement-import | discard (preview/re-discard/409 committed) | 7.3, 7.5, 9.5 |
| statement-import | auto-linking (exactly-one/multiple/already-linked) | 4.2, 7.3, 7.5, 9.5 |
| statement-import | list/read API (empty []/state+counts+lines/404) | 7.3, 7.5, 9.2, 9.5 |
| statement-import | ingestion bounds (line limit 422/60s budget) | 7.3, 7.5, 9.2, 9.5 |
| statement-import | tenant scoping (foreign batch 404/ignore other tenants/fail-closed) | 4.4, 7.5, 9.5 |
