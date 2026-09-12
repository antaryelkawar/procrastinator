import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { uploadDocument, uploadStatement } from './upload';
import { ApiError, errorCopy } from './errors';
import { API_BASE } from './config';

const ALICE = 'alice';
const ACCOUNT_ID = 'acc1';
const USER_BASE = `${API_BASE}/users`;

// Basic Auth env vars are required by ui/src/lib/api/auth.ts (basicAuthHeader),
// which upload.ts merges into every XHR request. Stub them for the whole file
// so the module-level import works and every request carries the header.
const AUTH_USER = 'app';
const AUTH_PASS = 'secret';
// base64("app:secret") — hard-coded, independent of the implementation.
const AUTH_HEADER = 'Basic YXBwOnNlY3JldA==';

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
  lines: null,
};

function makeFile(name: string, type: string): File {
  return new File(['fake-bytes'], name, { type });
}

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

  emitUploadProgress(loaded: number, total: number): void {
    this.upload.onprogress?.({ loaded, total });
  }

  respond(status: number, body: string): void {
    this.status = status;
    this.responseText = body;
    this.onload?.();
  }

  failNetwork(): void {
    this.onerror?.();
  }
}

function installFakeXhr(): void {
  vi.stubGlobal('XMLHttpRequest', FakeXHR);
}

function lastXhr(): FakeXHR {
  const xhr = FakeXHR.instances[FakeXHR.instances.length - 1];
  if (!xhr) {
    throw new Error('no XHR was created');
  }
  return xhr;
}

beforeEach(() => {
  vi.stubEnv('VITE_API_BASIC_AUTH_USER', AUTH_USER);
  vi.stubEnv('VITE_API_BASIC_AUTH_PASSWORD', AUTH_PASS);
});

