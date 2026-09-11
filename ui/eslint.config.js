/**
 * ESLint flat config for the procrastinator UI.
 *
 * Core purpose (asset-management-v2, Task 5.3): enforce the cross-feature
 * import boundary from the modular-structure spec / design D6.
 *
 *   BOUNDARY RULE
 *   Any file under `src/features/<x>/**` may NOT import from
 *   `@/features/<y>/**` for any `<y>` that is neither `<x>` (itself) nor
 *   `docs`. `docs` is the ONLY allowed shared cross-feature target.
 *   `components/`, `lib/`, `context/` and the rest of the app are always
 *   allowed. Files OUTSIDE `src/features/` (router, context, lib, components,
 *   top-level tests, `src/test/`) are NOT subject to the restriction at all.
 *
 * The per-feature overrides below are AUTO-GENERATED from the actual
 * `src/features/*` directory listing on disk (see `buildFeatureBoundary`),
 * rather than hand-maintained. Config files are evaluated once per run, so
 * reading the filesystem at load time is fine and idiomatic. Add a new feature
 * dir and the boundary rule picks it up on the next lint with no edits.
 *
 * The rule matches the raw import specifier (`@/features/...`), the alias form
 * used across the codebase (tsconfig + vite alias `@/*` -> `./src/*`), so the
 * `no-restricted-imports` `patterns` target that prefix directly.
 */
import { readdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import tseslint from 'typescript-eslint';

const here = path.dirname(fileURLToPath(import.meta.url));
const srcRoot = path.join(here, 'src');
const featuresRoot = path.join(srcRoot, 'features');

/**
 * `docs` is the shared group: it may import any feature, and imports INTO docs
 * are the only permitted cross-feature imports. So `docs` gets NO override.
 */
const SHARED_FEATURE = 'docs';

/**
 * Build the auto-generated per-feature `no-restricted-imports` overrides.
 *
 * For each feature `x` (except the shared `docs`), emit an override scoped to
 * `src/features/x/**` that forbids `@/features/y/**` for every sibling `y`
 * where `y !== x && y !== docs`. If a feature has no forbidden siblings the
 * override is skipped (an empty `patterns` list would be a no-op).
 */
function buildFeatureBoundaryOverrides() {
  let featureNames = [];
  try {
    featureNames = readdirSync(featuresRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name);
  } catch {
    // If `src/features` is missing (e.g. linting a partial checkout), the
    // boundary simply has nothing to generate — fall back to an empty set.
    featureNames = [];
  }

  const overrides = [];
  for (const x of featureNames) {
    if (x === SHARED_FEATURE) continue; // docs is unrestricted

    const forbidden = featureNames.filter(
      (y) => y !== x && y !== SHARED_FEATURE,
    );
    if (forbidden.length === 0) continue;

    overrides.push({
      name: `feature-boundary/${x}`,
      files: [`src/features/${x}/**/*.{ts,tsx,js,jsx}`],
      rules: {
        'no-restricted-imports': [
          'error',
          {
            patterns: forbidden.map((y) => ({
              // NOTE: in ESLint 9.x, `group` is an ARRAY of glob strings (not a
              // single string) — the rule schema is `group: { type: "array",
              // items: { type: "string" }, minItems: 1 }`.
              group: [`@/features/${y}/**`],
              message:
                `Feature '${x}' may not import from feature '${y}'. ` +
                `The only permitted cross-feature import target is ` +
                `'@/features/${SHARED_FEATURE}/**' (or this feature's own module). ` +
                `Share code via a common lib/context/component instead.`,
            })),
          },
        ],
      },
    });
  }
  return overrides;
}

export default tseslint.config(
  {
    // Ignored paths (flat config: a standalone `ignores` object ignores
    // everything it matches, and it is never linted). `node_modules` is ignored
    // automatically; `dist`, the codegen output, and the stray `ui/ui/` dup are
    // excluded explicitly.
    ignores: [
      'dist/**',
      'src/lib/api/generated/**',
      'ui/**',
      '*.tsbuildinfo',
    ],
  },
  // Base TS-aware config. `tseslint.configs.recommended` is itself an ARRAY of
  // config objects (the parser config + the recommended rules), so it is spread
  // into the surrounding array here — NOT into a single object.
  ...tseslint.configs.recommended,
  {
    // NOTE: this codebase was never linted before, so the full `recommended`
    // set surfaces many pre-existing, unrelated findings. This task's scope is
    // ONLY the cross-feature boundary rule, so we relax the broad `recommended`
    // rules to `warn` (they report but never fail the build) while keeping the
    // boundary rule (`no-restricted-imports`) a hard `error`. This keeps
    // `npm run lint` green and focused on the boundary.
    rules: {
      '@typescript-eslint/no-unused-vars': 'warn',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-require-imports': 'off',
      'no-undef': 'off',
    },
    languageOptions: {
      sourceType: 'module',
    },
  },
  // Auto-generated per-feature cross-feature import boundary overrides.
  ...buildFeatureBoundaryOverrides(),
);
