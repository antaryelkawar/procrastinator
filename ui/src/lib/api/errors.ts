/**
 * API error types and status → human-readable copy mapping (design D1/D5).
 *
 * The backend returns `{"error":"<message>"}` for every failure. `ApiError`
 * captures the HTTP status and that verbatim message; `errorCopy` provides the
 * status-based human copy. `status 0` denotes a network-level failure (no
 * response was received).
 */

/**
 * A failure surfaced by the API client. `status 0` means the request never got
 * a response (network failure); any other value is the HTTP status code.
 */
export class ApiError extends Error {
  readonly status: number;
  /** Verbatim message from the backend `{"error":"…"}` envelope, if any. */
  readonly detail: string;

  constructor(status: number, message: string, detail = '') {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.detail = detail;
  }

  /** `true` when no HTTP response was received (status 0). */
  isNetwork(): boolean {
    return this.status === 0;
  }
}

/** Network failures are represented as status `0`. */
export const NETWORK_STATUS = 0;

/**
 * Status → human-readable copy. Covers the statuses the backend emits for the
 * user-facing flows plus the network-failure pseudo-status.
 */
export function errorCopy(status: number): string {
  switch (status) {
    case 400:
      return 'That request was invalid. Check the details and try again.';
    case 404:
      return 'We couldn’t find that. It may have been removed.';
    case 409:
      return 'This clashes with the current state. Refresh and try again.';
    case 413:
      return 'That file is too large to upload.';
    case 415:
      return 'That file type isn’t supported.';
    case 422:
      return 'We couldn’t read anything usable from that file.';
    case 502:
      return 'The document processor hiccuped. Please try again.';
    case NETWORK_STATUS:
      return 'You’re offline or the server can’t be reached. Check your connection and retry.';
    default:
      return 'Something went wrong. Please try again.';
  }
}

/**
 * Extract the verbatim error message from a backend error body
 * (`{"error":"…"}`); returns `''` when the body is absent or malformed.
 */
export function errorDetailFromBody(body: unknown): string {
  if (typeof body !== 'object' || body === null) {
    return '';
  }
  const error = (body as { error?: unknown }).error;
  return typeof error === 'string' ? error : '';
}
