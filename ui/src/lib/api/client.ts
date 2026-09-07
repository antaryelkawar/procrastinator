/**
 * Thin wrapper over the orval-generated API client.
 *
 * The generated functions return `{ data, status, headers }` but the UI code
 * expects just the data (with errors thrown as ApiError). This module provides
 * convenience wrappers that unwrap the response.
 *
 * The actual HTTP calls and error handling are performed by the mutator
 * (./generated/mutator.ts), which throws ApiError on non-2xx or network failures.
 */
import type {
  Asset,
  Document,
  Account,
  Movement,
  ImportBatch,
  CommitSummary,
  Household,
  SearchQuickResponse,
  SearchResultsPage,
  IngestReview,
  ApproveReviewResponse,
  CreateAccountRequest,
  CreateMovementRequest,
  PatchMovementRequest,
  LinkMovementRequest,
  CreateHouseholdRequest,
  AddMemberRequest,
  AddItemOutcome,
  // Success-shaped response types. The mutator (customFetch) throws ApiError on
  // every non-2xx response, so the error union member is unreachable at runtime
  // and the resolved value is always the success shape. The cast to the
  // *ResponseSuccess type below narrows the `res` union so `.data` resolves to
  // exactly the payload this wrapper declares.
  listAssetsResponseSuccess,
  getAssetResponseSuccess,
  patchAssetResponseSuccess,
  restoreAssetResponseSuccess,
  mergeAssetResponseSuccess,
  addItemsResponseSuccess,
  listAssetDocumentsResponseSuccess,
  listAccountsResponseSuccess,
  getAccountResponseSuccess,
  createAccountResponseSuccess,
  listMovementsResponseSuccess,
  getMovementResponseSuccess,
  createMovementResponseSuccess,
  patchMovementResponseSuccess,
  linkMovementResponseSuccess,
  listImportBatchesResponseSuccess,
  getImportBatchResponseSuccess,
  commitImportBatchResponseSuccess,
  discardImportBatchResponseSuccess,
  listHouseholdsResponseSuccess,
  getHouseholdResponseSuccess,
  createHouseholdResponseSuccess,
  quickSearchResponseSuccess,
  searchResponseSuccess,
  listReviewsResponseSuccess,
  getReviewResponseSuccess,
  approveReviewResponseSuccess,
  rejectReviewResponseSuccess,
} from './generated/orval/procrastinator';

import type { CreateMovementInput, MergeRequest, PatchAssetRequest } from './schema';
import type { AddItemsBody } from './generated/orval/procrastinator';

import {
  listAssets as _listAssets,
  getAsset as _getAsset,
  deleteAsset as _deleteAsset,
  patchAsset as _patchAsset,
  restoreAsset as _restoreAsset,
  mergeAsset as _mergeAsset,
  addItems as _addItems,
  listAssetDocuments as _listAssetDocuments,
  listAccounts as _listAccounts,
  getAccount as _getAccount,
  createAccount as _createAccount,
  listMovements as _listMovements,
  getMovement as _getMovement,
  createMovement as _createMovement,
  patchMovement as _patchMovement,
  deleteMovement as _deleteMovement,
  linkMovement as _linkMovement,
  unlinkMovement as _unlinkMovement,
  listImportBatches as _listImportBatches,
  getImportBatch as _getImportBatch,
  commitImportBatch as _commitImportBatch,
  discardImportBatch as _discardImportBatch,
  listHouseholds as _listHouseholds,
  getHousehold as _getHousehold,
  createHousehold as _createHousehold,
  addHouseholdMember as _addHouseholdMember,
  quickSearch as _quickSearch,
  search as _search,
  listReviews as _listReviews,
  getReview as _getReview,
  approveReview as _approveReview,
  rejectReview as _rejectReview,
} from './generated/orval/procrastinator';

// ---------------------------------------------------------------------------
// Assets
// ---------------------------------------------------------------------------

