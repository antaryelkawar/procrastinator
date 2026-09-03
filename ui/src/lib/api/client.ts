/**
 * API client — the ONLY module that knows the transport (design D1).
 *
 * Every URL in the app is built here. Screens never build paths; they call the
 * resource-level helpers (in `hooks.ts`) which call into this module. The two
 * tenancy transports are:
 *   - documents/assets → path tenancy: `/api/users/{userId}/…`
 *   - finance          → header tenancy: `/api/finance/…` + `X-Tenant-ID: {userId}`
 *
 * The backend API is being reworked in parallel, so the app is built against
 * mocked responses; when paths finalise, re-pointing is a change to THIS file
 * only (the `*_BASE` constants and the path helpers below).
 */
import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from './errors';

/** Base for path-tenanted document/asset routes. */
export const USER_BASE = '/api/users';
/** Base for header-tenanted finance routes. */
export const FINANCE_BASE = '/api/finance';
/** Header carrying the tenant id on finance routes. */
export const TENANT_HEADER = 'X-Tenant-ID';

/** How a request is scoped to the active user. */
export type RequestScope =
  | { kind: 'user'; userId: string }
  | { kind: 'finance'; userId: string };

/** A resource path, relative to its base (e.g. `assets` or `accounts/1`). */
export type ResourcePath = string;

function joinPath(base: string, rest: string): string {
  if (rest === '') {
    return base;
  }
  const trimmed = rest.startsWith('/') ? rest.slice(1) : rest;
  return `${base}/${trimmed}`;
}

/** Build the full path for a user-tenanted document/asset resource. */
export function userPath(userId: string, resource: ResourcePath): string {
  return joinPath(joinPath(USER_BASE, encodeURIComponent(userId)), resource);
}

/** Build the full path for a header-tenanted finance resource. */
export function financePath(resource: ResourcePath): string {
  return joinPath(FINANCE_BASE, resource);
}

/** Init options for a request routed through {@link apiFetch}. */
export interface ApiInit {
  readonly method?: string;
  /** JSON-serialisable body; serialised here (the single serialisation point). */
  readonly body?: unknown;
  readonly headers?: Record<string, string>;
}

/**
 * Low-level fetch wrapper. Resolves the final URL from the scope, injects the
 * `X-Tenant-ID` header on finance routes, serialises a JSON body, and returns
 * the raw `Response`. Non-2xx responses and network failures are translated
 * into an `ApiError` throw; callers then unwrap the body via
 * {@link apiJson}/{@link apiVoid}.
 */
export async function apiFetch(
  scope: RequestScope,
  resource: ResourcePath,
  init: ApiInit = {},
): Promise<Response> {
  const url = scope.kind === 'user' ? userPath(scope.userId, resource) : financePath(resource);

  const headers: Record<string, string> = { ...init.headers };
  if (scope.kind === 'finance') {
    headers[TENANT_HEADER] = scope.userId;
  }

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

/** Unwrap a JSON response body. 204/empty bodies resolve to `undefined`. */
export async function apiJson<T>(scope: RequestScope, resource: ResourcePath, init: ApiInit = {}): Promise<T> {
  const response = await apiFetch(scope, resource, init);
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

/** Perform a request whose success body is empty (e.g. 204 deletes). */
export async function apiVoid(scope: RequestScope, resource: ResourcePath, init: ApiInit = {}): Promise<void> {
  await apiFetch(scope, resource, init);
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
