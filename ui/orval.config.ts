import { defineConfig } from 'orval';

/**
 * Orval configuration for the per-operation typed client.
 *
 * Design D4: fetch-mode per-operation client generated from the same
 * `openapi.yaml` the server uses, so each `operationId` maps to a typed
 * function whose request/response types match the documented schemas.
 *
 * The `fetch` client emits one function per `operationId` (no React Query
 * wrapper — the UI owns its query layer in `hooks.ts`). A custom `mutator`
 * wraps the generated fetch calls to reproduce the existing `apiFetch`
 * contract: non-2xx → `ApiError` with the envelope detail, network/timeout
 * failure → `ApiError(NETWORK_STATUS, ...)`.
 *
 * Multipart uploads (uploadDocument, createImportBatch) are NOT routed
 * through orval's fetch functions — the XHR helper in `upload.ts` preserves
 * progress reporting. Those uploads are still typed against the generated
 * multipart types and share the base URL.
 */
export default defineConfig({
  procrastinator: {
    input: {
      target: '../procrastinator-backend/api/openapi.yaml',
    },
    output: {
      target: 'src/lib/api/generated/orval/procrastinator.ts',
      client: 'fetch',
      mode: 'single',
      override: {
        mutator: {
          name: 'customFetch',
          path: 'src/lib/api/generated/mutator.ts',
        },
      },
    },
  },
});
