# 病案管理系统 API 契约（前端集成依据）

> 本文档是前端开发的唯一接口依据，与后端代码同步维护。接口变更请同步更新本文件。
>
> **本轮同步（2026-10-09）**：补齐功能扩展批次 1 的 **22 条**新接口，并修正 5 处与实现不符的旧描述（见文末「变更记录」）。含 D3（首页条件必填真字段）与 D4（医生端「我的待整改」）落地后的接口变更。
> 路由总量 **94 条**（`/api` 下 93 条 + `/healthz`），旧文档只覆盖约 46 条。
> **核对方式（自动，可重复）**：`go test ./internal/router/ -count=1` 内含 `TestApiCoverage`，自动做「文档 ↔ 真实路由」双向比对（当前 **94/94 = 100%**）；另有 `python tools/check-api-coverage.py` 供人工复核。**必须带 `-count=1`**——本测试读取 module 之外的 `frontend/` 与文档缓存无关，但同一包的前端调用核对测试读 `frontend/`，缓存会导致假绿灯。
>
> - Base URL：`http://localhost:8080`（`127.0.0.1:8080` 等价）
> - 全部业务接口挂 `/api` 前缀；除登录与 `/healthz` 外均需请求头 `Authorization: Bearer <token>`
> - 统一响应：`{ "code": 0, "msg": "ok", "data": ... }`；`code=0` 成功；非 0 时 `code` 与 HTTP 状态码一致（400 参数 / 401 未登录 / 403 无权限 / 404 不存在 / 409 状态冲突 / 500 服务器错误 / 501 未实现）
> - 分页接口统一参数 `page`（默认 1）、`size`（默认 10，**传 >100 或 <1 会被重置为 10**），返回 `data: { list: [...], total: n }`，列表按 `id DESC` 排序
> - 时间字段一律 RFC3339（如 `2026-09-30T09:00:00+08:00`），可空字段为 `null`
> - CORS 已全开放，前端任意本地端口可直接联调
> - 权限三档：**公开**（无 token）/ **登录**（任意登录用户）/ **ADMIN**（`roleCode=ADMIN`，否则 403）

## 目录

| 节 | 域 | 接口数 | 备注 |
|---|---|---|---|
| 0 | 健康检查 | 1 | `/healthz`，公开 |
| 1 | 认证 | 2 | |
| 2 | 病案主档 | 4 | |
| 3 | 归档 | 4 + 架位 CRUD | |
| 4 | 编目与编码库 | 6 | |
| 5 | 借阅申请与审批 | 5 | |
| 6 | 借阅台账 | 3 | |
| 7 | 统计分析 | 5 | `/stats/export` 为 501 占位 |
| 8 | 首页质控 | 5 | |
| 9 | 首页上报 | 4 | |
| 10 | 系统管理 | 32 | 6 类资源 × 5 + 日志 + 改密 |
| 11 | **扩展：智能编码辅助**（批次 1） | 5 | |
| 12 | **扩展：DRG/DIP 预检**（批次 1） | 5 | |
| 13 | **扩展：质量评分与退回返修闭环**（批次 1 + D4） | 7 | 含 D4 新增的 `my-pending` |
| 14 | **扩展：归档及时率与超期催缴**（批次 1） | 2 | |
| 15 | **扩展：上报一致性核对**（批次 1） | 2 | |
| 16 | **扩展：上报导出**（批次 1） | 2 | |
| 17 | 前端集成现状 | — | 哪些接口已被 `api-store.js` 调用 |

## 账号（种子数据）

| 账号 | 密码 | 角色 | 能力 |
|---|---|---|---|
| `admin` | `admin123` | ADMIN 病案管理员 | 全部操作（归档/编目/审批/上报/系统管理） |
| `deptuser` | `123456` | DEPT_USER 科室用户 | 查询、借阅申请、统计查看、提交整改 |

## 菜单 → 模块对照（可直接做前端路由/侧边栏）

| 菜单（12 节课件对应节） | 前端页面建议 | 主要接口 |
|---|---|---|
| 病案归档（节 3） | 病案列表 + 批量回收/上架 + 架位管理 + 流转履历 | `/records` `/archives` `/shelves` |
| 病案编目（节 4） | 编目工作台（选码）+ 编码库维护 + **智能推荐** | `/catalogs` `/icd-dicts` `/code-suggest` |
| 借阅申请与审批（节 5） | 我的申请 / 待审批列表 | `/borrows` |
| 借阅台账（节 6） | 台账 + 逾期筛选 + 催还 | `/borrow-ledger` |
| 统计分析（节 7） | 图表看板 + **归档及时率** | `/stats/*` `/archive-timeliness` |
| 首页质控（节 8） | 首页表单 + 异常列表 + 修正 + **质控工作台/退回返修** | `/front-pages` `/qc-results` |
| 首页上报（节 9） | 批次列表 + 明细回执 + **导出/一致性核对** | `/report-batches` `/report-consistency` |
| 系统管理（节 10/11/12） | 用户/角色/科室/医师/字典/日志 | `/users` `/roles` `/depts` `/doctors` `/dicts` `/oper-logs` |
| **质量看板**（批次 1 新增整页） | 归档时效专项：及时率指标 + 月度趋势 + 科室排名 + 超期催缴清单 | `/archive-timeliness` `/archive-overdue` |
| **我的待整改 / 整改督办**（D4 新增整页） | 临床侧：本科室待整改项 + 提交整改；病案室侧：全院督办视角（同一页面按角色切换名称） | `/qc-results/my-pending` `/qc-results/:id/rectify` |

---

## 0. 健康检查

### GET /healthz （公开，**不在 `/api` 前缀下**）
→ `data: { "status": "up" }`
用途：服务存活探针（部署与演示脚本用）。不受鉴权与操作日志中间件影响。

---

## 1. 认证

### POST /api/auth/login （公开）
```json
{ "username": "admin", "password": "admin123" }
```
→ `data: { "token": "eyJ...", "user": 用户对象 }`
账号不存在/密码错误/账号停用均返回 **401** `用户名或密码错误`。

### GET /api/auth/me
→ `data: 用户对象`（`password` 字段带 `json:"-"`，永不下发）

**用户对象字段**：`id, username, realName, deptId, deptName, phone, roleCode, enabled, createdAt, updatedAt`

---

## 2. 病案主档

### GET /api/records/page?keyword=&status=&deptId=&page=&size=
`keyword` 模糊匹配 `recordNo` / `patientName` / `hospitalNo`（ILIKE）。
`status` 取值（病案状态机，前端可做标签色）：`NOT_RECYCLED 未回收` `RECYCLED 已回收待编目` `CATALOGED 已编目` `ARCHIVED 已归档在架` `BORROWED 借出` `SEALED 封存`
→ `data: { list: [病案], total }`

