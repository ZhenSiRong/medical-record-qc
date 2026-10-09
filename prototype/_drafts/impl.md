# 住院病案管理系统 功能实现清单（实现盘点，事实基础）

> 生成方式：仅阅读代码/DDL/接口文档盘点，未运行、未修改任何代码。
> 事实来源：`mrs-backend/internal/{model,handler,service,router,seed}`、`mrs-backend/main.go`、`mrs-backend/cmd/createdb/main.go`、`mrs-backend/sql/schema.sql`、`mrs-backend/docs/API.md`、`frontend/js/api-store.js`、`frontend/js/components/*.js`、`frontend/index.html`、`docker-compose.yml`、`mrs-backend/config.yaml`、`prototype/BASELINE.md`、`DESIGN.md`。
> 页面/接口的「节」编号沿用 `DESIGN.md` §7 的 12 节调研规划口径（节 3 归档 / 节 4 编目 / 节 5 借阅申请审批 / 节 6 借阅台账 / 节 7 统计 / 节 8 首页质控 / 节 9 首页上报 / 节 10-12 系统管理与合规）。

---

## 一、数据模型

共 **20 张表**（`main.go AutoMigrate` 与 `sql/schema.sql` 一一对应）。GORM 实体位于 `internal/model/{record,borrow,report,system}.go`。

| 表名 | 中文含义 | 关键字段 | 所属域 |
|---|---|---|---|
| `med_record` | 病案主档（全系统锚点） | `record_no`(唯一) `patient_name` `gender` `birth_date` `hospital_no` `dept_id/dept_name` `doctor_name` `admit_time` `discharge_time` `status`(主状态机) `shelf_id` `archived_at` `cataloged_at` `remark` | 病案域 |
| `med_record_front_page` | 病案首页（规范字段骨架集+扩展 JSON） | `record_id`(唯一) 患者/住院信息 `actual_days` `admit_diagnosis` `discharge_diagnosis` `main_diagnosis_code` `other_diagnosis_codes` `main_operation_code` `other_operation_codes` `total_cost` `ext_json` `status`(首页子状态机) | 病案域 |
| `front_page_qc_result` | 首页质控结果（逐字段异常标记） | `record_id` `rule_id` `field_name` `error_level`(ERROR/WARN) `error_msg` `fixed` `fixed_at` `fixed_by` | 病案域 |
| `qc_rule` | 质控规则配置（可配置，不硬编码） | `rule_name` `rule_type`(REQUIRED/LOGIC/CODE_EXIST) `target_field` `expression` `error_level` `enabled` | 病案域 |
| `archive_record` | 归档流水（回收/上架/封存留痕） | `record_id` `action`(RECYCLE/ARCHIVE/SEAL) `batch_no` `shelf_id` `operator_name` | 病案域 |
| `shelf` | 病案架位 | `shelf_code`(唯一) `room` `location_desc` `capacity` `used_count` | 病案域 |
| `catalog_entry` | 编目记录（一病案多条诊断/手术） | `record_id` `catalog_type`(DIAGNOSIS/OPERATION) `icd_code` `icd_name` `is_main` `sort_order` `coder_name` | 病案域 |
| `icd_dict` | ICD 编码字典（支持整版换版） | `dict_type`(ICD10/ICD9CM3) `code` `name` `version_id` `enabled`，唯一键(dict_type,code) | 病案域 |
| `icd_dict_version` | 编码库更新记录 | `version` `dict_type` `importer_name` `total_count` `status`(ACTIVE/HISTORY) | 病案域 |
| `borrow_order` | 借阅单 | `order_no`(唯一) `record_id/record_no` `borrower_name` `borrower_dept` `reason` `urgent` `status`(借阅子状态机) `apply_time` `lend_time` `due_time` `return_time` `handler_name` | 借阅域 |
| `borrow_approval` | 借阅审批记录（多级审批链） | `order_id` `level` `approver_name` `result`(APPROVED/REJECTED) `opinion` | 借阅域 |
| `borrow_reminder` | 催还记录 | `order_id` `channel`(SYSTEM) `result` `remark` | 借阅域 |
| `report_batch` | 上报批次 | `batch_no`(唯一) `period`(如 2026-08) `status`(PENDING/SUCCESS/FAILED/PARTIAL) `total_count` `success_count` `fail_count` `submit_time` `operator_name` | 上报域 |
| `report_item` | 上报明细与回执 | `batch_id` `record_id` `receipt_status`(WAITING/SUCCESS/FAIL) `receipt_msg` `receipt_time`，唯一键(batch_id,record_id) | 上报域 |
| `sys_user` | 系统用户 | `username`(唯一) `password`(bcrypt，`json:"-"` 永不下发) `real_name` `dept_id/dept_name` `phone` `role_code`(ADMIN/DEPT_USER) `enabled` | 系统域 |
| `sys_role` | 角色（含数据权限） | `role_code`(唯一) `role_name` `data_scope`(ALL/DEPT) | 系统域 |
| `sys_dept` | 科室 | `dept_code`(唯一) `dept_name` `enabled` | 系统域 |
| `sys_doctor` | 医师字典 | `doctor_code`(唯一) `doctor_name` `dept_id/dept_name` `title` `enabled` | 系统域 |
| `sys_dict` | 通用字典 | `dict_type` `dict_key` `dict_value` `sort_order` | 系统域 |
| `oper_log` | 操作日志审计（写操作自动落库） | `module` `operation` `operator_name` `ip` `status_code` `cost_ms` | 系统域 |

