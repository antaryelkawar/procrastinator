import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import * as client from './client';
import { customFetch } from './generated/mutator';
import { ApiError, errorCopy } from './errors';
import { API_BASE } from './config';

const ALICE = 'alice';
const USER_BASE = `${API_BASE}/users`;

// Basic Auth env vars are required by ui/src/lib/api/auth.ts (basicAuthHeader),
// which the mutator merges into every request. Stub them for the whole file so
// every recorded call carries the header.
const AUTH_USER = 'app';
const AUTH_PASS = 'secret';
// base64("app:secret") — hard-coded, independent of the implementation.
const AUTH_HEADER = 'Basic YXBwOnNlY3JldA==';

interface RecordedCall {
  url: string;
  init: RequestInit | undefined;
}

/** Every call recorded during the current test, across all fetch stubs. */
const allCalls: RecordedCall[] = [];

function record(call: RecordedCall): void {
  allCalls.push(call);
}

/**
 * Case-insensitive header lookup over any RequestInit headers shape
 * (Headers / tuple array / plain object) — used to assert the merged
 * outgoing headers without depending on the mutator's normalization choice.
 */
function headerOf(init: RequestInit | undefined, name: string): string | undefined {
  const headers = init?.headers;
  if (!headers) return undefined;
  const want = name.toLowerCase();
  if (headers instanceof Headers) return headers.get(name) ?? undefined;
  if (Array.isArray(headers)) {
    return headers.find(([key]) => key.toLowerCase() === want)?.[1];
  }
  const key = Object.keys(headers).find((k) => k.toLowerCase() === want);
  return key === undefined ? undefined : headers[key];
}

function stubFetch(response: Response): RecordedCall[] {
  const calls: RecordedCall[] = [];
  const mock = vi.fn(async (url: string, init: RequestInit) => {
    const entry = { url, init };
    calls.push(entry);
    record(entry);
    return response;
  });
  vi.stubGlobal('fetch', mock);
  return calls;
}

