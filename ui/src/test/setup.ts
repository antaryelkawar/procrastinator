import type { TestingLibraryMatchers } from '@testing-library/jest-dom/matchers';
import * as jestDomMatchers from '@testing-library/jest-dom/matchers';
import type { AxeResults } from 'axe-core';
import * as axeMatchers from 'vitest-axe/matchers';
import { expect, beforeAll, afterEach, afterAll, vi } from 'vitest';
import { server } from '../mocks/server';

// Central matcher typing for vitest v5. Both `@testing-library/jest-dom` and
// `vitest-axe` register their matchers at runtime here and ship type
// augmentations, but the two target different shapes that no longer merge under
// vitest v5: jest-dom augments the root `vitest` module (via a
// `/// <reference>` that also pulls its runtime entry) while vitest-axe targets
// a `Vi` namespace vitest v5 dropped, and the internal `@vitest/expect` module
// the old per-file augmentations used is gone. So we register both families via
// `expect.extend` (below) and declare both on the root `Assertion` in this one
// file — the only place the two-param base `Assertion` merges cleanly — giving
// every test file (which runs through this setup) typed DOM + axe matchers.
declare module 'vitest' {
  interface Assertion<R extends void | Promise<void> = void, T = unknown>
    extends TestingLibraryMatchers<unknown, T> {
    toHaveNoViolations(): {
      actual: AxeResults['violations'];
      pass: boolean;
      message(): string;
    };
  }
}

expect.extend(jestDomMatchers);
expect.extend(axeMatchers);

// jsdom does not implement window.matchMedia. The theme provider and any
// media-query code call it, so stub it to return a light-mode MediaQueryList
// with no-op listeners.
if (typeof window !== "undefined" && typeof window.matchMedia === "undefined") {
  window.matchMedia = (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  });
}

// jsdom does not implement scrollIntoView; Radix Select (and other primitives)
// call it to position open content. Stub it so component tests can drive those
// controls.
if (typeof Element !== 'undefined' && !Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}

// jsdom does not implement PointerEvent. React's event delegation keys pointer
// handlers on the PointerEvent constructor, so without it `fireEvent.pointerDown`
// is silently dropped and Radix DropdownMenu triggers (which open on
// onPointerDown) never open. Polyfill it so pointer-driven controls work in tests.
if (typeof window !== 'undefined' && typeof (window as { PointerEvent?: unknown }).PointerEvent === 'undefined') {
  class PointerEventPolyfill extends MouseEvent {
    readonly pointerId: number;
    readonly pointerType: string;
    readonly width: number;
    readonly height: number;
    readonly pressure: number;
    readonly isPrimary: boolean;
    constructor(type: string, params: PointerEventInit = {}) {
      super(type, params);
      this.pointerId = params.pointerId ?? 0;
      this.pointerType = params.pointerType ?? 'mouse';
      this.width = params.width ?? 0;
      this.height = params.height ?? 0;
      this.pressure = params.pressure ?? 0;
      this.isPrimary = params.isPrimary ?? true;
    }
  }
  Object.defineProperty(window, 'PointerEvent', {
    value: PointerEventPolyfill,
    writable: true,
    configurable: true,
  });
}

// jsdom does not implement Element.prototype.hasPointerCapture /
// releasePointerCapture / setPointerCapture. Radix Popper (used by
// DropdownMenu content positioning) calls them unconditionally, so without
// stubs the content render throws and the menu never opens in tests.
if (typeof Element !== 'undefined') {
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.setPointerCapture) {
    Element.prototype.setPointerCapture = () => {};
  }
  if (!Element.prototype.releasePointerCapture) {
    Element.prototype.releasePointerCapture = () => {};
  }
}

// jsdom does not implement elementFromPoint. Radix Popper (used by
// DropdownMenu content positioning) calls it synchronously during the
// positioner render pass, so without a stub the content render throws and
// the menu never opens in tests.
if (typeof document !== 'undefined' && !document.elementFromPoint) {
  document.elementFromPoint = vi.fn(() => null);
}

// jsdom does not implement ResizeObserver. Radix Popper (DropdownMenu content
// positioning) subscribes to it, so stub it to no-op observers.
if (typeof window !== 'undefined' && typeof window.ResizeObserver === 'undefined') {
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(window, 'ResizeObserver', {
    value: ResizeObserverStub,
    writable: true,
    configurable: true,
  });
}

beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