**发现（与「schema.sql 是唯一事实来源」声明不符）**：`schema.sql` 的 `med_record` **缺少** `archived_at` / `cataloged_at` 两列，而 Go 实体 `model.MedRecord` 有 `ArchivedAt`/`CatalogedAt`（`internal/model/record.go:41-42`），归档上架/编目时写入（`archive.go:88`、`catalog.go:65`），STORY S2 亦注明「AutoMigrate 自动加列」。实际部署时由 AutoMigrate 补列，功能可用，但 DDL 文本已落后于模型。

---

## 二、接口清单

路由装配见 `internal/router/router.go`，契约文档 `mrs-backend/docs/API.md`。统一响应 `{code,msg,data}`（`pkg/resp/resp.go`）。权限三档：**公开 / 登录（任意登录用户）/ ADMIN（RequireRole("ADMIN")）**。共约 **71 条路由**。

### 0. 健康检查与认证

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `GET /healthz` | 存活探针 | 公开 | 是 |
| `POST /api/auth/login` | 登录签发 JWT（HS256，默认 72h） | 公开 | 是 |
| `GET /api/auth/me` | 取当前登录用户（密码字段不下发） | 登录 | 是 |

### 1. 病案主档（节 2/3 入口，`handler/record.go`）

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `GET /api/records/page` | 分页查询（keyword/status/deptId） | 登录 | 是 |
| `GET /api/records/:id` | 详情（主档+编目+首页聚合） | 登录 | 是 |
| `POST /api/records` | 病案登记（recordNo 空则自动生成，初始 NOT_RECYCLED） | ADMIN | 是 |
| `PUT /api/records/:id` | 部分更新（仅 NOT_RECYCLED/RECYCLED/CATALOGED 可改，否则 409） | ADMIN | 是 |

### 2. 归档（节 3，`handler/archive.go`）

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `POST /api/archives/recycle` | 批量回收 NOT_RECYCLED→RECYCLED，写流水（同批次号） | ADMIN | 是 |
| `POST /api/archives/shelve` | 批量归档上架 RECYCLED/CATALOGED→ARCHIVED 并绑定架位；架位行锁+容量截断 | ADMIN | 是 |
| `POST /api/archives/seal/:id` | 封存 ARCHIVED→SEALED（仅 ARCHIVED 可封） | ADMIN | 是 |
| `GET /api/archives/records/:id/flow` | 归档流转履历（RECYCLE/ARCHIVE/SEAL 流水） | 登录 | 是 |

### 3. 编目（节 4，`handler/catalog.go`、`handler/icd.go`）

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `GET /api/catalogs?recordId=` | 按病案列编目 | 登录 | 是 |
| `POST /api/catalogs` | 新增编目；编码须存在于字典（否则 409）；首条编目驱动 RECYCLED→CATALOGED | ADMIN | 是 |
| `DELETE /api/catalogs/:id` | 删除编目 | ADMIN | 是 |
| `GET /api/icd-dicts/search` | 编码检索（type=ICD10/ICD9CM3，最多 20 条，Redis 缓存 10min） | 登录 | 是 |
| `GET /api/icd-dicts/page` | 编码库分页 | 登录 | 是 |
| `POST /api/icd-dicts/import` | 编码库整版导入（JSON 列表，旧版转 HISTORY） | ADMIN | 是（JSON 桩，非文件上传） |