> ⚠️ **状态投影（前端契约，2026-10-09 修正）**：`frontend/js/api-store.js` 把后端枚举投影成界面中文词，其中 **`RECYCLED` 与 `ARCHIVED` 曾经都被投影成「已归档」**，导致「待编目队列」误按「已归档」筛选、把已归档在架的病案也装了进来（它们早已编完目）。现行投影为：
>
> | 后端 `status` | 界面中文 |
> |---|---|
> | `NOT_RECYCLED` | 未归档 |
> | **`RECYCLED`** | **待编目**（待编目队列筛的就是它） |
> | `ARCHIVED` | 已归档（在架，可借阅） |
> | `CATALOGED` / `BORROWED` / `SEALED` | 已编目 |
>
> **借阅可选集合**只应包含 `ARCHIVED`（后端 `POST /api/borrows` 要求 status=ARCHIVED，否则 409）。

**病案字段**：`id, recordNo, patientName, gender, birthDate, hospitalNo, deptId, deptName, doctorName, admitTime, dischargeTime, status, shelfId, archivedAt, catalogedAt, remark, createdAt, updatedAt`
> 注意：`archivedAt`（归档上架时间）与 `catalogedAt`（完成编目时间）是**归档及时率统计的口径来源**，前端做时间轴/及时率展示时需要。

### GET /api/records/:id
→ `data: { record: 病案, catalogs: [编目], frontPage: 首页 }`
> 实现细节：无首页时 `frontPage` 返回**字段全为空的首页对象**（零值），不是 `null`。判空请用 `frontPage.recordId` 是否为 0/缺失。

### POST /api/records  （ADMIN）
body 为病案字段子集；`recordNo` 留空自动生成（`BA` + `yyyyMMddHHmmss`）；初始状态 `NOT_RECYCLED`，忽略请求体里的 `status`。
→ `data: 创建后的病案`

### PUT /api/records/:id  （ADMIN，部分更新）
仅 `NOT_RECYCLED/RECYCLED/CATALOGED` 状态可改（`WHERE id=? AND status IN (...)`）；**409** 表示病案不存在或状态不允许。

## 3. 归档（节 3）

### POST /api/archives/recycle  （ADMIN）批量回收
```json
{ "recordIds": [1, 2, 3] }
```
→ `data: { "batchNo": "RC20261009153000" }`；仅 `NOT_RECYCLED` 生效，其余静默跳过（不报错）；命中与未命中都会写入归档流水（`archive_record`）。

### POST /api/archives/shelve  （ADMIN）批量归档上架
```json
{ "recordIds": [1,2], "shelfId": 1 }
```
→ `data: { "batchNo": "AR20261009153000" }`
- 效果：`RECYCLED/CATALOGED → ARCHIVED`，写 `shelf_id` + `archived_at`，架位 `usedCount` 同步累加
- 架位容量不足时**自动截断**到剩余容量（不报错）
- **409**：架位不存在、架位已满
- 并发安全：对架位行加 `FOR UPDATE` 锁

### POST /api/archives/seal/:id  （ADMIN）封存
> 路径参数名是 **`:id`**（与实现 `router.go` 一致；早期草稿误写作 `:recordId`，现更正）。
仅 `ARCHIVED` 可封；→ `data: null`；**409** 表示状态不允许。封存后禁止借阅、禁止改首页、禁止增删编目。

### GET /api/archives/records/:id/flow  归档流转履历
> 路径参数名是 **`:id`**。
→ `data: [{ id, recordId, action, batchNo, shelfId, operatorName, remark, createdAt }]`，按 `id ASC`
`action`：`RECYCLE 回收` `ARCHIVE 上架` `SEAL 封存`

### 架位 CRUD（通用，见 §10 通用 CRUD 语义）
`GET /api/shelves/page?keyword=`、`GET /api/shelves/:id`、`POST /api/shelves`、`PUT /api/shelves/:id`、`DELETE /api/shelves/:id`（写操作 ADMIN）
字段：`shelfCode, room, locationDesc, capacity, usedCount`

## 4. 编目（节 4）

### GET /api/catalogs?recordId=1
> 无分页，返回数组。`recordId` 必传，否则返回空数组。
→ `data: [{ id, recordId, catalogType, icdCode, icdName, isMain, sortOrder, coderName, createdAt, updatedAt }]`
`catalogType`：`DIAGNOSIS` / `OPERATION`；排序 `catalog_type, sort_order, id`

### POST /api/catalogs  （ADMIN）新增编目
```json
{ "recordId": 2, "catalogType": "DIAGNOSIS", "icdCode": "K35.900", "icdName": "急性阑尾炎", "isMain": true }
```
- `coderName` 由后端取当前登录用户名覆盖，前端不用传
- 编码必须在 `icd_dict` 存在（否则 **409** `ICD 编码在字典中不存在…`）
- 首条编目把 `RECYCLED → CATALOGED` 并写 `catalogedAt`（条件更新，天然幂等）
- **409 归档锁定**：病案已 `ARCHIVED/BORROWED/SEALED` 时禁止再编目（批次 1 新增，依据《电子病历应用管理规范（试行）》）
→ `data: 创建后的编目对象`

### DELETE /api/catalogs/:id  （ADMIN）
→ `data: null`；**404** 编目不存在；**409** 所属病案已归档锁定。

### GET /api/icd-dicts/search?type=ICD10&keyword=肺
编码检索（前端选码下拉框/远程搜索）。`type` 默认 `ICD10`，可 `ICD9CM3`。
→ `data: [{ code, name }]`（最多 20 条，Redis 缓存 10 分钟）

### GET /api/icd-dicts/page?type=&keyword=&page=&size=
> 只返回 `enabled = true` 的编码。
→ `data: { list: [icd_dict 对象], total }`，字段 `id, dictType, code, name, versionId, enabled, createdAt, updatedAt`

### POST /api/icd-dicts/import  （ADMIN）编码库整版导入
```json
{ "dictType": "ICD10", "version": "ICD-10-2025", "entries": [{ "code": "A00.000", "name": "霍乱" }] }
```
→ `data: { "count": 1, "version": "ICD-10-2025" }`
导入会把同类型旧版本置为 `HISTORY`（不物理删除，保编目可追溯）。
（TODO 官方码表格式确认后改为文件上传）

## 5. 借阅申请与审批（节 5）

### POST /api/borrows  （任意登录用户）
```json
{ "recordId": 3, "reason": "临床诊疗", "dueTime": "2026-10-01T00:00:00Z", "urgent": false }
```
- `recordId`、`reason` 必填（缺失 **400**）
- 仅 `ARCHIVED` 在架病案可申请（否则 **409**）；`dueTime` 留空默认 `config.yaml borrow.borrow_days` 天
- `borrowerName` 取当前登录用户，`borrowerDept` 取病案科室
→ `data: { "msg": "申请已提交" }`（**不返回借阅单对象**，前端需另行刷新列表）

