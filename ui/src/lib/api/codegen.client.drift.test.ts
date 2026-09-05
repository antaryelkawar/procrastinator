/**
 * Drift + budget check for the orval per-operation client generator (design D5,
 * task 9.3).
 *
 * This is the orval half of the codegen drift/budget gate, mirroring
 * `codegen.drift.test.ts` (which covers `openapi-typescript` → `paths.d.ts`).
 * It re-runs the pinned orval generator against the committed `openapi.yaml` into
 * a scratch location and asserts the result matches the committed generated
 * artifact (`src/lib/api/generated/orval/procrastinator.ts`). It verifies the
 * generator is deterministic, the committed client is in sync, the check has
 * teeth (a stale / edited doc is detected as drift), re-running codegen clears
 * drift, and the client codegen stays within the time budget.
 *
 * Why a scratch regeneration (not `npx orval@version` and not in-place):
 *  - orval's generated output embeds the RELATIVE import from the generated file
 *    to the custom mutator (`import { customFetch } from '../mutator'`). To make
 *    a scratch run byte-comparable to the committed artifact, the scratch output
 *    is emitted at `<scratch>/orval/procrastinator.ts` and a copy of the real
 *    mutator is placed at `<scratch>/mutator.ts`, so orval computes the identical
 *    `../mutator` import the committed file carries.
 *  - The pinned local CLI (`node_modules/orval/dist/bin/orval.mjs`) is invoked via
 *    Node's child process (not `npx ...@version`, which is slow/network-dependent),
 *    mirroring the openapi-typescript drift test's invocation strategy.
 *  - A scratch run never mutates the committed artifact (no in-place regeneration).
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { execFileSync } from 'node:child_process';
import {
  readFileSync,
  writeFileSync,
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
} from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';

// The `ui` package root is `process.cwd()` when vitest runs; resolve everything to
// absolute paths to avoid cwd ambiguity.
const UI_ROOT = process.cwd();
const ORVAL_CLI = resolve(UI_ROOT, 'node_modules/orval/dist/bin/orval.mjs');
const REAL_DOC = resolve(UI_ROOT, '../procrastinator-backend/api/openapi.yaml');
const COMMITTED_CLIENT = resolve(UI_ROOT, 'src/lib/api/generated/orval/procrastinator.ts');
const REAL_MUTATOR = resolve(UI_ROOT, 'src/lib/api/generated/mutator.ts');

// Time budget for the client codegen (spec: "Full deepened codegen is within the
// time budget"). The orval per-operation client is the new generator covered here;
// the Go/TS/docs generators are budgeted by the Go budget tests.
const BUDGET_MS = 30_000;

/** Converts a path to forward slashes so the orval config is valid on Windows. */
function posix(p: string): string {
  return p.replace(/\\/g, '/');
}

/**
 * Normalizes CRLF -> LF. On a fresh Windows clone (`core.autocrlf=true`) the
 * committed artifact may be checked out with CRLF while orval emits LF;
 * normalizing both sides keeps the drift check meaningful.
 */
function normalizeEol(text: string): string {
  return text.replace(/\r\n/g, '\n');
}

/** EOL-normalized equality used by every drift comparison. */
function normalizedEquals(a: string, b: string): boolean {
  return normalizeEol(a) === normalizeEol(b);
}

const STALE_MESSAGE =
  "STALE generated file: ui/src/lib/api/generated/orval/procrastinator.ts — re-run 'make codegen' (or `npx orval` in ui/)";

/**
 * Re-runs the pinned orval generator with `docPath` as the input spec, writing
 * the generated client into a scratch location under `scratchBase`, and returns
 * the generated text read back from the file.
 *
 * The scratch layout preserves the committed artifact's `../mutator` import:
 * the output lands at `<scratchBase>/orval/procrastinator.ts` and a copy of the
 * real mutator is placed at `<scratchBase>/mutator.ts`, so orval emits the same
 * relative import the committed file carries. `execFileSync` throws if orval
 * fails (non-zero exit).
 */
