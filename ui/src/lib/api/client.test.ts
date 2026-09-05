import { describe, it, expect, vi, afterEach } from 'vitest';
import * as client from './client';
import { ApiError, errorCopy } from './errors';
import { API_BASE } from './config';

const ALICE = 'alice';
const USER_BASE = `${API_BASE}/users`;

interface RecordedCall {
  url: string;
  init: RequestInit | undefined;
}

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
  const mock = vi.fn(async (_url: string, _init: RequestInit) => {
    calls.push({ url: '', init: undefined });
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

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('URL construction', () => {
  it('targets /api/users/alice/assets for the asset list', async () => {
    const calls = stubFetch(jsonResponse([]));
    const assets = await client.listAssets(ALICE);
    expect(assets).toEqual([]);
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/assets`);
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
    stubFetch(jsonResponse({ id: 'a1', brand: 'Dell', doc_type: 'invoice', metadata: {}, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' }));
    const asset = await client.getAsset(ALICE, 'a1');
    expect(asset.id).toBe('a1');
    expect(asset.brand).toBe('Dell');
  });

  it('deleteMovement resolves undefined for a 204 No Content response', async () => {
    const calls = stubFetch(new Response(null, { status: 204 }));
    const result = await client.deleteMovement(ALICE, 'mv1');
    expect(result).toBeUndefined();
    expect(calls[0]?.url).toBe(`${USER_BASE}/${ALICE}/finance/movements/mv1`);
    expect(calls[0]?.init?.method).toBe('DELETE');
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
