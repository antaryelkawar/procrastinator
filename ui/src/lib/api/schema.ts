/**
 * Re-exports of generated OpenAPI schema types for use throughout the UI.
 * This file provides the bridge between the generated paths.d.ts and the
 * application code, replacing the hand-written types.ts.
 */
import type { components } from './generated/paths';

// Response/resource types
export type Asset = components['schemas']['asset'];
export type Document = components['schemas']['document'];
export type Account = components['schemas']['account'];
export type Movement = components['schemas']['movement'];
export type ImportBatch = components['schemas']['import_batch'];
export type ImportLine = components['schemas']['import_line'];
export type ImportSource = components['schemas']['import_source'];
export type CommitSummary = components['schemas']['commit_summary'];
export type Household = components['schemas']['household'];
export type HouseholdMember = components['schemas']['household_member'];
export type ApiError = components['schemas']['error'];

// Request types
export type CreateAccountRequest = components['schemas']['create_account_request'];
export type CreateMovementRequest = components['schemas']['create_movement_request'];
export type CreateMovementInput = Omit<CreateMovementRequest, 'source_account_id' | 'destination_account_id'> & {
  source_account_id?: string;
  destination_account_id?: string;
};
export type PatchMovementRequest = components['schemas']['patch_movement_request'];
export type LinkMovementRequest = components['schemas']['link_movement_request'];
