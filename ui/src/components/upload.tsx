/**
 * Shared upload/dropzone wrapper (react-dropzone) — THE single upload
 * primitive for the app (consolidated-primitives decision). Any screen that
 * accepts dropped or picked files composes this component instead of wiring
 * `useDropzone` directly.
 *
 * a11y: the dropzone root is a focusable, labelled region (aria-label +
 * aria-description). It is intentionally NOT given `role="button"`: the
 * react-dropzone file input is nested inside the root, and axe
 * `nested-interactive` forbids an interactive control (role="button") from
 * containing a focusable input. Enter/Space on the focused root open the file
 * dialog, so the keyboard path is preserved without a button role.
 */
import { type ReactNode, type KeyboardEvent } from 'react';
import { useDropzone, type Accept } from 'react-dropzone';

import { cn } from '@/lib/utils';

export type UploadAccept = Accept;

export interface UploadProps {
  /** Invoked with the accepted files when a drop or pick happens. */
  onDrop: (files: File[]) => void;
  /** Accepted file types, react-dropzone `Accept` shape. */
  accept?: UploadAccept;
  /** Allow multiple files (default `true`). */
  multiple?: boolean;
  /** Maximum accepted file size in bytes. */
  maxSize?: number;
  /** Accessible label for the dropzone + hidden file input (default 'file upload'). */
  label?: string;
  /** Secondary helper text rendered below the label. */
  hint?: string;
  /** Extra classes merged onto the dropzone root. */
  className?: string;
  /** Custom content rendered inside the dropzone instead of label/hint. */
  children?: ReactNode;
}

export function Upload({
  onDrop,
  accept,
  multiple = true,
  maxSize,
  label = 'file upload',
  hint,
  className,
  children,
}: UploadProps) {
  const { getRootProps, getInputProps, isDragActive, open } = useDropzone({
    // react-dropzone's onDrop receives (accepted, rejections, event); the public
    // `onDrop` prop only exposes the accepted files.
    onDrop: (accepted) => onDrop(accepted),
    accept,
    multiple,
    maxSize,
  });

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      open();
    }
  };

  return (
    <div
      {...getRootProps({
        tabIndex: 0,
        'aria-label': label,
        'aria-description': hint,
        className: cn(
          'cursor-pointer rounded-lg border-2 border-dashed p-8 text-center outline-none transition-colors focus-visible:ring-3 focus-visible:ring-ring',
          isDragActive ? 'border-primary bg-primary/5' : 'border-input',
          className,
        ),
        onKeyDown: handleKeyDown,
      })}
    >
      <input {...getInputProps({ tabIndex: -1, 'aria-label': label, className: 'sr-only' })} />
      {children ?? (
        <>
          <p className="text-sm font-medium">{label}</p>
          {hint ? <p className="mt-1 text-sm text-muted-foreground">{hint}</p> : null}
        </>
      )}
    </div>
  );
}
