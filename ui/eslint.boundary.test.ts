/**
 * Boundary test for the cross-feature import rule (asset-management-v2, Task 5.3).
 *
 * Verifies the `no-restricted-imports` overrides that `eslint.config.js`
 * auto-generates per feature dir actually enforce the boundary:
 *   - a synthetic import that crosses a feature boundary (e.g. finance -> assets)
 *     MUST produce a `no-restricted-imports` error;
 *   - a compliant import (a feature -> docs, or a feature -> itself) MUST NOT.
 *
 * The rule matches the RAW import specifier (the `@/features/...` alias form), so
 * the ESLint `ESLint` class can lint synthetic text via `lintText` without the
 * alias being resolvable — the pattern match is a plain string match.
 */
import { describe, it, expect } from 'vitest';
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { ESLint } from 'eslint';

const here = path.dirname(fileURLToPath(import.meta.url));
const featuresDir = path.join(here, 'src', 'features');

const CONFIG_FILE = path.join(here, 'eslint.config.js');

/** Real feature dir names, read from disk (the same source the config scans). */
async function listFeatures(): Promise<string[]> {
  const entries = await readdir(featuresDir, { withFileTypes: true });
  return entries.filter((e) => e.isDirectory()).map((e) => e.name).sort();
}

/**
 * Lint a synthetic snippet as if it lived at `filePath`. Returns the rule
 * messages for `no-restricted-imports` only (the boundary rule under test).
 *
 * Uses `overrideConfigFile` so the freshly-generated `eslint.config.js` (which
 * scans `src/features` at load) is the one applied — exactly as `eslint .` does.
 * The rule matches the raw `@/features/...` import string, so no alias
 * resolution is required for the synthetic text.
 */
async function boundaryMessages(
  filePath: string,
  code: string,
): Promise<import('eslint').ESLint.LintMessage[]> {
  const eslint = new ESLint({
    fix: false,
    overrideConfigFile: CONFIG_FILE,
  });
  const results = await eslint.lintText(code, { filePath });
  return (results[0]?.messages ?? []).filter(
    (m) => m.ruleId === 'no-restricted-imports',
  );
}

describe('cross-feature import boundary (no-restricted-imports)', () => {
  it('generates overrides that flag a synthetic cross-feature import', async () => {
    const features = await listFeatures();
    expect(features).toContain('finance');

    // Pick a source feature (finance) and a forbidden sibling target (assets).
    // Both are guaranteed real feature dirs by the baseline listing.
    const source = 'finance';
    const target = features.find((f) => f !== source && f !== 'docs');
    expect(target).toBeDefined();

    const code = `import { x } from "@/features/${target}/data-table";\nexport const v = x;\n`;
    const filePath = path.join(featuresDir, source, '__synthetic__.ts');
    const boundary = await boundaryMessages(filePath, code);

    expect(boundary.length).toBeGreaterThan(0);
    expect(boundary[0].message).toContain(`@/features/${target}`);
  });

  it('does NOT flag a compliant import into docs', async () => {
    const features = await listFeatures();
    const source = 'finance';
    // docs is the only permitted cross-feature target.
    expect(features).toContain('docs');

    const code = `import { useAssets } from "@/features/docs/hooks";\nexport const v = useAssets;\n`;
    const filePath = path.join(featuresDir, source, '__synthetic__.ts');
    const boundary = await boundaryMessages(filePath, code);

    expect(boundary).toHaveLength(0);
  });

  it('does NOT flag a same-feature import', async () => {
    const source = 'finance';
    const code = `import { useAccounts } from "@/features/finance/hooks";\nexport const v = useAccounts;\n`;
    const filePath = path.join(featuresDir, source, '__synthetic__.ts');
    const boundary = await boundaryMessages(filePath, code);

    expect(boundary).toHaveLength(0);
  });

  it('applies NO cross-feature restriction to the docs feature', async () => {
    // docs is the shared group: it may import any sibling feature.
    const features = await listFeatures();
    const target = features.find((f) => f !== 'docs');
    expect(target).toBeDefined();

    const code = `import { useAssets } from "@/features/${target}/use-asset";\nexport const v = useAssets;\n`;
    const filePath = path.join(featuresDir, 'docs', '__synthetic__.ts');
    const boundary = await boundaryMessages(filePath, code);

    expect(boundary).toHaveLength(0);
  });

  it('applies NO restriction to files outside src/features/', async () => {
    // Top-level files (router, context, lib, components) may import any feature.
    const features = await listFeatures();
    const target = features.find((f) => f !== 'docs');
    expect(target).toBeDefined();

    const code = `import { useAssets } from "@/features/${target}/use-asset";\nexport const v = useAssets;\n`;
    const filePath = path.join(here, 'src', '__synthetic__.ts');
    const boundary = await boundaryMessages(filePath, code);

    expect(boundary).toHaveLength(0);
  });

  it('the feature set on disk is non-empty and includes docs + at least one non-docs feature', async () => {
    // Assert the structural invariants the rule depends on, WITHOUT hardcoding
    // the exact feature list — a legitimate new-feature addition must not break
    // this test (the whole point of auto-generation is to pick up new dirs).
    const features = await listFeatures();
    expect(features.length).toBeGreaterThan(0);
    expect(features).toContain('docs');
    expect(features.some((f) => f !== 'docs')).toBe(true);
  });

  it('the eslint config file is a non-empty file on disk', async () => {
    const raw = await readFile(CONFIG_FILE, 'utf8');
    expect(raw.trim().length).toBeGreaterThan(0);
  });
});
