<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { Act, BadCell, Sheet } from '../types'
import { parseRat } from '../types'
import {
  cssRGBA,
  fit,
  pathOf,
  ratPointsToScreen,
  toScreen,
  transformPoint,
  unionBoundsAll,
  type ViewTransform,
} from '../viewport'

const props = withDefaults(
  defineProps<{
    sheets: Sheet[]
    act: Act
    badCells?: BadCell[]
    width?: number
    height?: number
    showCells?: boolean
  }>(),
  { width: 640, height: 480, showCells: true },
)

const canvas = ref<HTMLCanvasElement | null>(null)

interface DrawnSheet {
  pts: { x: number; y: number }[]
  sheet: Sheet
}

const drawn = computed<DrawnSheet[]>(() => {
  const byId = new Map(props.sheets.map((s) => [s.id, s]))
  const out: DrawnSheet[] = []
  // Array order is bottom-to-top; draw in the same order.
  for (const a of props.act.sheets) {
    const s = byId.get(a.sheetId)
    if (!s) continue
    out.push({
      sheet: s,
      pts: s.vertices.map((v) => transformPoint(v, a.rotation, a.tx, a.ty)),
    })
  }
  return out
})

const regionPolys = computed(() =>
  props.act.regions.map((r) => r.vertices),
)

const bounds = computed(() => {
  const all = [
    ...drawn.value.map((d) => d.pts),
    ...regionPolys.value,
    ...(props.badCells ?? []).flatMap((c) => [
      ...[c.outer].map((r) => r.map((p) => ({ x: parseRat(p.x), y: parseRat(p.y) }))),
      ...(c.holes ?? []).map((r) => r.map((p) => ({ x: parseRat(p.x), y: parseRat(p.y) }))),
    ]),
  ]
  return unionBoundsAll(all, { minX: -1, minY: -1, maxX: 11, maxY: 11 })
})

const vt = computed<ViewTransform>(() => fit(bounds.value, props.width, props.height))

function draw() {
  const cv = canvas.value
  if (!cv) return
  const t = vt.value
  const ctx = cv.getContext('2d')!
  const dpr = window.devicePixelRatio || 1
  cv.width = props.width * dpr
  cv.height = props.height * dpr
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.fillStyle = '#ffffff'
  ctx.fillRect(0, 0, props.width, props.height)

  // Integer lattice.
  ctx.strokeStyle = '#f0f2f6'
  ctx.lineWidth = 1
  ctx.beginPath()
  for (let x = Math.ceil(t.bounds.minX); x <= t.bounds.maxX; x++) {
    const a = toScreen(t, { x, y: t.bounds.minY })
    const b = toScreen(t, { x, y: t.bounds.maxY })
    ctx.moveTo(a.x, a.y)
    ctx.lineTo(b.x, b.y)
  }
  for (let y = Math.ceil(t.bounds.minY); y <= t.bounds.maxY; y++) {
    const a = toScreen(t, { x: t.bounds.minX, y })
    const b = toScreen(t, { x: t.bounds.maxX, y })
    ctx.moveTo(a.x, a.y)
    ctx.lineTo(b.x, b.y)
  }
  ctx.stroke()

  // Regions first: target outline blue, blank sky outline dark with hatch.
  for (const r of props.act.regions) {
    const pts = r.vertices.map((p) => toScreen(t, p))
    pathOf(ctx, pts)
    if (r.kind === 'target') {
      ctx.fillStyle = 'rgba(60,120,220,0.07)'
      ctx.fill()
      ctx.strokeStyle = 'rgba(60,120,220,0.9)'
    } else {
      ctx.fillStyle = 'rgba(20,24,40,0.06)'
      ctx.fill()
      ctx.strokeStyle = 'rgba(30,36,60,0.9)'
    }
    ctx.setLineDash([7, 4])
    ctx.lineWidth = 1.5
    ctx.stroke()
    ctx.setLineDash([])
  }

  // Sheets, bottom-to-top, source-over with integer thousandths alpha.
  for (const d of drawn.value) {
    const pts = d.pts.map((p) => toScreen(t, p))
    pathOf(ctx, pts)
    ctx.fillStyle = cssRGBA(d.sheet.r, d.sheet.g, d.sheet.b, d.sheet.opacityMillis)
    ctx.fill()
    ctx.strokeStyle = 'rgba(20,24,40,0.55)'
    ctx.lineWidth = 1.25
    ctx.stroke()
  }

  // Bad cells: fill red translucent, bold outline; holes punched back to white.
  if (props.showCells) {
    for (const c of props.badCells ?? []) {
      const outer = ratPointsToScreen(t, c.outer)
      pathOf(ctx, outer)
      ctx.fillStyle = c.kind === 'leak' ? 'rgba(220,40,30,0.30)' : 'rgba(240,160,20,0.35)'
      ctx.fill()
      for (const h of c.holes) {
        pathOf(ctx, ratPointsToScreen(t, h))
        ctx.fillStyle = '#ffffff'
        ctx.fill()
      }
      pathOf(ctx, outer)
      ctx.strokeStyle = c.kind === 'leak' ? '#c01e12' : '#b87408'
      ctx.lineWidth = 2.5
      ctx.stroke()
    }
  }
}

onMounted(draw)
watch(() => [props.sheets, props.act, props.badCells, props.width], draw, { deep: true })
</script>

<template>
  <canvas
    ref="canvas"
    :style="{ width: width + 'px', height: height + 'px' }"
    class="proof-canvas"
  />
</template>

<style scoped>
.proof-canvas {
  border: 1px solid #d6dbe3;
  border-radius: 8px;
  background: #fff;
}
</style>
