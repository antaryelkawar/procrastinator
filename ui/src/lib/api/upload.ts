import { userPath, financePath, TENANT_HEADER } from './client';
import { ApiError, errorCopy, errorDetailFromBody, NETWORK_STATUS } from './errors';

/**
 * Perform a multipart upload using XHR to support progress tracking.
 */
async function performUpload(
  url: string,
  headers: Record<string, string>,
  formData: FormData,
  onProgress: (event: { loaded: number; total: number }) => void
): Promise<any> {
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
          resolve(JSON.parse(xhr.responseText));
        } catch {
          resolve(xhr.responseText);
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
        reject(new ApiError(xhr.status, errorCopy(xhr.status), detail));
      }
    };

    xhr.onerror = () => {
      reject(new ApiError(NETWORK_STATUS, errorCopy(NETWORK_STATUS)));
    };

    xhr.send(formData);
  });
}

export async function uploadDocument(
  userId: string,
  file: File,
  onProgress: (event: { loaded: number; total: number }) => void
): Promise<any> {
  const url = userPath(userId, 'documents');
  const formData = new FormData();
  formData.append('file', file);
  return performUpload(url, {}, formData, onProgress);
}

export async function uploadStatement(
  userId: string,
  accountId: string,
  file: File,
  onProgress: (event: { loaded: number; total: number }) => void
): Promise<any> {
  const url = financePath('import-batches');
  const headers = { [TENANT_HEADER]: userId };
  const formData = new FormData();
  formData.append('file', file);
  formData.append('account_id', accountId);
  return performUpload(url, headers, formData, onProgress);
}
