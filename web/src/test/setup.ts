import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'
import i18n from '../i18n'

// The app defaults to ru; tests assert the English source strings.
await i18n.changeLanguage('en')

// React Flow measures its container; jsdom has no layout, so these stubs
// report zero sizes, which is enough to render nodes.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver
class DOMMatrixStub {
  m22 = 1
  constructor(transform?: string) {
    const scale = transform?.match(/scale\(([^)]+)\)/)
    if (scale) this.m22 = Number(scale[1])
  }
}
globalThis.DOMMatrixReadOnly ??= DOMMatrixStub as unknown as typeof DOMMatrixReadOnly

// jsdom has no <dialog> behaviour; real browsers are covered by Playwright.
if (!HTMLDialogElement.prototype.showModal) {
  HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) {
    this.open = false
    this.dispatchEvent(new Event('close'))
  }
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
