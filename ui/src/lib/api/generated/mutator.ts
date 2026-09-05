/**
 * Orval custom mutator that wraps fetch to reproduce the existing apiFetch
 * contract: non-2xx → ApiError with status + detail, network/timeout → ApiError(0).
 *
 * The generated orval client calls this for every operation (except the XHR
 * uploads, which use the dedicated upload.ts helper for progress reporting).
 */
import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from '../errors';

/**
 * Custom fetch mutator for orval. Receives the URL and RequestInit from the
 * generated per-operation function, performs the fetch, and returns the
 * orval-shaped response `{ data, status, headers }`. On non-2xx responses or
 * network failures, throws `ApiError` to match the previous apiFetch contract.
 */
export const customFetch = async <T>(url: string, init: RequestInit): Promise<T> => {
  let response: Response;
  try {
    response = await fetch(url, init);
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
