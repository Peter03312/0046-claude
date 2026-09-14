# 🎬 透明故事校样台 StoryProof

七至十岁的孩子和家长把彩色透明片一层层叠成分场故事。窄窄的错位会让本该留白的夜空漏出颜色，而**对屏幕像素取样**又可能因为对齐运气把这种细缝漏掉、或把共边误判成漏色。

StoryProof 是一个**零像素抽样**的全栈校样台：

- 所有片形与检查区域都是**整数平面**上的简单多边形；
- 每幕指定启用哪些片、底→顶层序、绕原点的 **0/90/180/270°** 旋转与整数平移；
- 后端把变换后的片形与目标色区/留白区做**精确有理数平面细分**（`math/big`，无浮点）；
- 颜色在白底上直接对 **8 位 RGB 编码值**做 source-over，通道全程保持有理分数，**只在最终结果做一次 half-up 取整**；
- **目标区**：每个正面积细分单元每通道 `|实得−期望| ≤ 容差`；
- **留白区**：不得与任何片有**正面积**交叠——共边、共点面积为零，合法；
- 失败时只返回**最早不合格幕**的全部坏单元：外环（逆时针）、内环（顺时针）、各环从坐标字典序最小点起、实得颜色、贡献片（底→顶）；
- 自交/自触/重边等不合法多边形输入**整单拒绝**（422），不出报告性结论；
- 任何编辑都会使旧报告**过期**；通过报告被**确认**时冻结输入，改稿需显式解冻。

## 目录

```
backend/            Go · Chi · SQLite（纯 Go 驱动 modernc.org/sqlite，免 cgo）
  geom/             精确有理数几何引擎（细分、面提取、source-over、取整）
  store/            SQLite 持久层（项目/片/幕/区/报告、过期与冻结）
  api/              Chi HTTP API + 集成测试
  cmd/server/       API 服务入口
web/                Vue 3 + TypeScript + Vite
  src/components/   整数画布编辑器、校样画布（坏区/贡献层）、各面板
  e2e/              Playwright 浏览器测试
scripts/            e2e-api.sh（起真实 Go API）、verify.sh（容器内总验证）
docker/nginx.conf   生产静态托管 + /api 反代
Dockerfile          多目标：api / web / verify
docker-compose.yml  web、api 常驻 + 一次性 verify
```

## 快速开始（Docker Compose）

```bash
# 常驻组件：web(nginx+静态产物) 与 api(Go+SQLite)
WEB_PORT=8081 API_PORT=8080 docker compose up --build

# 打开 http://localhost:8081
# 健康检查：     curl http://localhost:8080/api/health
# 经 web 反代：  curl http://localhost:8081/api/health
```

`WEB_PORT` / `API_PORT` 控制宿主映射（默认 8081 / 8080）。SQLite 数据在
`storyproof-data` 卷中。

一次性总验证（Go 测试 → Playwright 浏览器测试 → 生产构建 → 对运行中的
web/api 做 HTTP 冒烟）：

```bash
docker compose --profile verify build verify
docker compose up -d web api
docker compose --profile verify run --rm verify
```

## 本地开发（无 Docker）

需要 Go ≥ 1.25、Node ≥ 20。

```bash
# 1) 后端（:8080，SQLite 默认写到 ./data）
cd backend
go test ./...
ADDR=:8080 DB_DIR=./data go run ./cmd/server

# 2) 前端（:5173，/api 代理到 8080）
cd web
npm ci
npm run dev          # 类型检查 + 构建： npm run build

# 3) 浏览器测试：脚本自动起一个临时 SQLite 的真实 Go API
go build -o ./bin/api ./cmd/server            # 可选；不给则用 go run
STORYPROOF_API_BIN=$(pwd)/../bin/api \
  ../scripts/e2e-api.sh npx playwright test
```

## HTTP API 摘要

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/projects` | 新建项目 |
| GET | `/api/projects/{id}` | 项目 + 全部片 + 全部幕 |
| POST/GET | `/api/projects/{id}/sheets` | 片列表/新建（编辑即过期） |
| PUT/DELETE | `/api/sheets/{id}` | 改/删片 |
| POST/GET | `/api/projects/{id}/acts` | 幕列表/新建（启用片、层序、旋转、平移、区） |
| PUT/DELETE | `/api/acts/{id}` | 改/删幕 |
| POST | `/api/projects/{id}/proofs` | 运行校样；201 成功（通过/不合格），422 输入无效 |
| GET | `/api/reports/{id}` | 报告全文（过期返回 410，仍附正文） |
| POST | `/api/reports/{id}/confirm` | 通过报告确认 → 冻结输入 |
| POST | `/api/projects/{id}/unfreeze` | 解冻并使已确认报告过期 |

片示例：

```json
{
  "name": "夜空蓝片",
  "vertices": [{"x":0,"y":0},{"x":10,"y":0},{"x":10,"y":10},{"x":0,"y":10}],
  "r": 12, "g": 28, "b": 80, "opacityMillis": 650
}
```

幕中启用片：数组顺序即底→顶层序，`rotation ∈ {0,90,180,270}`（逆时针绕
原点），`tx,ty` 为整数平移。区的 `kind` 为 `target`（带 `r,g,b,tolerance`）
或 `blank`。

## 为什么不能抽样

考虑一片右边界在 `x=10`（整数 10）而留白区左边界也在 `x=10`：两者共边，
**面积交为零**，不漏色。若片被画到 `x=11`，则存在 `[10,11]×[0,10]`
这一整单位的正面积漏色；若边界相交于有理点（如斜边交于 `x=17/3`），
漏色宽度可能小于任何固定像素栅格。引擎把所有边界在精确有理交点处切开，
对每个面取一个严格在面内的代表点分类，因此：

- 宽度为 1 或 `p/q` 的细缝**必然**作为正面积单元出现；
- 共边、共点只产生零面积接触，**永远**不会被报成漏色；
- 细分坐标在 API 中以分数字符串（`"17/3"`、`"-5"`）无损往返。

## 坏单元（BadCell）载荷

- `kind`：`leak`（正面积侵入留白）或 `color`（目标色不符）；
- `outer`：坏面外环，**逆时针**；`holes`：内环，**顺时针**；
- 每环都从坐标**字典序最小**点开始，坐标为精确分数字符串；
- `actual`：白底 source-over 后、仅最终 half-up 取整的 8 位 RGB；
- `expected` / `tolerance`：目标色与每通道容差；
- `contributors`：覆盖该面的片，按底→顶给出颜色与千分不透明度。

## 测试覆盖

- `backend/geom`：细缝（整数宽与有理宽）、共边、共点、内环方向、
  半透明多层叠色、仅最终 half-up 取整（三层 1‰ 的反例）、旋转/平移、
  多个坏区、蝴蝶结/捏点/重叠边等自交输入拒绝、容差按通道判定、分数 JSON。
- `backend/api`、`backend/store`：完整证明生命周期（过期/冻结/解冻）、
  最早不合格幕、422、旋转、校验错误、410 过期报告。
- `web/e2e`（Chromium）：细缝失败 → 改成共边通过 → 确认冻结 → 解冻编辑后
  报告过期、半透明叠色 `(128,64,191)`、270° 旋转 + 原位留白、多个坏区、
  蝴蝶结整单拒绝、画布点击吸附整数；每个测试结束断言无前端未捕获异常。