export async function listAssets(
  userId: string,
  params?: { include_deleted?: boolean }
): Promise<Asset[]> {
  const res = await _listAssets(userId, params);
  return (res as listAssetsResponseSuccess).data;
}

export async function getAsset(
  userId: string,
  assetId: string,
  params?: { include_deleted?: boolean }
): Promise<Asset> {
  const res = await _getAsset(userId, assetId, params);
  return (res as getAssetResponseSuccess).data;
}

export async function listAssetDocuments(userId: string, assetId: string): Promise<Document[]> {
  const res = await _listAssetDocuments(userId, assetId);
  return (res as listAssetDocumentsResponseSuccess).data;
}

export async function deleteAsset(userId: string, assetId: string): Promise<void> {
  await _deleteAsset(userId, assetId);
}

export async function patchAsset(
  userId: string,
  assetId: string,
  body: PatchAssetRequest
): Promise<Asset> {
  const res = await _patchAsset(userId, assetId, body);
  return (res as patchAssetResponseSuccess).data;
}

export async function restoreAsset(userId: string, assetId: string): Promise<Asset> {
  const res = await _restoreAsset(userId, assetId);
  return (res as restoreAssetResponseSuccess).data;
}

export async function mergeAsset(
  userId: string,
  assetId: string,
  body: MergeRequest
): Promise<Asset> {
  const res = await _mergeAsset(userId, assetId, body);
  return (res as mergeAssetResponseSuccess).data;
}

/**
 * Unified add: multipart file(s) and/or pasted text, optionally scoped to a
 * finance account (required for statement items). Returns one uniform
 * per-item outcome in the order the items were sent.
 *
 * The generated client types the multipart fields as `string[]` (OpenAPI
 * can't express binary), so the wrapper accepts `File[]` and casts to the
 * generated `AddItemsBody` — the generated FormData builder appends File
 * values verbatim, which is exactly the multipart wire format the server
 * expects.
 */
export async function addItems(
  userId: string,
  body: { files?: File[]; text?: string; account_id?: string }
): Promise<AddItemOutcome[]> {
  const res = await _addItems(userId, body as AddItemsBody);
  return (res as addItemsResponseSuccess).data;
}

// ---------------------------------------------------------------------------
// Accounts
// ---------------------------------------------------------------------------

export async function listAccounts(userId: string): Promise<Account[]> {
  const res = await _listAccounts(userId);
  return (res as listAccountsResponseSuccess).data;
}

export async function getAccount(userId: string, id: string): Promise<Account> {
  const res = await _getAccount(userId, id);
  return (res as getAccountResponseSuccess).data;
}

export async function createAccount(userId: string, body: CreateAccountRequest): Promise<Account> {
  const res = await _createAccount(userId, body);
  return (res as createAccountResponseSuccess).data;
}

// ---------------------------------------------------------------------------
// Movements
// ---------------------------------------------------------------------------

export async function listMovements(
  userId: string,
  params?: { account_id?: string; from?: string; to?: string }
): Promise<Movement[]> {
  const res = await _listMovements(userId, params);
  return (res as listMovementsResponseSuccess).data;
}

export async function getMovement(userId: string, id: string): Promise<Movement> {
  const res = await _getMovement(userId, id);
  return (res as getMovementResponseSuccess).data;
}

/**
 * Accepts the loose UI `CreateMovementInput` (source/destination account ids
 * optional — the form omits the non-applicable account) and sends it to the
 * generated client. The OpenAPI schema documents both account ids as required
 * strings, so the generated `CreateMovementRequest` marks them non-optional;
 * the Go handler treats an absent/"" one as "not set", which is why the
 * omission is valid on the wire. The cast to the strict wire type keeps
 * callers passing the UI input type type-checking while satisfying the
 * generated client's signature without injecting empty-string fields into the
 * JSON body (which would be a wire-format drift from the pre-orval client).
 */
export async function createMovement(userId: string, body: CreateMovementInput): Promise<Movement> {
  const res = await _createMovement(userId, body as CreateMovementRequest);
  return (res as createMovementResponseSuccess).data;
}