### PUT /api/borrows/:id/approve  （ADMIN）审批
```json
{ "result": "APPROVED", "opinion": "同意" }
```
`result` 只能是 `APPROVED`/`REJECTED`（否则 400）。多级审批（级数见 `config.yaml borrow.approve_levels`，当前 1 级）：全部通过 → `APPROVED`，未达级数保持 `APPLYING`，任一级驳回 → `REJECTED`。
→ `data: { "msg": "审批完成" }`

### PUT /api/borrows/:id/lend  （ADMIN）借出
`APPROVED → BORROWED`，同时病案 `ARCHIVED → BORROWED`（条件更新防并发双借）。
→ `data: { "msg": "借出完成" }`；**409**：非审批通过状态 / 病案不在架

### PUT /api/borrows/:id/return  （ADMIN）归还核销
`BORROWED/OVERDUE → RETURNED`，病案回 `ARCHIVED`。
→ `data: { "msg": "归还核销完成" }`

### PUT /api/borrows/:id/renew  （登录用户）续借
```json
{ "days": 7 }
```
`days` 必须 **1–60**（否则 400）；仅 `BORROWED` 可续（`OVERDUE` 需先归还，**409**）；`dueTime` 顺延 days 天。
→ `data: { "msg": "已续借 7 天" }`

## 6. 借阅台账（节 6）

### GET /api/borrow-ledger/page?status=&keyword=&overdueOnly=true&urgent=true&page=&size=
- `keyword` 模糊匹配 `orderNo` / `recordNo` / `borrowerName`
- `overdueOnly=true` → `status=OVERDUE` **或**（`status=BORROWED` 且 `dueTime < now`）
- `urgent=true` → 仅加急单

**借阅单字段**：`id, orderNo, recordId, recordNo, borrowerName, borrowerDept, reason, urgent, status, applyTime, lendTime, dueTime, returnTime, handlerName, remark, createdAt, updatedAt`
`status`：`APPLYING 审批中` `APPROVED 待借出` `REJECTED 已驳回` `BORROWED 借出` `OVERDUE 逾期` `RETURNED 已归还`

### GET /api/borrow-ledger/:id/approvals  审批链
→ `data: [{ id, orderId, level, approverName, result, opinion, createdAt }]`，按 `level ASC`

### POST /api/borrow-ledger/:id/remind  （ADMIN）人工催还
写一条 `borrow_reminder`（`channel=SYSTEM`）。**仅登记记录，无实际通知动作**（缺口 D6）。
→ `data: { id, orderId, channel, result, createdAt }`

## 7. 统计分析（节 7）

- `GET /api/stats/overview` → `data: [{ key: 病案状态, count }]`（各状态分布，饼图；Redis 缓存 5 分钟）
- `GET /api/stats/archive-rate` → `data: { total, archived, rate: 0-1 }`；**口径 =（ARCHIVED + BORROWED）/ 全部病案**（不含 SEALED 的说明见「待确认事项」）
- `GET /api/stats/disease-top` → `data: [{ key: "K35.900 急性阑尾炎", count }]`（编目诊断 Top10；缓存 10 分钟）
- `GET /api/stats/borrow-monthly` → `data: [{ key: "2026-08", count }]`（近 12 个月借出趋势，按 `lendTime`）
- `POST /api/stats/export` → **501** `导出功能待统计口径确认后实现（调研阶段三交付物）`

## 8. 病案首页质控（节 8）

首页状态机：`DRAFT 草稿` `WAIT_QC 待质控` `QC_PASSED 质控通过` `QC_REJECTED 质控退回` `REPORTED 已上报`

### GET /api/front-pages/page?status=&keyword=&page=&size=
`keyword` 模糊匹配 `patientName` / `hospitalNo`。
→ `data: { list: [首页], total }`

### GET /api/front-pages/:recordId
→ `data: { frontPage: 首页, qcResults: [质控项] }`（`qcResults` 按 `id DESC`；**404** 表示该病案尚无首页）

**首页字段**：`id, recordId, patientName, gender, birthDate, hospitalNo, deptName, doctorName, admitTime, dischargeTime, actualDays, admitDiagnosis, dischargeDiagnosis, mainDiagnosisCode, otherDiagnosisCodes, otherDiagnosisFlags, mainOperationCode, otherOperationCodes, totalCost, extJson, externalCauseCode, externalCauseName, newbornBirthWeight, newbornGestationalWeeks, status, createdAt, updatedAt`

**其他诊断的严重度标记（ADR-0002，2026-10-09 新增）**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `otherDiagnosisCodes` | string | 其他诊断编码，**半角逗号分隔**（已存在） |
| `otherDiagnosisFlags` | string | **与上面按逗号位置一一对应的严重度标记串**，取值 `0` 无 / `C` CC / `M` MCC；空串 = 未标记 |

> ⚠️ **这是等长对齐的隐式契约**（ADR-0002 §5）：
> - `otherDiagnosisCodes` 内**不允许重复编码**——重复会让"按位置对应"失去意义，解析层会拒绝
> - **两侧长度必须一致**（按槽位数比较，空槽也要占位）；长度不等时解析层拒绝，**不会**静默补 `0` 对齐
> - 空槽与其 flag **成对丢弃**，顺序不变
> - `flags` 整串为空时按"全部无严重度"处理（向后兼容历史数据，历史 57 份全空）
> - 非法标记值（如 `X`、`c`）**报错**，不降级为 `0`：空串是"没填"，非法值是"填错了"，二者语义不同
> - 编码做 `ToUpper+TrimSpace` 规范化；**标记串不做大小写宽容**
> - 只有显式列入 `Save` 白名单的字段才会写入（本字段已列入）
>
> **两个"无"不要混**：字段层的"无"是 `0`（与标记串同一套字面量）；展示层的分层结果用 `NONE`（见 `service/other_diagnosis.go` 的 `SeverityLevel`，返回 `MCC`/`CC`/`NONE`）。前端展示请以分层结果为准。

**条件必填真字段（D3，2026-10-09 新增）**：

| 字段 | 类型 | 可空 | 含义 |
|---|---|---|---|
| `externalCauseCode` | string | 是 | 损伤中毒外部原因**编码**（ICD-10 第二十章 V01–Y98）；条件必填规则的校验目标 |
| `externalCauseName` | string | 是 | 外部原因**名称**，与编码配对，不参与校验 |
| `newbornBirthWeight` | int（可 `null`） | 是 | 新生儿出生体重（克） |
| `newbornGestationalWeeks` | int（可 `null`） | 是 | 新生儿孕周（周） |

