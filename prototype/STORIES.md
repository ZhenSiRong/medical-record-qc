# 前端集成 · 故事清单（WP2 产出，基线=prototype/BASELINE.md）

> **状态（2026-09-21）**：S1-S6 全部完成。E2E 验收截图见 `prototype/screenshots/e2e/`。

> 执行顺序即编号顺序；S3 是基础件，必须在 S4 之前完成并设检查点。

## S1 后端：续借端点
`PUT /api/borrows/:id/renew {days}` — 仅 BORROWED 可续借，due_time 顺延，逾期单拒绝并提示。
**验收**：curl 续借成功/逾期拒绝两例；`go build` + `go vet` 绿。

## S2 后端：时间戳列 + 种子扩充
med_record 加 `archived_at`/`cataloged_at`（AutoMigrate 自动加列；归档上架/编目流转时写入）。
种子扩充至：10 科室、12 医师、ICD-10 24 条 + ICD-9-CM3 11 条（demo 同款码表）、近 6 个月约 90 条病案（状态按月份加权）、12 条借阅单、部分首页+质控结果。
**验收**：空库启动自动灌数；/records/page total≈90；六个月每月均有数据。

## S3 前端：适配层 api-store.js（基础件，检查点 story）
替换 index.html 中的 store.js 引用，导出同构 `window.MRMS`：
- 登录态：sessionStorage 存 token；401 → route='login'；403 → toast 无权限
- bootstrap：分页拉全量 records/front-pages/borrows/batches/oper-logs → 状态投影 → 写入响应式 model
- 投影：病案 NOT_RECYCLED→未归档、RECYCLED→已归档、其余→已编目；reportStatus ← 首页状态映射；批次/借阅状态映射见基线表
- util 字典：departments/doctors/icd10(含 cat 映射)/icd9 → Vue.reactive，从 /depts /doctors /icd-dicts 拉取
- stats 纯函数原样迁移（内存计算）
**检查点（S3 出口）**：登录后六屏全部渲染真数据、无 JS 报错、徽章颜色正确。

## S4 前端：动作链接线
addRecord/archive(批量)/catalog(编目+首页+质控+自动上架链)/addBorrow/approve(=approve+lend)/reject/return/renew/removeRecord(合规提示)/createReport(同步占位+异步回填)/submitReport/reset(不可用提示)；写操作后局部刷新对应集合。
**验收**：登记→归档→编目→借阅→上报全链路走通，数据落 PG，页面即时反映。

## S5 前端：登录页 + 角色
layout.js 加 route='login' 登录卡（admin/deptuser 快捷按钮）、顶栏退出、currentUser 从 /auth/me 回填；删除「重置数据」按钮（D1）。
**验收**：deptuser 登录触发归档类操作 → 403 toast；admin 全通。

## S6 质量门 + 交付
`go build/vet/test`；浏览器全链路逐屏截图归档至 prototype/screenshots/e2e/；API.md 增补 renew 端点；DESIGN.md 状态同步。
**验收**：验收人（用户）过目截图清单即闭环。
