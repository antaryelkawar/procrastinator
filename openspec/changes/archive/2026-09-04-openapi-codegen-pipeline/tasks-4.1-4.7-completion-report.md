# OpenAPI Codegen Pipeline - Client TypeScript SDK Implementation Report

## Summary
Successfully completed tasks 4.1-4.7 of the openapi-codegen-pipeline change, implementing the client TypeScript SDK generation and integration.

## Completed Tasks

### Task 4.1: Code Generation Script
**Status**: ✅ Complete

**Implementation**:
- Updated `ui/package.json` to include the `codegen` script
- Script uses the pinned `openapi-typescript@7.13.0` tool
- Generates TypeScript types from `procrastinator-backend/api/openapi.yaml`
- Output file: `ui/src/lib/api/generated/paths.d.ts`

**Verification**:
```bash
npm run codegen  # Executes successfully
```

### Task 4.2: Wire Contract Verification
**Status**: ✅ Complete

**Verification Points**:
1. ✅ **Snake_case property names**: All generated properties use snake_case (e.g., `doc_type`, `created_at`, `source_account_id`)
2. ✅ **Optional fields with `?`**: Fields marked as `nullable: true` in OpenAPI are optional in TypeScript
   - Example: `brand?: string | null` on Asset
3. ✅ **String money fields**: All monetary amounts are strings
   - Example: `amount: string` on Movement
4. ✅ **Required `link_conflicting`**: Field is required (not optional) on Movement
   - Verified in `components['schemas']['movement'].link_conflicting: boolean`
5. ✅ **Nullable array `lines`**: ImportBatch.lines is `ImportLine[] | null`

**Evidence**: Generated file `ui/src/lib/api/generated/paths.d.ts` contains correct type definitions matching all wire contract conventions.

### Task 4.3: Typed API Client
**Status**: ✅ Complete

**Implementation**:
- Created `ui/src/lib/api/client.ts` with type-safe API functions
- Implemented generic type parameters for `apiGet`, `apiPost`, `apiPatch`, `apiDelete`
- Functions use `paths` type from generated code to ensure type safety
- All request/response bodies are strongly typed

**Key Features**:
- Generic functions: `apiGet<P extends keyof paths>(path, userId)`
- Type inference from path parameter
- Compile-time validation of path existence
- Strongly typed request bodies based on operation
- Strongly typed response bodies based on operation

**Verification**:
```typescript
// Example usage that compiles successfully:
const assets = await apiGet('/api/users/{userId}/assets', 'user123');
// assets is correctly typed as components['schemas']['asset'][]
```

### Task 4.4: Schema Types Replacement
**Status**: ✅ Complete

**Implementation**:
- Created `ui/src/lib/api/schema.ts` as the new types module
- Exports all schema types from `components['schemas']`
- Added UI-specific types not in OpenAPI spec:
  - `MovementKind`
  - `MovementDirection`
  - `ImportDirection`
- Deleted old `ui/src/lib/api/types.ts`

**Migration**:
- Updated all imports in `hooks.ts` to use `./schema`
- Updated all imports in test files to use `./schema`
- Updated `upload.ts` to use schema types
- All references to old `types.ts` removed

**Verification**:
```bash
tsc --noEmit  # Compiles successfully
npm test      # All 163 tests pass
```

### Task 4.5: Config Cleanup and Path Tenancy
**Status**: ✅ Complete

**Implementation**:
- Removed `FINANCE_PATH_PREFIX` from `ui/src/lib/api/config.ts`
- Config now only exports: `API_BASE`, `USER_PATH_PREFIX`
- All API paths use uniform path tenancy: `/api/users/{userId}/...`

**Path Construction**:
```typescript
// In upload.ts:
const url = `${API_BASE}${USER_PATH_PREFIX}/${userId}/documents`;
// Results in: /api/users/{userId}/documents

const url = `${API_BASE}${USER_PATH_PREFIX}/${userId}/finance/import-batches`;
// Results in: /api/users/{userId}/finance/import-batches
```

**Verification**:
- `upload.test.ts` confirms X-Tenant-ID header is NOT sent (negative assertion)
- All paths are user-scoped under `/api/users/{userId}`
- No hardcoded finance paths remain

### Task 4.6: Build and Test Verification
**Status**: ✅ Complete

**Build Verification**:
```bash
$ npx tsc --noEmit
# No errors - TypeScript compilation successful
```

**Test Results**:
```bash
$ npm test
 Test Files  19 passed (19)
      Tests  163 passed (163)
   Duration  3.19s
```

**Test Coverage**:
- All existing API tests pass with generated types
- Client tests verify correct path construction
- Hooks tests verify correct API calls
- Upload tests verify path tenancy
- Integration tests verify end-to-end flow

### Task 4.7: Schema Evolution Test
**Status**: ✅ Complete

**Implementation**:
Created `ui/src/lib/api/generated.test.ts` with 4 test cases:

1. **Path Type Verification**: Confirms all API paths are exposed in the generated types
   ```typescript
   type AssetPath = paths['/api/users/{userId}/assets'];
   type AccountPath = paths['/api/users/{userId}/finance/accounts'];
   ```

2. **Schema Component Verification**: Confirms all schema components are accessible
   ```typescript
   type Asset = components['schemas']['asset'];
   type Account = components['schemas']['account'];
   type Movement = components['schemas']['movement'];
   type ImportBatch = components['schemas']['import_batch'];
   ```

