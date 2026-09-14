<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api'
import { errorMessage, getSheet, refreshProject, state } from '../store'
import type { Act, Point, Region, Rotation } from '../types'
import IntegerCanvas from './IntegerCanvas.vue'
import ProofCanvas from './ProofCanvas.vue'

const selectedId = ref<number | null>(state.acts[0]?.id ?? null)
const message = ref('')
const editingId = ref<number | null>(null)
const draftName = ref('')
const draftSheets = ref<{ sheetId: number; rotation: Rotation; tx: number; ty: number }[]>([])
const draftRegions = ref<Region[]>([])
const regionEditIndex = ref<number>(-1)

const selected = computed<Act | undefined>(() =>
  state.acts.find((a) => a.id === selectedId.value),
)

const rotations: Rotation[] = [0, 90, 180, 270]

function flash(m: string) {
  message.value = m
}

function startCreate() {
  selectedId.value = null
  editingId.value = -1 // creating
  draftName.value = '第 ' + (state.acts.length + 1) + ' 幕'
  draftSheets.value = []
  draftRegions.value = []
  regionEditIndex.value = -1
}

function startEdit(a: Act) {
  selectedId.value = a.id
  editingId.value = a.id
  draftName.value = a.name
  draftSheets.value = a.sheets.map((s) => ({
    sheetId: s.sheetId,
    rotation: s.rotation as Rotation,
    tx: s.tx,
    ty: s.ty,
  }))
  draftRegions.value = a.regions.map((r) => ({ ...r, vertices: r.vertices.map((v) => ({ ...v })) }))
  regionEditIndex.value = draftRegions.value.length ? 0 : -1
}

function cancel() {
  editingId.value = null
  regionEditIndex.value = -1
}

function addSheetToAct(sheetId: number) {
  if (draftSheets.value.some((s) => s.sheetId === sheetId)) return
  draftSheets.value.push({ sheetId, rotation: 0, tx: 0, ty: 0 })
}

function onAddSheet(ev: Event) {
  const el = ev.target as HTMLSelectElement
  addSheetToAct(Number(el.value))
  el.selectedIndex = 0
}
function removeStack(i: number) {
  draftSheets.value.splice(i, 1)
}
function move(i: number, dir: -1 | 1) {
  const j = i + dir
  if (j < 0 || j >= draftSheets.value.length) return
  const arr = draftSheets.value
  ;[arr[i], arr[j]] = [arr[j], arr[i]]
}

function addRegion(kind: 'target' | 'blank') {
  draftRegions.value.push({
    name: (kind === 'target' ? '目标区 ' : '留白区 ') + (draftRegions.value.length + 1),
    kind,
    vertices: [],
    r: kind === 'target' ? 128 : 0,
    g: kind === 'target' ? 128 : 0,
    b: kind === 'target' ? 128 : 0,
    tolerance: kind === 'target' ? 0 : 0,
  })
  regionEditIndex.value = draftRegions.value.length - 1
}
function removeRegion(i: number) {
  draftRegions.value.splice(i, 1)
  regionEditIndex.value = -1
}

const editingRegion = computed(() =>
  regionEditIndex.value >= 0 ? draftRegions.value[regionEditIndex.value] : null,
)

async function save() {
  message.value = ''
  const body = {
    name: draftName.value.trim(),
    sheets: draftSheets.value,
    regions: draftRegions.value.map((r) => ({
      name: r.name,
      kind: r.kind,
      vertices: r.vertices,
      r: r.r,
      g: r.g,
      b: r.b,
      tolerance: r.tolerance,
    })),
  }
  try {
    let a: Act
    if (editingId.value === -1) {
      a = await api.createAct(state.currentId!, body)
    } else {
      a = await api.updateAct(editingId.value!, body)
    }
    await refreshProject()
    selectedId.value = a.id
    editingId.value = null
    flash('幕已保存，旧校样已过期')
  } catch (e) {
    flash('保存失败：' + errorMessage(e))
  }
}

