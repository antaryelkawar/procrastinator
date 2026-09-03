import React, { useCallback, useState, useRef, useEffect } from 'react';
import { useDropzone } from 'react-dropzone';
import { useQueryClient } from '@tanstack/react-query';
import { Card } from '../../components/ui/card';
import { uploadDocument } from '../../lib/api/upload';
import { ApiError } from '../../lib/api/errors';
import { useActiveUser } from '../../context/active-user';

type UploadState = 'queued' | 'uploading' | 'success' | 'error';

interface FileUpload {
  id: string;
  file: File;
  state: UploadState;
  progress: number;
  error?: string;
  errorDetail?: string;
  /** `true` when the failure is retryable (502 or network) — gates the Retry button. */
  retryable?: boolean;
  assetId?: string;
  retryCount: number;
}

const MAX_CONCURRENCY = 3;

export const UploadPage: React.FC = () => {
  const [files, setFiles] = useState<FileUpload[]>([]);
  const queryClient = useQueryClient();
  const activeUploads = useRef(0);
  const { activeUser } = useActiveUser();
  // Keep a ref so the memoised processQueue always reads the latest user
  // without needing activeUser in its dependency array.
  const activeUserRef = useRef(activeUser);
  useEffect(() => {
    activeUserRef.current = activeUser;
  }, [activeUser]);

  const processQueue = useCallback(() => {
    if (activeUploads.current >= MAX_CONCURRENCY) return;

    setFiles((prev) => {
      const nextFiles = [...prev];
      let updated = false;

      for (const file of nextFiles) {
        if (file.state === 'queued' && activeUploads.current < MAX_CONCURRENCY) {
          activeUploads.current++;
          file.state = 'uploading';
          updated = true;
          
          // Trigger async upload without waiting for queue logic
          processFileUpload(file);
        }
      }
      return updated ? nextFiles : prev;
    });
  }, []);

  const processFileUpload = async (fileUpload: FileUpload) => {
    const user = activeUserRef.current;
    if (!user) {
      activeUploads.current--;
      setFiles((prev) => prev.map(f => f.id === fileUpload.id ? { ...f, state: 'error', error: 'No active user selected' } : f));
      processQueue();
      return;
    }

    try {
      const asset = await uploadDocument(user, fileUpload.file, ({ loaded, total }) => {
        if (total > 0) {
          setFiles((prev) => prev.map(f => f.id === fileUpload.id ? { ...f, progress: Math.round((loaded / total) * 100) } : f));
        }
      });
      
      activeUploads.current--;
      setFiles((prev) => prev.map(f => f.id === fileUpload.id ? { ...f, state: 'success', assetId: asset.id } : f));
      queryClient.invalidateQueries({ queryKey: ['assets'] });
      processQueue();
    } catch (error: unknown) {
      activeUploads.current--;
      
      const message = error instanceof ApiError ? error.message : 'Upload failed';
      const detail = error instanceof ApiError ? error.detail : '';
      // 502 and network failures (ApiError status 0) are the retryable cases.
      const retryable = error instanceof ApiError && (error.status === 502 || error.isNetwork());

      setFiles((prev) => prev.map(f => f.id === fileUpload.id ? { ...f, state: 'error', error: message, errorDetail: detail, retryable } : f));
      processQueue();
    }
  };

  const retryUpload = (id: string) => {
    setFiles((prev) => prev.map(f => f.id === id ? { ...f, state: 'queued', error: undefined, progress: 0, retryCount: f.retryCount + 1 } : f));
    processQueue();
  };

  const onDrop = useCallback((acceptedFiles: File[]) => {
    const newFiles: FileUpload[] = acceptedFiles.map((file) => ({
      id: Math.random().toString(36).substring(7),
      file,
      state: 'queued',
      progress: 0,
      retryCount: 0,
    }));
    setFiles((prev) => [...prev, ...newFiles]);
    processQueue();
  }, [processQueue]);

  const { getRootProps, getInputProps, isDragActive } = useDropzone({ 
    onDrop, 
    accept: { 'application/pdf': ['.pdf'], 'image/png': ['.png'], 'image/jpeg': ['.jpg', '.jpeg'] } 
  });

  return (
    <div className="p-4">
      <h1 className="text-2xl font-bold mb-4">Upload Documents</h1>
      <Card {...getRootProps()} className={`p-8 border-dashed border-2 cursor-pointer ${isDragActive ? 'border-primary' : 'border-muted'}`}>
        <input {...getInputProps()} aria-label="file upload" />
        <p className="text-center">Drag & drop files here, or click to select files</p>
      </Card>
      <div className="mt-4">
        {files.map((file) => (
          <div key={file.id} className="p-2 border rounded mb-2 flex justify-between items-center">
            <span>{file.file.name}</span>
            <span>{file.state} {file.progress}%</span>
            {file.error && (
              <div className="text-red-500">
                <div>{file.error}</div>
                {file.errorDetail && <div className="text-sm">Details: {file.errorDetail}</div>}
                {file.retryable && (
                  <button onClick={() => retryUpload(file.id)} className="ml-2 text-sm underline">Retry</button>
                )}
              </div>
            )}
            {file.assetId && <span>Asset ID: {file.assetId}</span>}
          </div>
        ))}
      </div>
    </div>
  );
};