### 4. 借阅申请与审批（节 5，`handler/borrow.go`）

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `POST /api/borrows` | 借阅申请（仅 ARCHIVED 可申请，否则 409；dueTime 空则默认 30 天） | 登录 | 是 |
| `PUT /api/borrows/:id/renew` | 续借（仅 BORROWED，days 1-60，逾期单拒绝） | 登录 | 是（STORY S1 补充） |
| `PUT /api/borrows/:id/approve` | 审批（多级，全部通过→APPROVED，任一级驳回→REJECTED） | ADMIN | 是 |
| `PUT /api/borrows/:id/lend` | 借出 APPROVED→BORROWED；病案 ARCHIVED→BORROWED（条件更新防并发） | ADMIN | 是 |
| `PUT /api/borrows/:id/return` | 归还核销 BORROWED/OVERDUE→RETURNED；病案回 ARCHIVED | ADMIN | 是 |

### 5. 借阅台账（节 6，`handler/ledger.go`）

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `GET /api/borrow-ledger/page` | 台账分页（status/keyword/overdueOnly/urgent） | 登录 | 是 |
| `GET /api/borrow-ledger/:id/approvals` | 审批链追溯（按 level 升序） | 登录 | 是 |
| `POST /api/borrow-ledger/:id/remind` | 人工催还登记（写 borrow_reminder） | ADMIN | 是 |

### 6. 首页质控（节 8，`handler/frontpage.go`）

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `GET /api/front-pages/page` | 首页分页（status/keyword） | 登录 | 是 |
| `GET /api/front-pages/:recordId` | 首页数据 + 质控结果列表 | 登录 | 是 |
| `PUT /api/front-pages/:recordId` | 首页 upsert；保存后置 WAIT_QC；REPORTED 禁改(409) | ADMIN | 是 |
| `POST /api/front-pages/:recordId/qc` | 执行质控（规则引擎），置 QC_PASSED/QC_REJECTED | ADMIN | 是 |
| `PUT /api/qc-results/:id/fixed` | 异常项修正确认（标记 fixed/fixed_at/fixed_by） | ADMIN | 是 |

### 7. 首页上报（节 9，`handler/report.go`）

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `GET /api/report-batches/page` | 批次分页（status/period） | 登录 | 是 |
| `GET /api/report-batches/:id/items` | 批次明细与回执（失败原因排查） | 登录 | 是 |
| `POST /api/report-batches` | 创建批次（默认纳入全部 QC_PASSED，可指定 recordIds） | ADMIN | 是 |
| `POST /api/report-batches/:id/submit` | 提交上报（逐条走防腐层，回执落库，批次置 SUCCESS/FAILED/PARTIAL） | ADMIN | 是（本地 XML 桩） |

### 8. 统计分析（节 7，`handler/statistics.go`）

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `GET /api/stats/overview` | 病案各状态分布（Redis 5min） | 登录 | 是 |
| `GET /api/stats/archive-rate` | 归档率（口径=(ARCHIVED+BORROWED)/全部） | 登录 | 是 |
| `GET /api/stats/disease-top` | 疾病谱 Top10（编目诊断计数，Redis 10min） | 登录 | 是 |
| `GET /api/stats/borrow-monthly` | 近 12 个月借出趋势 | 登录 | 是 |
| `POST /api/stats/export` | 报表导出 | 登录 | **否，返回 501**（`statistics.go:118` 待统计口径确认） |

### 9. 系统管理（节 10/11，`handler/system.go`，泛型 CRUD）

`registerCrud[T]` 统一生成 5 条路由/资源：`GET /page`、`GET /:id`（均登录）、`POST`、`PUT /:id`、`DELETE /:id`（均 ADMIN）。

| 资源 | 路径前缀 | 专属查询参数 | 是否已实现 |
|---|---|---|---|
| 科室 | `/api/depts` | keyword | 是 |
| 医师 | `/api/doctors` | keyword | 是 |
| 字典 | `/api/dicts` | type | 是 |
| 角色 | `/api/roles` | — | 是 |
| 架位 | `/api/shelves` | keyword | 是 |
| 用户 | `/api/users` | keyword | 是 |

| 方法 + 路径 | 功能 | 权限 | 是否已实现 |
|---|---|---|---|
| `PUT /api/users/:id/password` | 重置用户密码（bcrypt，≥6 位） | ADMIN | 是 |
| `GET /api/oper-logs/page` | 操作日志查询（module 过滤，只读） | 登录 | 是 |

---

## 三、状态机

### 3.1 病案主状态机（`internal/model/record.go:8-15`，`med_record.status`）

| 取值 | 中文 | 说明 |
|---|---|---|
| `NOT_RECYCLED` | 未回收 | 新登记病案初始态（`record.go:43`、DDL default） |
| `RECYCLED` | 已回收（待编目） | 批量回收后 |
| `CATALOGED` | 已编目 | 首条编目后 |
| `ARCHIVED` | 已归档（在架） | 归档上架后 |
| `BORROWED` | 借出 | 借出后 |
| `SEALED` | 封存 | 封存后（冻结借阅） |