async function remove(a: Act) {
  if (!confirm(`删除「${a.name}」？旧校样会过期。`)) return
  try {
    await api.deleteAct(a.id)
    if (selectedId.value === a.id) selectedId.value = null
    await refreshProject()
  } catch (e) {
    flash(errorMessage(e))
  }
}

const previewAct = computed<Act>(() => ({
  id: -1,
  projectId: state.currentId ?? -1,
  name: draftName.value || '预览',
  position: 0,
  sheets: draftSheets.value.map((s, i) => ({ ...s, stack: i })),
  regions: draftRegions.value,
}))

const editingRegionVerts = computed<Point[]>({
  get: () => editingRegion.value?.vertices ?? [],
  set: (v) => {
    if (editingRegion.value) editingRegion.value.vertices = v
  },
})
</script>

<template>
  <div class="panel" data-testid="acts-panel">
    <div class="panel-head">
      <h2>分幕故事</h2>
      <button class="btn" data-testid="new-act" @click="startCreate">＋新建幕</button>
    </div>
    <p v-if="message" class="msg err">{{ message }}</p>

    <div class="two-col">
      <ul class="item-list" data-testid="act-list">
        <li v-if="state.acts.length === 0" class="muted">还没有幕。</li>
        <li
          v-for="(a, i) in state.acts"
          :key="a.id"
          :class="{ active: selectedId === a.id }"
        >
          <button class="link" @click="selectedId = a.id">第 {{ i + 1 }} 幕 · {{ a.name }}</button>
          <span class="muted small">{{ a.sheets.length }} 片 / {{ a.regions.length }} 区</span>
          <span class="spacer" />
          <button class="btn small" @click="startEdit(a)">编辑</button>
          <button class="btn small danger" :disabled="state.project?.frozen" @click="remove(a)">删</button>
        </li>
      </ul>

      <div v-if="selected && editingId === null" class="editor">
        <h3>{{ selected.name }}</h3>
        <ProofCanvas :sheets="state.sheets" :act="selected" :show-cells="false" />
        <h4>启用片（底→顶）</h4>
        <ol class="stack-list">
          <li v-for="as in selected.sheets" :key="as.sheetId">
            <span class="swatch" :style="{ background: `rgba(${(getSheet(as.sheetId)?.r) ?? 0},${(getSheet(as.sheetId)?.g) ?? 0},${(getSheet(as.sheetId)?.b) ?? 0},${((getSheet(as.sheetId)?.opacityMillis) ?? 0) / 1000})` }" />
            {{ getSheet(as.sheetId)?.name ?? ('#' + as.sheetId) }}
            <span class="muted small">旋 {{ as.rotation }}° 平移 ({{ as.tx }}, {{ as.ty }})</span>
          </li>
        </ol>
        <h4>检查区域</h4>
        <ul class="region-list">
          <li v-for="r in selected.regions" :key="r.id">
            <b :class="r.kind">{{ r.kind === 'target' ? '目标' : '留白' }}</b>
            {{ r.name }}
            <span v-if="r.kind === 'target'" class="muted small">
              RGB {{ r.r }},{{ r.g }},{{ r.b }} 容差 {{ r.tolerance }}
            </span>
          </li>
        </ul>
        <button class="btn" @click="startEdit(selected)">编辑这一幕</button>
      </div>

      <div v-if="editingId !== null" class="editor" data-testid="act-editor">
        <h3>{{ editingId === -1 ? '新建幕' : '编辑幕' }}</h3>
        <label>幕名 <input v-model="draftName" data-testid="act-name" /></label>

        <h4>启用片与层序（底→顶）</h4>
        <p class="hint" v-if="state.sheets.length === 0">请先在「透明片」页创建片。</p>
        <div class="add-sheet-row">
          <select data-testid="add-sheet-select" @change="onAddSheet">
            <option :value="0" disabled selected>＋加入片…</option>
            <option v-for="s in state.sheets.filter((x) => !draftSheets.some((d) => d.sheetId === x.id))" :key="s.id" :value="s.id">
              {{ s.name }}
            </option>
          </select>
        </div>
        <ol class="stack-list edit" data-testid="stack-list">
          <li v-for="(as, i) in draftSheets" :key="as.sheetId">
            <b class="ord">{{ i }}</b>
            <span class="swatch" :style="{ background: `rgba(${getSheet(as.sheetId)?.r},${getSheet(as.sheetId)?.g},${getSheet(as.sheetId)?.b},${(getSheet(as.sheetId)?.opacityMillis ?? 0) / 1000})` }" />
            {{ getSheet(as.sheetId)?.name }}
            <label>旋转
              <select v-model.number="as.rotation" data-testid="rotation">
                <option v-for="d in rotations" :key="d" :value="d">{{ d }}°</option>
              </select>
            </label>
            <label>x <input type="number" v-model.number="as.tx" class="tiny-input" data-testid="tx" /></label>
            <label>y <input type="number" v-model.number="as.ty" class="tiny-input" data-testid="ty" /></label>
            <span class="spacer" />
            <button class="btn tiny" :disabled="i === 0" @click="move(i, -1)">↑</button>
            <button class="btn tiny" :disabled="i === draftSheets.length - 1" @click="move(i, 1)">↓</button>
            <button class="btn tiny danger" @click="removeStack(i)">×</button>
          </li>
        </ol>

        <h4>目标色区 / 留白区</h4>
        <div class="region-tabs">
          <button class="btn small" data-testid="add-target" @click="addRegion('target')">＋目标色区</button>
          <button class="btn small" data-testid="add-blank" @click="addRegion('blank')">＋留白区</button>
        </div>
        <ul class="region-list edit" data-testid="region-list">
          <li v-for="(r, i) in draftRegions" :key="i" :class="{ active: regionEditIndex === i }">
            <button class="link" @click="regionEditIndex = i">
              <b :class="r.kind">{{ r.kind === 'target' ? '目标' : '留白' }}</b> {{ r.name }}
            </button>
            <span class="spacer" />
            <button class="btn tiny danger" @click="removeRegion(i)">×</button>
          </li>
        </ul>

        <div v-if="editingRegion" class="region-edit">
          <label>区名 <input v-model="editingRegion.name" data-testid="region-name" /></label>
          <IntegerCanvas v-model="editingRegionVerts" :fill="editingRegion.kind === 'target' ? 'rgba(60,120,220,0.25)' : 'rgba(30,36,60,0.25)'" />
          <table class="vert-table" data-testid="region-vert-table">
            <thead>
              <tr><th>#</th><th>x</th><th>y</th><th></th></tr>
            </thead>
            <tbody>
              <tr v-for="(v, i) in editingRegion.vertices" :key="i">
                <td>{{ i }}</td>
                <td><input type="number" v-model.number="v.x" /></td>
                <td><input type="number" v-model.number="v.y" /></td>
                <td>
                  <button class="btn tiny danger" :disabled="i === 0" @click="editingRegion.vertices.splice(i, 1)">×</button>
                </td>
              </tr>
            </tbody>
          </table>
          <button class="btn small" @click="editingRegion.vertices.push({ x: 0, y: 0 })">＋用数字加顶点</button>
          <div v-if="editingRegion.kind === 'target'" class="color-row">
            <label>R <input type="number" min="0" max="255" v-model.number="editingRegion.r" data-testid="target-r" /></label>
            <label>G <input type="number" min="0" max="255" v-model.number="editingRegion.g" /></label>
            <label>B <input type="number" min="0" max="255" v-model.number="editingRegion.b" /></label>
            <label>每通道容差 <input type="number" min="0" max="255" v-model.number="editingRegion.tolerance" data-testid="target-tol" /></label>
          </div>
        </div>

        <ProofCanvas :sheets="state.sheets" :act="previewAct" :show-cells="false" :width="560" :height="320" />

        <div class="actions">
          <button class="btn primary" data-testid="save-act" :disabled="state.project?.frozen" @click="save">保存幕</button>
          <button class="btn" @click="cancel">取消</button>
        </div>
      </div>
    </div>
  </div>
</template>