function stubFetchThatFails(): RecordedCall[] {
  const calls: RecordedCall[] = [];
  const mock = vi.fn(async (_url: string, init: RequestInit) => {
    const entry = { url: '', init };
    calls.push(entry);
    record(entry);
    throw new TypeError('fetch failed');
  });
  vi.stubGlobal('fetch', mock);
  return calls;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function errorResponse(status: number, body: string): Response {
  return new Response(body, { status, headers: { 'Content-Type': 'application/json' } });
}

beforeEach(() => {
  vi.stubEnv('VITE_API_BASIC_AUTH_USER', AUTH_USER);
  vi.stubEnv('VITE_API_BASIC_AUTH_PASSWORD', AUTH_PASS);
});

afterEach(() => {
  // Sweep: every request this file made must have carried the header. The
  // mutator's contract (task 4.2) is "present on every request", so a single
  // call without it fails the test that made it, not just a dedicated case.
  for (const call of allCalls) {
    expect(headerOf(call.init, 'Authorization'), `missing Authorization on ${call.init?.method ?? 'GET'} ${call.url}`).toBe(AUTH_HEADER);
  }
  allCalls.length = 0;
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

describe('URL construction', () => {
  it('targets /api/users/alice/assets for the asset list', async () => {
    const calls = stubFetch(jsonResponse([]));
    const assets = await client.listAssets(ALICE);
    expect(assets).toEqual([]);
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets`);
    // Every request carries the shared Basic Auth header (task 4.2 merge).
    expect(headerOf(calls[0]?.init, 'Authorization')).toBe(AUTH_HEADER);
  });

  it('targets user-scoped paths for finance routes', async () => {
    const calls = stubFetch(jsonResponse([]));
    await client.listAccounts(ALICE);
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/finance/accounts`);
  });

  it('targets user-scoped paths for movements with query params', async () => {
    const calls = stubFetch(jsonResponse([]));
    await client.listMovements(ALICE, { account_id: 'acc1', from: '2026-01-01' });
    expect(calls[0]?.url).toContain(`${USER_BASE}/${ALICE}/finance/movements?`);
    expect(calls[0]?.url).toContain('account_id=acc1');
    expect(calls[0]?.url).toContain('from=2026-01-01');
  });
});

describe('request serialization', () => {
  it('defaults to GET with no body', async () => {
    const calls = stubFetch(jsonResponse([]));
    await client.listMovements(ALICE);
    expect(calls[0]?.init?.method).toBe('GET');
    expect(calls[0]?.init?.body).toBeUndefined();
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(headers['Content-Type']).toBeUndefined();
  });

  it('serialises the body to JSON and sets Content-Type for POST', async () => {
    const calls = stubFetch(jsonResponse({ id: 'acc1' }, 201));
    await client.createAccount(ALICE, { name: 'Main', type: 'bank', currency: 'EUR' });
    expect(calls[0]?.init?.method).toBe('POST');
    expect(calls[0]?.init?.body).toBe(JSON.stringify({ name: 'Main', type: 'bank', currency: 'EUR' }));
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(headers['Content-Type']).toBe('application/json');
  });
});

describe('success unwrapping', () => {
  it('listAssets resolves the parsed JSON body', async () => {
    stubFetch(jsonResponse({ id: 'a1', data: { brand: 'Dell', doc_type: 'invoice', metadata: {} }, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' }));
    const asset = await client.getAsset(ALICE, 'a1');
    expect(asset.id).toBe('a1');
    expect(asset.data.brand).toBe('Dell');
  });

  it('deleteMovement resolves undefined for a 204 No Content response', async () => {
    const calls = stubFetch(new Response(null, { status: 204 }));
    const result = await client.deleteMovement(ALICE, 'mv1');
    expect(result).toBeUndefined();
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/finance/movements/mv1`);
    expect(calls[0]?.init?.method).toBe('DELETE');
  });
});

describe('asset lifecycle', () => {
  const asset = { id: 'a1', data: { brand: 'Dell', metadata: {} }, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };

  it('deleteAsset resolves undefined for a 204 and targets the asset path', async () => {
    const calls = stubFetch(new Response(null, { status: 204 }));
    const result = await client.deleteAsset(ALICE, 'a1');
    expect(result).toBeUndefined();
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets/a1`);
    expect(calls[0]?.init?.method).toBe('DELETE');
  });

  it('restoreAsset unwraps the asset body and posts to /restore', async () => {
    const calls = stubFetch(jsonResponse(asset));
    const restored = await client.restoreAsset(ALICE, 'a1');
    expect(restored.id).toBe('a1');
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets/a1/restore`);
    expect(calls[0]?.init?.method).toBe('POST');
  });

  it('patchAsset sends the patch body and unwraps the asset', async () => {
    const calls = stubFetch(jsonResponse({ ...asset, data: { ...asset.data, name: 'Microwave Oven' } }));
    const patched = await client.patchAsset(ALICE, 'a1', { name: 'Microwave Oven', asset_category: 'appliance' });
    expect(patched.data.name).toBe('Microwave Oven');
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets/a1`);
    expect(calls[0]?.init?.method).toBe('PATCH');
    expect(calls[0]?.init?.body).toBe(JSON.stringify({ name: 'Microwave Oven', asset_category: 'appliance' }));
  });

  it('mergeAsset sends the duplicate id and unwraps the survivor', async () => {
    const calls = stubFetch(jsonResponse({ ...asset, id: 'survivor' }));
    const survivor = await client.mergeAsset(ALICE, 'survivor', { duplicate_asset_id: 'dup1' });
    expect(survivor.id).toBe('survivor');
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets/survivor/merge`);
    expect(calls[0]?.init?.method).toBe('POST');
    expect(calls[0]?.init?.body).toBe(JSON.stringify({ duplicate_asset_id: 'dup1' }));
  });

  it('listAssets forwards include_deleted as a query param', async () => {
    const calls = stubFetch(jsonResponse([]));
    await client.listAssets(ALICE, { include_deleted: true });
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets?include_deleted=true`);
  });

  it('getAsset forwards include_deleted as a query param', async () => {
    const calls = stubFetch(jsonResponse(asset));
    await client.getAsset(ALICE, 'a1', { include_deleted: true });
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets/a1?include_deleted=true`);
  });

  it('addItems posts a multipart body and unwraps the per-item outcomes', async () => {
    const calls = stubFetch(jsonResponse([{ kind: 'asset_committed', asset_id: 'a1' }]));
    const file = new File(['receipt-bytes'], 'receipt.jpg', { type: 'image/jpeg' });
    const outcomes = await client.addItems(ALICE, { files: [file], text: 'note', account_id: 'acc1' });
    expect(outcomes).toHaveLength(1);
    expect(outcomes[0]?.kind).toBe('asset_committed');
    expect(outcomes[0]?.asset_id).toBe('a1');
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/add`);
    expect(calls[0]?.init?.method).toBe('POST');
    // Multipart body — the generated client sets no Content-Type (FormData does).
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(headers['Content-Type']).toBeUndefined();
    expect(calls[0]?.init?.body).toBeInstanceOf(FormData);
    expect((calls[0]?.init?.body as FormData).get('text')).toBe('note');
    expect((calls[0]?.init?.body as FormData).get('account_id')).toBe('acc1');
  });

  it('search forwards typed filter params as query params', async () => {
    const calls = stubFetch(jsonResponse({ hits: [], page: 1, page_size: 20, total: 0, total_pages: 0 }));
    await client.search(ALICE, {
      q: 'microwave',
      category: 'appliance',
      brand: 'LG',
      purchase_from: '2026-01-01',
      purchase_to: '2026-06-30',
      warranty_status: 'expiring_within:90',
      has_documents: true,
      doc_classification: 'amc',
    });
    const url = calls[0]?.url ?? '';
    expect(url).toContain('q=microwave');
    expect(url).toContain('category=appliance');
    expect(url).toContain('brand=LG');
    expect(url).toContain('purchase_from=2026-01-01');
    expect(url).toContain('purchase_to=2026-06-30');
    expect(url).toContain('warranty_status=expiring_within%3A90');
    expect(url).toContain('has_documents=true');
    expect(url).toContain('doc_classification=amc');
  });
});

describe('document reprocess / keep (duplicate resolution)', () => {
  const documentBody = { id: 'd1', data: { doc_type: 'invoice' }, source_filename: 'invoice.pdf' };

  it('parseDocumentChoiceUri extracts userId + documentId from a reprocess URI', () => {
    expect(client.parseDocumentChoiceUri(`/api/users/alice/documents/d1/reprocess`, 'reprocess')).toEqual({
      userId: 'alice',
      documentId: 'd1',
    });
    expect(client.parseDocumentChoiceUri(`/api/users/alice/documents/d1/keep`, 'keep')).toEqual({
      userId: 'alice',
      documentId: 'd1',
    });
    // Action mismatch → null
    expect(client.parseDocumentChoiceUri(`/api/users/alice/documents/d1/keep`, 'reprocess')).toBeNull();
    // Malformed → null
    expect(client.parseDocumentChoiceUri(`/not/a/match`, 'keep')).toBeNull();
    // Trailing query string is tolerated
    expect(client.parseDocumentChoiceUri(`/api/users/alice/documents/d1/keep?x=1`, 'keep')).toEqual({
      userId: 'alice',
      documentId: 'd1',
    });
  });

  it('reprocessDocument POSTs to the reprocess URI and unwraps the document', async () => {
    const calls = stubFetch(jsonResponse(documentBody));
    const doc = await client.reprocessDocument(ALICE, 'd1');
    expect(doc.id).toBe('d1');
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/documents/d1/reprocess`);
    expect(calls[0]?.init?.method).toBe('POST');
    // No comment → body is undefined (JSON.stringify(undefined) === undefined),
    // but the orval-generated client still sets Content-Type application/json.
    expect(calls[0]?.init?.body).toBeUndefined();
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(headers['Content-Type']).toBe('application/json');
  });

  it('reprocessDocument sends the comment as a JSON body when provided', async () => {
    const calls = stubFetch(jsonResponse(documentBody));
    await client.reprocessDocument(ALICE, 'd1', 'treat as warranty');
    expect(calls[0]?.init?.method).toBe('POST');
    expect(calls[0]?.init?.body).toBe(JSON.stringify({ comment: 'treat as warranty' }));
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(headers['Content-Type']).toBe('application/json');
  });

  it('keepDocument POSTs to the keep URI with no body and unwraps the document', async () => {
    const calls = stubFetch(jsonResponse(documentBody));
    const doc = await client.keepDocument(ALICE, 'd1');
    expect(doc.id).toBe('d1');
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/documents/d1/keep`);
    expect(calls[0]?.init?.method).toBe('POST');
    expect(calls[0]?.init?.body).toBeUndefined();
  });

  it('reprocess/keep throw an ApiError on a non-2xx response', async () => {
    stubFetch(errorResponse(404, JSON.stringify({ error: 'not found' })));
    const err = await client.keepDocument(ALICE, 'ghost').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(404);
    expect(apiError.detail).toBe('not found');
  });
});