**流转条件（代码事实）**
- `NOT_RECYCLED → RECYCLED`：`POST /archives/recycle`，条件更新 `status=NOT_RECYCLED`（`archive.go:37`）。
- `RECYCLED → CATALOGED`：`POST /catalogs` 首条编目时条件更新（`catalog.go:63-65`）。
- `RECYCLED|CATALOGED → ARCHIVED`：`POST /archives/shelve` 条件更新 + 写 `shelf_id`/`archived_at`（`archive.go:86-88`）。
- `ARCHIVED → BORROWED`：`PUT /borrows/:id/lend` 条件更新（`borrow.go:125-127`）。
- `BORROWED → ARCHIVED`：`PUT /borrows/:id/return` 条件更新（`borrow.go:156-158`）。
- `ARCHIVED → SEALED`：`POST /archives/seal/:id` 条件更新，仅 ARCHIVED（`archive.go:117-119`）。
- 注：`BORROWED ⇄ ARCHIVED` 构成借出-归还回环；`SEALED` 为终态（代码中无离开路径）。

### 3.2 借阅子状态机（`internal/model/borrow.go:8-15`，`borrow_order.status`）

| 取值 | 中文 | 说明 |
|---|---|---|
| `APPLYING` | 审批中（多级） | 申请初始态 |
| `APPROVED` | 已批准（待借出） | 各级通过 |
| `REJECTED` | 已驳回 | 任一级驳回 |
| `BORROWED` | 借出 | 借出后 |
| `OVERDUE` | 逾期 | 定时任务标记 |
| `RETURNED` | 已归还 | 归还核销 |

**流转条件（代码事实）**
- `→ APPLYING`：`POST /borrows`，仅 `ARCHIVED` 病案可申请（`borrow.go:43-44,58`）。
- `APPLYING → APPROVED`：`PUT /borrows/:id/approve` 且 `result=APPROVED` 且累计审批级数 `≥ cfg.Borrow.ApproveLevels`（默认 1 级，`borrow.go:102-103`）；否则保持 APPLYING（多级续审）。
- `APPLYING → REJECTED`：同接口 `result=REJECTED`（`borrow.go:99-100`）。
- `APPROVED → BORROWED`：`PUT /borrows/:id/lend`，仅 APPROVED（`borrow.go:122-123`），同步病案 ARCHIVED→BORROWED。
- `BORROWED → OVERDUE`：`service/overdue.go` 进程内 ticker（默认 60min，启动即扫一次），条件更新 `status=BORROWED AND due_time<now`，并写 `borrow_reminder`（`overdue.go:32-48`）。
- `BORROWED|OVERDUE → RETURNED`：`PUT /borrows/:id/return`（`borrow.go:153`），同步病案回 ARCHIVED。
- 续借 `PUT /borrows/:id/renew`：仅 `BORROWED`，`due_time += days`（不改状态；逾期单拒绝，`borrow.go:190-191`）。

### 3.3 首页数据子状态机（`internal/model/record.go:17-25`，`med_record_front_page.status`）

| 取值 | 中文 | 说明 |
|---|---|---|
| `DRAFT` | 草稿 | DDL 默认值 |
| `WAIT_QC` | 待质控 | 保存首页后 |
| `QC_PASSED` | 质控通过（可上报） | 无 ERROR |
| `QC_REJECTED` | 质控退回 | 命中 ERROR |
| `REPORTED` | 已上报 | 上报成功后 |

**流转条件（代码事实）**
- `→ WAIT_QC`：`PUT /front-pages/:recordId` upsert 后强制置 WAIT_QC（`frontpage.go:68,76,80`）；**代码中无显式 DRAFT→WAIT_QC 之外的入口**，DRAFT 仅存在于 DDL 默认值。
- `WAIT_QC → QC_PASSED / QC_REJECTED`：`POST /front-pages/:recordId/qc` 跑 `service.RunQc`；`errCount>0`（命中 ERROR 级规则）→ QC_REJECTED，否则 QC_PASSED（`qcengine.go:68-73`），并**先清空该病案旧质控结果再写入**（`qcengine.go:60`）。
- `QC_REJECTED → WAIT_QC`：修正后重新 `PUT /front-pages/:recordId` 保存（回到待质控，形成闭环）。
- `QC_PASSED → REPORTED`：`POST /report-batches/:id/submit` 中该条回执为 SUCCESS 时，更新首页 `status=REPORTED`（`report.go:110-112`）。
- `REPORTED → *`：`PUT /front-pages` 遇 REPORTED 直接 409「首页数据已上报，禁止修改」（`frontpage.go:63-65`），REPORTED 为终态。

