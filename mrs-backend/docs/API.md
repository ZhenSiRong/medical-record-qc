# 病案管理系统 API 契约（前端集成依据）

> 本文档是前端开发的唯一接口依据，与后端代码同步维护。接口变更请同步更新本文件。
>
> - Base URL：`http://localhost:8080`
> - 全部业务接口挂 `/api` 前缀；登录除外均需请求头 `Authorization: Bearer <token>`
> - 统一响应：`{ "code": 0, "msg": "ok", "data": ... }`；`code=0` 成功；非 0 时 `code` 与 HTTP 状态码一致（400 参数 / 401 未登录 / 403 无权限 / 404 不存在 / 409 状态冲突 / 500 服务器错误 / 501 未实现）
> - 分页接口统一参数 `page`（默认 1）、`size`（默认 10，上限 100），返回 `data: { list: [...], total: n }`
> - CORS 已全开放，前端任意本地端口可直接联调

## 账号（种子数据）

| 账号 | 密码 | 角色 | 能力 |
|---|---|---|---|
| `admin` | `admin123` | ADMIN 病案管理员 | 全部操作（归档/编目/审批/上报/系统管理） |
| `deptuser` | `123456` | DEPT_USER 科室用户 | 查询、借阅申请、统计查看 |

## 菜单 → 模块对照（可直接做前端路由/侧边栏）

| 菜单（12 节课件对应节） | 前端页面建议 | 主要接口 |
|---|---|---|
| 病案归档（节 3） | 病案列表 + 批量回收/上架 + 架位管理 + 流转履历 | `/records` `/archives` `/shelves` |
| 病案编目（节 4） | 编目工作台（选码）+ 编码库维护 | `/catalogs` `/icd-dicts` |
| 借阅申请与审批（节 5） | 我的申请 / 待审批列表 | `/borrows` |
| 借阅台账（节 6） | 台账 + 逾期筛选 + 催还 | `/borrow-ledger` |
| 统计分析（节 7） | 图表看板 | `/stats/*` |
| 首页质控（节 8） | 首页表单 + 异常列表 + 修正 | `/front-pages` `/qc-results` |
| 首页上报（节 9） | 批次列表 + 明细回执 | `/report-batches` |
| 系统管理（节 10/11/12） | 用户/角色/科室/医师/字典/日志 | `/users` `/roles` `/depts` `/doctors` `/dicts` `/oper-logs` |

---

## 1. 认证

### POST /api/auth/login
```json
{ "username": "admin", "password": "admin123" }
```
→ `data: { "token": "eyJ...", "user": { id, username, realName, deptName, roleCode, ... } }`

### GET /api/auth/me
→ `data: 用户对象`（密码字段永不下发）

---

## 2. 病案主档

### GET /api/records/page?keyword=&status=&deptId=&page=&size=
`status` 取值（病案状态机，前端可做标签色）：`NOT_RECYCLED 未回收` `RECYCLED 已回收待编目` `CATALOGED 已编目` `ARCHIVED 已归档在架` `BORROWED 借出` `SEALED 封存`
→ `data: { list: [病案], total }`，病案字段：`id, recordNo, patientName, gender, birthDate, hospitalNo, deptId, deptName, doctorName, admitTime, dischargeTime, status, shelfId, remark`

### GET /api/records/:id
→ `data: { record: 病案, catalogs: [编目], frontPage: 首页或null }`

### POST /api/records  （ADMIN）
body 为病案字段子集；`recordNo` 留空自动生成；初始状态 `NOT_RECYCLED`。

### PUT /api/records/:id  （ADMIN，部分更新）
仅 `NOT_RECYCLED/RECYCLED/CATALOGED` 状态可改；409 表示状态不允许。

## 3. 归档（节 3）

### POST /api/archives/recycle  （ADMIN）批量回收
```json
{ "recordIds": [1, 2, 3] }
```
→ `data: { "batchNo": "RC2026..." }`；仅 `NOT_RECYCLED` 生效，其余自动跳过

