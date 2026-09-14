import type {
  Act,
  Point,
  Project,
  Report,
  ReportMeta,
  Sheet,
} from './types'

export class ApiError extends Error {
  status: number
  data?: unknown
  constructor(status: number, message: string, data?: unknown) {
    super(message)
    this.status = status
    this.data = data
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const data = text ? JSON.parse(text) : undefined
  if (!res.ok) {
    const msg = data && typeof data.error === 'string' ? data.error : `HTTP ${res.status}`
    throw new ApiError(res.status, msg, data)
  }
  return data as T
}

export const api = {
  health: () => req<{ status: string }>('GET', '/api/health'),

  listProjects: () => req<Project[]>('GET', '/api/projects'),
  createProject: (name: string) => req<Project>('POST', '/api/projects', { name }),
  getProject: (id: number) =>
    req<{ project: Project; sheets: Sheet[]; acts: Act[] }>('GET', `/api/projects/${id}`),
  renameProject: (id: number, name: string) =>
    req<Project>('PATCH', `/api/projects/${id}`, { name }),
  deleteProject: (id: number) => req<void>('DELETE', `/api/projects/${id}`),
  unfreezeProject: (id: number) => req<Project>('POST', `/api/projects/${id}/unfreeze`),

  listSheets: (pid: number) => req<Sheet[]>('GET', `/api/projects/${pid}/sheets`),
  createSheet: (pid: number, body: SheetBody) =>
    req<Sheet>('POST', `/api/projects/${pid}/sheets`, body),
  updateSheet: (id: number, body: SheetBody) => req<Sheet>('PUT', `/api/sheets/${id}`, body),
  deleteSheet: (id: number) => req<void>('DELETE', `/api/sheets/${id}`),

  listActs: (pid: number) => req<Act[]>('GET', `/api/projects/${pid}/acts`),
  createAct: (pid: number, body: ActBody) =>
    req<Act>('POST', `/api/projects/${pid}/acts`, body),
  updateAct: (id: number, body: ActBody) => req<Act>('PUT', `/api/acts/${id}`, body),
  deleteAct: (id: number) => req<void>('DELETE', `/api/acts/${id}`),

  runProof: (pid: number) => req<Report>('POST', `/api/projects/${pid}/proofs`),
  listReports: (pid: number) => req<ReportMeta[]>('GET', `/api/projects/${pid}/reports`),
  getReport: (id: number) => req<Report>('GET', `/api/reports/${id}`),
  confirmReport: (id: number) => req<Report>('POST', `/api/reports/${id}/confirm`),
}

export interface SheetBody {
  name: string
  vertices: Point[]
  r: number
  g: number
  b: number
  opacityMillis: number
}

export interface ActSheetBody {
  sheetId: number
  rotation: number
  tx: number
  ty: number
}

export interface RegionBody {
  name: string
  kind: 'target' | 'blank'
  vertices: Point[]
  r: number
  g: number
  b: number
  tolerance: number
}

export interface ActBody {
  name: string
  sheets: ActSheetBody[]
  regions: RegionBody[]
}