describe('Basic Auth header merge (mutator task 4.2)', () => {
  it('attaches the Authorization header to a GET that sends no headers', async () => {
    const calls = stubFetch(jsonResponse([]));
    await client.listAssets(ALICE);
    const init = calls[0]?.init;
    expect(headerOf(init, 'Authorization')).toBe(AUTH_HEADER);
    // Augment, don't replace: nothing else is dropped or invented.
    expect(headerOf(init, 'Content-Type')).toBeUndefined();
  });

  it('preserves the generated Content-Type while adding Authorization on POST', async () => {
    const calls = stubFetch(jsonResponse({ id: 'acc1' }, 201));
    await client.createAccount(ALICE, { name: 'Main', type: 'bank', currency: 'EUR' });
    const init = calls[0]?.init;
    expect(headerOf(init, 'Content-Type')).toBe('application/json');
    expect(headerOf(init, 'Authorization')).toBe(AUTH_HEADER);
  });

  it('preserves caller-supplied headers passed through options', async () => {
    const calls = stubFetch(jsonResponse([]));
    // Exercise the mutator's options-passthrough path directly: the generated
    // client spreads `options` into init, so a caller header must survive the merge.
    await customFetch('/api/users/alice/assets?probe=1', {
      method: 'GET',
      headers: { 'X-Probe': 'yes' },
    });
    const init = calls[0]?.init;
    expect(headerOf(init, 'X-Probe')).toBe('yes');
    expect(headerOf(init, 'Authorization')).toBe(AUTH_HEADER);
  });

  it('merges into a Headers-instance init.headers without dropping its entries', async () => {
    const calls = stubFetch(jsonResponse([]));
    await customFetch('/api/users/alice/assets?hdrs=1', {
      method: 'GET',
      headers: new Headers({ 'X-Probe': 'headers-instance' }),
    });
    const init = calls[0]?.init;
    expect(headerOf(init, 'X-Probe')).toBe('headers-instance');
    expect(headerOf(init, 'Authorization')).toBe(AUTH_HEADER);
  });

  it('merges into a tuple-array init.headers without dropping its entries', async () => {
    const calls = stubFetch(jsonResponse([]));
    await customFetch('/api/users/alice/assets?arr=1', {
      method: 'GET',
      headers: [['X-Probe', 'array-form']],
    });
    const init = calls[0]?.init;
    expect(headerOf(init, 'X-Probe')).toBe('array-form');
    expect(headerOf(init, 'Authorization')).toBe(AUTH_HEADER);
  });

  it('throws naming the missing var and sends nothing when credentials are absent', async () => {
    vi.unstubAllEnvs();
    vi.stubEnv('VITE_API_BASIC_AUTH_PASSWORD', '');
    const calls = stubFetch(jsonResponse([]));
    const err = await client.listAssets(ALICE).catch((e) => e);
    expect(err).toBeInstanceOf(Error);
    expect((err as Error).message).toContain('VITE_API_BASIC_AUTH_PASSWORD');
    // Fail-fast: no request hits the wire with empty credentials.
    expect(calls).toHaveLength(0);
    // Re-stub so this test's (empty) sweep sees no recorded calls, and the
    // remaining tests run with credentials present.
    vi.stubEnv('VITE_API_BASIC_AUTH_USER', AUTH_USER);
    vi.stubEnv('VITE_API_BASIC_AUTH_PASSWORD', AUTH_PASS);
  });
});

