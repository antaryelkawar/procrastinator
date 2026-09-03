import { describe, it, expect, vi, afterEach } from 'vitest';
import { uploadDocument, uploadStatement } from './upload';
import { userPath, financePath, TENANT_HEADER } from './client';
import { ApiError, errorCopy } from './errors';

const ALICE = 'alice';
const ACCOUNT_ID = 'acc1';

/** 201 document-upload response body (asset JSON, backend wire shape). */
const ASSET_JSON = {
  id: 'a1',
  brand: 'Dell',
  model: 'XPS 13',
  doc_type: 'invoice',
  metadata: {},
  created_at: '2026-09-01T10:00:00Z',
  updated_at: '2026-09-01T10:00:00Z',
  scope_type: 'personal',
};

/** 201 statement-upload response body (import batch JSON, backend wire shape). */
const BATCH_JSON = {
  id: 'b1',
  state: 'preview',
  account_id: ACCOUNT_ID,
  source: {
    id: 's1',
    filename: 'statement.csv',
    content_type: 'text/csv',
    size: 42,
    sha256: 'abc123',
    uploaded_at: '2026-09-01T10:00:00Z',
  },
  filename: 'statement.csv',
  format: 'csv',
  line_count_valid: 8,
  line_count_duplicate: 1,
  line_count_possible_dup: 0,
  line_count_error: 1,
  created_at: '2026-09-01T10:00:00Z',
  updated_at: '2026-09-01T10:00:00Z',
  lines: [
    {
      line_ref: 1,
      raw_line: '2026-08-20,100,out,coffee',
      occurred_on: '2026-08-20',
      amount: '100',
      direction: 'out',
      description: 'coffee',
      status: 'valid',
    },
  ],
};

/** Build a real (jsdom) File for upload assertions. */
function makeFile(name: string, type: string): File {
  return new File(['fake-bytes'], name, { type });
}

/**
 * Minimal `XMLHttpRequest` stand-in the tests drive manually:
 * `emitUploadProgress`, `respond`, `failNetwork`. Records the URL, method,
 * request headers, and the sent `FormData` so multipart fields can be
 * asserted (design D6/D10 — fake-XHR tests, no network).
 */
class FakeXHR {
  static instances: FakeXHR[] = [];

  method: string | null = null;
  url: string | null = null;
  readonly requestHeaders: Record<string, string> = {};
  sentForm: FormData | null = null;

  status = 0;
  responseText = '';

  readonly upload: {
    onprogress: ((event: { loaded: number; total: number }) => void) | null;
  } = { onprogress: null };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;

  constructor() {
    FakeXHR.instances.push(this);
  }

  open(method: string, url: string): void {
    this.method = method;
    this.url = url;
  }

  setRequestHeader(name: string, value: string): void {
    this.requestHeaders[name] = value;
  }

  send(body: FormData): void {
    this.sentForm = body;
  }

  /** Fire an upload progress event. */
  emitUploadProgress(loaded: number, total: number): void {
    this.upload.onprogress?.({ loaded, total });
  }

  /** Complete with an HTTP response (any status). */
  respond(status: number, body: string): void {
    this.status = status;
    this.responseText = body;
    this.onload?.();
  }

  /** Complete with no response (network failure). */
  failNetwork(): void {
    this.onerror?.();
  }
}

function installFakeXhr(): void {
  vi.stubGlobal('XMLHttpRequest', FakeXHR);
}

/** The XHR the upload helper created (cleared per test). */
function lastXhr(): FakeXHR {
  const xhr = FakeXHR.instances[FakeXHR.instances.length - 1];
  if (!xhr) {
    throw new Error('no XHR was created');
  }
  return xhr;
}

afterEach(() => {
  FakeXHR.instances = [];
  vi.unstubAllGlobals();
});

describe('uploadDocument — multipart `file` to the user documents path', () => {
  it('POSTs the file as field `file` to the centralized user path and resolves the 201 asset', async () => {
    installFakeXhr();
    const file = makeFile('invoice.pdf', 'application/pdf');
    const promise = uploadDocument(ALICE, file, () => {});
    const xhr = lastXhr();

    expect(xhr.method).toBe('POST');
    expect(xhr.url).toBe(userPath(ALICE, 'documents'));

    const form = xhr.sentForm;
    expect(form).not.toBeNull();
    expect(form?.get('file')).toBe(file);
    expect(form?.getAll('file').length).toBe(1);
    expect(form?.get('account_id')).toBeNull();

    // Path tenancy: the user is in the URL, no tenant header.
    expect(TENANT_HEADER in xhr.requestHeaders).toBe(false);
    // The browser must own Content-Type (multipart boundary) — never set it.
    expect('Content-Type' in xhr.requestHeaders).toBe(false);

    xhr.respond(201, JSON.stringify(ASSET_JSON));
    const asset = await promise;
    expect(asset).toEqual(ASSET_JSON);
    expect(asset.id).toBe('a1');
    expect(asset.doc_type).toBe('invoice');
  });
});