### 3.4 上报批次/明细子状态（`internal/model/report.go:8-17`）
- 批次 `status`：`PENDING`（初始）→ `SUCCESS`（fail=0）／`FAILED`（success=0）／`PARTIAL`（部分成功）（`report.go:121-128`）；已提交批次再次提交 409（`report.go:86-88`）。
- 明细 `receipt_status`：`WAITING`（初始）→ `SUCCESS`／`FAIL`（`report.go:104-116`）。

---

## 四、前端结构

技术形态：**纯静态、零构建、零第三方依赖**的 Vue 3 单页应用（`lib/vue.global.prod.js` 本地引入），脚本按 `index.html` 顺序加载。入口 `js/app.js` 全局注册组件并挂载 `#app`；数据层 `js/api-store.js`（REST 适配层，替代旧 `js/store.js`），导出同构 `window.MRMS`。

| 模块名 | 组件文件 | 核心功能 | 对应调研节 |
|---|---|---|---|
| 登录页 | `js/components/layout.js`（`MRPages.Login`） | 用户名/密码登录、admin/deptuser 快捷登录、token 存 sessionStorage | 基础框架（节 10 权限前提） |
| 应用外壳 | `js/components/layout.js`（`MRApp`） | 侧边导航（6 项）、顶栏、内容区动态组件路由、侧栏待办角标、全局 toast | 菜单结构（`DESIGN.md` §7） |
| 工作台 | `js/components/dashboard.js` | 4 项 KPI（病案总数/归档率/编目率/本月出院）、月度归档趋势、流转状态环形、科室 Top7、待办中心、上报进度、**最近操作动态（读 `/oper-logs/page`）** | 无独立节（跨模块总览） |
| 病案归档 | `js/components/archive.js` | 病案登记（含 ICD 选码）、多条件检索/分页、单个+批量归档、详情抽屉、CSV 导出、删除（禁用+合规提示） | 节 3 |
| 病案编目 | `js/components/catalog.js` | 待编目队列、编目工作台（ICD-10/ICD-9-CM-3 选码）、质量等级、ICD 编码库检索、最近编目。**编目动作链内部串联：编目→首页保存→执行质控→质控通过自动上架**（`api-store.js:411-460`） | 节 4（并承载节 8 质控、节 3 上架） |
| 病案借阅 | `js/components/borrow.js` | 借阅指标、申请弹窗、审批通过/驳回、归还、续借、动态逾期计算、CSV 导出台账 | 节 5 + 节 6（合并一页） |
| 数据统计 | `js/components/statistics.js` | 病案量趋势、疾病分类构成、流转状态、质量等级、住院天数/费用区间、诊断 Top5、科室明细表、CSV 导出 | 节 7 |
| 首页数据上报 | `js/components/report.js` | 批次列表/新建批次、上报前完整性校验面板、执行上报、数据包 JSON 导出、问题清单导出、首页字段规范表 | 节 9 |
| 通用组件 | `js/components/common.js` | 分页 `MrPager`、空状态 `MrEmpty`、模态 `MrModal`、进度条 `MrProgress`、状态色映射 `mrTone` | 基础件 |
| 图表组件 | `js/components/charts.js` | 手写 SVG：折线/面积 `MrLineChart`、柱状 `MrBarChart`、环形 `MrDonutChart` | 基础件 |
| 主题层 | `css/theme-xiayi.css` + `js/theme-xiayi.js` | 侠医配色对齐层（交互蓝 #1898f8 / 品牌青 #009088 / 炭灰侧栏 #283038，来自像素实测） | 原型基线 D9 |

**状态投影层（`api-store.js:67-77`，关键发现）**：适配层把后端枚举投影为界面中文词，**存在信息折叠**：
- `REC_PROJ`：`NOT_RECYCLED→未归档`，`RECYCLED→已归档`，**`CATALOGED`/`ARCHIVED`/`BORROWED`/`SEALED` 全部投影为「已编目」**——前端界面因此**无法区分**已编目/在架/借出/封存四种后端状态。
- `BOR_PROJ`：`APPROVED` 与 `BORROWED` 均投影为「已借出」（中间态折叠）。
- `BAT_PROJ`：`FAILED` 与 `PARTIAL` 均投影为「已退回」。
- 质量等级（甲/乙/丙）由首页 `status` 推导（`QC_PASSED→甲级`、`QC_REJECTED→丙级`、`WAIT_QC→乙级`），或由编目动作链的质控结果即时推导（`api-store.js:119-121,445`）。

