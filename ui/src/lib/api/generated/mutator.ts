/**
 * Orval custom mutator that wraps fetch to reproduce the existing apiFetch
 * contract: non-2xx → ApiError with status + detail, network/timeout → ApiError(0).
 *
 * The generated orval client calls this for every operation (except the XHR
 * uploads, which use the dedicated upload.ts helper for progress reporting).
 *
 * It is also the single centralized injection point for the shared Basic Auth
 * credential (design D5): every outgoing request carries the `Authorization`
 * header built by auth.ts. The upload helper (upload.ts) is the other consumer
 * of the same helper.
 */
import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from '../errors';
import { basicAuthHeader } from '../auth';

/**
 * Merge `extra` into `init.headers` — augment, never replace. Headers already
 * on the request (Content-Type set by the generated client, caller-supplied
 * options) survive and win on conflict (same precedence as upload.ts). The
 * three RequestInit header shapes (plain object / Headers / tuple array) are
 * normalized into a plain record so no entry is dropped; key casing is
 * preserved for the object shape, which is what the generated client emits.
 */
function mergeHeaders(base: RequestInit['headers'], extra: Record<string, string>): Record<string, string> {
  const existing: Record<string, string> =
    base instanceof Headers
      ? Object.fromEntries(base.entries())
      : Array.isArray(base)
        ? Object.fromEntries(base)
        : { ...base };
  return { ...extra, ...existing };
}

/**
 * Custom fetch mutator for orval. Receives the URL and RequestInit from the
 * generated per-operation function, performs the fetch, and returns the
 * orval-shaped response `{ data, status, headers }`. On non-2xx responses or
 * network failures, throws `ApiError` to match the previous apiFetch contract.
 */
export const customFetch = async <T>(url: string, init: RequestInit): Promise<T> => {
  // Centralized Basic Auth injection for the fetch paths (design D5): merge the
  // shared `Authorization` header into every request, call-site headers
  // overriding on conflict (none set today). Read per request rather than at
  // module scope so this stays mockable in tests (`vi.mock` factories hoist
  // above env stubbing) — the fail-fast guarantee is unaffected:
  // `basicAuthHeader()` throws naming the missing var(s) *before* fetch is
  // called, so no request is ever sent with empty credentials (spec scenario
  // "Frontend credentials are absent").
  const authedInit: RequestInit = { ...init, headers: mergeHeaders(init.headers, basicAuthHeader()) };

  let response: Response;
  try {
    response = await fetch(url, authedInit);
  } catch {
    // Network failure (no response received) → ApiError with status 0
    throw new ApiError(NETWORK_STATUS, errorCopy(NETWORK_STATUS));
  }

  // Parse the response body. For 204 No Content, data is undefined.
  const status = response.status;
  const headers = response.headers;

  let data: unknown;
  let rawText: string | null = null;
  if (status === 204 || status === 205) {
    data = undefined;
  } else {
    const text = await response.text();
    rawText = text;
    try {
      data = text ? JSON.parse(text) : undefined;
    } catch {
      // If JSON parsing fails, use the raw text (shouldn't happen for our API)
      data = text;
    }
  }

  if (!response.ok) {
    // Non-2xx response → extract error detail and throw ApiError
    let detail: string;
    if (typeof data === 'object' && data !== null) {
      detail = errorDetailFromBody(data);
    } else if (typeof rawText === 'string' && rawText) {
      // Fallback to raw text when body isn't a JSON object
      detail = rawText;
    } else {
      detail = '';
    }
    throw new ApiError(status, errorCopy(status), detail);
  }

  // Success: return orval-shaped response
  return { data, status, headers } as T;
};
