/// <reference types="vitest/config" />
import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import react, { reactCompilerPreset } from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { VitePWA } from 'vite-plugin-pwa';

// React Compiler is enabled via the plugin's `compiler` option (design D6 /
// framework-currency spec "React Compiler participation"): existing components
// compile without manual `useMemo`/`memo`. `reactCompilerPreset()` returns the
// compiler options object (rolldown filter + optimizeDeps + the Babel preset
// that wires `babel-plugin-react-compiler`); passing it as `compiler` makes the
// plugin unshift its React-compiler transform plugin.
const reactPlugin = react({ compiler: reactCompilerPreset() });

export default defineConfig({
  // React Compiler is enabled via the opt-in `reactCompilerPreset` (design D6 /
  // framework-currency spec "React Compiler participation"): existing components
  // compile without manual `useMemo`/`memo`. `reactCompilerPreset` bundles
  // `babel-plugin-react-compiler` (devDependency) and wires it through Babel.
  plugins: [
    reactPlugin,
    tailwindcss(),
    // App-shell service worker (change asset-management-v2, task 9.1): precaches
    // the built app shell so the installed PWA opens offline, with
    // NetworkFirst runtime caching for /api/* and the SPA navigation fallback
    // kept out of API routes. The SW is generated/registered only on production
    // builds (devOptions.enabled:false) — dev never generates or registers it —
    // per the framework-currency spec ("PWA installability as mobile-readiness")
    // + design D11.
    VitePWA({
      strategies: 'generateSW',
      registerType: 'autoUpdate',
      devOptions: { enabled: false },
      // The web manifest is owned by the static `public/manifest.webmanifest`
      // (task 9.1), which Vite copies as-is to `dist/manifest.webmanifest`.
      // `manifest: false` disables the plugin's own generated manifest (which
      // would otherwise overwrite the static file in the build output).
      manifest: false,
      includeAssets: ['icons/*.png'],
      workbox: {
        globPatterns: ['**/*.{js,css,html,svg,png,ico,webmanifest,woff,woff2}'],
        // Exclude the MSW test-runtime worker (dev-only, inert in production) so it
        // is not shipped in the precache (ui-reviewer M-1).
        globIgnores: ['mockServiceWorker.js'],
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//],
        runtimeCaching: [
          {
            urlPattern: ({ url }) => url.pathname.startsWith('/api/'),
            handler: 'NetworkFirst',
            options: {
              cacheName: 'api-cache',
              networkTimeoutSeconds: 5,
              expiration: { maxEntries: 100, maxAgeSeconds: 86400 },
              cacheableResponse: { statuses: [0, 200] },
            },
          },
        ],
      },
    }),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true,
  },
});