---

## 五、实现三态表

### A. 已完整实现（后端 API + 前端界面，可走通闭环）

| 能力 | 后端 | 前端 |
|---|---|---|
| 登录/JWT 认证/角色区分（ADMIN/DEPT_USER） | `auth.go`、`middleware/jwt.go` | `layout.js` 登录页 + 快捷登录 |
| 病案登记 / 查询 / 修改 | `record.go` | `archive.js` |
| 批量回收、单个+批量归档 | `archive.go`（recycle/shelve） | `archive.js`（归档按钮、批量归档） |
| 编目（诊断/手术选码 + 编码存在性校验） | `catalog.go` | `catalog.js` 编目工作台 |
| 首页质控执行 + 异常落库 + 状态流转 | `frontpage.go`、`service/qcengine.go` | `catalog.js` 编目动作链内自动触发（无独立质控页） |
| 质控通过自动上架 | `archive.go`（shelve） | `api-store.js` 编目链自动选空架位 |
| 借阅申请/审批/借出/归还/续借 | `borrow.go` | `borrow.js` |
| 借阅台账/逾期视角 | `ledger.go` | `borrow.js`（前端动态计算逾期天数并标红） |
| 上报批次创建 + 提交 + 回执 + 状态回写 | `report.go`、`service/reportclient.go`（本地桩） | `report.js` |
| 统计（状态分布/归档率/疾病谱/借阅趋势） | `statistics.go` 4 个只读接口 | `statistics.js` + `dashboard.js`（另大量指标由前端内存计算） |
| 操作日志审计与展示 | `middleware/operlog.go` + `system.go` 查询 | `dashboard.js` 最近操作动态 |
| ICD 编码检索（Redis 加速） | `icd.go` Search | `archive.js`/`catalog.js` 选码下拉 |
| 字典下拉数据（科室/医师/ICD 码） | `system.go`、`icd.go` | `api-store.js` bootstrap 回填 |

### B. 仅后端 API，无前端界面（BASELINE 已冻结为「仅 API」或未接页面）

| 能力 | 接口 | 说明 |
|---|---|---|
| 架位管理 CRUD | `/api/shelves` 系列 | 前端仅只读调用 `GET /shelves/page` 自动选空架，无管理界面 |
| 病案封存 | `POST /api/archives/seal/:id` | 无封存按钮/入口 |
| 归档流转履历 | `GET /api/archives/records/:id/flow` | 无履历查看入口 |
| 借阅审批链追溯 | `GET /api/borrow-ledger/:id/approvals` | 无审批链展示（多级审批节点不可见） |
| 人工催还 | `POST /api/borrow-ledger/:id/remind` | 无催还按钮/记录展示 |
| 首页质控独立表单/异常修正界面 | `/api/front-pages/*`、`/api/qc-results/:id/fixed` | 首页表单与异常修正仅被编目动作链间接调用，无独立质控工作台页面 |
| 编码库整版导入 | `POST /api/icd-dicts/import` | 无导入界面 |
| 系统管理全部 CRUD | `/api/{depts,doctors,dicts,roles,users}` 写操作 | **无系统管理页面**（前端只读拉字典） |
| 用户改密 | `PUT /api/users/:id/password` | 无界面 |
| 病案删除 | —（后端本就无删除接口） | 前端按钮禁用 + 合规提示（D2） |
| 统计导出 | `POST /api/stats/export` | 前端仅本地 CSV 导出，未调用该接口 |

> 说明：BASELINE「全局不做清单」明确 **封存/架位/日志界面仅 API**；而系统管理 CRUD 有完整 API 但前端无页面，是本盘点中「后端超出前端」最集中的一处。

### C. 明确未做 / 显式占位

