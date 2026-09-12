# OpenSpec Changes — Implementation Checklist

**Source:** `docs/MASTER_FEATURE_DOC.md` (resolved architecture) → decomposition in `.tmp/architect-doc-decomposition-20260911.md`.
**How to use:** One row per OpenSpec change. Work top-to-bottom in the **Dependency order** column. Open each change with `openspec new "<slug>"`, then run the lifecycle (SPEC → REVIEW(SPEC) → SUBSPEC → REVIEW(SUBSPEC) → APPLY → REVIEW(CODE) ⇄ APPLY → COMMIT → ARCHIVE) per the OpenSpec workflow. Check a box when that change is fully committed.

**Shared-file rule:** Changes that share files must NOT run in parallel — sequence them (see the Shared-ownership row and the dependency order). Backend changes 3→4→5→6→7 and UI changes 15→16→17 each touch the same files.

---

## Dependency Order (work in this sequence)

| # | Slug | Title | Size | Deps must be done first | Shared-ownership | Status |
|---|------|-------|------|--------------------------|------------------|--------|
| 1 | `fix-review-cycle-criticals` | Review-cycle bug fixes + doc/code correctness consolidation | M | *(none — first)* | touches doc + whatever OCC test coverage exists | ☐ |
| 2 | `add-basic-auth` | Token/session auth at middleware, OAuth-ready boundary | M | #1 | backend only | ☐ |
| 3 | `payload-link-model-guardrails` | R1 mechanics: typed accessors, mandatory OCC, scalar-lift rule | M | #2 | **commons/entity + migration** — strictly before #4–#7 | ☐ |
| 4 | `tag-model-for-documents` | Key-value + value-only payload tags, typed accessors, searchability | M | #3 | **commons/entity + features/docs/composer** — sequence #3→#4 | ☐ |
| 5 | `entity-deletion-matrix` | Deletion matrix + delete-hygiene for all entities, Source delete prompt | M | #3 (and #4 so matrix tag rows exist) | re-opened by #9, #10, #11 (append matrix rows) | ☐ |
| 6 | `facet-model-doc-links` | Doc→0..n facets, doc detail linked-items UI, thumbnails, proxy download/view | L | #3, #5 | **commons/entity + `Document.AssetID` read path** — #6 before #7 | ☐ |
| 7 | `assembly-layer-reconciliation` | core/assembly: per-doc classifier, additive facets, event-driven orphan reconciliation | XL | #3, #5, #6 | **commons/entity + core/identity read path** — #6 before #7 | ☐ |
| 8 | `core-confidence-framework` | General core/confidence: signal registry, weighted-geometric-mean, configurable thresholds | M | #3 (tag signal after #4, #8) | **core/confidence** — #10, #11 append consumers | ☐ |
| 9 | `memory-entity` | Memory entity: type-tagged notes, per-type dedupe keys, CRUD page | M | #3, #5, #6 | re-opens #5 (appends matrix rows) | ☐ |
| 10 | `identity-tags-person-version-chain` | Identity-tagged docs → Person via §5.10 confidence gate, version-chain Person | L | #4, #8, #7, #9, #3, #5 | review-queue producer (see #12) | ☐ |
| 11 | `contacts-entity-derivation` | Contacts entity + confidence-gated auto-derivation, review-queue fallback | L | #3, #5, #7, consumes #8 | review-queue producer (see #12) | ☐ |
| 12 | `review-queue-enrichment` | Detailed review queue: view doc, edit inferred, comment + reprocess; low-confidence items | M | #6, #11 | review-queue consumer — after #10, #11 | ☐ |
| 13 | `embedding-provider-and-chunk-index` | core/embedding provider isolation + chunked-text child table, ≤5s eager-write budget | M | #2 (before #4, #9, #14) | embedding contract — #14 consumes | ☐ |
| 14 | `search-provider-registry-hybrid` | Provider-registry search over tsvector+pg_trgm+pgvector, RRF k=60 | XL | #3, #4, #6, #9, #10, #11, #13 | search providers — #9/#10/#11 register into same contract | ☐ |
| 15 | `nav-chrome-consistency` | Nav sheet left inside top-left ☰, back/home to `/`, coming-soon placeholders | S | *(none — any time after auth)* | **app-shell.tsx, add-button.tsx, router.tsx** — sequential with #16, #17 | ☐ |
| 16 | `unified-add-menu` | One global unified [+] add menu identical on every screen | M | #15 (shared shell files) | **app-shell.tsx, add-button.tsx, features/docs/composer** — never parallel with #15, #17 | ☐ |
| 17 | `accounts-tables-ux-redesign` | Accounts page revamp + app-native tables restyle | M | #6 (documents part); accounts side independent | **app-shell.tsx, shared components** — never parallel with #15, #16 | ☐ |
| 18 | `external-ingest-source-abstraction` | Ingest-source abstraction for async dir/object-storage (S3/GCS) | L (research) | #7 | research-tinged — proposal includes boundary investigation | ☐ |

---

## Recommended working order (single-threaded, safe)

```
1  →  2  →  3  →  4  →  5  →  6  →  7
                                  ↳  8  (can start right after 3)
       ↳  9  →  10  →  11  →  12
                              ↳  13  (can start right after 2)
                                    ↳  14  (after 9/10/11)
   UI (a different pair of eyes, from step 6 onward):  15  →  16  →  17
   18  last (research)
```

**Do NOT parallelize within these groups (shared files):**
- Backend data plane: `3 → 4 → 5 → 6 → 7` (all mutate `commons/entity` payload/accessor + migration).
- Review queue: `10, 11` (producers) **before** `12` (consumer) — same `core/review` + review UI.
- Search providers: `9, 10, 11` register into the same `core/search` contract that `14` edits.
- Confidence framework: `8` owns `core/confidence`; `10, 11` append consumers.
- UI shell: `15 → 16 → 17` (all own `app-shell.tsx`, `add-button.tsx`, `router.tsx`).
- Composer: `4` (tag chips) and `16` (add menu) both edit `features/docs/composer` — sequence, never parallel.

---

## Open items to resolve before or during the work

- **Confidence thresholds** — `derive_threshold` / `review_threshold` are configurable (architecture decided in §5.10). Suggested starting values to ratify: derive **0.80**, review **0.45**, equal weights (1/3 each). Numeric values TBD until real docs are run.
- **Tag vocabulary** — free-form by design. Suggested starter set (non-binding): value tags `passport`, `driving_license`, `pan`; key tags `id_number`, `name`, `expiry`.
- **Tag UX surface** — default proposed: tag chips on both doc detail and review queue (Change 12).
- **Memory vs identity order** — build order keeps #9 before #10; if you want Person sooner, #10 can move ahead of #9 with the memory-free-text branch stubbed.
- **Search #14 pre-slicing** — whether to split the XL engine (5 existing types first, then entity-provider deltas). Affects #9/#10/#11 parallelization; flag for the reviewer gate.

---

## Per-change template (what a finished change looks like)

```
openspec/changes/<slug>/
  proposal.md
  specs/<module>/delta-<module>.md
  design.md        (SUBSPEC phase)
  tasks.md         (SUBSPEC phase)
```

A change is DONE when: REVIEW(CODE) passes (0 critical findings), it is committed, and archived (`openspec validate --change "<slug>"` + archive). Keep `.autopilot/change-<slug>/state.md` current as you go.
