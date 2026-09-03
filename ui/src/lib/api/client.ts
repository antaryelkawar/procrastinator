import type { paths } from './generated/paths';
import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from './errors';
import { API_BASE, USER_PATH_PREFIX } from './config';

/** HTTP verbs the thin fetch client supports. */
export type ApiMethod = 'GET' | 'POST' | 'PATCH' | 'DELETE';

/**
 * Tenant-scoped resources addressable through this client, relative to
 * `/api/users/{userId}` (path tenancy — every route in the generated contract
 * lives under that prefix). `documents` upload is `multipart/form-data` and is
 * addressed by `upload.ts` instead. Each pattern maps 1:1 to a generated
 * `paths` entry (see `PathFor`).
 */
export type Resource =
  | 'assets'
  | `assets/${string}`
  | `assets/${string}/documents`
  | 'finance/accounts'
  | `finance/accounts/${string}`
  | 'finance/movements'
  | `finance/movements?${string}`
  | `finance/movements/${string}`
  | `finance/movements/${string}/link`
  | 'finance/import-batches'
  | `finance/import-batches/${string}`
  | `finance/import-batches/${string}/commit`
  | `finance/import-batches/${string}/discard`
  | 'households'
  | `households/${string}`
  | `households/${string}/members`;

/**
 * Maps a resource pattern to its generated `paths` entry. Most-specific
 * patterns first (e.g. `assets/{id}/documents` before `assets/{id}`).
 */
type PathFor<R extends Resource> =
  R extends `assets/${string}/documents`
    ? paths['/api/users/{userId}/assets/{assetId}/documents']
    : R extends `assets/${string}`
      ? paths['/api/users/{userId}/assets/{assetId}']
      : R extends `finance/accounts/${string}`
        ? paths['/api/users/{userId}/finance/accounts/{id}']
        : R extends `finance/movements?${string}`
          ? paths['/api/users/{userId}/finance/movements']
          : R extends `finance/movements/${string}/link`
            ? paths['/api/users/{userId}/finance/movements/{id}/link']
            : R extends `finance/movements/${string}`
              ? paths['/api/users/{userId}/finance/movements/{id}']
              : R extends `finance/import-batches/${string}/commit`
                ? paths['/api/users/{userId}/finance/import-batches/{id}/commit']
                : R extends `finance/import-batches/${string}/discard`
                  ? paths['/api/users/{userId}/finance/import-batches/{id}/discard']
                  : R extends `finance/import-batches/${string}`
                    ? paths['/api/users/{userId}/finance/import-batches/{id}']
                    : R extends `households/${string}/members`
                      ? paths['/api/users/{userId}/households/{householdId}/members']
                      : R extends `households/${string}`
                        ? paths['/api/users/{userId}/households/{householdId}']
                        : R extends 'assets'
                          ? paths['/api/users/{userId}/assets']
                          : R extends 'finance/accounts'
                            ? paths['/api/users/{userId}/finance/accounts']
                            : R extends 'finance/movements'
                              ? paths['/api/users/{userId}/finance/movements']
                              : R extends 'finance/import-batches'
                                ? paths['/api/users/{userId}/finance/import-batches']
                                : R extends 'households'
                                  ? paths['/api/users/{userId}/households']
                                  : never;

type MethodKey<M extends ApiMethod> = M extends 'GET'
  ? 'get'
  : M extends 'POST'
    ? 'post'
    : M extends 'PATCH'
      ? 'patch'
      : 'delete';

/** The operation (method entry) of a path item, or `never` when that method is not documented on the path. */
type OperationFor<R extends Resource, M extends ApiMethod> = PathFor<R> extends Record<MethodKey<M>, unknown>
  ? PathFor<R>[MethodKey<M>]
  : never;

/** The `application/json` request-body schema of an operation; `never` when the operation takes no JSON body. */
type JsonBodyOf<Op> = Op extends { requestBody: infer RB }
  ? RB extends { content: { 'application/json': infer B } }
    ? B
    : never
  : never;

