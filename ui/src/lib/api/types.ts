/**
 * DTO interfaces mirroring the Procrastinator backend JSON wire contract
 * (verified 2026-09-01 in `.tmp/explore-api-json-contracts-20260901.md`).
 *
 * Property names are the **exact backend JSON keys** (snake_case), so a parsed
 * response body is assignable to these types without a transform layer.
 *
 * Nullability convention: the backend marshals Go pointer fields with `omitempty`,
 * so a null value is **key-omitted**, never `"key": null`. These interfaces model
 * that with optional properties (`?`). Two documented exceptions:
 *   - `link_conflicting` is ALWAYS present (no omitempty) → plain `boolean`.
 *   - `lines` on `ImportBatch` is `null` (nil slice, no omitempty) in list responses
 *     → `ImportLine[] | null`.
 *
 * Money/price is an exact-decimal string (pattern `^[0-9]+(\.[0-9]+)?$`, unsigned);
 * it is NEVER a number. Movement sign is carried by `kind`, import-line sign by
 * `direction`.
 */

// ---- Documents & assets (path tenancy under /api/users/{userId}) ----

/** Document type, as extracted and classified by ingestion. */
export type DocumentType = 'invoice' | 'warranty' | 'amc' | 'other';

/** Scope of a record. */
export type ScopeType = 'personal' | 'household';

/**
 * Asset as returned by `GET /api/users/{userId}/assets`, `GET …/assets/{assetId}`,
 * and the `201` body of `POST /api/users/{userId}/documents`.
 */
export interface Asset {
  readonly id: string;
  /** Omitted when null. */
  readonly brand?: string;
  /** Omitted when null. */
  readonly model?: string;
  /** Omitted when null. */
  readonly serial_number?: string;
  /** RFC3339Nano UTC (date-only values arrive as UTC midnight). Omitted when null. */
  readonly purchase_date?: string;
  /** RFC3339Nano UTC. Omitted when null. */
  readonly warranty_end?: string;
  /** Exact-decimal string. Omitted when null. */
  readonly price?: string;
  /** ISO 4217 code. Omitted when null. */
  readonly currency?: string;
  readonly doc_type: DocumentType;
  /** Free-form structured metadata; `{}` (never null) when empty. */
  readonly metadata: Record<string, string>;
  readonly created_at: string;
  readonly updated_at: string;
  readonly scope_type: ScopeType;
  /** Omitted for personal rows. */
  readonly owner_household_id?: string;
}

/** Document entry from `GET /api/users/{userId}/assets/{assetId}/documents`. */
export interface Document {
  readonly id: string;
  readonly doc_type: DocumentType;
  readonly source_filename: string;
  readonly source_uploaded_at: string;
  readonly created_at: string;
  readonly scope_type: ScopeType;
  /** Omitted for personal rows. */
  readonly owner_household_id?: string;
}

// ---- Finance: accounts (header tenancy under /api/finance) ----

/** Account type. */
export type AccountType = 'bank' | 'wallet' | 'cash' | 'credit_card';

/** Account as returned from `/api/finance/accounts`. */
export interface Account {
  readonly id: string;
  readonly name: string;
  readonly type: AccountType;
  readonly currency: string;
  /** Omitted when null. */
  readonly institution?: string;
  /** Omitted when null. */
  readonly external_descriptor?: string;
  /** Derived exact-decimal string; `"0"` when no movements; never client-settable. */
  readonly balance: string;
  readonly created_at: string;
  readonly updated_at: string;
}

/** Request body for `POST /api/finance/accounts`. */
export interface CreateAccountRequest {
  readonly name: string;
  readonly type: AccountType;
  readonly currency: string;
  /** Omitted when null. */
  readonly institution?: string;
  /** Omitted when null. */
  readonly external_descriptor?: string;
}

// ---- Finance: movements ----

/** Movement kind. */
export type MovementKind = 'expense' | 'income' | 'transfer';

/** Where a movement originated. */
export type MovementOrigin = 'manual' | 'import';

/** Who created a movement–document link. */
export type LinkCreator = 'manual' | 'auto';

/**
 * Movement as returned from `/api/finance/movements`. The movement–document link
 * is three **flat** fields (there is no nested `link` object):
 * `linked_document_id`, `link_creator`, `link_conflicting`.
 */
