/**
 * Centralized Basic Auth header for every API request (design D5).
 *
 * Credentials come from two Vite-inlined build-time env vars —
 * `VITE_API_BASIC_AUTH_USER` and `VITE_API_BASIC_AUTH_PASSWORD`. Only
 * `VITE_`-prefixed vars are exposed to the browser, so these are the only names
 * Vite inlines into the bundle. They are an application-level shared transport
 * secret (not per-user credentials); the accepted exposure model — anyone who
 * can load the bundle can read them — is deliberate, per the api-basic-auth spec.
 *
 * `basicAuthHeader()` is the single source of the `Authorization` value. Its
 * two consumers (the fetch mutator and the multipart XHR upload helper) merge
 * its returned record into their outgoing headers rather than re-deriving the
 * value, so header construction lives in exactly one place.
 *
 * Fail-fast: when either var is unset or empty the function throws naming the
 * missing var(s). Consumers invoke it at module load (startup), so a
 * provisioning gap surfaces immediately instead of as a stream of 401s sent
 * with empty credentials.
 */

const USER_VAR = 'VITE_API_BASIC_AUTH_USER';
const PASSWORD_VAR = 'VITE_API_BASIC_AUTH_PASSWORD';

/**
 * UTF-8-safe base64. Plain `btoa` is Latin-1 only and throws
 * `InvalidCharacterError` on any code point > U+00FF, yet non-ASCII credentials
 * are a supported class per the backend grammar. We UTF-8-encode the input
 * first and hand `btoa` one 0x00–0xFF char per byte, so the result is
 * byte-identical to the base64 the backend computes over the raw `user:pass`
 * UTF-8 bytes.
 */
function utf8ToBase64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  return btoa(String.fromCharCode(...bytes));
}

/** `true` when an env var value is absent (undefined/null) or blank. */
function isMissing(value: unknown): boolean {
  return value === undefined || value === null || value === '';
}

/**
 * Build the `Authorization: Basic <base64(user:pass)>` header record.
 *
 * @throws {Error} when either credential var is unset or empty, naming the
 *   missing variable(s) so the provisioning gap is obvious at startup.
 */
export function basicAuthHeader(): Record<string, string> {
  const user: string | undefined = import.meta.env[USER_VAR];
  const password: string | undefined = import.meta.env[PASSWORD_VAR];

  const missing: string[] = [];
  if (isMissing(user)) missing.push(USER_VAR);
  if (isMissing(password)) missing.push(PASSWORD_VAR);
  if (missing.length > 0) {
    throw new Error(
      `Missing required Basic Auth env var${missing.length > 1 ? 's' : ''}: ` +
        `${missing.join(', ')}. Set them in the frontend build environment ` +
        '(see ui/.env.example).',
    );
  }

  return { Authorization: `Basic ${utf8ToBase64(`${user}:${password}`)}` };
}
