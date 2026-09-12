/**
 * Document / statement upload over raw XHR (so progress can be reported).
 *
 * `uploadDocument` resolves a discriminated union covering the three outcomes
 * of the document-upload pipeline: 201 → committed asset, 202 → held for
 * review, and 409 → a structured `DuplicateReport` (reprocess/keep prompt).
 * The 409 is a *structured, expected* outcome rather than a hard failure, so
 * `performUpload` resolves it (with the parsed report) instead of rejecting.
 */
import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from './errors';
import { basicAuthHeader } from './auth';
import { getUploadDocumentUrl, getCreateImportBatchUrl } from './generated/orval/procrastinator';
import type { Asset, DuplicateReport, ImportBatch, IngestReview } from './generated/orval/procrastinator';

/** A single XHR response mapped into a result, or `null` to fall through. */
type MapResponse<T> = (status: number, body: unknown) => T | null;

async function performUpload<T>(
  url: string,
  headers: Record<string, string>,
  formData: FormData,
  onProgress: (event: { loaded: number; total: number }) => void,
  mapResponse?: MapResponse<T>,
  /**
   * When provided, a 409 response is resolved with the value returned by this
   * callback (instead of being rejected). Used to surface the structured
   * `DuplicateReport` as an expected outcome.
   */
  onDuplicate?: (report: DuplicateReport) => T
): Promise<T> {
  // Centralized Basic Auth injection for the XHR paths (design D5): merge the
  // shared `Authorization` header into every upload request, call-site headers
  // overriding on conflict (none set today). Read per request rather than at
  // module scope so this stays mockable in tests (`vi.mock` factories hoist
  // above env stubbing) — the fail-fast guarantee is unaffected:
  // `basicAuthHeader()` throws naming the missing var(s) *before* the XHR is
  // created, so no upload is ever sent with empty credentials (spec scenario
  // "Frontend credentials are absent").
  const merged: Record<string, string> = { ...basicAuthHeader(), ...headers };
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url);

    Object.entries(merged).forEach(([key, value]) => {
      xhr.setRequestHeader(key, value);
    });

    xhr.upload.onprogress = (event) => {
      onProgress({ loaded: event.loaded, total: event.total });
    };

    xhr.onload = async () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        let body: unknown;
        try {
          body = JSON.parse(xhr.responseText);
        } catch {
          body = xhr.responseText;
        }

        if (mapResponse) {
          const mapped = mapResponse(xhr.status, body);
          if (mapped !== null) {
            resolve(mapped);
            return;
          }
        }

        resolve(body as T);
      } else if (xhr.status === 409 && onDuplicate) {
        // Duplicate: a structured, expected outcome. Parse the report and
        // resolve it so the caller can render the reprocess/keep prompt.
        let report: DuplicateReport;
        try {
          report = JSON.parse(xhr.responseText) as DuplicateReport;
        } catch {
          console.error(`Upload duplicate (409) had an unparseable body: ${xhr.responseText}`);
          reject(new ApiError(409, errorCopy(409), 'duplicate'));
          return;
        }
        resolve(onDuplicate(report));
      } else {
        let detail = '';
        try {
          if (xhr.responseText) {
            const body = JSON.parse(xhr.responseText);
            detail = errorDetailFromBody(body);
          }
        } catch {
          detail = xhr.responseText;
        }
        console.error(`Upload failed: status=${xhr.status}, response=${xhr.responseText}`);
        reject(new ApiError(xhr.status, errorCopy(xhr.status), detail));
      }
    };

    xhr.onerror = () => {
      console.error('Upload network error');
      reject(new ApiError(NETWORK_STATUS, errorCopy(NETWORK_STATUS)));
    };

    xhr.send(formData);
  });
}

export type UploadDocumentResult =
  | { kind: 'committed'; asset: Asset }
  | { kind: 'held'; review: IngestReview }
  | { kind: 'duplicate'; report: DuplicateReport };

/**
 * Upload a document (multipart `file` + optional `note`). Resolves with a
 * discriminated union:
 *   - 201 → `{ kind: 'committed', asset }`
 *   - 202 → `{ kind: 'held', review }`
 *   - 409 → `{ kind: 'duplicate', report }` (the structured DuplicateReport)
 * Any other non-2xx status rejects with `ApiError`.
 *
 * `note` is an optional free-text user directive; it is appended to the
 * multipart body as the `note` field when non-empty.
 */
export async function uploadDocument(
  userId: string,
  file: File,
  onProgress: (event: { loaded: number; total: number }) => void,
  note?: string
): Promise<UploadDocumentResult> {
  const url = getUploadDocumentUrl(userId);
  const formData = new FormData();
  formData.append('file', file);
  if (note !== undefined && note !== null && note.trim() !== '') {
    formData.append('note', note);
  }

  return performUpload<UploadDocumentResult>(
    url,
    {},
    formData,
    onProgress,
    (status, body) => {
      if (status === 201) {
        return { kind: 'committed', asset: body as Asset };
      }
      if (status === 202) {
        return { kind: 'held', review: body as IngestReview };
      }
      return null;
    },
    (report) => ({ kind: 'duplicate', report })
  );
}

export async function uploadStatement(
  userId: string,
  accountId: string,
  file: File,
  onProgress: (event: { loaded: number; total: number }) => void
): Promise<ImportBatch> {
  const url = getCreateImportBatchUrl(userId);
  const formData = new FormData();
  formData.append('file', file);
  formData.append('account_id', accountId);
  return performUpload<ImportBatch>(url, {}, formData, onProgress);
}