| 事项 | 代码位置 | 状态 |
|---|---|---|
| 统计报表导出（Excel/CSV 服务端） | `statistics.go:118` `Export` | **返回 501**，待统计口径确认 |
| 科室级数据隔离 | `sys_role.data_scope` 字段存在 | **未生效**，登录用户可见全量（API.md 待确认事项 4） |
| 官方上报接口对接 | `service/reportclient.go` `LocalStubReportClient` | 本地 XML 落盘桩，官方文档到位后替换 |
| 编码库文件（CSV/Excel）上传 | `icd.go:70-71` TODO | 当前接收 JSON 列表 |
| 质控规则表达式引擎 | `qcengine.go:113-128` `checkLogic` | 仅内置 2 条逻辑表达式（`discharge_time>admit_time`、`actual_days>0`），其余表达式打印「未实现」跳过 |
| 催还通知渠道（短信/站内信） | `overdue.go:14` TODO | 仅落库记录 |
| 病案复印服务 | BASELINE D7 | 本期不做，仅留位 |
| DRG/DIP 预分组、无纸化/HIS-EMR 采集、条码/RFID 示踪 | BASELINE 全局不做清单 | 未做 |
| 服务端分页 | BASELINE 冻结 | 前端一次性拉全量（`fetchAll` size=100 循环）后内存分页 |
| go:embed 前端单二进制 | README「P2 收口项」 | 未做，当前需外挂静态服务器 |
| 上报中状态 `REPORTING` | `report.go` 无此态 | 批次状态机仅 PENDING/SUCCESS/FAILED/PARTIAL（DESIGN 曾提「上报中」，代码未实现） |

---

## 六、技术栈与运行形态

### 6.1 技术选型

| 层 | 选型 | 依据 |
|---|---|---|
| 后端语言/框架 | Go + Gin（`gin.New()`，ReleaseMode） | `main.go:30-31,58` |
| ORM | GORM + `gorm.io/driver/postgres`；`AutoMigrate` 建表 | `main.go:38-46` |
| 认证 | JWT（`golang-jwt/jwt/v5`，HS256，secret 默认 `mrs-dev-secret-change-me`，72h） | `config.yaml`、`middleware/jwt.go` |
| 密码 | bcrypt `DefaultCost`，`json:"-"` 不下发 | `seed.go:51`、`auth.go:35` |
| 数据库 | PostgreSQL（容器 `postgres:16-alpine`；本地可用 `tools/pg` 便携版 PG16） | `docker-compose.yml`、README |
| 缓存 | Redis（`go-redis/v9`）；用于 ICD 检索、统计热点；**连不上自动降级直查**（`cache.go:24-27`） | `internal/cache/cache.go` |
| 配置 | 三级覆盖：代码默认 ← `config.yaml` ← `MRS_*` 环境变量 | `config.go:10,54-65` |
| 定时任务 | 进程内 `time.Ticker`（逾期扫描，默认 60min，启动即扫一次） | `service/overdue.go:19-28` |
| 前端框架 | Vue 3（global prod 构建，本地化离线） | `index.html:16` |
| 前端依赖 | **零第三方**（无 Element Plus/ECharts/Axios），图表手写 SVG | `charts.js`、`frontend/README.md` |
| 前端数据层 | `Vue.reactive` 轻量 store（替代 Pinia）；`api-store.js` REST 适配层 | `api-store.js` |
| 前端样式 | CSS Grid + 媒体查询；`app.css` + `theme-xiayi.css` 主题层 | `index.html:8-10` |
| 部署 | Docker Compose（postgres + redis + backend 三容器，后端多阶段构建 alpine 单镜像） | `docker-compose.yml`、`mrs-backend/Dockerfile` |
| 响应外壳 | 统一 `{code,msg,data}`；`code=0` 成功；分页 `{list,total}`，size 上限 100 | `pkg/resp/resp.go` |

### 6.2 运行形态

- **容器形态**：`docker compose up -d --build` → pg 首次执行 `sql/schema.sql` 建表 → 后端 `AutoMigrate` 补差 → 空库灌种子 → 起 Redis → 挂路由 → 起逾期扫描器；后端服务 `:8080`。
- **本地免 Docker 形态**：便携 PG（`tools/pg`）+ `go run ./cmd/createdb` 建库 + `cd mrs-backend && go run .`；Redis 缺失自动降级。
- **前端形态**：`index.html` 直接双击，或用 `python -m http.server 8090` 静态服务（此时 `api-store.js` 自动把 API 指向 `host:8080`，见 `api-store.js:13`）；**非同源、非 embed**，依赖后端 CORS 全开放。
- **CORS**：`middleware/cors.go` 全开放（API.md 声明「前端任意本地端口可联调」）。
- **上报产物**：本地桩 XML 落盘 `report-out/批次号/病案号.xml`（容器挂载 `./data/report-out`）。

### 6.3 种子数据规模（实际数字，`internal/seed/seed.go`）