> - 出生体重/孕周用**可空指针**语义：`null` = 未采集/不适用（非新生儿），填 `0` 视为**可疑值**，同样会被"必填字段为空"命中。
> - 这三个数据元此前无独立列，批次 1 曾用 `otherDiagnosisCodes` / `extJson` 顶替（缺口 D3），已于本批次改为真字段。
> - 前端首页/编目表单需相应补输入项；对应规则：主诊断命中 `S%` 校验 `externalCauseCode`；命中 `P%` 分别校验 `newbornBirthWeight` 与 `newbornGestationalWeeks`（**两条独立规则**，互不顶替）。
> - 已知限制：条件表达式引擎只支持**单前缀** `LIKE`，故规则为 `LIKE 'S%'`，**T 编码目前漏报外部原因必填**（标准要求 S 或 T）。

### PUT /api/front-pages/:recordId  （ADMIN）保存首页（upsert）
- 白名单字段更新，**忽略** `id/recordId/status`（由后端决定）
- 保存后状态一律置 `WAIT_QC`（形成「修正 → 重新质控」闭环），并**写一条变更留痕**（`front_page_revision`）
- **409 归档锁定**：病案 `ARCHIVED/BORROWED/SEALED` 时禁止修改（批次 1 新增）
- **409 已上报禁改**：首页 `status=REPORTED` 时禁止修改
- 可选变更事由：请求头 `X-Change-Reason: xxx` 或查询参数 `?reason=xxx`（默认「首页数据修改」）
→ `data: { "msg": "已保存并进入待质控" }`

### POST /api/front-pages/:recordId/qc  （ADMIN）执行质控
→ `data: { status: "QC_PASSED|QC_REJECTED", results: [质控项], quality: 质量评分对象 }`
- **幂等重跑**：同键（`recordId + ruleId + fieldName`）的历史质控项会**保留其人工处理痕迹**（`handleStatus`、退回/整改/复核/fixed 各字段），只刷新 `errorLevel`/`errorMsg`；本次新命中的项以 `handleStatus=OPEN` 落库；本次不再命中的项（已修正）**仍按删除处理**（因此 `results` 始终只包含"本次命中的问题"）。
  - 修复前是「先全删该病案质控结果再重建」，会把在办的 `RETURNED`/`RECTIFIED` 状态一并清空，导致医生端「我的待整改」项凭空消失（缺陷 D9，已修）。
- 判定：命中任一 `ERROR` → `QC_REJECTED`；仅 `WARN` 或无误 → `QC_PASSED`
- **批次 1 新增**：同一次调用即产出 `quality`（质量评分 + 甲乙丙等级）
- 已 `CLOSED`（闭环）的项若再次命中同一违规：**保持 `CLOSED` 不重开**，仅刷新 `errorMsg` 并在服务端打「疑似回归」日志；本次命中的 `ERROR` 仍照常参与评分与 `QC_REJECTED` 判定

**质控项字段**：`id, recordId, ruleId, fieldName, errorLevel, errorMsg, fixed, fixedAt, fixedBy, handleStatus, returnOpinion, returnedBy, returnedAt, rectifiedBy, rectifiedAt, reviewOpinion, reviewedBy, reviewedAt, createdAt, updatedAt`
`handleStatus`：`OPEN 待处理` `RETURNED 已退回临床` `RECTIFIED 已整改待复核` `CLOSED 已闭环` `REJECTED_BY_QC 复核不通过`

### PUT /api/qc-results/:id/fixed  （ADMIN）异常修正确认
置 `fixed=true` + `fixedAt/fixedBy`，并同步置 `handleStatus=CLOSED`（语义上等同于走完退回返修闭环）。
→ `data: null`；**404** 质控项不存在

### 质控规则（`qc_rule` 表配置，不硬编码）
`ruleType` 取值与语义：

| ruleType | 语义 | 关键配置 |
|---|---|---|
| `REQUIRED` | 必填校验 | `targetField` = 首页结构体字段名；`conditionExpr` 非空时**先判条件**（条件必填，批次 1 新增） |
| `LOGIC` | 逻辑校验 | `expression`：`discharge_time>admit_time`、`actual_days>0`（其余表达式打印提示并跳过） |
| `CODE_EXIST` | 编码存在性 | `expression` = `ICD10` / `ICD9CM3` |
| `DIAG_OP_MATCH` | 主诊断-主手术匹配（防 DRG 歧义组 QY，批次 1 新增） | 判定表见 `service/drg_rules.go` |
| `MAIN_DIAG_RULE` | 主要诊断选择合理性提示（批次 1 新增） | 慢性病主诊断+手术、后遗症类编码作主诊断时提示 |

**`conditionExpr` 支持的写法**（条件必填，`service/qc_condition.go`）：
- `字段名 LIKE '前缀%'`，**仅支持单前缀**（无 OR / 多前缀）。已映射的字段名**仅限**：`main_diagnosis_code, main_operation_code, other_diagnosis_codes, other_operation_codes, admit_diagnosis, discharge_diagnosis, patient_name, gender, dept_name, doctor_name, hospital_no, ext_json`
- `actual_days>0`、`discharge_time>admit_time`、`has_operation`（已填主要手术编码）、`no_operation`
- 未识别的表达式**保守按「条件成立」处理**（宁可多校验不漏校验），并打日志
- ⚠️ **两个已知陷阱**：
  1. **不要把未映射字段写进 `conditionExpr`**——例如 D3 新增的 `external_cause_code` 不在映射表内，取值恒为空串 → 前缀匹配必然不成立 → **规则被静默跳过**（无报错）。若今后需要按新字段判条件，必须先在 `frontPageFieldString` 中补映射。
  2. **不要写 `OR` 复合条件**——`LIKE 'S%' OR LIKE 'T%'` 无法解析，会回退为「条件成立」，使规则在**全部病案**上生效（误报面扩大）。标准要求损伤中毒覆盖 S **或** T，当前规则只写 `'S%'`，**T 编码属已知漏报**，待条件引擎支持 OR 后补齐。

## 9. 首页上报（节 9）

### GET /api/report-batches/page?status=&period=&page=&size=
**批次字段**：`id, batchNo, period("2026-08"), status, totalCount, successCount, failCount, submitTime, operatorName, remark, createdAt, updatedAt`
`status`：`PENDING 待上报` `SUCCESS 已接收` `FAILED 已退回` `PARTIAL 部分成功`

### POST /api/report-batches  （ADMIN）
```json
{ "period": "2026-08" }
{ "period": "2026-08", "recordIds": [1,2] }
```
- `period` 必填；不传 `recordIds` 则纳入**全部** `QC_PASSED` 首页数据
- `batchNo` 服务端生成：`SB` + `yyyyMMddHHmmss`
- **409**：没有 `QC_PASSED` 的首页数据可上报
→ `data: 批次对象`

### POST /api/report-batches/:id/submit  （ADMIN）提交上报
→ `data: 批次对象`（含 `successCount/failCount/status/submitTime`）
- 本地桩：每条生成 XML 落盘 `config.yaml report.out_dir/批次号/`，回执写成功；成功明细的首页状态回写 `REPORTED`
- **409**：批次不是 `PENDING`（不可重复上报）
- 回执失败原因排查见下条

