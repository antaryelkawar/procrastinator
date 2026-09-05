# Proposal: proper-multi-tenant-search

## Why

The Procrastinator API can list a user's assets or movements, but there is no way to *find* anything by name, and its document-ingestion pipeline commits LLM guesses to canonical data with no notion of *how sure* the model is: an upload that extracts a serial number is merged into (or creates) an Asset the moment the LLM returns it, even when the model was unsure. Two problems compound:

1. **No search.** A user who knows their "MacBook" or their "rent" transaction cannot type that term and be told which assets, accounts, or movements match.
2. **Unhinged ingest.** The intelligent pipeline (upload → LLM categorization/extraction → identity merge) treats every extraction as equally trustworthy, so a low-confidence read is silently merged into the wrong Asset (or spawns a duplicate), and there is no place for a human to look at and approve an uncertain resolution.

This change broadens the original search-only scope into the **full intelligent multi-tenant ingestion/search surface**: it adds general **search** across a user's own data, and makes the existing ingestion pipeline **confidence-aware** — every upload's LLM extraction carries a confidence, the identity resolution is **gated by that confidence** (high confidence auto-commits exactly as it does today; low confidence is held as a **pending review** for a human to approve or reject), and the resulting records are **searchable** with their confidence surfaced. Every stage is a **proper multi-tenant** capability: scoped to the requesting user under the existing owner-model visibility rule, with the RLS backstop and no cross-owner leakage. This is an MVP: the data is not real and the matching / confidence strategy are expected to evolve, so the design is deliberately simple and easy to redesign.

## What Changes

- **NEW** — A `search` capability: tenant-scoped, read-only general search across the user's owned resources — assets, finance accounts, money movements, documents, and statement import batches — exposed as a fast quick-dropdown endpoint (capped, no pagination) and a paged results-page endpoint, with a shared hit shape and a single deterministic combined ordering. Search hits MAY carry the underlying record's resolution `confidence`.
- **NEW** — A `confidence-review` capability: the confidence model and the human-in-the-loop surface for uncertain resolutions. It defines the configurable **review threshold**, the **confidence gate** on identity resolution, the **pending-review** data model (owner-scoped, RLS-protected), and the review endpoints to list pending reviews, inspect one, **approve** it (commit: merge into the matched Asset or create a new one, reusing the existing identity-resolution and field-merge logic), or **reject** it (discard the candidate, retain the Source).
- **MODIFIED** — `llm-extraction`: the extraction result additionally carries a single `confidence` in `[0, 1]`; absent or out-of-range confidence degrades to "absent" (never an extraction failure).
- **MODIFIED** — `document-ingestion`: the upload endpoint response is now determined by the confidence gate — `201 Created` with the Asset when the resolution auto-commits (confidence at/above threshold), `202 Accepted` with a pending-review reference when it is held (confidence below threshold or absent), and `422` when there is no usable identity (unchanged).
- **NEW** — A `search-review-ui` capability: the React UI surfaces that consume the search and review contracts — a quick-search typeahead in the app shell, a paged search results page, and an ingest review queue (list pending/reviewed reviews, approve, reject) — plus the upload `202` held-for-review state. All API access is OpenAPI-first: the UI consumes the generated client (orval) and generated types (openapi-typescript) from `make codegen`, and the generated files are never hand-edited.

## Out of Scope

- **No new authentication or authorization model.** Search and review reuse the existing path tenancy (`/api/users/{userId}`), the existing user middleware, and the existing RLS backstop; no header tenancy, tokens, or new middleware are introduced.
- **No per-field confidence, relevance ranking, or full-text search.** MVP confidence is a single overall value from the LLM; per-field confidence, tokenization, stemming, and relevance ranking are documented future enhancements. Search matching is a literal case-insensitive substring with a fixed deterministic ordering.
- **No automatic re-extraction or correction.** A rejected review discards its candidate; the user re-uploads to try again. There is no re-run of the LLM and no editing of a candidate's fields through the API.
- **No UI beyond the three surfaces.** The UI work is limited to the quick-search typeahead, the paged search results page, and the ingest review queue (list/approve/reject), plus the upload `202` held-for-review state. It does NOT add new per-resource detail pages (account/movement/document), theming, search-history persistence, or client-side full-text filtering.
- **No rollback or reversal of committed data.** Approving a review commits it (Asset created/merged, Document linked); there is no "un-approve" and no reversal of an already-committed resolution.
- **Households are not searchable in MVP.** They are a small, separately-listed navigation concern; adding them to search is a follow-up.

## Capabilities

### New Capabilities