| 实体 | 数量 | 依据 |
|---|---|---|
| 角色 `sys_role` | 2（ADMIN 病案管理员 / DEPT_USER 科室用户） | `seed.go:30-34` |
| 科室 `sys_dept` | **10**（心血管内科…泌尿外科，编码 D01–D10） | `seed.go:37-45` |
| 用户 `sys_user` | 2（admin/admin123；deptuser/123456） | `seed.go:55-59` |
| 医师 `sys_doctor` | **12**（张建国…何静怡，D101–D112，3 种职称轮转） | `seed.go:62-74` |
| 通用字典 `sys_dict` | **9**（gender×2 + borrow_reason×7） | `seed.go:78-90` |
| ICD-10 `icd_dict` | **24** | `seed.go:94-103` |
| ICD-9-CM3 `icd_dict` | **11** | `seed.go:104-109` |
| 编码库版本 `icd_dict_version` | 2（ICD-10-DEMO-2024、ICD-9CM3-DEMO-2024，ACTIVE） | `seed.go:110-115` |
| 质控规则 `qc_rule` | **8**（4 必填 + 2 逻辑 + 2 编码存在性） | `seed.go:134-145` |
| 架位 `shelf` | **3**（A-01/A-02 各 60、B-01 80） | `seed.go:147-153` |
| 病案 `med_record` | 近 6 个月，前 5 月各 14–18 条、本月 8–12 条；**约 90 条**（区间 78–102，固定种子 `20260921`） | `seed.go:180-186` |
| 首页 `med_record_front_page` | 状态 ≥ RECYCLED 的病案各 1 条 | `seed.go:308-345` |
| 编目 `catalog_entry` | 状态 ≥ RECYCLED 的病案各 1 诊断 + 约 70% 加 1 手术 | `seed.go:313-322` |
| 借阅单 `borrow_order` | 代码注释写「12 条」，**实际为 `len(借出中病案) + min(12, 在架病案数)` 条（约 19 条，README 记 19）** | `seed.go:358-435` |
| 历史上报批次 `report_batch` | **2**（前第 2、前第 3 个自然月，回执全成功），明细 `report_item` 对应各批次病案数 | `seed.go:452-486` |

**种子相关发现**：
1. 借阅单数量：`seed.go:358` 注释「借阅单（12 条」与实际生成逻辑（借出病案各 1 单 + 在架病案最多补 12 单）不符，实际约 19 条。
2. 病案数：README 写「约 87 份」、`frontend/README.md`（旧 demo）写「约 108 份」、`seed.go:18` 注释写「约 90 条」，三者口径不一致；以种子逻辑看约 90 条最接近。
3. 种子上报批次会把对应首页置为 `REPORTED`（`seed.go:480`），故历史月份病案上报为「已上报」。

---

## 七、附：本盘点发现的「与预期不符 / 需注意」清单

1. **schema.sql 与模型不一致**：`med_record` 缺 `archived_at`/`cataloged_at` 两列（模型有、流水写入用），依赖 AutoMigrate 补列，DDL 文本待同步。
2. **前端状态投影丢失信息**：`REC_PROJ` 把 `CATALOGED/ARCHIVED/BORROWED/SEALED` 全投影为「已编目」，界面无法区分在架/借出/封存（`api-store.js:68-71`）。
3. **节 8 首页质控无独立页面**：质控执行、首页保存、异常修正全部内嵌在编目动作链，前端无「首页质控工作台」；§5.B 中同列。
4. **节 10/11 系统管理无前端页面**：`/api/{users,roles,depts,doctors,dicts}` 后端 CRUD 齐全，前端零界面。
5. **`js/store.js` 为遗留死代码**：`index.html` 只加载 `api-store.js`，`store.js`（localStorage 模拟层）不再被引用，但文件仍存在；`api-store.js` 顶部注释「8 个页面组件」与实际 6 页 + 登录不符。
6. **`statistics.js`/`dashboard.js` 大量指标为前端内存计算**（`MRMS.stats.*`），与后端 `/stats/*` 四接口并存，两套口径可能不一致（后端归档率=(ARCHIVED+BORROWED)/全部；前端归档率=非「未归档」/全部，因投影折叠两者未必等价）。
7. **上报批次状态无 `REPORTING`（上报中）**：DESIGN §3.3 曾提「上报中」，代码仅 PENDING 直接跳终态。
8. **借阅单种子数量与注释不符**（见六.3）。
9. **登录返回体含完整用户对象且 `password` 用 `json:"-"` 屏蔽**——正确；但用户列表 `GET /api/users/page` 亦靠同一 tag 屏蔽，未见其它泄露点。
10. **多级审批已具备数据模型与逻辑，但前端不可见审批链**：`cfg.Borrow.ApproveLevels` 默认 1 级（`config.yaml`），`borrow_approval` 逐级落库，但无界面展示（§5.B）。
