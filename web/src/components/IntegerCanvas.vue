<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { Point } from '../types'
import {
  fit,
  pathOf,
  toScreen,
  toWorldInteger,
  unionBoundsAll,
  type ViewTransform,
} from '../viewport'

const props = withDefaults(
  defineProps<{
    modelValue: Point[]
    width?: number
    height?: number
    disabled?: boolean
    fill?: string
    stroke?: string
    showGrid?: boolean
  }>(),
  {
    width: 560,
    height: 420,
    disabled: false,
    fill: 'rgba(70,130,220,0.35)',
    stroke: '#1f3a68',
    showGrid: true,
  },
)

const emit = defineEmits<{
  'update:modelValue': [Point[]]
}>()

const canvas = ref<HTMLCanvasElement | null>(null)
const vt = ref<ViewTransform | null>(null)
const hover = ref<Point | null>(null)
const mouse = ref<Point | null>(null)
const HANDLE = 6

const bounds = computed(() =>
  unionBoundsAll(props.modelValue.length ? [props.modelValue] : []),
)

function recompute() {
  vt.value = fit(bounds.value, props.width, props.height)
  draw()
}

function draw() {
  const cv = canvas.value
  const t = vt.value
  if (!cv || !t) return
  const ctx = cv.getContext('2d')!
  const dpr = window.devicePixelRatio || 1
  if (cv.width !== props.width * dpr) {
    cv.width = props.width * dpr
    cv.height = props.height * dpr
  }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, props.width, props.height)

  // Paper background.
  ctx.fillStyle = '#ffffff'
  ctx.fillRect(0, 0, props.width, props.height)

  if (props.showGrid) {
    ctx.strokeStyle = '#eef1f5'
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
    // Axes.
    const o = toScreen(t, { x: 0, y: 0 })
    ctx.strokeStyle = '#d6dbe3'
    ctx.beginPath()
    ctx.moveTo(0, o.y)
    ctx.lineTo(props.width, o.y)
    ctx.moveTo(o.x, 0)
    ctx.lineTo(o.x, props.height)
    ctx.stroke()
  }

  const pts = props.modelValue.map((p) => toScreen(t, p))
  if (pts.length >= 1) {
    pathOf(ctx, pts)
    ctx.fillStyle = props.fill
    ctx.fill()
    ctx.strokeStyle = props.stroke
    ctx.lineWidth = 2
    ctx.stroke()
  }

  // Hover preview segment from last vertex.
  if (!props.disabled && mouse.value && pts.length >= 2 && hover.value) {
    const h = toScreen(t, hover.value)
    const last = pts[pts.length - 1]
    const first = pts[0]
    ctx.setLineDash([5, 4])
    ctx.strokeStyle = '#8aa0c0'
    ctx.beginPath()
    ctx.moveTo(last.x, last.y)
    ctx.lineTo(h.x, h.y)
    const nearFirst = dist(h, first) < HANDLE * 1.6
    if (nearFirst) {
      ctx.lineTo(first.x, first.y)
    }
    ctx.stroke()
    ctx.setLineDash([])
  }

  if (!props.disabled) {
    pts.forEach((p, i) => {
      ctx.beginPath()
      ctx.arc(p.x, p.y, HANDLE / 2 + 1, 0, Math.PI * 2)
      ctx.fillStyle = i === 0 ? '#d8442e' : '#ffffff'
      ctx.fill()
      ctx.lineWidth = 2
      ctx.strokeStyle = props.stroke
      ctx.stroke()
    })
    if (hover.value && !props.disabled) {
      const h = toScreen(t, hover.value)
      ctx.strokeStyle = '#d8442e'
      ctx.beginPath()
      ctx.arc(h.x, h.y, HANDLE / 2, 0, Math.PI * 2)
      ctx.stroke()
    }
  }
}

function dist(a: Point, b: Point) {
  return Math.hypot(a.x - b.x, a.y - b.y)
}

function eventPoint(ev: MouseEvent): Point {
  const r = canvas.value!.getBoundingClientRect()
  return { x: ev.clientX - r.left, y: ev.clientY - r.top }
}

function hitVertex(s: Point): number {
  const t = vt.value!
  for (let i = 0; i < props.modelValue.length; i++) {
    if (dist(toScreen(t, props.modelValue[i]), s) <= HANDLE * 1.4) return i
  }
  return -1
}

let dragging = -1

function onDown(ev: MouseEvent) {
  if (props.disabled) return
  const s = eventPoint(ev)
  dragging = hitVertex(s)
  if (dragging >= 0) {
    moveVertex(s)
    return
  }
  // Clicking near the first vertex when 3+ exist closes the ring (no duplicate
  // is stored); clicking elsewhere appends a new integer vertex.
  const t = vt.value!
  const first = toScreen(t, props.modelValue[0] ?? { x: 0, y: 0 })
  if (props.modelValue.length >= 3 && dist(s, first) <= HANDLE * 1.6) return
  const p = toWorldInteger(t, s)
  emit('update:modelValue', [...props.modelValue, p])
}

function onMove(ev: MouseEvent) {
  if (props.disabled) return
  const s = eventPoint(ev)
  mouse.value = s
  hover.value = toWorldInteger(vt.value!, s)
  if (dragging >= 0) moveVertex(s)
}

function moveVertex(s: Point) {
  const p = toWorldInteger(vt.value!, s)
  const next = props.modelValue.map((v, i) => (i === dragging ? p : v))
  emit('update:modelValue', next)
}

function onUp() {
  dragging = -1
}
function onLeave() {
  mouse.value = null
  hover.value = null
  dragging = -1
}

function onDblClick(ev: MouseEvent) {
  if (props.disabled) return
  const idx = hitVertex(eventPoint(ev))
  if (idx <= 0 || idx >= props.modelValue.length - 1) return // keep first/last
  emit(
    'update:modelValue',
    props.modelValue.filter((_, i) => i !== idx),
  )
}

defineExpose({ recompute })

onMounted(recompute)
watch(() => [props.modelValue, props.width, props.height], draw, { deep: true })
watch(() => props.modelValue.length, recompute)
</script>

<template>
  <canvas
    ref="canvas"
    :style="{ width: width + 'px', height: height + 'px' }"
    :class="{ disabled }"
    @mousedown="onDown"
    @mousemove="onMove"
    @mouseup="onUp"
    @mouseleave="onLeave"
    @dblclick="onDblClick"
  />
</template>

<style scoped>
canvas {
  border: 1px solid #d6dbe3;
  border-radius: 8px;
  background: #fff;
  cursor: crosshair;
}
canvas.disabled {
  cursor: default;
}
</style>
