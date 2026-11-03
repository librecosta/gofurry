import type { Locator } from '@playwright/test'

// Inspect painted geometry/opacity rather than CSS Transition internals. Works
// for WAAPI and Motion's layout animations, and stops when exit removes a node.
export async function sampleMotion(element: Locator, duration = 480) {
  return element.evaluate((el, duration) => new Promise<{ t: number; x: number; y: number; w: number; h: number; opacity: number }[]>((resolve) => {
    const start = performance.now(), frames: { t: number; x: number; y: number; w: number; h: number; opacity: number }[] = []
    const sample = () => {
      const t = performance.now() - start
      if (!el.isConnected) { resolve(frames); return }
      const r = el.getBoundingClientRect()
      frames.push({ t, x: r.x, y: r.y, w: r.width, h: r.height, opacity: Number(getComputedStyle(el).opacity) })
      if (t >= duration) resolve(frames)
      else requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  }), duration)
}