### GET /api/report-batches/:id/items
→ `data: [{ id, batchId, recordId, receiptStatus, receiptMsg, receiptTime, createdAt, updatedAt }]`，按 `id ASC`
`receiptStatus`：`WAITING` / `SUCCESS` / `FAIL`（失败原因在 `receiptMsg`）

---

## 10. 系统管理（节 10/11/12）

### 通用 CRUD 语义（`registerCrud[T]` 统一生成，6 类资源共用）

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/{资源}/page` | 登录 | 分页，支持资源各自的查询参数 |
| GET | `/api/{资源}/:id` | 登录 | 404 记录不存在 |
| POST | `/api/{资源}` | ADMIN | body 直接绑定模型 |
| PUT | `/api/{资源}/:id` | ADMIN | **部分更新**：GORM `Updates(结构体)`，**忽略零值字段**（无法把字段改回 `0`/`""`/`false`）；404 记录不存在 |
| DELETE | `/api/{资源}/:id` | ADMIN | 物理删除，`RowsAffected=0` 也返回成功 |

**资源清单**：

| 资源 | 路径 | 查询参数 | 关键字段 |
|---|---|---|---|
| 科室 | `/api/depts` | keyword（`deptName`/`deptCode`） | `deptCode, deptName, enabled` |
| 医师 | `/api/doctors` | keyword（`doctorName`/`doctorCode`） | `doctorCode, doctorName, deptId, deptName, title, enabled` |
| 字典 | `/api/dicts` | type（`dictType` 精确） | `dictType, dictKey, dictValue, sortOrder` |
| 角色 | `/api/roles` | — | `roleCode, roleName, dataScope(ALL/DEPT), remark` |
| 用户 | `/api/users` | keyword（`username`/`realName`） | `username, realName, deptId, deptName, phone, roleCode, enabled`（`password` 不下发） |
| 架位 | `/api/shelves` | keyword（`shelfCode`/`room`） | 见 §3 |

> ℹ️ **关于表迁移**：以上 6 类资源的表均已登记在 `main.go` 的 `AutoMigrate` 列表中（该列表共 24 张表），空库启动即自动建表，无需手工执行 SQL。

### GET /api/oper-logs/page?module=&page=&size=  （登录，只读）
→ `data: { list, total }`，字段 `id, module, operation, operatorName, ip, statusCode, costMs, createdAt, updatedAt`
写操作由 `middleware.OperLog` 自动落库。

### PUT /api/users/:id/password  （ADMIN）用户改密
```json
{ "password": "新密码" }
```
`min=6`（否则 400）；→ `data: null`；404 用户不存在。

---

## 11. 扩展：智能编码辅助（规则版预编码）· 批次 1

> **设计立场**：这是「编码辅助」而非「自动编码」。国家规范要求编码员依据病案内容自主判断并承担编码责任，系统只给候选提示。
> 规则表 `code_suggestion_rule`，内置 31 条示例规则（`handler/branch_ext.go` 的 `seedCodeSuggestionRules`）。

### POST /api/code-suggest  （登录）
```json
{ "text": "急性阑尾炎 高血压 阑尾切除术", "type": "ICD10" }
```
- `text` 必填（缺失 400）；`type` 可选，限定 `ICD10`/`ICD9CM3`
- 逻辑：关键词命中即推荐；同编码去重取最大权重；编码名称**回查 `icd_dict`** 保证与系统码表一致
→ `data: [{ code, name, matchedKeyword, weight }]`（按 `weight DESC`，**最多 10 条**）

### GET /api/code-suggestion-rules/page?keyword=&type=&page=&size=  （登录）
`keyword` 匹配 `keyword`/`icdCode`；`type` 精确匹配 `dictType`。
→ `data: { list: [{ id, keyword, dictType, icdCode, weight, enabled, remark, createdAt, updatedAt }], total }`

### POST /api/code-suggestion-rules  （ADMIN）
`keyword`/`dictType`/`icdCode` 必填（否则 400）；`weight` 传 0 时后端置默认 **10**。
→ `data: 创建后的规则对象`

### PUT /api/code-suggestion-rules/:id  （ADMIN）
采用 map 更新，**零值可写入**（能把 `enabled` 改回 `false`、`weight` 改回 `0`），与 §10 通用 CRUD 的部分更新语义不同。
→ `data: null`；404 规则不存在

### DELETE /api/code-suggestion-rules/:id  （ADMIN）→ `data: null`

> ⚠️ **权限说明**：`POST /api/code-suggest` 与规则分页为**登录可访问**；但 `api-store.js` 目前只封装了 `suggestCodes`、`suggestionRulesPage`（读）；规则的增删改尚未接入界面。

## 12. 扩展：DRG/DIP 预分组预检（查表版）· 批次 1

> **重要声明**：**非官方 DRG/DIP 分组器**，仅按示例规则做编码质控阶段的入组提示，**不得作为医保结算依据**。所有响应都带 `disclaimer` 字段。
> 规则表 `drg_group_rule`，内置 15 条示例规则。真正入组须用国家医保局官方分组器与权重表。

### POST /api/drg-preview  （登录）
```json
{ "recordId": 123 }
{ "diagCode": "K35.801", "opCode": "47.0900" }
```
- 二选一：传 `recordId` 则取其首页的主诊断/主手术；否则传 `diagCode`（无 `recordId` 又无 `diagCode` → 400）
- `opCode = "00.0000"`（字典占位码「无手术操作」）按**无手术**处理
- 选规则：优先「限定了手术前缀」的规则，其次前缀更长的（更具体）
→
```json
{
  "diagCode": "K35.801", "opCode": "47.0900",
  "groupCode": "GB25", "groupName": "阑尾切除术", "weight": 0.85,
  "matched": true,
  "hint": "匹配到示例分组规则：…",
  "disclaimer": "本功能为查表提示，非官方 DRG/DIP 分组器，仅供编码质控参考，不作为医保结算依据。"
}
```
- 未匹配到规则也是**有价值的信号**（`matched:false` + `hint` 提示编码不完整/填写缺失，可能是漏填导致无法入组）
- 风险提示：主诊断属慢性病类前缀（`I10/I25/I50/E11/J44/G40/I63`）**且**存在主手术但未匹配到带手术前缀的规则 → `hint` 追加「存在入歧义组(QY)风险」
- **404**：传了 `recordId` 但该病案无首页数据

### GET /api/drg-group-rules/page?keyword=&page=&size=  （登录）
`keyword` 匹配 `diagPrefix`/`opPrefix`/`groupCode`/`groupName`。
→ `data: { list: [{ id, diagPrefix, opPrefix, groupCode, groupName, weight, remark, enabled, createdAt, updatedAt }], total }`

### POST /api/drg-group-rules  （ADMIN）
`diagPrefix`/`groupCode` 必填（否则 400）；`remark` 留空自动填「示例数据，非官方分组器」。
### PUT /api/drg-group-rules/:id  （ADMIN）map 更新，零值可写；404 规则不存在。
### DELETE /api/drg-group-rules/:id  （ADMIN）

> ⚠️ **权限说明**：`POST /api/drg-preview` 登录可访问；前端 `ext.drgPreview` / `ext.drgRulesPage` 已封装，规则的增删改未接入界面。

## 13. 扩展：质量评分与质控退回返修闭环 · 批次 1

### GET /api/front-pages/:recordId/quality  （登录）质量评分查询
→ `data: 质量评分对象`；**404** `尚无质量评分，请先执行质控`（每次质控按 recordId 覆盖一条）

**质量评分字段**：`id, recordId, score(0-100), gradeCode(A/B/C), gradeName(甲级/乙级/丙级), errorCount, warnCount, vetoed, vetoReason, scoredAt, createdAt, updatedAt`

**评分口径（自定，非官方细则）**：基础 100 分；每条 `ERROR` −15；每条 `WARN` −5；最低 0 分。
等级：`≥90 甲级(A)`、`75–89 乙级(B)`、`<75 丙级(C)`。
**单项否决**（直接判丙级，即使总分达标）：患者姓名缺失、主要诊断编码缺失、出院时间不晚于入院时间。

### GET /api/front-pages/:recordId/qc-workbench  （登录）质控工作台视图
→ `data: { frontPage: 首页, qcResults: [质控项], quality: 质量评分或null, frontPageStatus: 首页状态 }`
**404** `该病案尚无首页数据`。

### GET /api/front-pages/:recordId/revisions  （登录）首页变更留痕（分页）
→ `data: { list: [{ id, recordId, beforeJson, afterJson, changedBy, reason, changedAt, createdAt }], total }`
`beforeJson`/`afterJson` 为首页快照 JSON 字符串，前端需自行 `JSON.parse`。
> 留痕目前**只覆盖首页**（`med_record_front_page`）。编目变更与病案主档变更未留痕（缺口 D5）。

### POST /api/qc-results/:id/return  （ADMIN）退回临床整改
```json
{ "opinion": "主要诊断编码与手术不匹配，请核对" }
```
置 `handleStatus=RETURNED` + `returnOpinion/returnedBy/returnedAt`。
→ `data: { "msg": "已退回临床整改" }`；**409** 该质控项已闭环（`CLOSED`）。

### POST /api/qc-results/:id/rectify  （DEPT_USER / ADMIN）临床整改提交
> **权限（D4 收紧后）**：`DEPT_USER` + `ADMIN` 放行，其它角色 **403**。
> - `DEPT_USER` 额外做**本科室归属校验**：病案不属于该用户科室时 **403** `该病案不属于您所在科室，无法提交整改`。
> - 保留 `ADMIN` 的原因：病案室在编目页质控工作台的「模拟整改」按钮仍调用本接口（演示路径），若只放行 DEPT_USER 会让该按钮立刻 403；语义上 ADMIN 只是「代临床登记整改结果」，写入的仅是整改人与时间，不改质控结论。
```json
{ "opinion": "已补充外部原因编码，请复核" }
```
`opinion` 可选（允许空体或 `{}`）。置 `handleStatus=RECTIFIED` + `rectifiedBy/rectifiedAt`。
若带 `opinion`，会把整改说明写入 `front_page_revision`（`reason` 字段承载，`beforeJson/afterJson` 为 `null`），使病案室复核时能看到「临床说了什么」；**该留痕失败不阻断整改主流程**。
→ `data: { "msg": "已提交整改，待病案室复核", "handleStatus": "RECTIFIED", "recordId": 12, "rectifiedBy": "deptuser" }`
**409** 已闭环；**403** 跨科室（仅 DEPT_USER）。

### GET /api/qc-results/my-pending  （登录）临床端「我的待整改」/ 病案室「整改督办」
**D4 新增**。返回处于「需临床处理」状态的质控项（`handleStatus ∈ {RETURNED, REJECTED_BY_QC}`），并按调用者角色决定数据范围：

| 角色 | `scope` | 数据范围 | 页面表述 |
|---|---|---|---|
| `ADMIN` | `ALL` | **全院**（病案室是整改督办方） | 整改督办 |
| `DEPT_USER`（有科室） | `DEPT` | **本科室**（按 `med_record.dept_name = sys_user.dept_name` 文本匹配；`dept_name` 为空则退化为 `dept_id`） | 我的待整改 |
| 无科室归属 | `NONE` | **空清单**（fail-closed，不返回全院） | — |

> 刻意**不含** `OPEN`（球在病案室）、`RECTIFIED`（球在病案室）、`CLOSED`（已闭环）。
→
```json
{
  "scope": "DEPT", "deptName": "心血管内科", "count": 2,
  "handleStatuses": ["RETURNED", "REJECTED_BY_QC"],
  "list": [{
    "id": 31, "recordId": 12, "recordNo": "BA20260901001", "patientName": "张三",
    "gender": "男", "deptName": "心血管内科", "doctorName": "李医生",
    "dischargeTime": "2026-09-20T09:00:00+08:00",
    "fieldName": "ExternalCauseCode", "ruleName": "损伤中毒类主诊断需填外部原因（条件必填）",
    "errorLevel": "WARN", "errorMsg": "必填字段为空",
    "handleStatus": "REJECTED_BY_QC", "returnOpinion": "请补充损伤中毒外部原因",
    "returnedBy": "admin", "returnedAt": "2026-10-08T16:20:00+08:00",
    "reviewOpinion": "外部原因编码仍缺失，请继续补充", "reviewedBy": "admin",
    "reviewedAt": "2026-10-09T09:10:00+08:00"
  }]
}
```
- 排序：`returnedAt ASC, id ASC`（最早退回的排最前，作待办优先级）
- ⚠️ **前端计算「已等待天数」时要注意**：`REJECTED_BY_QC`（复核不通过）项的 `returnedAt` **不会被刷新**（`review` 只写 `reviewOpinion/reviewedBy/reviewedAt`），"最近一次反馈时间"应取 `reviewedAt`，否则会把刚复核不通过的项误算成等待很久（或 0 天）
- 无科室归属时额外返回 `hint`（说明请联系病案室维护账号科室）
- 列表项已**一次查全**病案展示字段与规则名，前端无需再回查（避免 N+1）

### POST /api/qc-results/:id/review  （ADMIN）病案室复核
```json
{ "result": "PASS", "opinion": "已核对无误" }
```
`result` 只能是 `PASS`/`FAIL`（否则 400）。`PASS → CLOSED`（闭环）；`FAIL → REJECTED_BY_QC`（退回临床）。
→ `data: { "handleStatus": "CLOSED" }`；**409** 已闭环。

**闭环状态机**（对应 `front_page_qc_result.handle_status`）：
```
OPEN ──退回──→ RETURNED ──临床整改──→ RECTIFIED ──复核PASS──→ CLOSED（闭环）
                                         │
                                         └──复核FAIL──→ REJECTED_BY_QC ──→ （再整改）