export interface Movement {
  readonly id: string;
  readonly kind: MovementKind;
  /** Unsigned exact-decimal string; sign is derived from `kind`. */
  readonly amount: string;
  readonly currency: string;
  /** Plain ISO date, `"2006-01-02"` (e.g. `"2026-08-20"`). */
  readonly occurred_on: string;
  readonly recorded_at: string;
  readonly description: string;
  readonly origin: MovementOrigin;
  /** Omitted for income (no source). */
  readonly source_account_id?: string;
  /** Omitted for income. */
  readonly destination_account_id?: string;
  /** Import-origin movements only. */
  readonly import_batch_id?: string;
  /** Import-origin movements only; integer. */
  readonly import_line?: number;
  /** Import provenance; omitted when null. */
  readonly external_reference?: string;
  /** Flat link field — omitted when the movement has no link. */
  readonly linked_document_id?: string;
  /** Flat link field — omitted when the movement has no link. */
  readonly link_creator?: LinkCreator;
  /** Flat link field — ALWAYS present (no omitempty), even when unlinked. */
  readonly link_conflicting: boolean;
  readonly created_at: string;
  readonly updated_at: string;
}

/** Request body for `POST /api/finance/movements`. */
export interface CreateMovementRequest {
  readonly kind: MovementKind;
  readonly amount: string;
  readonly currency: string;
  /** Plain ISO date, `"2006-01-02"`. */
  readonly occurred_on: string;
  readonly description: string;
  /** expense/transfer only. */
  readonly source_account_id?: string;
  /** income/transfer only. */
  readonly destination_account_id?: string;
}

/** Query params for `GET /api/finance/movements` (exact wire names). */
export interface MovementFilters {
  readonly account_id?: string;
  /** Inclusive occurred-on lower bound; ISO date. */
  readonly from?: string;
  /** Inclusive occurred-on upper bound; ISO date. */
  readonly to?: string;
}

/** Request body for `PATCH /api/finance/movements/{id}` (only `description`). */
export interface PatchMovementRequest {
  readonly description: string;
}

/** Request body for `POST /api/finance/movements/{id}/link`. */
export interface LinkMovementRequest {
  readonly document_id: string;
}

// ---- Finance: import batches ----

/** Import batch state. */
export type BatchState = 'preview' | 'committed' | 'discarded';

/** Detected statement format. */
export type ImportFormat = 'csv' | 'pdf';

/** Status of a single parsed import line. */
export type ImportLineStatus = 'valid' | 'duplicate' | 'possible-duplicate' | 'error';

/** Direction of an import line; sign is NOT encoded in `amount`. */
export type ImportDirection = 'in' | 'out';

/** Nested source-metadata block on an import batch. */
export interface ImportSource {
  readonly id: string;
  readonly filename: string;
  readonly content_type: string;
  readonly size: number;
  readonly sha256: string;
  readonly uploaded_at: string;
}

/**
 * Import batch as returned from `/api/finance/import-batches`.
 * `lines` is `null` in list responses (nil slice, no omitempty) and populated
 * in upload / get / discard responses.
 */
export interface ImportBatch {
  readonly id: string;
  readonly state: BatchState;
  readonly account_id: string;
  readonly source: ImportSource;
  readonly filename: string;
  readonly format: ImportFormat;
  readonly line_count_valid: number;
  readonly line_count_duplicate: number;
  readonly line_count_possible_dup: number;
  readonly line_count_error: number;
  readonly created_at: string;
  readonly updated_at: string;
  /** `null` in list responses; populated elsewhere. */
  readonly lines: ImportLine[] | null;
}

/** A single parsed import line. */
export interface ImportLine {
  /** 1-based line number. */
  readonly line_ref: number;
  readonly raw_line: string;
  /** Plain ISO date; omitted when null. */
  readonly occurred_on?: string;
  /** Unsigned exact-decimal string; sign carried by `direction`. Omitted when null. */
  readonly amount?: string;
  /** Omitted when null. */
  readonly direction?: ImportDirection;
  /** Omitted when null. */
  readonly description?: string;
  /** Omitted when null. */
  readonly external_reference?: string;
  readonly status: ImportLineStatus;
  /** Set only when `status` is `"error"`. */
  readonly error_reason?: string;
}

/** Commit summary returned by `POST /api/finance/import-batches/{id}/commit`. */
export interface CommitSummary {
  readonly created: number;
  readonly skipped: number;
}