describe('progress reporting (design D6 — XHR upload.onprogress)', () => {
  it('uploadDocument reports every upload progress event to onProgress', async () => {
    installFakeXhr();
    const progress = vi.fn();
    const promise = uploadDocument(ALICE, makeFile('invoice.pdf', 'application/pdf'), progress);
    const xhr = lastXhr();

    xhr.emitUploadProgress(10, 100);
    xhr.emitUploadProgress(60, 100);
    xhr.emitUploadProgress(100, 100);
    xhr.respond(201, JSON.stringify(ASSET_JSON));
    await promise;

    expect(progress).toHaveBeenCalledTimes(3);
    expect(progress).toHaveBeenNthCalledWith(1, { loaded: 10, total: 100 });
    expect(progress).toHaveBeenNthCalledWith(2, { loaded: 60, total: 100 });
    expect(progress).toHaveBeenNthCalledWith(3, { loaded: 100, total: 100 });
  });

  it('uploadStatement reports every upload progress event to onProgress', async () => {
    installFakeXhr();
    const progress = vi.fn();
    const promise = uploadStatement(ALICE, ACCOUNT_ID, makeFile('statement.csv', 'text/csv'), progress);
    const xhr = lastXhr();

    xhr.emitUploadProgress(1, 3);
    xhr.emitUploadProgress(3, 3);
    xhr.respond(201, JSON.stringify(BATCH_JSON));
    await promise;

    expect(progress).toHaveBeenCalledTimes(2);
    expect(progress).toHaveBeenNthCalledWith(1, { loaded: 1, total: 3 });
    expect(progress).toHaveBeenNthCalledWith(2, { loaded: 3, total: 3 });
  });
});

describe('uploadStatement — multipart `file` + `account_id` to the finance import path', () => {
  it('POSTs fields `file` and `account_id` with X-Tenant-ID and resolves the 201 batch', async () => {
    installFakeXhr();
    const file = makeFile('statement.csv', 'text/csv');
    const promise = uploadStatement(ALICE, ACCOUNT_ID, file, () => {});
    const xhr = lastXhr();

    expect(xhr.method).toBe('POST');
    expect(xhr.url).toBe(financePath('import-batches'));

    const form = xhr.sentForm;
    expect(form).not.toBeNull();
    expect(form?.get('file')).toBe(file);
    expect(form?.get('account_id')).toBe(ACCOUNT_ID);

    // Header tenancy: the tenant rides X-Tenant-ID, not the URL.
    expect(xhr.requestHeaders[TENANT_HEADER]).toBe(ALICE);
    // The browser must own Content-Type (multipart boundary) — never set it.
    expect('Content-Type' in xhr.requestHeaders).toBe(false);

    xhr.respond(201, JSON.stringify(BATCH_JSON));
    const batch = await promise;
    expect(batch).toEqual(BATCH_JSON);
    expect(batch.id).toBe('b1');
    expect(batch.state).toBe('preview');
  });
});

describe('error handling — ApiError rejection per status', () => {
  it.each([400, 404, 409, 413, 415, 422, 502])(
    'uploadDocument rejects a %i response with an ApiError carrying status, copy, and backend detail',
    async (status) => {
      installFakeXhr();
      const promise = uploadDocument(ALICE, makeFile('invoice.pdf', 'application/pdf'), () => {});
      lastXhr().respond(status, JSON.stringify({ error: 'boom' }));

      const err = await promise.catch((e: unknown) => e);
      expect(err).toBeInstanceOf(ApiError);
      const apiError = err as ApiError;
      expect(apiError.status).toBe(status);
      expect(apiError.message).toBe(errorCopy(status));
      expect(apiError.detail).toBe('boom');
    },
  );

  it.each([400, 404, 409, 413, 415, 422, 502])(
    'uploadStatement rejects a %i response with an ApiError carrying status, copy, and backend detail',
    async (status) => {
      installFakeXhr();
      const promise = uploadStatement(ALICE, ACCOUNT_ID, makeFile('statement.csv', 'text/csv'), () => {});
      lastXhr().respond(status, JSON.stringify({ error: 'boom' }));

      const err = await promise.catch((e: unknown) => e);
      expect(err).toBeInstanceOf(ApiError);
      const apiError = err as ApiError;
      expect(apiError.status).toBe(status);
      expect(apiError.message).toBe(errorCopy(status));
      expect(apiError.detail).toBe('boom');
    },
  );

  it('rejects with a network ApiError (status 0) when the upload fails without a response', async () => {
    installFakeXhr();
    const promise = uploadDocument(ALICE, makeFile('invoice.pdf', 'application/pdf'), () => {});
    lastXhr().failNetwork();

    const err = await promise.catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(0);
    expect(apiError.isNetwork()).toBe(true);
    expect(apiError.message).toBe(errorCopy(0));
  });

  it('uploadStatement also rejects with a network ApiError (status 0) on failure without a response', async () => {
    installFakeXhr();
    const promise = uploadStatement(ALICE, ACCOUNT_ID, makeFile('statement.csv', 'text/csv'), () => {});
    lastXhr().failNetwork();

    const err = await promise.catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).isNetwork()).toBe(true);
  });

  it('falls back to the raw body as detail when the error body is not JSON', async () => {
    installFakeXhr();
    const promise = uploadStatement(ALICE, ACCOUNT_ID, makeFile('statement.csv', 'text/csv'), () => {});
    lastXhr().respond(415, 'unsupported statement type');

    const err = await promise.catch((e: unknown) => e);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(415);
    expect(apiError.detail).toBe('unsupported statement type');
    expect(apiError.message).toBe(errorCopy(415));
  });

  it('leaves detail empty when the error body is empty', async () => {
    installFakeXhr();
    const promise = uploadDocument(ALICE, makeFile('invoice.pdf', 'application/pdf'), () => {});
    lastXhr().respond(502, '');

    const err = await promise.catch((e: unknown) => e);
    const apiError = err as ApiError;
    expect(apiError.status).toBe(502);
    expect(apiError.detail).toBe('');
    expect(apiError.message).toBe(errorCopy(502));
  });
});