afterEach(() => {
  FakeXHR.instances = [];
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

describe('uploadDocument', () => {
  it('POSTs the file as field `file` to the centralized user path and resolves the 201 asset', async () => {
    installFakeXhr();
    const file = makeFile('invoice.pdf', 'application/pdf');
    const promise = uploadDocument(ALICE, file, () => {});
    const xhr = lastXhr();

    expect(xhr.method).toBe('POST');
    expect(xhr.url).toBe(`${USER_BASE}/${ALICE}/documents`);

    const form = xhr.sentForm;
    expect(form).not.toBeNull();
    expect(form?.get('file')).toBe(file);
    // Every XHR upload carries the shared Basic Auth header (spec scenario
    // "Multipart upload carries authorization header"); value is the fixed
    // known-answer base64 of "app:secret", independent of the implementation.
    expect(xhr.requestHeaders['Authorization']).toBe(AUTH_HEADER);
    expect('X-Tenant-ID' in xhr.requestHeaders).toBe(false);
    expect('Content-Type' in xhr.requestHeaders).toBe(false);

    xhr.respond(201, JSON.stringify(ASSET_JSON));
    const result = await promise;
    expect(result.kind).toBe('committed');
    expect((result as { kind: 'committed'; asset: unknown }).asset).toEqual(ASSET_JSON);
  });

  it('resolves with held state on 202', async () => {
    installFakeXhr();
    const file = makeFile('low-conf.pdf', 'application/pdf');
    const promise = uploadDocument(ALICE, file, () => {});
    const xhr = lastXhr();

    const review = { id: 'rev-1', state: 'pending' };
    xhr.respond(202, JSON.stringify(review));
    const result = await promise;
    expect(result.kind).toBe('held');
    expect((result as { kind: 'held'; review: unknown }).review).toEqual(review);
  });

  it('appends the `note` field to the multipart body when a note is provided', async () => {
    installFakeXhr();
    const file = makeFile('invoice.pdf', 'application/pdf');
    const promise = uploadDocument(ALICE, file, () => {}, 'under warranty');
    const xhr = lastXhr();

    const form = xhr.sentForm;
    expect(form).not.toBeNull();
    expect(form?.get('file')).toBe(file);
    expect(form?.get('note')).toBe('under warranty');

    xhr.respond(201, JSON.stringify(ASSET_JSON));
    const result = await promise;
    expect(result.kind).toBe('committed');
  });

  it('omits the `note` field when no note is provided', async () => {
    installFakeXhr();
    const file = makeFile('invoice.pdf', 'application/pdf');
    const promise = uploadDocument(ALICE, file, () => {});
    const xhr = lastXhr();

    const form = xhr.sentForm;
    expect(form).not.toBeNull();
    expect(form?.get('note')).toBeNull();

    xhr.respond(201, JSON.stringify(ASSET_JSON));
    const result = await promise;
    expect(result.kind).toBe('committed');
  });

  it('omits the `note` field when the note is blank/whitespace-only', async () => {
    installFakeXhr();
    const file = makeFile('invoice.pdf', 'application/pdf');
    const promise = uploadDocument(ALICE, file, () => {}, '   ');
    const xhr = lastXhr();

    const form = xhr.sentForm;
    expect(form).not.toBeNull();
    expect(form?.get('note')).toBeNull();

    xhr.respond(201, JSON.stringify(ASSET_JSON));
    const result = await promise;
    expect(result.kind).toBe('committed');
  });

  it('resolves the structured DuplicateReport on 409 (does not reject)', async () => {
    installFakeXhr();
    const file = makeFile('invoice.pdf', 'application/pdf');
    const promise = uploadDocument(ALICE, file, () => {});
    const xhr = lastXhr();

    const report = {
      code: 'duplicate',
      existing_document_id: 'doc-1',
      existing_source_filename: 'existing.pdf',
      existing_source_uploaded_at: '2026-09-01T10:00:00Z',
      existing_asset_id: 'a1',
      prompt: {
        reprocess_uri: `/api/users/${ALICE}/documents/doc-1/reprocess`,
        keep_uri: `/api/users/${ALICE}/documents/doc-1/keep`,
        expires_at: '2026-09-01T10:10:00Z',
        timeout_toast: 'no response — keeping existing document',
      },
    };
    xhr.respond(409, JSON.stringify(report));
    const result = await promise;
    expect(result.kind).toBe('duplicate');
    expect((result as { kind: 'duplicate'; report: unknown }).report).toEqual(report);
  });
});

describe('error handling (401)', () => {
  it('surfaces a 401 (Basic Auth rejected) as an ApiError, not a crash or hang', async () => {
    installFakeXhr();
    const file = makeFile('x.pdf', 'application/pdf');
    const promise = uploadDocument(ALICE, file, () => {});
    const xhr = lastXhr();

    xhr.respond(401, JSON.stringify({ error: 'unauthorized' }));
    const err = (await promise.catch((e) => e)) as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(401);
    expect(err.message).toBe(errorCopy(401));
    expect(err.detail).toBe('unauthorized');
  });
});

describe('uploadStatement', () => {
  it('POSTs fields `file` and `account_id` and resolves the 201 batch', async () => {
    installFakeXhr();
    const file = makeFile('statement.csv', 'text/csv');
    const promise = uploadStatement(ALICE, ACCOUNT_ID, file, () => {});
    const xhr = lastXhr();

    expect(xhr.method).toBe('POST');
    expect(xhr.url).toBe(`${USER_BASE}/${ALICE}/finance/import-batches`);

    const form = xhr.sentForm;
    expect(form).not.toBeNull();
    expect(form?.get('file')).toBe(file);
    expect(form?.get('account_id')).toBe(ACCOUNT_ID);
    // The import-batch XHR path shares the same header merge as uploads.
    expect(xhr.requestHeaders['Authorization']).toBe(AUTH_HEADER);
    expect(xhr.requestHeaders['X-Tenant-ID']).toBeUndefined();
    expect('Content-Type' in xhr.requestHeaders).toBe(false);

    xhr.respond(201, JSON.stringify(BATCH_JSON));
    const batch = await promise;
    expect(batch).toEqual(BATCH_JSON);
  });
});
