<script setup lang="ts">
import { computed, ref } from 'vue'
import { api, ApiError } from '../api'
import { errorMessage, refreshProject, state } from '../store'
import type { Act, BadCell } from '../types'
import ProofCanvas from './ProofCanvas.vue'

const running = ref(false)
const message = ref('')
const selectedCell = ref(0)

const report = computed(() => state.lastReport)
const result = computed(() => report.value?.result ?? null)

const failedActResult = computed(() => {
  const r = result.value
  if (!r || r.failedAct < 0) return null
  return r.acts[r.failedAct] ?? null
})

const failedAct = computed<Act | null>(() => {
  const ar = failedActResult.value
  if (!ar) return null
  return state.acts.find((a) => a.id === ar.actId) ?? null
})

// An "invalid" result means malformed input (e.g. a self-intersecting sheet);
// no bad cells exist — the exact engine rejected the whole order.
const invalidReason = computed(() => {
  const ar = failedActResult.value
  return ar && ar.status === 'invalid' ? ar.error ?? '输入无效，整单拒绝' : ''
})

const cells = computed<BadCell[]>(() => failedActResult.value?.badCells ?? [])
const currentCell = computed(() => cells.value[selectedCell.value] ?? null)

function contributorName(id: string) {
  const n = Number(id.replace('sheet-', ''))
  return state.sheets.find((s) => s.id === n)?.name ?? id
}

async function runProof() {
  running.value = true
  message.value = ''
  selectedCell.value = 0
  try {
    state.lastReport = await api.runProof(state.currentId!)
    await refreshProject()
  } catch (e) {
    // A 422 (invalid geometry, e.g. self-intersection) still records a report
    // and carries its payload in the error body.
    if (e instanceof ApiError && e.status === 422 && e.data) {
      state.lastReport = e.data as typeof state.lastReport
      await refreshProject()
    } else {
      message.value = '校样失败：' + errorMessage(e)
    }
  } finally {
    running.value = false
  }
}

async function confirm() {
  if (!report.value) return
  try {
    state.lastReport = await api.confirmReport(report.value.id)
    await refreshProject()
  } catch (e) {
    message.value = '无法确认：' + errorMessage(e)
  }
}

async function unfreeze() {
  try {
    const previousId = state.lastReport?.id
    await api.unfreezeProject(state.currentId!)
    await refreshProject()
    if (previousId != null) {
      // The old report is expired now; the 410 response still carries its body.
      try {
        state.lastReport = await api.getReport(previousId)
      } catch (e) {
        if (e instanceof ApiError && e.status === 410 && e.data) {
          state.lastReport = e.data as typeof state.lastReport
        }
      }
    }
  } catch (e) {
    message.value = errorMessage(e)
  }
}

function rgbCss(c: [number, number, number]) {
  return `rgb(${c[0]},${c[1]},${c[2]})`
}
</script>

