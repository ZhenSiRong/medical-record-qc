# 前端集成方案：适配层接入同事 demo（已含三项决策 + 视觉伴侣验收环）

## 0. 已拍板决策
1. **定位**：适配层集成，同事 demo 成为演示主力，数据走真后端落 PG
2. **状态机**：以后端为准（回收→编目→上架），适配层把状态投影成 demo 的中文词
3. **登录**：加登录页 + 角色切换（admin / deptuser）
4. **工作方式**：视觉伴侣——所有 UI 相关改动用 Browser Use 真渲染截图核对，不盲写

## 1. 前端正式入库
- `.tmp-frontend/medical-record-system` → 移入 `frontend/`（vue.global.prod.js、css、js/components、charts 全部保留）
- 删除 GBK 乱码的单文档版 HTML 和乱码 docs/（内容已被 README 覆盖）
- 组件目标：**除 layout.js（登录改造）外 8 个组件零改动**

## 2. 核心：js/api-store.js 替换数据层
替换 store.js，对外仍是同一个 `window.MRMS`（state/actions/stats/util），脚本顺序不变：
- **stats 纯函数**：从 store.js 原样搬（overview/monthly/byDept/byStatus/filter/validate…），对内存 model 计算，零重写
- **model**：启动时 REST 拉全量驻留内存（records 分页循环拉、borrows、reports、oper_logs→logs）
- **actions**：改 async 调后端，成功后更新 model + toast，失败 toast('err')；createReport 同步返回占位对象（id=period），服务端响应后回填真实 batchNo（report.js 的 watch 按 id 找回，不用改组件）
- **util 字典**：departments/doctors/icd10Dict/icd9Dict 启动时从 /depts /doctors /icd-dicts 拉；icd10 的 cat 分类前端内嵌映射表（后端 icd_dict 无此列，不动表）
- **登录态**：token 存 sessionStorage；fetch 统一带 Authorization；401 → route='login'

## 3. 状态投影表（适配层单向翻译，真实状态仍在库里）
| 后端 | demo 显示 |
|---|---|
| NOT_RECYCLED | 未归档 |
| RECYCLED | 已归档（即"待编目"，喂给编目队列，语义与 demo 完全一致）|
| CATALOGED / ARCHIVED / BORROWED / SEALED | 已编目 |
| 借阅 APPLYING/RETURNED/OVERDUE/REJECTED | 待审批/已归还/已逾期/已驳回 |
| 首页 QC_PASSED→待上报、REPORTED→已上报、其余→未上报 |
| 批次 PENDING→待上报、SUCCESS→已接收、FAILED/PARTIAL→已退回 |

## 4. 动作链映射（demo 一个按钮 = 后端一组真实调用）
- **登记** addRecord → POST /records + PUT /front-pages（诊断/费用落首页表）
- **归档** archive/archiveBatch → POST /archives/recycle（批量、落流水）
- **编目** catalog → POST /catalogs + PUT /front-pages + POST /qc（自动质控）+ 自动上架（POST /archives/shelve 选首个有空位架）
- **借阅"通过"** → PUT approve + PUT lend 链式（折叠后端两步）；驳回/归还各对应一个 PUT
- **续借** renewBorrow → 后端**新增** PUT /api/borrows/:id/renew（约 15 行）
- **删除** removeRecord → 不删，toast「病案依法不得删除」（合规，零组件改动）
- **质量等级** 甲/乙/丙 ← 真实 QC 结果推导：无异常→甲级、仅 WARN→乙级、有 ERROR→丙级
- **新建批次** createReport → 按 period 收集 QC_PASSED 的 recordIds 显式传给 POST /report-batches

## 5. 后端小增量（共 3 处，均不影响既有接口）
1. med_record 加 `archived_at`/`cataloged_at` 两列（AutoMigrate 自动加列；shelve/catalog 时写入）——demo 的 archiveDate/catalogDate/今日编目数需要真数据
2. 新增 PUT /api/borrows/:id/renew（延期 due_time，写 oper_log）
3. 种子扩充：科室 6→10、医师 4→12、ICD10 6→24 条、ICD9 4→11 条（用 demo 同款码表，保证编目表单选码能过后端存在性校验）

## 6. 登录页 + 角色切换（唯一动组件的部分）
- layout.js：新增 route='login' 登录卡片（用户名/密码 + admin/deptuser 快捷按钮），顶栏加退出；currentUser 从 /auth/me 回填（name/role→roleName/dept→deptName），侧栏底部显示真实身份
- 角色差异演示：deptuser 登录后 ADMIN 接口返回 403 → toast 提示无权限（后端已有 RequireRole，前端零权限代码）

## 7. 部署
- `go:embed frontend` 嵌进 Go 二进制：docker-compose 仍是 3 个服务，交付一个 URL（8080），符合轻量要求；本地开发时 go run 即同源服务

## 8. 工作分解
- **P0 打通闭环**：前端入库 → api-store 骨架（登录/拉数/投影）→ 登录页改造 → 登记→归档→编目→借阅→上报全链路走通（真数据落 PG）
- **P1 打磨**：dashboard 用 oper_log 真动态、质量等级推导、续借端点、统计图表真数据核对、种子扩充
- **P2 可选**：go:embed 部署收口、CSV 导出列与后端字段对齐
- **视觉伴侣验收环（贯穿）**：每个里程碑用 Browser Use 打开页面实际操作并截图——登录页样式、状态徽章颜色是否符合投影表、编目选码联动、图表渲染真实数据、上报批次流转；截图给你确认关键节点

## 9. 已知取舍（接受才做）
- 前端全量内存模型：演示规模上限约数百条病案（demo 原本就是此模型，不改组件则无法服务端分页）
- 借阅 APPROVED（待借出）中间态被"通过"按钮折叠，不单独展示
- 病案删除功能禁用（合规）；demo 的 pageCount 字段无后端对应，隐藏或固定值
- 封存/架位管理等后端能力 demo 无界面，仅 API 可用