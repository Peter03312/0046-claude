export interface Point {
  x: number
  y: number
}

export interface Project {
  id: number
  name: string
  version: number
  frozen: boolean
  createdAt: string
  updatedAt: string
}

export interface Sheet {
  id: number
  projectId: number
  name: string
  vertices: Point[]
  r: number
  g: number
  b: number
  opacityMillis: number
}

export type Rotation = 0 | 90 | 180 | 270

export interface ActSheet {
  sheetId: number
  stack: number
  rotation: Rotation
  tx: number
  ty: number
}

export interface Region {
  id?: number
  actId?: number
  name: string
  kind: 'target' | 'blank'
  vertices: Point[]
  r: number
  g: number
  b: number
  tolerance: number
}

export interface Act {
  id: number
  projectId: number
  name: string
  position: number
  sheets: ActSheet[]
  regions: Region[]
}

export interface Contributor {
  id: string
  name: string
  order: number
  r: number
  g: number
  b: number
  opacityMillis: number
}

/** Exact rational point as returned by the proof engine: "3/2", "-5", ... */
export interface RatPoint {
  x: string
  y: string
}

export interface BadCell {
  kind: 'color' | 'leak'
  regionId: string
  regionName: string
  outer: RatPoint[]
  holes: RatPoint[][]
  actual: [number, number, number]
  expected: [number, number, number]
  tolerance: number
  contributors: Contributor[]
}

export interface ActResult {
  actId: number
  name: string
  position: number
  status: 'passed' | 'failed' | 'invalid'
  badCells?: BadCell[]
  error?: string
}

export interface ProofResult {
  projectId: number
  version: number
  status: 'passed' | 'failed'
  acts: ActResult[]
  failedAct: number
}

export interface ReportMeta {
  id: number
  projectId: number
  version: number
  status: 'passed' | 'failed'
  confirmed: boolean
  expired: boolean
  createdAt: string
  confirmedAt?: string
}

export interface Report extends ReportMeta {
  result: ProofResult
  input: { project: Project; sheets: Sheet[]; acts: Act[] }
}

/** Parse "p/q" or integer strings exactly as fractions. */
export function parseRat(s: string): number {
  const i = s.indexOf('/')
  if (i < 0) return Number(s)
  return Number(s.slice(0, i)) / Number(s.slice(i + 1))
}