<template>
  <div class="panel" data-testid="proof-panel">
    <div class="panel-head">
      <h2>校样</h2>
      <div class="head-actions">
        <button class="btn primary" data-testid="run-proof" :disabled="running || state.project?.frozen" @click="runProof">
          {{ running ? '精确计算中…' : '运行校样（禁止抽样）' }}
        </button>
        <button v-if="state.project?.frozen" class="btn warn" data-testid="unfreeze" @click="unfreeze">
          解冻并改稿
        </button>
      </div>
    </div>

    <div v-if="state.project?.frozen" class="banner frozen" data-testid="frozen-banner">
      🔒 输入已被确认报告冻结，裁剪前不会再有意外漏色。需要改稿请先「解冻并改稿」。
    </div>
    <p v-if="message" class="msg err">{{ message }}</p>

    <div v-if="!report" class="muted">
      校样会对每一幕把变换后的片形与目标/留白区域做精确有理数平面细分：共边、共点不算漏色；
      半透明按 source-over 精确叠加，通道只在最终 half-up 取整；只报告最早不合格幕的全部坏单元。
    </div>

    <div v-else>
      <div :class="['banner', result?.status === 'passed' ? 'ok' : 'bad']" data-testid="proof-status">
        <template v-if="result?.status === 'passed'">
          ✅ 全部 {{ result.acts.length }} 幕通过。可以确认并冻结输入，交给孩子裁剪。
        </template>
        <template v-else>
          ❌ 第 {{ (result?.failedAct ?? 0) + 1 }} 幕
          「{{ failedActResult?.name }}」
          <template v-if="failedActResult?.status === 'invalid'">输入无效，整单拒绝</template>
          <template v-else>不合格，共 {{ cells.length }} 个坏单元。</template>
        </template>
      </div>

      <div v-if="invalidReason" class="invalid-box" data-testid="invalid-reason">
        精确几何引擎拒绝了这一幕的输入（未做任何像素抽样）：
        <pre>{{ invalidReason }}</pre>
      </div>

      <div v-if="result?.status === 'passed' && !state.project?.frozen" class="actions">
        <button class="btn primary" data-testid="confirm" @click="confirm">确认并冻结输入</button>
      </div>

      <div v-if="failedAct" class="proof-body">
        <ProofCanvas
          :sheets="state.sheets"
          :act="failedAct"
          :bad-cells="cells"
          :width="680"
          :height="460"
        />
        <div class="cell-picker" v-if="cells.length > 1">
          <button
            v-for="(_c, i) in cells"
            :key="i"
            :class="['btn', 'small', { active: i === selectedCell }]"
            @click="selectedCell = i"
          >
            坏区 {{ i + 1 }}
          </button>
        </div>

        <div v-if="currentCell" class="cell-detail" data-testid="cell-detail">
          <h3>
            坏区 {{ selectedCell + 1 }}：
            <b :class="currentCell.kind">{{ currentCell.kind === 'leak' ? '留白漏色' : '颜色不符' }}</b>
            @ {{ currentCell.regionName }}
          </h3>
          <div v-if="currentCell.kind === 'color'" class="swatches">
            <div>
              <span class="chip" :style="{ background: rgbCss(currentCell.actual) }" />
              实得 {{ currentCell.actual.join(',') }}
            </div>
            <div>
              <span class="chip" :style="{ background: rgbCss(currentCell.expected) }" />
              期望 {{ currentCell.expected.join(',') }}（容差 {{ currentCell.tolerance }}）
            </div>
          </div>
          <div v-else class="swatches">
            <div>
              <span class="chip" :style="{ background: rgbCss(currentCell.actual) }" />
              片形以实得色 {{ currentCell.actual.join(',') }} 正面积侵入留白区
            </div>
          </div>
          <h4>贡献片（底→顶）</h4>
          <ol class="contrib-list" data-testid="contrib-list">
            <li v-for="c in currentCell.contributors" :key="c.order + c.id">
              <span class="ord">{{ c.order }}</span>
              <span class="swatch" :style="{ background: `rgba(${c.r},${c.g},${c.b},${c.opacityMillis / 1000})` }" />
              {{ contributorName(c.id) }}
              <span class="muted small">RGB {{ c.r }},{{ c.g }},{{ c.b }} · {{ c.opacityMillis }}‰</span>
            </li>
            <li v-if="currentCell.contributors.length === 0" class="muted">无片（白底）</li>
          </ol>
          <details>
            <summary>坏单元边界环（精确分数坐标）</summary>
            <p class="hint">外环逆时针、内环顺时针，各从字典序最小点起。</p>
            <div class="rings">
              <div>外环（{{ currentCell.outer.length }} 点）：</div>
              <code v-for="(p, i) in currentCell.outer" :key="'o' + i">({{ p.x }}, {{ p.y }}) </code>
              <template v-for="(h, hi) in currentCell.holes" :key="'h' + hi">
                <div>内环 {{ hi + 1 }}（{{ h.length }} 点）：</div>
                <code v-for="(p, i) in h" :key="'h' + hi + '-' + i">({{ p.x }}, {{ p.y }}) </code>
              </template>
            </div>
          </details>
        </div>
      </div>
    </div>

    <div v-if="state.reports.length" class="report-history">
      <h4>历史校样</h4>
      <ul>
        <li v-for="r in state.reports" :key="r.id" data-testid="report-row">
          #{{ r.id }} · {{ r.status === 'passed' ? '通过' : '不合格' }}
          · {{ r.createdAt }}
          <span v-if="r.expired" class="tag stale">已过期（输入改过）</span>
          <span v-else-if="r.confirmed" class="tag ok">已确认冻结</span>
        </li>
      </ul>
    </div>
  </div>
</template>
