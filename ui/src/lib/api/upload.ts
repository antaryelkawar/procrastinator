import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from './errors';
import { getUploadDocumentUrl, getCreateImportBatchUrl } from './generated/orval/procrastinator';
import type { Asset, ImportBatch, IngestReview } from './generated/orval/procrastinator';

async function performUpload<T>(
  url: string,
  headers: Record<string, string>,
  formData: FormData,
  onProgress: (event: { loaded: number; total: number }) => void,
  mapResponse?: (status: number, body: unknown) => T | null
): Promise<T> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url);

    Object.entries(headers).forEach(([key, value]) => {
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
  | { kind: 'held'; review: IngestReview };

export async function uploadDocument(
  userId: string,
  file: File,
  onProgress: (event: { loaded: number; total: number }) => void
): Promise<UploadDocumentResult> {
  const url = getUploadDocumentUrl(userId);
  const formData = new FormData();
  formData.append('file', file);
  return performUpload<UploadDocumentResult>(url, {}, formData, onProgress, (status, body) => {
    if (status === 201) {
      return { kind: 'committed', asset: body as Asset };
    }
    if (status === 202) {
      return { kind: 'held', review: body as IngestReview };
    }
    return null;
  });
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