function generateClient(scratchBase: string, docPath: string): string {
  const outPath = join(scratchBase, 'orval', 'procrastinator.ts');
  const mutatorCopy = join(scratchBase, 'mutator.ts');
  mkdirSync(join(scratchBase, 'orval'), { recursive: true });
  copyFileSync(REAL_MUTATOR, mutatorCopy);

  const configPath = join(scratchBase, 'orval.tmp.config.ts');
  const config = [
    'export default {',
    '  procrastinator: {',
    `    input: { target: '${posix(docPath)}' },`,
    '    output: {',
    `      target: '${posix(outPath)}',`,
    "      client: 'fetch',",
    "      mode: 'single',",
    `      override: { mutator: { name: 'customFetch', path: '${posix(mutatorCopy)}' } },`,
    '    },',
    '  },',
    '};',
    '',
  ].join('\n');
  writeFileSync(configPath, config, 'utf8');

  execFileSync(process.execPath, [ORVAL_CLI, '--config', configPath], { cwd: UI_ROOT });
  return readFileSync(outPath, 'utf8');
}

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
  '      responses:',
  '        "200":',
  '          description: OK',
  '',
].join('\n');

/** Builds a modified copy of the document in a temp file (never touches the real doc). */
function buildModifiedDoc(scratchBase: string): string {
  const original = readFileSync(REAL_DOC, 'utf8');
  // Insert the new path just before the top-level (col-0) `components:` key.
  const idx = original.indexOf('\ncomponents:');
  expect(idx, 'real openapi.yaml must contain a top-level components: key').toBeGreaterThanOrEqual(0);
  const modified = original.slice(0, idx + 1) + PING_OPERATION_YAML + original.slice(idx + 1);
  const docPath = join(scratchBase, 'openapi.modified.yaml');
  writeFileSync(docPath, modified, 'utf8');
  return docPath;
}

describe('codegen drift check (orval per-operation client)', { timeout: 120_000 }, () => {
  let tempDir: string;

  beforeEach(() => {
    tempDir = mkdtempSync(join(tmpdir(), 'orval-client-drift-'));
  });

  afterEach(() => {
    rmSync(tempDir, { recursive: true, force: true });
  });

  it('deterministic: two runs produce byte-for-byte identical output', () => {
    const a = generateClient(join(tempDir, 'run-a'), REAL_DOC);
    const b = generateClient(join(tempDir, 'run-b'), REAL_DOC);
    expect(
      normalizedEquals(a, b),
      'orval client output must be deterministic across runs',
    ).toBe(true);
  });

  it('regen matches committed client (in-sync)', () => {
    const regenerated = generateClient(join(tempDir, 'regen'), REAL_DOC);
    const committed = readFileSync(COMMITTED_CLIENT, 'utf8');
    expect(
      normalizedEquals(regenerated, committed),
      STALE_MESSAGE,
    ).toBe(true);
  });

  it('stale committed file is detected (failure path)', () => {
    const committed = readFileSync(COMMITTED_CLIENT, 'utf8');
    const stale = committed + '\n// __drift_sentinel__\n';
    const regenerated = generateClient(join(tempDir, 'regen'), REAL_DOC);
    expect(
      normalizedEquals(regenerated, stale),
      'a stale (sentinel-modified) committed client must be reported as drift',
    ).toBe(false);
  });

  it('editing the document creates drift; re-running codegen clears it', () => {
    const modifiedDoc = buildModifiedDoc(tempDir);
    const committed = readFileSync(COMMITTED_CLIENT, 'utf8');

    // (a) regenerating from the edited doc diverges from the committed client.
    const mod1 = generateClient(join(tempDir, 'mod1'), modifiedDoc);
    expect(
      normalizedEquals(mod1, committed),
      'editing the document must make the committed client stale',
    ).toBe(false);

    // (b) re-running codegen against the same edited doc is stable (in-sync after commit).
    const mod2 = generateClient(join(tempDir, 'mod2'), modifiedDoc);
    expect(
      normalizedEquals(mod1, mod2),
      're-running codegen against an edited doc must be stable',
    ).toBe(true);
  });

  describe('budget', () => {
    it('orval client codegen is within the time budget', () => {
      // WARM-UP (discarded): load orval + its transpiled config so the timed run
      // measures steady-state generation, not one-time bootstrap.
      generateClient(join(tempDir, 'warmup'), REAL_DOC);

      const start = process.hrtime.bigint();
      generateClient(join(tempDir, 'timed'), REAL_DOC);
      const elapsedMs = Number(process.hrtime.bigint() - start) / 1e6;

      expect(
        elapsedMs,
        `orval client codegen took ${elapsedMs.toFixed(0)}ms (budget ${BUDGET_MS}ms)`,
      ).toBeLessThan(BUDGET_MS);
    });
  });
});
