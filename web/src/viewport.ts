import type { Point, RatPoint } from './types'
import { parseRat } from './types'

export interface Bounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

const DEFAULT_BOUNDS: Bounds = { minX: -1, minY: -1, maxX: 11, maxY: 11 }

export function unionBoundsAll(
  polys: Array<Array<{ x: number; y: number }>>,
  hint?: Bounds,
): Bounds {
  const b: Bounds = hint
    ? { ...hint }
    : { minX: Infinity, minY: Infinity, maxX: -Infinity, maxY: -Infinity }
  for (const poly of polys) {
    for (const p of poly) {
      b.minX = Math.min(b.minX, Math.floor(p.x))
      b.maxX = Math.max(b.maxX, Math.ceil(p.x))
      b.minY = Math.min(b.minY, Math.floor(p.y))
      b.maxY = Math.max(b.maxY, Math.ceil(p.y))
    }
  }
  if (!isFinite(b.minX)) return DEFAULT_BOUNDS
  if (b.maxX - b.minX < 2) {
    b.minX -= 1
    b.maxX += 1
  }
  if (b.maxY - b.minY < 2) {
    b.minY -= 1
    b.maxY += 1
  }
  return b
}

export interface ViewTransform {
  scale: number
  originX: number
  originY: number
  w: number
  h: number
  bounds: Bounds
}

/** Fit world bounds into w×h CSS pixels with a margin, keeping square aspect. */
export function fit(bounds: Bounds, w: number, h: number, pad = 28): ViewTransform {
  const spanX = Math.max(1, bounds.maxX - bounds.minX)
  const spanY = Math.max(1, bounds.maxY - bounds.minY)
  const scale = Math.min((w - 2 * pad) / spanX, (h - 2 * pad) / spanY)
  const usedW = spanX * scale
  const usedH = spanY * scale
  const originX = (w - usedW) / 2 - bounds.minX * scale
  const originY = (h + usedH) / 2 + bounds.minY * scale
  return { scale, originX, originY, w, h, bounds }
}

export function toScreen(t: ViewTransform, p: { x: number; y: number }): Point {
  return { x: t.originX + p.x * t.scale, y: t.originY - p.y * t.scale }
}

export function toWorld(t: ViewTransform, s: Point): Point {
  return { x: (s.x - t.originX) / t.scale, y: (t.originY - s.y) / t.scale }
}

/** Snap a screen point to the integer lattice of the world plane. */
export function toWorldInteger(t: ViewTransform, s: Point): Point {
  const w = toWorld(t, s)
  return { x: Math.round(w.x), y: Math.round(w.y) }
}

export function ratPointsToScreen(t: ViewTransform, ring: RatPoint[]): Point[] {
  return ring.map((p) => toScreen(t, { x: parseRat(p.x), y: parseRat(p.y) }))
}

/** Apply a 0/90/180/270 CCW rotation around the origin, then translation. */
export function transformPoint(
  p: Point,
  rotation: number,
  tx: number,
  ty: number,
): Point {
  switch (rotation) {
    case 90:
      return { x: -p.y + tx, y: p.x + ty }
    case 180:
      return { x: -p.x + tx, y: -p.y + ty }
    case 270:
      return { x: p.y + tx, y: -p.x + ty }
    default:
      return { x: p.x + tx, y: p.y + ty }
  }
}

export function pathOf(ctx: CanvasRenderingContext2D, pts: Point[]) {
  if (pts.length === 0) return
  ctx.beginPath()
  ctx.moveTo(pts[0].x, pts[0].y)
  for (let i = 1; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y)
  ctx.closePath()
}

export function cssRGBA(r: number, g: number, b: number, opacityMillis: number) {
  return `rgba(${r},${g},${b},${(opacityMillis / 1000).toFixed(3)})`
}