export async function patchMovement(
  userId: string,
  id: string,
  body: PatchMovementRequest
): Promise<Movement> {
  const res = await _patchMovement(userId, id, body);
  return (res as patchMovementResponseSuccess).data;
}

export async function deleteMovement(userId: string, id: string): Promise<void> {
  await _deleteMovement(userId, id);
}

export async function linkMovement(
  userId: string,
  id: string,
  body: LinkMovementRequest
): Promise<Movement> {
  const res = await _linkMovement(userId, id, body);
  return (res as linkMovementResponseSuccess).data;
}

export async function unlinkMovement(userId: string, id: string): Promise<void> {
  await _unlinkMovement(userId, id);
}

// ---------------------------------------------------------------------------
// Import Batches
// ---------------------------------------------------------------------------

export async function listImportBatches(userId: string): Promise<ImportBatch[]> {
  const res = await _listImportBatches(userId);
  return (res as listImportBatchesResponseSuccess).data;
}

export async function getImportBatch(userId: string, id: string): Promise<ImportBatch> {
  const res = await _getImportBatch(userId, id);
  return (res as getImportBatchResponseSuccess).data;
}

export async function commitImportBatch(userId: string, id: string): Promise<CommitSummary> {
  const res = await _commitImportBatch(userId, id);
  return (res as commitImportBatchResponseSuccess).data;
}

export async function discardImportBatch(userId: string, id: string): Promise<ImportBatch> {
  const res = await _discardImportBatch(userId, id);
  return (res as discardImportBatchResponseSuccess).data;
}

// ---------------------------------------------------------------------------
// Households
// ---------------------------------------------------------------------------

export async function listHouseholds(userId: string): Promise<Household[]> {
  const res = await _listHouseholds(userId);
  return (res as listHouseholdsResponseSuccess).data;
}

export async function getHousehold(userId: string, id: string): Promise<Household> {
  const res = await _getHousehold(userId, id);
  return (res as getHouseholdResponseSuccess).data;
}

export async function createHousehold(userId: string, body: CreateHouseholdRequest): Promise<Household> {
  const res = await _createHousehold(userId, body);
  return (res as createHouseholdResponseSuccess).data;
}

export async function addHouseholdMember(
  userId: string,
  householdId: string,
  body: AddMemberRequest
): Promise<void> {
  await _addHouseholdMember(userId, householdId, body);
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

export async function quickSearch(
  userId: string,
  params?: { q?: string; limit?: number },
): Promise<SearchQuickResponse> {
  const res = await _quickSearch(userId, params);
  return (res as quickSearchResponseSuccess).data;
}

export async function search(
  userId: string,
  params?: {
    q?: string;
    page?: number;
    page_size?: number;
    category?: string;
    brand?: string;
    purchase_from?: string;
    purchase_to?: string;
    warranty_status?: string;
    has_documents?: boolean;
    doc_classification?: 'invoice' | 'receipt' | 'warranty' | 'amc' | 'statement' | 'other';
  },
): Promise<SearchResultsPage> {
  const res = await _search(userId, params);
  return (res as searchResponseSuccess).data;
}

// ---------------------------------------------------------------------------
// Ingest Reviews
// ---------------------------------------------------------------------------

export async function listReviews(
  userId: string,
  params?: { status?: 'pending' | 'approved' | 'rejected' },
): Promise<IngestReview[]> {
  const res = await _listReviews(userId, params);
  return (res as listReviewsResponseSuccess).data;
}

export async function getReview(userId: string, id: string): Promise<IngestReview> {
  const res = await _getReview(userId, id);
  return (res as getReviewResponseSuccess).data;
}

export async function approveReview(userId: string, id: string): Promise<ApproveReviewResponse> {
  const res = await _approveReview(userId, id);
  return (res as approveReviewResponseSuccess).data;
}

export async function rejectReview(userId: string, id: string): Promise<IngestReview> {
  const res = await _rejectReview(userId, id);
  return (res as rejectReviewResponseSuccess).data;
}
