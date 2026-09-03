/**
 * Drift check for the TypeScript OpenAPI generator (design D5).
 *
 * This is the `vitest`-reachable half of the codegen drift check. It re-runs the
 * pinned `openapi-typescript` CLI against the committed `openapi.yaml` into a
 * scratch location and asserts the result matches the committed generated
 * artifact (`src/lib/api/generated/paths.d.ts`). It also verifies the generator
 * is deterministic and that the check has teeth (a stale / edited doc is
 * detected as drift, and re-running codegen clears it).
 *
 * The local installed CLI is invoked via Node's child process (not `npx ...@version`,
 * which is slow/network-dependent). The exact invocation the `codegen` npm script
 * uses (`openapi-typescript <doc> --output <out>`) is reproduced with the pinned
 * `node_modules/openapi-typescript/bin/cli.js` entry and `-o` output flag.
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { execFileSync } from 'node:child_process';
import {
  readFileSync,
  writeFileSync,
  mkdtempSync,
  rmSync,
} from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';

// The `ui` package root is `process.cwd()` when vitest runs; resolve everything to
// absolute paths to avoid cwd ambiguity.
const UI_ROOT = process.cwd();
const CLI_PATH = resolve(UI_ROOT, 'node_modules/openapi-typescript/bin/cli.js');
const REAL_DOC = resolve(UI_ROOT, '../procrastinator-backend/api/openapi.yaml');
const COMMITTED_PATHS = resolve(UI_ROOT, 'src/lib/api/generated/paths.d.ts');

/**
 * Runs the local pinned openapi-typescript CLI against `docPath`, writing the
 * generated output to `outPath`, and returns the generated text read back from
 * the file. `execFileSync` throws if the generator fails (non-zero exit).
 */
function generateTo(outPath: string, docPath: string = REAL_DOC): string {
  execFileSync(
    process.execPath,
    [CLI_PATH, docPath, '-o', outPath],
    { cwd: UI_ROOT },
  );
  return readFileSync(outPath, 'utf8');
}

/**
 * Normalizes CRLF -> LF. On a fresh Windows clone (`core.autocrlf=true`) the
 * committed artifact may be checked out with CRLF while the generator emits LF;
 * normalizing both sides keeps the drift check meaningful (it compares the
 * actual generated content, not line-ending artifacts).
 */
function normalizeEol(text: string): string {
  return text.replace(/\r\n/g, '\n');
}

/** EOL-normalized equality used by every drift comparison. */
function normalizedEquals(a: string, b: string): boolean {
  return normalizeEol(a) === normalizeEol(b);
}

const STALE_MESSAGE =
  "STALE generated file: ui/src/lib/api/generated/paths.d.ts — re-run 'npm run codegen' (ui) or 'make codegen'";

/** A minimal new operation inserted just before the top-level `components:` key. */
const PING_OPERATION_YAML = [
  '  /api/users/{userId}/ping:',
  '    parameters:',
  '      - name: userId',
  '        in: path',
  '        required: true',
  '        schema:',
  '          type: string',
  '    get:',
  '      operationId: ping',
  '      summary: Ping a user',
  '      description: Probes whether the user route resolves.',
  '      responses:',
  '        "200":',
  '          description: OK',
  '',
].join('\n');

/** Builds a modified copy of the document in a temp file (never touches the real doc). */
function buildModifiedDoc(tempDir: string): string {
  const original = readFileSync(REAL_DOC, 'utf8');
  // Insert the new path just before the top-level (col-0) `components:` key.
  const idx = original.indexOf('\ncomponents:');
  expect(idx, 'real openapi.yaml must contain a top-level components: key').toBeGreaterThanOrEqual(0);
  const modified = original.slice(0, idx + 1) + PING_OPERATION_YAML + original.slice(idx + 1);
  const docPath = join(tempDir, 'openapi.modified.yaml');
  writeFileSync(docPath, modified, 'utf8');
  return docPath;
}

describe('codegen drift check (openapi-typescript)', () => {
  let tempDir: string;

  beforeEach(() => {
    tempDir = mkdtempSync(join(tmpdir(), 'ot-drift-'));
  });

  afterEach(() => {
    rmSync(tempDir, { recursive: true, force: true });
  });

  it('deterministic: two runs produce byte-for-byte identical output', () => {
    const a = generateTo(join(tempDir, 'run-a.d.ts'));
    const b = generateTo(join(tempDir, 'run-b.d.ts'));
    expect(
      normalizedEquals(a, b),
      'openapi-typescript output must be deterministic across runs',
    ).toBe(true);
  });

  it('regen matches committed paths.d.ts (in-sync)', () => {
    const regenerated = generateTo(join(tempDir, 'regen.d.ts'));
    const committed = readFileSync(COMMITTED_PATHS, 'utf8');
    expect(
      normalizedEquals(regenerated, committed),
      STALE_MESSAGE,
    ).toBe(true);
  });

  it('stale committed file is detected (failure path)', () => {
    const committed = readFileSync(COMMITTED_PATHS, 'utf8');
    const stale = committed + '\n// __drift_sentinel__\n';
    const regenerated = generateTo(join(tempDir, 'regen.d.ts'));
    expect(
      normalizedEquals(regenerated, stale),
      'a stale (sentinel-modified) committed file must be reported as drift',
    ).toBe(false);
  });

  it('editing the document creates drift; re-running codegen clears it', () => {
    const modifiedDoc = buildModifiedDoc(tempDir);
    const committed = readFileSync(COMMITTED_PATHS, 'utf8');

    // (a) regenerating from the edited doc diverges from the committed artifact.
    const scratchMod = generateTo(join(tempDir, 'scratch-mod.d.ts'), modifiedDoc);
    expect(
      normalizedEquals(scratchMod, committed),
      'editing the document must make the committed artifact stale',
    ).toBe(false);

    // (b) re-running codegen against the same edited doc is stable (in-sync after commit).
    const scratchMod2 = generateTo(join(tempDir, 'scratch-mod2.d.ts'), modifiedDoc);
    expect(
      normalizedEquals(scratchMod, scratchMod2),
      're-running codegen against an edited doc must be stable',
    ).toBe(true);
  });
});