### POST /api/archives/shelve  （ADMIN）批量归档上架
```json
{ "recordIds": [1, 2], "shelfId": 1 }
```
架位容量不足时自动截断到剩余容量；409 返回原因（架位不存在/已满）

### POST /api/archives/seal/:recordId  （ADMIN）封存（仅 ARCHIVED 可封）
### GET /api/archives/records/:recordId/flow  归档流转履历
→ `data: [{ action: "RECYCLE|ARCHIVE|SEAL", batchNo, operatorName, createdAt }]`

### 架位 CRUD（通用）
`GET /api/shelves/page?keyword=`、`GET /api/shelves/:id`、`POST /api/shelves`、`PUT /api/shelves/:id`、`DELETE /api/shelves/:id`（写操作 ADMIN）
字段：`shelfCode, room, locationDesc, capacity, usedCount`

## 4. 编目（节 4）

### GET /api/catalogs?recordId=1
→ `data: [{ id, catalogType: "DIAGNOSIS|OPERATION", icdCode, icdName, isMain, sortOrder, coderName }]`

### POST /api/catalogs  （ADMIN）新增编目
```json
{ "recordId": 2, "catalogType": "DIAGNOSIS", "icdCode": "K35.900", "icdName": "急性阑尾炎", "isMain": true }
```
编码必须在编码库存在（409 提示）；首条编目自动把病案推到 `CATALOGED`

### DELETE /api/catalogs/:id  （ADMIN）

### GET /api/icd-dicts/search?type=ICD10&keyword=肺
编码检索（前端选码下拉框/远程搜索），`type`: `ICD10` 诊断 / `ICD9CM3` 手术
→ `data: [{ code, name }]`（最多 20 条，Redis 缓存）

### GET /api/icd-dicts/page?type=&keyword=&page=&size=
### POST /api/icd-dicts/import  （ADMIN）编码库整版导入
```json
{ "dictType": "ICD10", "version": "ICD-10-2025", "entries": [{ "code": "A00.000", "name": "霍乱" }] }
```
（TODO 官方码表格式确认后改为文件上传）

## 5. 借阅申请与审批（节 5）

### POST /api/borrows  （任意登录用户）
```json
{ "recordId": 3, "reason": "临床诊疗", "dueTime": "2026-10-01T00:00:00Z", "urgent": false }
```
仅 `ARCHIVED` 在架病案可申请（409 提示）；`dueTime` 留空默认 30 天

### PUT /api/borrows/:id/approve  （ADMIN）审批
```json
{ "result": "APPROVED", "opinion": "同意" }
```
多级审批（当前配置 1 级）：全部通过 → `APPROVED`；任一级驳回 → `REJECTED`

### PUT /api/borrows/:id/lend  （ADMIN）借出（APPROVED → BORROWED，病案变 BORROWED）
### PUT /api/borrows/:id/return  （ADMIN）归还核销（→ RETURNED，病案回 ARCHIVED）
### PUT /api/borrows/:id/renew  （登录用户）续借
```json
{ "days": 7 }
```
仅 `BORROWED` 状态可续借，`dueTime` 顺延 days 天；`OVERDUE` 借阅需先归还（409）

## 6. 借阅台账（节 6）

### GET /api/borrow-ledger/page?status=&keyword=&overdueOnly=true&urgent=true&page=&size=
借阅单字段：`orderNo, recordNo, borrowerName, borrowerDept, reason, urgent, status, applyTime, lendTime, dueTime, returnTime, handlerName`
`status`：`APPLYING 审批中` `APPROVED 待借出` `REJECTED 已驳回` `BORROWED 借出` `OVERDUE 逾期` `RETURNED 已归还`

### GET /api/borrow-ledger/:id/approvals  审批链
### POST /api/borrow-ledger/:id/remind  （ADMIN）人工催还

## 7. 统计分析（节 7）

