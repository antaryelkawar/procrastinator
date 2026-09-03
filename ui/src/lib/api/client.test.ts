import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  apiFetch,
  apiJson,
  apiVoid,
  userPath,
  financePath,
  TENANT_HEADER,
  USER_BASE,
  FINANCE_BASE,
} from './client';
import { ApiError, errorCopy } from './errors';
import type { Asset } from './types';

const ALICE = 'alice';

interface RecordedCall {
  url: string;
  init: RequestInit | undefined;
}

/**
 * Install a stubbed global fetch that records every call and returns the
 * supplied response. Returns the recorded calls so assertions can inspect the
 * exact URL, method, and headers the client produced.
 */
function stubFetch(response: Response): RecordedCall[] {
  const calls: RecordedCall[] = [];
  const mock = vi.fn(async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    return response;
  });
  vi.stubGlobal('fetch', mock);
  return calls;
}

function stubFetchThatFails(): RecordedCall[] {
  const calls: RecordedCall[] = [];
  const mock = vi.fn(async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    throw new TypeError('fetch failed');
  });
  vi.stubGlobal('fetch', mock);
  return calls;
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

function errorResponse(status: number, body: string): Response {
  return new Response(body, { status, headers: { 'Content-Type': 'application/json' } });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('URL construction (centralized path builders)', () => {
  it('userPath puts the user in the path for documents/assets', () => {
    expect(userPath(ALICE, 'assets')).toBe(`${USER_BASE}/${ALICE}/assets`);
    expect(userPath(ALICE, 'assets/a1/documents')).toBe(`${USER_BASE}/${ALICE}/assets/a1/documents`);
    expect(userPath(ALICE, 'documents')).toBe(`${USER_BASE}/${ALICE}/documents`);
  });

  it('financePath targets /api/finance with no user in the path', () => {
    expect(financePath('accounts')).toBe(`${FINANCE_BASE}/accounts`);
    expect(financePath('movements')).toBe(`${FINANCE_BASE}/movements`);
    expect(financePath('movements/mv1/link')).toBe(`${FINANCE_BASE}/movements/mv1/link`);
    expect(financePath('import-batches')).toBe(`${FINANCE_BASE}/import-batches`);
  });

  it('targets /api/users/alice/assets for the asset list (spec scenario)', async () => {
    const calls = stubFetch(jsonResponse([]));
    const assets = await apiJson<Asset[]>({ kind: 'user', userId: ALICE }, 'assets');
    expect(assets).toEqual([]);
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets`);
  });
});

describe('tenancy transport', () => {
  it('sends NO X-Tenant-ID header for user-tenanted routes (path carries the user)', async () => {
    const calls = stubFetch(jsonResponse({ id: 'a1' }));
    await apiJson<Asset>({ kind: 'user', userId: ALICE }, 'assets/a1');
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets/a1`);
    expect(TENANT_HEADER in headers).toBe(false);
  });

  it('injects X-Tenant-ID for finance routes', async () => {
    const calls = stubFetch(jsonResponse([]));
    await apiJson({ kind: 'finance', userId: ALICE }, 'accounts');
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(calls[0]?.url).toBe(`${FINANCE_BASE}/accounts`);
    expect(headers[TENANT_HEADER]).toBe(ALICE);
  });

  it('keeps a custom header alongside the injected tenant header', async () => {
    const calls = stubFetch(jsonResponse({}));
    await apiJson({ kind: 'finance', userId: ALICE }, 'accounts', { headers: { 'X-Trace': 't1' } });
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(headers[TENANT_HEADER]).toBe(ALICE);
    expect(headers['X-Trace']).toBe('t1');
  });
});

describe('request serialization', () => {
  it('defaults to GET with no body', async () => {
    const calls = stubFetch(jsonResponse([]));
    await apiJson({ kind: 'finance', userId: ALICE }, 'movements');
    expect(calls[0]?.init?.method).toBe('GET');
    expect(calls[0]?.init?.body).toBeUndefined();
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(headers['Content-Type']).toBeUndefined();
  });

  it('serialises the body to JSON and sets Content-Type for POST', async () => {
    const calls = stubFetch(jsonResponse({ id: 'acc1' }));
    await apiJson({ kind: 'finance', userId: ALICE }, 'accounts', {
      method: 'POST',
      body: { name: 'Main', type: 'bank', currency: 'EUR' },
    });
    expect(calls[0]?.init?.method).toBe('POST');
    expect(calls[0]?.init?.body).toBe(JSON.stringify({ name: 'Main', type: 'bank', currency: 'EUR' }));
    const headers = (calls[0]?.init?.headers ?? {}) as Record<string, string>;
    expect(headers['Content-Type']).toBe('application/json');
  });
});

describe('success unwrapping', () => {
  it('apiJson resolves the parsed JSON body', async () => {
    stubFetch(jsonResponse({ id: 'a1', brand: 'Dell' }));
    const asset = await apiJson<Asset>({ kind: 'user', userId: ALICE }, 'assets/a1');
    expect(asset.id).toBe('a1');
    expect(asset.brand).toBe('Dell');
  });

  it('apiJson resolves undefined for a 204 No Content response', async () => {
    const calls = stubFetch(new Response(null, { status: 204 }));
    const result = await apiJson({ kind: 'finance', userId: ALICE }, 'accounts/acc1');
    expect(result).toBeUndefined();
    expect(calls[0]?.url).toBe(`${FINANCE_BASE}/accounts/acc1`);
  });

  it('apiVoid resolves for an empty success response', async () => {
    const calls = stubFetch(new Response(null, { status: 204 }));
    await expect(
      apiVoid({ kind: 'finance', userId: ALICE }, 'movements/mv1', { method: 'DELETE' }),
    ).resolves.toBeUndefined();
    expect(calls[0]?.init?.method).toBe('DELETE');
  });
});

describe('error envelope → ApiError', () => {
  it('throws ApiError with status and the verbatim backend detail', async () => {
    stubFetch(errorResponse(404, JSON.stringify({ error: 'unknown user' })));
    const err = await apiJson({ kind: 'finance', userId: 'ghost' }, 'accounts').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(404);
    expect(apiError.detail).toBe('unknown user');
    expect(apiError.message).toBe(errorCopy(404));
  });

  it.each([400, 404, 409, 413, 415, 422, 502])('maps a %i response to status + copy', async (status) => {
    stubFetch(errorResponse(status, JSON.stringify({ error: 'boom' })));
    const err = await apiFetch({ kind: 'finance', userId: ALICE }, 'accounts').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(status);
    expect(apiError.message).toBe(errorCopy(status));
    expect(apiError.detail).toBe('boom');
  });

  it('falls back to the raw body as detail when it is not JSON', async () => {
    stubFetch(new Response('unsupported statement type', { status: 415 }));
    const err = await apiFetch({ kind: 'finance', userId: ALICE }, 'import-batches').catch((e) => e);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(415);
    expect(apiError.detail).toBe('unsupported statement type');
  });

  it('leaves detail empty when the error body is empty', async () => {
    stubFetch(new Response('', { status: 502 }));
    const err = await apiFetch({ kind: 'finance', userId: ALICE }, 'accounts').catch((e) => e);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(502);
    expect(apiError.detail).toBe('');
    expect(apiError.message).toBe(errorCopy(502));
  });
});

describe('network failure', () => {
  it('throws ApiError with status 0 when fetch rejects', async () => {
    stubFetchThatFails();
    const err = await apiJson({ kind: 'finance', userId: ALICE }, 'accounts').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(0);
    expect(apiError.isNetwork()).toBe(true);
    expect(apiError.message).toBe(errorCopy(0));
  });
});
