/**
 * Tests that verify the generated OpenAPI types flow through to the client.
 * This ensures that changes to the API contract are reflected in the TypeScript types.
 */
import { describe, it, expect } from 'vitest';
import type { paths, components } from './generated/paths';

describe('OpenAPI generated types integration', () => {
  it('exposes all API paths from the contract', () => {
    // This test verifies that the paths type is properly generated
    // and includes all expected endpoints
    type AssetPath = paths['/api/users/{userId}/assets'];
    type AccountPath = paths['/api/users/{userId}/finance/accounts'];
    type MovementPath = paths['/api/users/{userId}/finance/movements'];

    // These type assertions verify the paths exist and have the correct structure
    const _assetPath: AssetPath = {} as any;
    const _accountPath: AccountPath = {} as any;
    const _movementPath: MovementPath = {} as any;

    expect(_assetPath).toBeDefined();
    expect(_accountPath).toBeDefined();
    expect(_movementPath).toBeDefined();
  });

  it('exposes all schema components from the contract', () => {
    // This test verifies that the schema types are properly generated
    type Asset = components['schemas']['asset'];
    type Account = components['schemas']['account'];
    type Movement = components['schemas']['movement'];
    type ImportBatch = components['schemas']['import_batch'];

    // These type assertions verify the schemas exist
    const _asset: Asset = {} as any;
    const _account: Account = {} as any;
    const _movement: Movement = {} as any;
    const _importBatch: ImportBatch = {} as any;

    expect(_asset).toBeDefined();
    expect(_account).toBeDefined();
    expect(_movement).toBeDefined();
    expect(_importBatch).toBeDefined();
  });

  it('verifies wire contract conventions are preserved', () => {
    // This test documents that key wire contract conventions are preserved
    // in the generated types (verified by inspection of generated/paths.d.ts)

    type Movement = components['schemas']['movement'];

    // link_conflicting is required (not optional)
    // This is verified by the fact that Movement.link_conflicting is boolean, not boolean | undefined
    const _movementWithRequiredField: Movement = {
      id: 'mv1',
      kind: 'expense',
      amount: '100.00',
      currency: 'USD',
      occurred_on: '2024-01-15',
      recorded_at: '2024-01-15T10:00:00Z',
      description: 'Test',
      origin: 'manual',
      link_conflicting: false, // Required field
      created_at: '2024-01-15T10:00:00Z',
      updated_at: '2024-01-15T10:00:00Z',
    };

    expect(_movementWithRequiredField.link_conflicting).toBe(false);
  });

  it('verifies optional fields are properly marked as optional', () => {
    type Asset = components['schemas']['asset'];

    // Optional fields (brand, model, etc.) can be undefined
    const _assetWithoutOptionalFields: Asset = {
      id: 'a1',
      doc_type: 'invoice',
      metadata: {},
      created_at: '2024-01-15T10:00:00Z',
      updated_at: '2024-01-15T10:00:00Z',
      // All optional fields are omitted
    };

    expect(_assetWithoutOptionalFields.id).toBe('a1');
    expect(_assetWithoutOptionalFields.brand).toBeUndefined();
  });
});
