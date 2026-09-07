import '@testing-library/jest-dom/vitest';
import * as matchers from 'vitest-axe/matchers';
import 'vitest-axe/extend-expect';
import { expect, beforeAll, afterEach, afterAll, vi } from 'vitest';
import { server } from '../mocks/server';

expect.extend(matchers);

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