describe('error envelope → ApiError', () => {
  it('throws ApiError with status and the verbatim backend detail', async () => {
    stubFetch(errorResponse(404, JSON.stringify({ error: 'unknown user' })));
    const err = await client.listAccounts('ghost').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(404);
    expect(apiError.detail).toBe('unknown user');
    expect(apiError.message).toBe(errorCopy(404));
  });

  it.each([400, 404, 409, 413, 415, 422, 502])('maps a %i response to status + copy', async (status) => {
    stubFetch(errorResponse(status, JSON.stringify({ error: 'boom' })));
    const err = await client.listAccounts(ALICE).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(status);
    expect(apiError.message).toBe(errorCopy(status));
    expect(apiError.detail).toBe('boom');
  });

  it('surfaces a 401 (Basic Auth rejected) as an ApiError, not a crash or hang', async () => {
    stubFetch(errorResponse(401, JSON.stringify({ error: 'unauthorized' })));
    const err = await client.listAccounts(ALICE).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(401);
    expect(apiError.message).toBe(errorCopy(401));
    expect(apiError.detail).toBe('unauthorized');
  });

  it('falls back to the raw body as detail when it is not JSON', async () => {
    stubFetch(new Response('unsupported statement type', { status: 415 }));
    const err = await client.listImportBatches(ALICE).catch((e) => e);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(415);
    expect(apiError.detail).toBe('unsupported statement type');
  });
});

describe('network failure', () => {
  it('throws ApiError with status 0 when fetch rejects', async () => {
    stubFetchThatFails();
    const err = await client.listAccounts(ALICE).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(0);
    expect(apiError.isNetwork()).toBe(true);
    expect(apiError.message).toBe(errorCopy(0));
  });
});
