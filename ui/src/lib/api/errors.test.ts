import { describe, it, expect } from 'vitest';
import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from './errors';

describe('errorCopy — status → human-readable copy', () => {
  it('maps every specified status to a distinct non-empty message', () => {
    const expected: Record<number, string> = {
      400: 'That request was invalid. Check the details and try again.',
      404: 'We couldn’t find that. It may have been removed.',
      409: 'This clashes with the current state. Refresh and try again.',
      413: 'That file is too large to upload.',
      415: 'That file type isn’t supported.',
      422: 'We couldn’t read anything usable from that file.',
      502: 'The document processor hiccuped. Please try again.',
      0: 'You’re offline or the server can’t be reached. Check your connection and retry.',
    };
    for (const [status, copy] of Object.entries(expected)) {
      expect(errorCopy(Number(status))).toBe(copy);
    }
  });

  it('covers exactly the required set of statuses (no gaps)', () => {
    const required = [400, 404, 409, 413, 415, 422, 502, NETWORK_STATUS];
    for (const status of required) {
      expect(errorCopy(status)).not.toBe('Something went wrong. Please try again.');
    }
  });

  it('falls back to a generic message for unmapped statuses', () => {
    expect(errorCopy(500)).toBe('Something went wrong. Please try again.');
    expect(errorCopy(503)).toBe('Something went wrong. Please try again.');
  });

  it('NETWORK_STATUS is 0', () => {
    expect(NETWORK_STATUS).toBe(0);
  });
});

describe('ApiError', () => {
  it('carries status and message', () => {
    const err = new ApiError(413, 'That file is too large to upload.');
    expect(err.status).toBe(413);
    expect(err.message).toBe('That file is too large to upload.');
    expect(err).toBeInstanceOf(Error);
  });

  it('isNetwork() is true only for status 0', () => {
    expect(new ApiError(0, 'x').isNetwork()).toBe(true);
    expect(new ApiError(404, 'x').isNetwork()).toBe(false);
  });

  it('preserves the backend detail when provided', () => {
    const err = new ApiError(409, errorCopy(409), 'conflicting state');
    expect(err.detail).toBe('conflicting state');
  });

  it('defaults detail to empty string', () => {
    expect(new ApiError(404, errorCopy(404)).detail).toBe('');
  });
});

describe('errorDetailFromBody — backend error envelope parsing', () => {
  it('extracts the error string from a valid envelope', () => {
    expect(errorDetailFromBody({ error: 'unsupported statement type' })).toBe(
      'unsupported statement type',
    );
  });

  it('returns empty string for a body without an error key', () => {
    expect(errorDetailFromBody({ message: 'oops' })).toBe('');
  });

  it('returns empty string for non-object bodies', () => {
    expect(errorDetailFromBody('unsupported statement type')).toBe('');
    expect(errorDetailFromBody(null)).toBe('');
    expect(errorDetailFromBody(undefined)).toBe('');
    expect(errorDetailFromBody(42)).toBe('');
  });

  it('returns empty string when error is not a string', () => {
    expect(errorDetailFromBody({ error: 500 })).toBe('');
    expect(errorDetailFromBody({ error: null })).toBe('');
  });
});
