import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from './errors';
import { getUploadDocumentUrl, getCreateImportBatchUrl } from './generated/orval/procrastinator';
import type { Asset, ImportBatch } from './generated/orval/procrastinator';

async function performUpload<T>(
  url: string,
  headers: Record<string, string>,
  formData: FormData,
  onProgress: (event: { loaded: number; total: number }) => void
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
        try {
          resolve(JSON.parse(xhr.responseText) as T);
        } catch {
          resolve(xhr.responseText as unknown as T);
        }
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

export async function uploadDocument(
  userId: string,
  file: File,
  onProgress: (event: { loaded: number; total: number }) => void
): Promise<Asset> {
  const url = getUploadDocumentUrl(userId);
  const formData = new FormData();
  formData.append('file', file);
  return performUpload<Asset>(url, {}, formData, onProgress);
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