```
> 旁路：`PUT /api/qc-results/:id/fixed`（病案室直接标记修正）会**跳过**上述流程直接置 `CLOSED`。

## 14. 扩展：归档及时率与超期催缴 · 批次 1

> 统计口径（`handler/archive_stats.go`）：分母 = 已出院病案总数（`discharge_time IS NOT NULL`）；分子 = 其中已归档（`archived_at IS NOT NULL`）且归档耗时 ≤ N 天者；归档耗时 = `archived_at − discharge_time`。未归档记录计入分母、不计入分子。

### GET /api/archive-timeliness?months=6  （登录）
`months` 默认 6，超出 1–36 会被重置为 6。
→
```json
{
  "overall": { "total": 100, "archived": 92, "within2": 85, "within3": 90,
               "rate2": 0.85, "rate3": 0.9, "avgDays": 2.35, "target": 0.9, "met": false },
  "trend":   [{ "month": "2026-09", "total": 30, "within2": 26, "within3": 28, "rate2": 0.8667, "rate3": 0.9333 }],
  "byDept":  [{ "deptName": "普外科", "total": 40, "archived": 38, "within2": 36, "rate2": 0.9, "avgDays": 2.1 }]
}
```
- `rate2/rate3` 保留 4 位小数，`avgDays` 保留 2 位
- `target` 固定 `0.90`（依据《病案管理质量控制指标（2021 版）》2 日归档率 ≥90%），`met` = `rate2 >= target`
- `trend` 按出院月份分组，取近 N 个月；`byDept` 按 2 日归档率**降序**（用于科室责任制）

### GET /api/archive-overdue?days=3  （登录）超期未归档催缴清单
`days` 默认 3，<1 时重置为 3。筛选：已出院、未归档（`archived_at IS NULL`）且出院时间早于 `now - days`，按出院时间升序。
→
```json
{ "days": 3, "count": 2,
  "list": [{ "recordId": 7, "recordNo": "BA2026...", "patientName": "张三", "deptName": "普外科",
             "doctorName": "李医生", "dischargeTime": "2026-09-25T10:00:00+08:00",
             "elapsedDays": 8, "overdueDays": 5, "status": "CATALOGED" }] }