/**
 * The success JSON body of an operation: the 200 schema when documented, else
 * the 201 schema (create endpoints), else `undefined` (204 No Content ops).
 */
type JsonResponseOf<Op> = Op extends { responses: { 200: { content: { 'application/json': infer B } } } }
  ? B
  : Op extends { responses: { 201: { content: { 'application/json': infer B } } } }
    ? B
    : undefined;

/** Non-inferable body slot: keeps `B` from being inferred back out of the caller's literal. */
type BodySlot<B> = [B] extends [never] ? undefined : B;

/**
 * Typed init for a documented endpoint: the method is constrained to `M` (the
 * operation actually used), and the body is checked against the operation's
 * generated `application/json` request schema (no `any`, no untyped bodies).
 * Callers that must send a body the OpenAPI `required` list over-specifies
 * (kind-conditional movement endpoints) pass the derived input type as `B`.
 */
export interface ApiInit<
  R extends Resource = Resource,
  M extends ApiMethod = 'GET',
  B = JsonBodyOf<OperationFor<R, M>>,
> {
  readonly method?: M;
  readonly body?: BodySlot<B>;
  readonly headers?: Record<string, string>;
}

/**
 * Build the full URL. ALL routes are /api/users/{userId}/{resource} (path
 * tenancy). Resource examples: 'assets', 'assets/a1', 'finance/accounts',
 * 'finance/movements?from=...'
 */
function buildUrl(userId: string, resource: string): string {
  const base = `${API_BASE}/${USER_PATH_PREFIX.replace(/^\//, '')}/${encodeURIComponent(userId)}`;
  const trimmed = resource.replace(/^\/+/, '');
  return trimmed ? `${base}/${trimmed}` : base;
}

export async function apiFetch<
  R extends Resource,
  M extends ApiMethod = 'GET',
  B = JsonBodyOf<OperationFor<R, M>>,
>(userId: string, resource: R, init: ApiInit<R, M, B> = {}): Promise<Response> {
  const url = buildUrl(userId, resource);
  const headers: Record<string, string> = { ...init.headers };
  let body: string | undefined;
  if (init.body !== undefined) {
    body = JSON.stringify(init.body);
    headers['Content-Type'] = 'application/json';
  }
  let response: Response;
  try {
    response = await fetch(url, { method: init.method ?? 'GET', headers, body });
  } catch {
    throw new ApiError(NETWORK_STATUS, errorCopy(NETWORK_STATUS));
  }
  if (!response.ok) {
    const detail = await extractErrorDetail(response);
    throw new ApiError(response.status, errorCopy(response.status), detail);
  }
  return response;
}

/**
 * Perform a documented endpoint and resolve its generated success body
 * (200 schema, else 201 schema, else `undefined` for 204 ops).
 */
export async function apiJson<
  R extends Resource,
  M extends ApiMethod = 'GET',
  B = JsonBodyOf<OperationFor<R, M>>,
>(
  userId: string,
  resource: R,
  init: ApiInit<R, M, B> = {},
): Promise<JsonResponseOf<OperationFor<R, M>>> {
  const response = await apiFetch<R, M, B>(userId, resource, init);
  const body: unknown = response.status === 204 ? undefined : await response.json();
  return body as JsonResponseOf<OperationFor<R, M>>;
}

/** Perform a documented endpoint whose success is 204 No Content (or whose body the caller ignores). */
export async function apiVoid<
  R extends Resource,
  M extends ApiMethod = 'GET',
  B = JsonBodyOf<OperationFor<R, M>>,
>(userId: string, resource: R, init: ApiInit<R, M, B> = {}): Promise<void> {
  await apiFetch<R, M, B>(userId, resource, init);
}

async function extractErrorDetail(response: Response): Promise<string> {
  let raw: string;
  try {
    raw = await response.text();
  } catch {
    return '';
  }
  if (raw === '') {
    return '';
  }
  try {
    return errorDetailFromBody(JSON.parse(raw));
  } catch {
    return raw;
  }
}