- `search`: Tenant-scoped general search across a user's owned resources (assets, finance accounts, money movements, documents, statement import batches), exposed as a fast quick-dropdown endpoint (capped, no pagination) and a paged results-page endpoint, with a shared hit shape (optionally carrying `confidence`) and a single deterministic combined ordering. Strictly read-only; conforms to the existing owner-model visibility rule, cross-owner isolation, RLS backstop, and OpenAPI source-of-truth conventions.
- `confidence-review`: The confidence model and review surface for uncertain ingest resolutions. Owns the configurable review threshold, the confidence gate applied to identity resolution (auto-commit at/above threshold, hold for review below/absent), the owner-scoped + RLS-protected pending-review data model, and the endpoints to list pending reviews, inspect one, approve it (commit via the existing identity-resolution/merge logic), or reject it (discard the candidate, retain the Source). Conforms to the existing owner-model visibility rule, cross-owner isolation, RLS backstop, and OpenAPI source-of-truth conventions.
- `search-review-ui`: The React UI surfaces that consume the `search` and `confidence-review` contracts: a quick-search typeahead in the app shell, a paged search results page, and an ingest review queue (list pending/reviewed reviews, approve, reject). All API access is OpenAPI-first (the orval-generated client + openapi-typescript types, never hand-edited generated code) and every screen is tenant-scoped to the active user, rendered inside the existing app shell.

### Modified Capabilities

- `llm-extraction`: the structured extraction result additionally carries a single `confidence` in `[0, 1]` (absent/out-of-range → treated as absent, not a failure). No existing requirement's behavior changes; this is additive.
- `document-ingestion`: the `POST /api/users/{userId}/documents` upload endpoint now responds `201` (auto-committed) or `202` (held for review) based on the confidence gate, in addition to the existing `422` (no usable identity) and `502` (LLM failure) paths.

## Impact

- **Code (backend):** a new `core/search` application service (federated, owner-scoped search over the existing `repo` repositories) and a new `core/review` application service (confidence gate + pending-review lifecycle) over the existing `repo` repositories; the existing `ingest.Service` and `identity.Resolve` gain a confidence-gated path (auto-commit vs. hold); new handlers in `procrastinator-backend/api/` (search + review); OpenAPI additions for the new operations and schemas. Reuses `repo` options (`Owner`, `Where`, `Limit`, `Offset`, `OrderBy`), the existing `UserMiddleware` path tenancy, the existing identity-resolution/field-merge logic (reused on approve), and the RLS backstop.
- **Code (UI):** new pages `/search` and `/ingest/reviews` + a quick-search typeahead in `ui/src/components/layout/app-shell.tsx`, wired through the existing OpenAPI-first data path — hand-written `ui/src/lib/api/{schema.ts,client.ts,hooks.ts}` wrappers/hooks over the orval-generated client (`ui/src/lib/api/generated/*`, produced by `make codegen` in Package 9 and never hand-edited); routes added in `ui/src/router.tsx`; the upload flow gains the `202` held-for-review outcome; a confidence formatter added to `ui/src/lib/format/`.
- **Contract:** two new GET search operations, four new review operations (list, get, approve, reject), and their schemas (`search_hit`, `search_quick_response`, `search_results_page`, `ingest_review`, `approve_review_response`); the `asset` and `document` schemas gain an optional `confidence`; all fully additive except the `document` upload response, which gains the `202` outcome. Routes↔operations stay in 1:1 lockstep.
- **Persistence:** a new owner-scoped, RLS-protected `ingest_reviews` table; optional `confidence` columns on `assets` and `documents`; optional text-column indexes for search. All behind versioned goose migrations; search itself is read-only.
- **Risk:** the principal risk is accidental cross-owner leakage (search and review both expose rows across types), mitigated by reusing the existing visibility rule + RLS backstop and by dedicated isolation scenarios (another owner's personal row is never returned; a member sees only their households' rows; an unbound session sees nothing). The secondary risk is a low-confidence read being auto-committed, mitigated by the confidence gate (fail-safe: absent confidence is held for review, never auto-committed).

## Non-Functional Requirements

- **Bounded quick results:** the quick search endpoint SHALL return at most `limit` hits (default 10, maximum 50) and SHALL return no pagination metadata.
- **Paged results:** the results-page search endpoint SHALL default to `page_size` 20 (maximum 100) and SHALL report the total number of matching hits (`total`).
- **Query bound:** the `q` value SHALL be accepted up to 200 characters; a longer value is rejected with `400`.
- **Confidence bounds:** `confidence` SHALL be a real in `[0.0, 1.0]`; the review threshold SHALL default to `0.7` and SHALL be configurable in `[0, 1]` via a `PROCRASTINATOR_`-prefixed environment variable.
- **Fail-safe gating:** an extraction with absent confidence SHALL be treated as below the threshold (held for review), never auto-committed.
- **Read-only search:** search SHALL NOT create, update, or delete any row; only the review approve/reject endpoints mutate canonical data.
- **Quick-endpoint latency (soft):** the quick endpoint is optimized for the typeahead hot path — it returns a bounded top-N set and does not compute a whole-collection count. (Soft goal; there is no hard latency budget because MVP data is not real.)
