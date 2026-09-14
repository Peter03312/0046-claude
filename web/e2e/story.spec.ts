import { test, expect, type Page } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', (e) => pageErrors.push('pageerror: ' + e.message))
  ;(page as unknown as { __errs: string[] }).__errs = pageErrors
})

test.afterEach(async ({ page }) => {
  const errs = (page as unknown as { __errs: string[] }).__errs
  // Uncaught frontend exceptions are always a defect (they previously left
  // the run button stuck at "computing").
  expect(errs, errs.join('\n')).toEqual([])
})

async function uniqueProject(page: Page, tag: string) {
  const name = `e2e-${tag}-${Date.now()}`
  await page.getByTestId('new-project-name').fill(name)
  await page.getByTestId('new-project').click()
  await expect(page.getByTestId('sheets-panel')).toBeVisible()
  return name
}

async function addNumericVertices(page: Page, tableTestId: string, verts: Array<[number, number]>) {
  for (const [x, y] of verts) {
    await page.getByRole('button', { name: '＋用数字加顶点' }).click()
    const row = page.getByTestId(tableTestId).locator('tbody tr').last()
    await row.locator('input').nth(0).fill(String(x))
    await row.locator('input').nth(1).fill(String(y))
  }
}

async function createSheet(
  page: Page,
  name: string,
  verts: Array<[number, number]>,
  rgb: [number, number, number],
  opacity: number,
) {
  await page.getByTestId('new-sheet').click()
  await page.getByTestId('sheet-name').fill(name)
  await addNumericVertices(page, 'vert-table', verts)
  const colors = page.locator('.color-row input[type="number"]')
  await colors.nth(0).fill(String(rgb[0]))
  await colors.nth(1).fill(String(rgb[1]))
  await colors.nth(2).fill(String(rgb[2]))
  await page.getByTestId('sheet-opacity').fill(String(opacity))
  await page.getByTestId('save-sheet').click()
  await expect(page.getByText('片已添加')).toBeVisible()
}

async function createAct(page: Page, name: string) {
  await page.getByTestId('tab-acts').click()
  await page.getByTestId('new-act').click()
  await page.getByTestId('act-name').fill(name)
}

async function addRegion(
  page: Page,
  kind: 'target' | 'blank',
  name: string,
  verts: Array<[number, number]>,
  rgb?: [number, number, number],
  tol?: number,
) {
  await page.getByTestId(kind === 'target' ? 'add-target' : 'add-blank').click()
  await page.getByTestId('region-name').fill(name)
  await addNumericVertices(page, 'region-vert-table', verts)
  if (kind === 'target' && rgb) {
    const colors = page.locator('.region-edit .color-row input[type="number"]')
    await colors.nth(0).fill(String(rgb[0]))
    await colors.nth(1).fill(String(rgb[1]))
    await colors.nth(2).fill(String(rgb[2]))
    await page.getByTestId('target-tol').fill(String(tol ?? 0))
  }
}

async function addSheetByName(page: Page, name: string) {
  // Only still-available sheets are listed; select by visible label so the
  // option index does not depend on how many sheets were added before.
  await page.getByTestId('add-sheet-select').selectOption({ label: name })
}

const SQUARE: Array<[number, number]> = [[0, 0], [10, 0], [10, 10], [0, 10]]

test('细缝漏色失败 → 共边修正通过 → 确认冻结 → 解冻编辑使报告过期', async ({ page }) => {
  await page.goto('/')
  await uniqueProject(page, 'sliver')

  // Black pane 0..11 intrudes one integer unit into the blank sky at x>=10.
  await createSheet(page, '黑片', [[0, 0], [11, 0], [11, 10], [0, 10]], [0, 0, 0], 1000)

  await createAct(page, '第一幕')
  await addSheetByName(page, '黑片')
  await addRegion(page, 'blank', '夜空留白', [[10, 0], [20, 0], [20, 10], [10, 10]])
  await page.getByTestId('save-act').click()
  await expect(page.getByText('幕已保存')).toBeVisible()

  await page.getByTestId('tab-proof').click()
  await page.getByTestId('run-proof').click()
  await expect(page.getByTestId('proof-status')).toContainText('不合格')
  await expect(page.getByTestId('run-proof')).toContainText('运行校样')
  await expect(page.getByTestId('cell-detail')).toContainText('留白漏色')
  await expect(page.getByTestId('contrib-list')).toContainText('黑片')

  // Shrink the pane to end exactly at x=10: edge contact only, legal.
  await page.getByTestId('tab-sheets').click()
  await expect(page.getByTestId('sheets-panel')).toBeVisible()
  await page.locator('[data-testid^="edit-sheet-"]').first().click()
  const rows = page.getByTestId('vert-table').locator('tbody tr')
  await rows.nth(1).locator('input').nth(0).fill('10')
  await rows.nth(2).locator('input').nth(0).fill('10')
  await page.getByTestId('save-sheet').click()

  await page.getByTestId('tab-proof').click()
  await page.getByTestId('run-proof').click()
  await expect(page.getByTestId('proof-status')).toContainText('全部')
  await expect(page.getByTestId('proof-status')).toContainText('通过')

  // Confirm freezes inputs.
  await page.getByTestId('confirm').click()
  await expect(page.getByTestId('frozen-banner')).toBeVisible()
  // The run button is locked while frozen; on the sheets tab the delete
  // button is disabled too.
  await expect(page.getByTestId('run-proof')).toBeDisabled()
  await page.getByTestId('tab-sheets').click()
  await expect(page.getByRole('button', { name: '删' }).first()).toBeDisabled()
  await page.getByTestId('tab-proof').click()

  // Unfreeze, edit, previous report is expired.
  await page.getByTestId('unfreeze').click()
  await page.getByTestId('tab-sheets').click()
  await page.locator('[data-testid^="edit-sheet-"]').first().click()
  await page.getByTestId('sheet-opacity').fill('500')
  await page.getByTestId('save-sheet').click()
  await page.getByTestId('tab-proof').click()
  await expect(page.getByTestId('report-row').first()).toContainText('已过期')
})

