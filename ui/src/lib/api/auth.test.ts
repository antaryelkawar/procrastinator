import { describe, it, expect, afterEach, vi } from 'vitest';
import { basicAuthHeader } from './auth';

const USER_VAR = 'VITE_API_BASIC_AUTH_USER';
const PASSWORD_VAR = 'VITE_API_BASIC_AUTH_PASSWORD';

/**
 * Independent UTF-8 → base64 reference. Node's `Buffer` encodes the string as
 * UTF-8 and base64s the resulting bytes — the exact bytes the Go backend
 * base64-encodes for `Authorization: Basic`. Deliberately a different code path
 * than the implementation (which uses TextEncoder + btoa), so equality proves
 * the impl matches what the server would compute.
 */
function backendBase64(user: string, password: string): string {
  return Buffer.from(`${user}:${password}`, 'utf8').toString('base64');
}

afterEach(() => {
  vi.unstubAllEnvs();
});

describe('basicAuthHeader — Authorization: Basic <base64(user:pass)>', () => {
  it('returns the Authorization header when both vars are set', () => {
    vi.stubEnv(USER_VAR, 'app');
    vi.stubEnv(PASSWORD_VAR, 'secret');
    expect(basicAuthHeader()).toEqual({
      Authorization: `Basic ${backendBase64('app', 'secret')}`,
    });
  });

  it('matches the fixed known-answer vector for ASCII credentials', () => {
    vi.stubEnv(USER_VAR, 'app');
    vi.stubEnv(PASSWORD_VAR, 'secret');
    // base64 of the ASCII bytes of "app:secret", hard-coded and independent.
    expect(basicAuthHeader()).toEqual({ Authorization: 'Basic YXBwOnNlY3JldA==' });
  });

  it('throws naming the user var when only the user var is missing', () => {
    vi.stubEnv(PASSWORD_VAR, 'secret');
    expect(() => basicAuthHeader()).toThrowError(USER_VAR);
  });

  it('throws naming the password var when only the password var is missing', () => {
    vi.stubEnv(USER_VAR, 'app');
    expect(() => basicAuthHeader()).toThrowError(PASSWORD_VAR);
  });

  it('throws naming both vars when both are missing', () => {
    expect(() => basicAuthHeader()).toThrowError(USER_VAR);
    expect(() => basicAuthHeader()).toThrowError(PASSWORD_VAR);
  });

  it('treats a blank (empty-string) var as missing', () => {
    vi.stubEnv(USER_VAR, '');
    vi.stubEnv(PASSWORD_VAR, 'secret');
    expect(() => basicAuthHeader()).toThrowError(USER_VAR);
  });

  it('encodes a non-ASCII password to the same base64 the backend computes', () => {
    const user = 'app';
    // € (U+20AC) is > U+00FF, so plain btoa would throw InvalidCharacterError;
    // the UTF-8-safe path must still produce the correct, backend-matching value.
    const password = 's3cr€t';
    vi.stubEnv(USER_VAR, user);
    vi.stubEnv(PASSWORD_VAR, password);
    expect(basicAuthHeader()).toEqual({
      Authorization: `Basic ${backendBase64(user, password)}`,
    });
    // Fixed known-answer vector: base64 of the UTF-8 bytes of "app:s3cr€t".
    expect(basicAuthHeader()).toEqual({ Authorization: 'Basic YXBwOnMzY3Ligqx0' });
  });
});