- `GET /api/stats/overview` → `[{ key: 病案状态, count }]`（各状态分布，饼图）
- `GET /api/stats/archive-rate` → `{ total, archived, rate: 0-1 }`（归档率）
- `GET /api/stats/disease-top` → `[{ key: "K35.900 急性阑尾炎", count }]`（疾病谱 Top10）
- `GET /api/stats/borrow-monthly` → `[{ key: "2026-08", count }]`（近 12 个月借出趋势）
- `POST /api/stats/export` → 501（导出待统计口径确认后实现）

## 8. 病案首页质控（节 8）

首页状态机：`DRAFT 草稿` `WAIT_QC 待质控` `QC_PASSED 质控通过` `QC_REJECTED 质控退回` `REPORTED 已上报`

### GET /api/front-pages/page?status=&keyword=
### GET /api/front-pages/:recordId
→ `data: { frontPage: 首页数据, qcResults: [{ id, fieldName, errorLevel, errorMsg, fixed }] }`
首页字段：`patientName, gender, birthDate, hospitalNo, deptName, doctorName, admitTime, dischargeTime, actualDays, admitDiagnosis, dischargeDiagnosis, mainDiagnosisCode, otherDiagnosisCodes, mainOperationCode, otherOperationCodes, totalCost, extJson, status`

### PUT /api/front-pages/:recordId  （ADMIN）保存首页（upsert，保存后自动进入 WAIT_QC；REPORTED 后禁改）
### POST /api/front-pages/:recordId/qc  （ADMIN）执行质控
→ `data: { status: "QC_PASSED|QC_REJECTED", results: [异常列表] }`
质控规则配置在 `qc_rule` 表：必填 / 逻辑（出院>入院、天数>0）/ 编码存在性

### PUT /api/qc-results/:id/fixed  （ADMIN）异常修正确认

## 9. 首页上报（节 9）

### GET /api/report-batches/page?status=&period=
批次：`batchNo, period("2026-08"), status: PENDING|SUCCESS|FAILED|PARTIAL, totalCount, successCount, failCount, submitTime`

### POST /api/report-batches  （ADMIN）
```json
{ "period": "2026-08" }
```
自动纳入全部 `QC_PASSED` 首页数据；也可传 `recordIds` 指定

### POST /api/report-batches/:id/submit  （ADMIN）提交上报
→ `data: 批次`（骨架阶段为本地桩：每条数据生成 XML 落盘 `report-out/批次号/`，回执写成功）

### GET /api/report-batches/:id/items
→ `data: [{ recordId, receiptStatus: WAITING|SUCCESS|FAIL, receiptMsg, receiptTime }]`（失败原因排查）

## 10. 系统管理（节 10/11）

以下资源均支持通用 CRUD（`GET /page`、`GET /:id`、`POST`、`PUT /:id`、`DELETE /:id`，写操作 ADMIN）：

| 资源 | 路径 | 查询参数 | 关键字段 |
|---|---|---|---|
| 科室 | `/api/depts` | keyword | deptCode, deptName, enabled |
| 医师 | `/api/doctors` | keyword | doctorCode, doctorName, deptName, title |
| 字典 | `/api/dicts` | type | dictType, dictKey, dictValue |
| 角色 | `/api/roles` | - | roleCode, roleName, dataScope(ALL/DEPT) |
| 用户 | `/api/users` | keyword | username, realName, deptName, roleCode, enabled（password 不下发） |
| 架位 | `/api/shelves` | keyword | 见 §3 |
| 操作日志 | `/api/oper-logs/page` | module | 只读：module, operation, operatorName, ip, statusCode, costMs |

用户改密：`PUT /api/users/:id/password` → `{ "password": "新密码(≥6位)" }`

---

## 待确认事项（影响前端但结构已预留）

1. 审批层级数（当前 1 级，配置化）——审批页可能需要显示多级节点
2. 统计口径与报表清单、导出格式（当前归档率口径=（在架+借出）/全部）
3. 上报官方接口文档（当前本地 XML 桩；前端只需按批次/回执状态展示）
4. 科室级数据隔离粒度（当前未生效，登录用户可见全量）
