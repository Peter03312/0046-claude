import { reactive } from 'vue'
import { api, ApiError } from './api'
import type { Act, Project, Report, Sheet } from './types'

interface State {
  ready: boolean
  apiError: string
  projects: Project[]
  currentId: number | null
  project: Project | null
  sheets: Sheet[]
  acts: Act[]
  reports: { id: number; status: string; expired: boolean; confirmed: boolean; createdAt: string }[]
  lastReport: Report | null
  busy: string
}

export const state = reactive<State>({
  ready: false,
  apiError: '',
  projects: [],
  currentId: null,
  project: null,
  sheets: [],
  acts: [],
  reports: [],
  lastReport: null,
  busy: '',
})

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message
  if (e instanceof Error) return e.message
  return String(e)
}

export async function init() {
  try {
    await api.health()
    state.projects = await api.listProjects()
    if (state.projects.length > 0) await selectProject(state.projects[0].id)
  } catch (e) {
    state.apiError = errorMessage(e)
  } finally {
    state.ready = true
  }
}

export async function createProject(name: string) {
  const p = await api.createProject(name)
  state.projects = await api.listProjects()
  await selectProject(p.id)
}

export async function refreshProject() {
  if (state.currentId == null) return
  const bundle = await api.getProject(state.currentId)
  state.project = bundle.project
  state.sheets = bundle.sheets
  state.acts = bundle.acts
  state.reports = await api.listReports(state.currentId)
}

export async function selectProject(id: number) {
  state.currentId = id
  state.lastReport = null
  await refreshProject()
}

export function getSheet(id: number): Sheet | undefined {
  return state.sheets.find((s) => s.id === id)
}
