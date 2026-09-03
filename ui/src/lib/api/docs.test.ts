/**
 * Tests for the generated API documentation (Design D4).
 *
 * The docs generator is the pinned npm tool `redoc-cli@0.13.21` (a devDependency
 * of this package, matching the version pinned in the top-level Makefile `docs`
 * target). These tests invoke the local redoc-cli to regenerate the docs from the
 * OpenAPI document and assert:
 *
 *   - the committed static docs cover every documented operation
 *     (method, path, params, request/response schemas, error responses)
 *   - generation is idempotent (byte-for-byte stable — Design D4 determinism)
 *   - regenerating after a document change reflects the added operation/field
 *
 * redoc-cli bundles the spec into a self-contained HTML file; because the output
 * is byte-for-byte stable for a pinned version + input document (no volatile
 * timestamps or version banners are embedded), the committed file can be
 * drift-checked.
 */
import { describe, it, expect } from 'vitest';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';

const require = createRequire(import.meta.url);
// redoc-cli is a pinned devDependency; resolve its local CLI entry so no network
// download is required at test time (mirrors the pinned Makefile invocation).
const redocEntry = require.resolve('redoc-cli');

// Resolve the OpenAPI document + committed docs relative to this test file
// (ui/src/lib/api/ -> repo root is four levels up) so the test is cwd-independent.
const here = path.dirname(fileURLToPath(import.meta.url));
const openapiDoc = path.resolve(here, '../../../../procrastinator-backend/api/openapi.yaml');
const committedDocs = path.resolve(here, '../../../../procrastinator-backend/api/docs/index.html');

/** Run the pinned redoc-cli against `spec` and return the generated HTML. */
function buildDocs(spec: string): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'redoc-'));
  const out = path.join(dir, 'index.html');
  execFileSync(process.execPath, [redocEntry, 'build', spec, '--output', out], {
    stdio: 'pipe',
  });
  return fs.readFileSync(out, 'utf8');
}

// Every documented operation (method, path, params, request/response schemas and
// error responses) must be covered by the generated docs.
const OPERATION_IDS = [
  'uploadDocument',
  'listAssets',
  'getAsset',
  'listAssetDocuments',
  'createAccount',
  'listAccounts',
  'getAccount',
  'createMovement',
  'listMovements',
  'getMovement',
  'patchMovement',
  'deleteMovement',
  'linkMovement',
  'unlinkMovement',
  'createImportBatch',
  'listImportBatches',
  'getImportBatch',
  'commitImportBatch',
  'discardImportBatch',
  'createHousehold',
  'listHouseholds',
  'getHousehold',
  'addHouseholdMember',
];

describe('OpenAPI documentation generation (Design D4)', () => {
  it('committed docs cover every documented operation', () => {
    expect(fs.existsSync(committedDocs), 'committed docs/index.html missing — run `make docs`').toBe(true);
    const html = fs.readFileSync(committedDocs, 'utf8');

    for (const opId of OPERATION_IDS) {
      expect(html, `committed docs missing operation ${opId}`).toContain(opId);
    }
    // Request/response schema content, the error envelope, and multipart bodies
    // are all rendered (redoc embeds the full spec, so these appear in the output).
    expect(html).toContain('multipart/form-data');
    expect(html).toContain('link_conflicting');
    expect(html, 'committed docs missing the error envelope').toContain('"error"');
  });

  it('doc generation is idempotent (byte-for-byte stable)', () => {
    const first = buildDocs(openapiDoc);
    const second = buildDocs(openapiDoc);
    expect(first.length).toBeGreaterThan(0);
    expect(first).toBe(second);
  });

  it('regenerated docs reflect a document change (new operation + field)', () => {
    const base = fs.readFileSync(openapiDoc, 'utf8');

    // Inject a new operation whose response schema carries a distinctive field,
    // inserted before the components section (a clean, valid YAML insertion).
    const newOp = [
      '',
      '  /api/users/{userId}/warranty:',
      '    get:',
      '      operationId: getWarrantyNote',
      '      summary: Warranty note',
      '      responses:',
      '        "200":',
      '          description: OK',
      '          content:',
      '            application/json:',
      '              schema:',
      '                type: object',
      '                properties:',
      '                  warranty_note:',
      '                    type: string',
    ].join('\n');

    const idx = base.indexOf('\ncomponents:');
    expect(idx, 'openapi.yaml missing components section').toBeGreaterThanOrEqual(0);
    const modified = base.slice(0, idx) + newOp + base.slice(idx);

    const tmpDoc = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'redoc-')), 'openapi.yaml');
    fs.writeFileSync(tmpDoc, modified, 'utf8');

    const html = buildDocs(tmpDoc);
    // The added operation and its schema field are reflected in the docs.
    expect(html).toContain('getWarrantyNote');
    expect(html).toContain('/api/users/{userId}/warranty');
    expect(html).toContain('warranty_note');
    // Regeneration is additive, not destructive — a pre-existing op remains.
    expect(html).toContain('uploadDocument');
  });
});