test('半透明叠色按底到顶顺序：蓝顶在红底上得到 (128,64,191)', async ({ page }) => {
  await page.goto('/')
  await uniqueProject(page, 'alpha')
  await createSheet(page, '红半透', SQUARE, [255, 0, 0], 500)
  await createSheet(page, '蓝半透', SQUARE, [0, 0, 255], 500)

  await createAct(page, '叠色幕')
  // Option indices: 0 is the placeholder; sheets appear in creation order.
  await addSheetByName(page, '红半透')
  await addSheetByName(page, '蓝半透')
  await addRegion(page, 'target', '目标', SQUARE, [128, 64, 191], 0)
  await page.getByTestId('save-act').click()

  await page.getByTestId('tab-proof').click()
  await page.getByTestId('run-proof').click()
  await expect(page.getByTestId('proof-status')).toContainText('通过')
})

test('270 度旋转后的片落入目标象限，原象限留白无漏色', async ({ page }) => {
  await page.goto('/')
  await uniqueProject(page, 'rotate')
  await createSheet(page, '片', SQUARE, [33, 44, 55], 1000)

  await createAct(page, '旋转幕')
  await addSheetByName(page, '片')
  await page.getByTestId('rotation').selectOption('270')
  await addRegion(page, 'target', '转后目标', [[0, -10], [10, -10], [10, 0], [0, 0]], [33, 44, 55], 0)
  await addRegion(page, 'blank', '原位留白', SQUARE)
  await page.getByTestId('save-act').click()

  await page.getByTestId('tab-proof').click()
  await page.getByTestId('run-proof').click()
  await expect(page.getByTestId('proof-status')).toContainText('通过')
})

test('多个坏区：两片各在上下漏入同一块留白，报告 2 个坏区', async ({ page }) => {
  await page.goto('/')
  await uniqueProject(page, 'multi')
  await createSheet(page, '上片', [[0, 5], [11, 5], [11, 10], [0, 10]], [0, 0, 0], 1000)
  await createSheet(page, '下片', [[0, 0], [11, 0], [11, 5], [0, 5]], [0, 0, 0], 1000)

  await createAct(page, '双缝幕')
  await addSheetByName(page, '上片')
  await addSheetByName(page, '下片')
  await addRegion(page, 'blank', '夜空', [[10, 0], [20, 0], [20, 10], [10, 10]])
  await page.getByTestId('save-act').click()

  await page.getByTestId('tab-proof').click()
  await page.getByTestId('run-proof').click()
  await expect(page.getByTestId('proof-status')).toContainText('不合格')
  await expect(page.getByRole('button', { name: /坏区 2/ })).toBeVisible()
})

test('自交（蝴蝶结）片整单拒绝', async ({ page }) => {
  await page.goto('/')
  await uniqueProject(page, 'bowtie')
  await createSheet(page, '蝴蝶结', [[0, 0], [10, 10], [10, 0], [0, 10]], [10, 20, 30], 500)

  await createAct(page, '坏幕')
  await addSheetByName(page, '蝴蝶结')
  await addRegion(page, 'blank', '远空白', [[-30, -30], [-20, -30], [-20, -20], [-30, -20]])
  await page.getByTestId('save-act').click()

  await page.getByTestId('tab-proof').click()
  await page.getByTestId('run-proof').click()
  await expect(page.getByTestId('proof-status')).toContainText('输入无效，整单拒绝')
  await expect(page.getByTestId('invalid-reason')).toContainText('self-intersecting')
  await expect(page.getByTestId('run-proof')).toContainText('运行校样')
})

test('画布点击吸附整数坐标', async ({ page }) => {
  await page.goto('/')
  await uniqueProject(page, 'canvas')
  await page.getByTestId('new-sheet').click()
  const canvas = page.locator('canvas').first()
  const box = await canvas.boundingBox()
  if (!box) throw new Error('no canvas')
  for (const [x, y] of [[120, 120], [250, 120], [250, 250], [120, 250]] as Array<[number, number]>) {
    await canvas.click({ position: { x, y } })
  }
  const rows = page.getByTestId('vert-table').locator('tbody tr')
  await expect(rows).toHaveCount(4)
  const vals = await rows.locator('input').evaluateAll((els) =>
    els.map((e) => Number((e as HTMLInputElement).value)),
  )
  for (const v of vals) expect(Number.isInteger(v)).toBeTruthy()
})