```
> ⚠️ **仅输出清单，无任何通知动作**（无站内信/短信/对接医生站）。调研中「每天电话催办 50 次」的核心痛点**尚未真正解除**，这是缺口 D6。清单可导出由前端做（`api-store.js` 未封装此导出）。

## 15. 扩展：上报数据与院内原始数据一致性核对 · 批次 1

> 依据：三级医院评审现场核查会抽取 ≥20% 上报数据，偏差 ≥10% 判为错误数据。只读实现，不新增表。

### GET /api/report-consistency?period=2026-09  （登录）
`period` 必填（否则 400），格式 `YYYY-MM`，周期以**首页出院时间所属月份**为准。
→
```json
{
  "period": "2026-09", "expected": 30, "submitted": 28, "missing": 2, "mismatch": 1,
  "consistent": false,
  "detail": [{ "kind": "MISSING", "recordId": 12, "recordNo": "BA2026...",
               "patientName": "张三", "deptName": "普外科",
               "detail": "符合上报条件但未纳入该周期上报批次" }]
}
```
- `expected` = 该周期出院且首页为 `QC_PASSED`/`REPORTED` 的数量（院内应有）
- `submitted` = 该周期上报批次下的明细数
- `kind`：`MISSING 漏报` / `MODIFIED 上报后被修改`
- `consistent` = `missing == 0 && mismatch == 0`

### GET /api/report-consistency/summary  （登录）
近 6 个月（含当月）每月一行，**无 `detail`**：
→ `data: [{ period, expected, submitted, missing, mismatch, consistent }]`（按时间正序）

> ⚠️ **近似实现**：`MODIFIED` 用首页 `updatedAt` 与「批次落库/上报/回执时间中较晚者」比较，并留 **24 小时容忍窗口**（`consistencyTolerance`）来区分「上报动作本身的状态回写」与「人为改数据」。可能误判（缺口 D8）。

## 16. 扩展：上报数据导出（国家接口标准 CSV） · 批次 1

> 字段集为**骨架版核心 18 列**；《住院病案首页数据采集质量与接口标准》完整采集项 90+，后续对齐官方文档后应替换（缺口 D7）。

### GET /api/report-batches/:id/export  （ADMIN）导出 CSV
- **不返回 JSON**，直接返回 `text/csv; charset=utf-8` 文件流，带 UTF-8 BOM（Excel 可直接打开）
- 文件名：`hqmsts_YYYYMM.csv`（如 `hqmsts_202609.csv`），对齐接口标准命名要求
- 无数据字段一律填 `-`；性别转国标码（男=1、女=2）
- **前端注意**：导出需带 token，不能直接用 `<a href>`；`api-store.js` 已用 `fetch + Blob + Authorization` 封装（`ext.downloadReportExport`）
- **404**：批次不存在

**列顺序**：

| 列 | 含义 | 列 | 含义 |
|---|---|---|---|
| P900 | 医疗机构代码（骨架未存，固定 `-`） | P31 | 出院诊断 |
| P1 | 住院号 | P32 | 主要诊断编码(ICD-10) |
| P3 | 病案号 | P33 | 主要诊断名称 |
| P4 | 患者姓名 | P36 | 主要手术操作编码(ICD-9-CM3) |
| P5 | 性别代码 | P37 | 主要手术操作名称 |
| P6 | 出生日期 | P40 | 总费用 |
| P22 | 入院日期 | P23 | 入院科别 |
| P25 | 出院日期 | P26 | 出院科别 |
| P27 | 实际住院天数 | P30 | 入院诊断 |

### GET /api/report-batches/:id/export-check  （登录）导出前检查
→
```json
{
  "batchId": 3, "batchNo": "SB20261009...", "period": "2026-09",
  "total": 28, "problemItems": 2, "problemCount": 3,
  "items": [{ "recordId": 9, "recordNo": "BA2026...", "patientName": "张三",
              "issues": ["患者姓名(P4)为空", "总费用(P40)为空或为 0"] }],
  "message": "存在 3 项问题，导出后可能被平台退回"
}
```
- `total` 为批次明细数；`problemItems` 为有问题的病案数；`problemCount` 为**问题项总条数**（非病案数，两者不同）
- 无问题时 `message` 为 `字段完整，可正常导出`
- 检查项：P4 姓名、P5 性别、P1 住院号、P26 出院科别、P22 入院日期、P25 出院日期、P27 住院天数 ≤0、P32 主要诊断编码、P40 总费用 ≤0；首页缺失记 1 条「首页数据缺失，无法导出」
- **404**：批次不存在

---

## 17. 前端集成现状（`frontend/js/api-store.js`）

> 统计口径：`internal/router/frontend_calls_test.go` 的 `TestFrontendCallsResolveToRegisteredRoutes` 会**扫描前端源码**提取后端请求并与真实路由比对，当前报告的实测值为：**前端共 51 条请求，全部能匹配到已注册路由（0 条指向不存在的接口、0 条方法不匹配）**；其余未被前端调用的接口即「仅后端 API」，与 SPEC.md §2.3 的「仅后端 API，无界面」一致。
> ⚠️ 该测试读取 module 之外的 `frontend/`，Go 测试缓存不跟踪它——改动前端后请用 `go test -count=1`，否则可能拿到缓存的假绿灯。
> **新增能力已封装在 `MRMS.ext` 命名空间**（与 `MRMS.actions` 分离，避免污染既有调用）：`ext.suggestCodes`、`ext.suggestionRulesPage`、`ext.drgPreview`、`ext.drgRulesPage`、`ext.quality`、`ext.qcWorkbench`、`ext.qcReturn`、`ext.qcRectify`、`ext.qcReview`、`ext.revisions`、`ext.archiveTimeliness`、`ext.archiveOverdue`、`ext.reportConsistency`、`ext.reportConsistencySummary`、`ext.reportExportCheck`、`ext.downloadReportExport`、**`ext.myPendingQc`（D4）**、**`ext.getFrontPage` / `ext.saveFrontPage` / `ext.runQc`（D3 前端）**（`ext.call(promise, okMsg)` 为带 toast 的便捷包装）。
>
> **前端行为约定（D3）**：首页数据修正入口（`PUT /front-pages/:recordId` + `POST .../qc`）**仅病案室（ADMIN）可写**；临床（`DEPT_USER`）打开同一入口是**只读视图**（输入禁用、无保存动作），并提示"请在电子病历系统补充后由病案室同步，或联系病案室代改"——因为 `PUT /front-pages` 本来就是 ADMIN only，不做必然 403 的按钮。
> 同时 `api-store.js` 用 `frontPagePayload(快照, 改动)` 做**全量回传**：后端 `Save` 是「显式白名单 + 全量赋值」语义（未传字段写零值），故任何写入前都必须带上完整快照，否则会把未涉及的字段（如 `otherDiagnosisCodes`/`extJson`/`birthDate`）清空。

**已接入界面**：登录/会话恢复、病案/首页/借阅/上报/日志列表、病案登记与修改、批量回收、编目动作链（含自动质控与自动上架）、借阅申请/审批/借出/归还/续借、批次创建/提交、字典与编码库下拉、以及上表 `ext.*` 中已封装的能力。

**尚未接入界面（后端完备）**：病案封存、归档流转履历、借阅审批链追溯、人工催还、架位管理 CRUD、编码库整版导入、**系统管理全部 CRUD**（科室/医师/字典/角色/用户）、用户改密、`/stats/export`、编目删除、编码辅助规则与 DRG 规则的增删改。

---

## 待确认事项（影响前端但结构已预留）

1. 审批层级数（当前 1 级，配置化）——审批页可能需要显示多级节点
2. 统计口径与报表清单、导出格式（当前归档率口径 =（在架+借出）/全部；`/stats/export` 为 501）
3. 上报官方接口文档（当前本地 XML 桩；导出 CSV 为 18 列骨架，官方约 90+ 采集项）
4. 科室级数据隔离粒度（`sys_role.data_scope` 已留字段，当前**未生效**，登录用户可见全量）
5. 质量评分口径（基础 100 / ERROR−15 / WARN−5）为自定，待换官方细则
6. 「上报后被修改」的判定容忍窗口（当前 24h）是否收紧

---

## 变更记录

**2026-10-09 · 本轮同步（补齐批次 1 扩展 + 修正旧描述）**

新增 22 条接口文档：
- §11 智能编码辅助：`POST /code-suggest`、`GET /code-suggestion-rules/page`、规则 `POST/PUT/DELETE`（5 条）
- §12 DRG 预检：`POST /drg-preview`、`GET /drg-group-rules/page`、规则 `POST/PUT/DELETE`（5 条）
- §13 质量评分与退回返修：`GET /front-pages/:recordId/quality`、`/qc-workbench`、`/revisions`、`POST /qc-results/:id/return|rectify|review`（6 条）
- §14 归档及时率与催缴：`GET /archive-timeliness`、`GET /archive-overdue`（2 条）
- §15 一致性核对：`GET /report-consistency`、`/report-consistency/summary`（2 条）
- §16 上报导出：`GET /report-batches/:id/export`、`/export-check`（2 条）

同时补齐 1 条历史遗漏：`GET /healthz`（§0，公开探针，此前从未文档化）。
另有 **10 个既有接口在同批被增强**（编目增删的归档锁定、首页质控返回质量评分、首页保存的锁定与留痕、上报创建与提交的冲突校验等），增强说明已并入 §3/§4/§8/§9 对应条目。

修正 5 处与实现不符的旧描述：
1. **§3 封存与流转履历的路径参数**：实现是 `/archives/seal/:id` 与 `/archives/records/:id/flow`（`router.go`），文档**两轮都写错**——旧版写 `:id` 被改成 `:recordId`，反而是改错了；现更正为 `:id`。
   注：同名字样在 `/front-pages/:recordId/*` 上是**正确的**（实现确实用 `:recordId`），不要一并改掉。
2. §4 编目列表回复为数组而非分页对象，且新增归档锁定（409）说明
3. §6 人工催还明确「仅登记记录、无实际通知动作」
4. §8 首页保存补充归档锁定、变更事由请求头、变更留痕与 `quality` 返回；§9 批次创建补充 409 条件与 `batchNo` 生成规则
5. 目录「接口数」列修正：§11 由 6 改为 **5**、§13 由 6 改为 **7**（D4 的 `my-pending`）。此前两个偏差方向相反，合计恰好等于实测值 94，属**巧合**而非正确——故作废该口径的"合计校验"意义，一律以 `TestApiCoverage` 的实测为准。

另记录 1 处实现与文档/设计不一致（不改代码，仅标注，供后续批次决策）：
- §13 变更留痕只覆盖首页（`front_page_revision` 现也承载「临床整改说明」），编目与病案主档变更未留痕（缺口 D5）
  - 附带说明：整改说明复用了变更留痕表（`reason` 承载），因为 `front_page_qc_result` 没有「整改说明」列；后续建议为其加 `rectify_opinion` 列并在质控工作台展示

**勘误**：本文件早期草稿曾写「`sys_dept`/`sys_doctor`/`sys_dict`/`sys_role` 四表未纳入 `AutoMigrate`」——该说法**错误**，经核对 `main.go` 与 `git show HEAD:mrs-backend/main.go`，四表**均在**迁移列表中（共 24 张表）。相关内容已删除，特此记录以免误传。
