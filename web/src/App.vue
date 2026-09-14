<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from './api'
import {
  createProject,
  errorMessage,
  init,
  selectProject,
  state,
} from './store'
import SheetsPanel from './components/SheetsPanel.vue'
import ActsPanel from './components/ActsPanel.vue'
import ProofPanel from './components/ProofPanel.vue'

const tab = ref<'sheets' | 'acts' | 'proof'>('sheets')
const newName = ref('')
const creating = ref(false)
const createError = ref('')

onMounted(init)

async function onCreate() {
  const name = newName.value.trim()
  if (!name) return
  creating.value = true
  createError.value = ''
  try {
    await createProject(name)
    newName.value = ''
    tab.value = 'sheets'
  } catch (e) {
    createError.value = errorMessage(e)
  } finally {
    creating.value = false
  }
}

async function rename() {
  if (!state.project) return
  const name = prompt('项目新名称', state.project.name)
  if (!name) return
  try {
    await api.renameProject(state.project.id, name.trim())
    await selectProject(state.project.id)
  } catch (e) {
    alert(errorMessage(e))
  }
}

const frozen = computed(() => state.project?.frozen ?? false)
</script>

<template>
  <div class="app">
    <header>
      <h1>🎬 透明故事校样台 <span class="sub">StoryProof · 精确有理数，不抽样</span></h1>
      <div class="project-bar">
        <label>
          项目
          <select
            :value="state.currentId ?? ''"
            data-testid="project-select"
            @change="selectProject(Number(($event.target as HTMLSelectElement).value))"
          >
            <option value="" disabled>选择…</option>
            <option v-for="p in state.projects" :key="p.id" :value="p.id">
              {{ p.name }}（v{{ p.version }}{{ p.frozen ? ' · 已冻结' : '' }}）
            </option>
          </select>
        </label>
        <input v-model="newName" placeholder="新项目名称" data-testid="new-project-name" @keydown.enter="onCreate" />
        <button class="btn" :disabled="creating" data-testid="new-project" @click="onCreate">新建项目</button>
        <button v-if="state.project" class="btn small" @click="rename">重命名</button>
        <span v-if="createError" class="msg err">{{ createError }}</span>
        <span class="spacer" />
        <span v-if="state.apiError" class="msg err">API 不可用：{{ state.apiError }}</span>
      </div>
    </header>

    <main v-if="!state.ready" class="loading">加载中…</main>
    <main v-else-if="!state.project" class="empty">
      <p>从一个空仓库开始：新建项目，然后在整数画布上画单片多边形的彩色透明片。</p>
    </main>
    <main v-else>
      <nav class="tabs">
        <button :class="{ active: tab === 'sheets' }" data-testid="tab-sheets" @click="tab = 'sheets'">① 透明片</button>
        <button :class="{ active: tab === 'acts' }" data-testid="tab-acts" @click="tab = 'acts'">② 分幕</button>
        <button :class="{ active: tab === 'proof' }" data-testid="tab-proof" @click="tab = 'proof'">③ 校样</button>
        <span class="spacer" />
        <span v-if="frozen" class="tag ok">🔒 已确认冻结</span>
      </nav>
      <SheetsPanel v-if="tab === 'sheets'" />
      <ActsPanel v-else-if="tab === 'acts'" />
      <ProofPanel v-else />
    </main>

    <footer>
      共边、共点不算漏色；颜色通道全程精确有理，仅最终 half-up 取整到 8 位；
      任何编辑都会使旧校样过期，确认后输入冻结。
    </footer>
  </div>
</template>