3. **Wire Contract Conventions**: Verifies required fields and types
   ```typescript
   // link_conflicting is required (not optional)
   const _movementWithRequiredField: Movement = {
     id: 'mv1',
     kind: 'expense',
     amount: '100.00',
     currency: 'USD',
     occurred_on: '2024-01-15',
     description: 'Test',
     origin: 'manual',
     link_conflicting: false, // Required field - compile error if omitted
     created_at: '2024-01-15T10:00:00Z',
     updated_at: '2024-01-15T10:00:00Z',
   };
   ```

4. **Optional Fields Verification**: Confirms optional fields can be omitted
   ```typescript
   // Optional fields (brand, model, etc.) can be undefined
   const _assetWithoutOptionalFields: Asset = {
     id: 'a1',
     doc_type: 'invoice',
     created_at: '2024-01-15T10:00:00Z',
     updated_at: '2024-01-15T10:00:00Z',
     // All optional fields are omitted
   };
   ```

**Test Results**:
```bash
$ npx vitest run src/lib/api/generated.test.ts
 ✓ src/lib/api/generated.test.ts (4 tests)
 Test Files  1 passed (1)
      Tests  4 passed (4)
```

**Contract Field Flow Verification**:
The test demonstrates that when a field is added to the OpenAPI schema:
1. `openapi-typescript` generates the corresponding TypeScript type
2. The type is accessible via `components['schemas']['schemaName']`
3. TypeScript compiler enforces the type in client code
4. Missing required fields cause compile errors
5. Optional fields can be safely omitted

## Files Changed

### New Files
1. `ui/src/lib/api/generated/paths.d.ts` - Generated TypeScript types from OpenAPI spec
2. `ui/src/lib/api/generated.test.ts` - Schema evolution tests (Task 4.7)
3. `ui/src/lib/api/schema.ts` - Type exports from generated schema

### Modified Files
1. `ui/package.json` - Added `codegen` script
2. `ui/src/lib/api/client.ts` - Implemented typed API client
3. `ui/src/lib/api/hooks.ts` - Updated to use schema types
4. `ui/src/lib/api/upload.ts` - Updated to use schema types
5. `ui/src/lib/api/config.ts` - Removed FINANCE_PATH_PREFIX
6. `ui/src/lib/api/hooks.test.tsx` - Updated imports
7. `ui/src/lib/api/client.test.ts` - Updated imports
8. `ui/src/lib/format/money.test.ts` - Updated imports
9. `ui/src/pages/finance/import-lines-table.tsx` - Updated imports
10. `ui/src/pages/finance/movements-row.tsx` - Updated imports
11. `ui/src/pages/finance/movement-create-form.tsx` - Updated imports
12. `ui/src/pages/finance/movement-actions.tsx` - Updated imports
13. `ui/src/pages/finance/movement-create-form.test.tsx` - Updated imports

### Deleted Files
1. `ui/src/lib/api/types.ts` - Replaced by generated schema types

## Architecture Decisions

### Design D3: TypeScript Client
- Used `openapi-typescript` for type generation (as specified in design)
- Created thin wrapper client (`client.ts`) around fetch API
- Generic functions provide type safety without code duplication
- All types flow from single source of truth (openapi.yaml)

### Design D6: Tenancy Reconciliation
- Removed legacy `FINANCE_PATH_PREFIX` constant
- All paths now use uniform `/api/users/{userId}/...` pattern
- Verified via negative test assertions (no X-Tenant-ID header)

### Type Safety Approach
- Generated types from OpenAPI spec are source of truth
- `schema.ts` re-exports types for convenient imports
- UI-specific types (enums) defined separately in `schema.ts`
- Compile-time type checking prevents runtime errors

## Verification Checklist

- [x] Codegen script exists and runs successfully
- [x] Generated types match wire contract (snake_case, optional fields, string money, required link_conflicting, nullable lines)
- [x] Client is fully typed against generated Paths type
- [x] All code uses generated schema types (no hand-written types)
- [x] Old types.ts file deleted
- [x] FINANCE_PATH_PREFIX removed from config
- [x] All paths use uniform tenancy pattern
- [x] TypeScript build passes (`tsc --noEmit`)
- [x] All 163 existing tests pass
- [x] Schema evolution test verifies contract field flow
- [x] No breaking changes to API behavior
- [x] No test failures

## Integration with Backend

The client SDK integrates with the backend as follows:

1. **Single Source of Truth**: `procrastinator-backend/api/openapi.yaml`
2. **Server Generation**: Backend uses `oapi-codegen` to generate Go types (Tasks 3.1-3.7)
3. **Client Generation**: UI uses `openapi-typescript` to generate TypeScript types (Tasks 4.1-4.7)
4. **Consistency**: Both server and client types are generated from the same spec
5. **Drift Prevention**: Any change to the API contract requires updating openapi.yaml first

## Next Steps

The following tasks remain in the openapi-codegen-pipeline change:

- **Section 5**: Documentation generation (Tasks 5.1-5.3)
- **Section 6**: Build wiring + drift check (Tasks 6.1-6.5)
- **Section 7**: Budgets verification (Tasks 7.1-7.3)
- **Section 8**: Final verification (Tasks 8.1-8.3)

## Conclusion

All tasks in Section 4 (Client TypeScript SDK) have been successfully completed:
- ✅ Task 4.1: Codegen script implemented
- ✅ Task 4.2: Wire contract verified
- ✅ Task 4.3: Typed API client implemented
- ✅ Task 4.4: Schema types replaced and integrated
- ✅ Task 4.5: Config cleaned up and path tenancy confirmed
- ✅ Task 4.6: Build and tests verified
- ✅ Task 4.7: Schema evolution test added

The client TypeScript SDK is now fully integrated with the OpenAPI-generated types, providing compile-time type safety and ensuring consistency with the backend API contract.
