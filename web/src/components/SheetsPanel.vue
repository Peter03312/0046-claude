<script setup lang="ts">
import { computed, ref } from 'vue'
import { api } from '../api'
import { errorMessage, refreshProject, state } from '../store'
import type { Point, Sheet } from '../types'
import IntegerCanvas from './IntegerCanvas.vue'

const selectedId = ref<number | null>(null)
const message = ref('')
const messageKind = ref<'ok' | 'err'>('ok')

const draftName = ref('')
const draftColor = ref({ r: 0, g: 0, b: 0 })
const draftOpacity = ref(500)
const draftVerts = ref<Point[]>([])
const editingId = ref<number | null>(null)

function flash(kind: 'ok' | 'err', msg: string) {
  messageKind.value = kind
  message.value = msg
}

function resetDraft() {
  draftName.value = ''
  draftColor.value = { r: 200, g: 40, b: 40 }
  draftOpacity.value = 500
  draftVerts.value = []
  editingId.value = null
}
resetDraft()

function startCreate() {
  selectedId.value = null
  resetDraft()
  draftName.value = '新片 ' + (state.sheets.length + 1)
}

function startEdit(s: Sheet) {
  selectedId.value = s.id
  editingId.value = s.id
  draftName.value = s.name
  draftColor.value = { r: s.r, g: s.g, b: s.b }
  draftOpacity.value = s.opacityMillis
  draftVerts.value = s.vertices.map((v) => ({ ...v }))
}

function cancel() {
  editingId.value = null
  selectedId.value = null
  resetDraft()
}

function addVertex() {
  draftVerts.value = [...draftVerts.value, { x: 0, y: 0 }]
}

async function save() {
  message.value = ''
  const body = {
    name: draftName.value.trim(),
    vertices: draftVerts.value,
    r: draftColor.value.r,
    g: draftColor.value.g,
    b: draftColor.value.b,
    opacityMillis: draftOpacity.value,
  }
  try {
    if (editingId.value == null) {
      const s = await api.createSheet(state.currentId!, body)
      selectedId.value = s.id
    } else {
      await api.updateSheet(editingId.value, body)
    }
    await refreshProject()
    flash('ok', editingId.value == null ? '片已添加' : '片已保存，旧校样已过期')
    resetDraft()
  } catch (e) {
    flash('err', '保存失败：' + errorMessage(e))
  }
}

async function remove(s: Sheet) {
  if (!confirm(`删除片「${s.name}」？会从所有幕移除并使旧校样过期。`)) return
  try {
    await api.deleteSheet(s.id)
    if (selectedId.value === s.id) selectedId.value = null
    await refreshProject()
    flash('ok', '已删除')
  } catch (e) {
    flash('err', errorMessage(e))
  }
}

function hex() {
  const c = draftColor.value
  return '#' + [c.r, c.g, c.b].map((v) => v.toString(16).padStart(2, '0')).join('')
}
function setHex(v: string) {
  const n = parseInt(v.slice(1), 16)
  draftColor.value = { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 }
}

const draftFill = computed(
  () =>
    `rgba(${draftColor.value.r},${draftColor.value.g},${draftColor.value.b},${(
      draftOpacity.value / 1000
    ).toFixed(3)})`,
)
</script>

<template>
  <div class="panel" data-testid="sheets-panel">
    <div class="panel-head">
      <h2>彩色透明片</h2>
      <button class="btn" data-testid="new-sheet" @click="startCreate">＋新建片</button>
    </div>
    <p v-if="message" :class="['msg', messageKind]">{{ message }}</p>

    <div class="two-col">
      <ul class="item-list" data-testid="sheet-list">
        <li v-if="state.sheets.length === 0" class="muted">还没有片，点「新建片」开始。</li>
        <li
          v-for="s in state.sheets"
          :key="s.id"
          :class="{ active: selectedId === s.id }"
        >
          <span class="swatch" :style="{ background: `rgba(${s.r},${s.g},${s.b},${s.opacityMillis / 1000})` }" />
          <button class="link" @click="selectedId = s.id">{{ s.name }}</button>
          <span class="muted small">
            RGB {{ s.r }},{{ s.g }},{{ s.b }} · {{ s.opacityMillis }}‰
          </span>
          <span class="spacer" />
          <button class="btn small" :data-testid="'edit-sheet-' + s.id" @click="startEdit(s)">编辑</button>
          <button class="btn small danger" :disabled="state.project?.frozen" @click="remove(s)">删</button>
        </li>
      </ul>

      <div v-if="editingId !== null || draftName" class="editor" data-testid="sheet-editor">
        <h3>{{ editingId == null ? '新建片' : '编辑片' }}</h3>
        <label>名称 <input v-model="draftName" data-testid="sheet-name" /></label>
        <IntegerCanvas
          :model-value="draftVerts"
          :fill="draftFill"
          :disabled="state.project?.frozen"
          @update:model-value="draftVerts = $event"
        />
        <p class="hint">
          在整数画布上单击添加顶点（自动吸附整数坐标），拖动顶点修改，双击中间顶点删除。
          至少 3 个不共线、不交叉的顶点；红色点为起点。
        </p>
        <table class="vert-table" data-testid="vert-table">
          <thead>
            <tr><th>#</th><th>x</th><th>y</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="(v, i) in draftVerts" :key="i">
              <td>{{ i }}</td>
              <td><input type="number" v-model.number="v.x" /></td>
              <td><input type="number" v-model.number="v.y" /></td>
              <td>
                <button class="btn tiny danger" :disabled="i === 0" @click="draftVerts = draftVerts.filter((_, j) => j !== i)">×</button>
              </td>
            </tr>
          </tbody>
        </table>
        <button class="btn small" @click="addVertex">＋用数字加顶点</button>

        <div class="color-row">
          <label>R <input type="number" min="0" max="255" v-model.number="draftColor.r" data-testid="sheet-r" /></label>
          <label>G <input type="number" min="0" max="255" v-model.number="draftColor.g" /></label>
          <label>B <input type="number" min="0" max="255" v-model.number="draftColor.b" /></label>
          <label>取色 <input type="color" :value="hex()" @input="setHex(($event.target as HTMLInputElement).value)" /></label>
        </div>
        <label class="op">
          不透明度 {{ draftOpacity }} / 1000（千分整数）
          <input type="range" min="0" max="1000" step="1" v-model.number="draftOpacity" data-testid="sheet-opacity" />
        </label>

        <div class="actions">
          <button class="btn primary" data-testid="save-sheet" :disabled="state.project?.frozen" @click="save">保存</button>
          <button class="btn" @click="cancel">取消</button>
        </div>
      </div>
    </div>
  </div>
</template>
