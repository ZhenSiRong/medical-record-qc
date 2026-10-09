# 交接文档：住院病案管理系统（yrzk）

> **交接时间**：2026-10-09 ｜ **工作区**：`D:\ZcodeWorktree\yrzk`
> **本文档只写「接手必须知道的事」**，其余一律**引用已有产物路径**，不复述。
> **读法建议**：先看第一节（一句话现状）与第二节（必读顺序），再看第七节（陷阱清单）——**第七节是省时间最多的一节**。

---

## 一、一句话现状

Go + PostgreSQL + 零构建 Vue3 WebUI 的住院病案管理系统。业务闭环完整（归档/编目/借阅/统计/首页质控/上报），功能扩展批次 1 的 10 项能力已交付，本轮完成**文档同步（D1/D2）**、**三个功能缺口修复（D3/D4/D9）**、以及**两项新能力的方案落地（ADR-0001 本地 AI 编码、ADR-0002 DRG 严重度预警，均只完成部分批次）**。

**有 3 份 ADR、9 项已确认缺口（D1–D9）、若干明确移交项**。其中 **ADR-0003 专门记录编目职责建模的三处偏差**——那是本轮验收时用户发现的最有价值的问题。

---

## 二、接手前必读（按顺序）

| 顺序 | 文档 | 为什么读 |
|---|---|---|
| 1 | **本文档第七节** | 陷阱清单（含 5 条本轮新踩的坑），先读省最多时间 |
| 2 | `docs/BACKLOG.md` | 待办计划：D3 前端收尾、D5–D8、批次 3、**并发拆分建议** |
| 3 | `docs/adr/`（3 份 + README） | 已定的架构决策与**明确不做**的边界 |
| 4 | `SPEC.md` V1.1 | 系统规格：功能域、状态机、24 表、§2.3 已知缺口表、§5.3 验证边界 |
| 5 | `mrs-backend/docs/API.md` | 接口契约（94 条，与实现双向核对 100%）；文末「变更记录」有勘误 |
| 6 | `docs/extension-guide.html` | 十项扩展功能怎么操作，**已按界面实况标注「已接入/部分接入/未接入」** |
| 7 | `docs/research/A1`、`A2` | 需求依据（岗位画像、12 条政策刚性需求）——**ADR-0003 的依据全在这里** |
| 8 | `prototype/BASELINE.md` | 原型冻结范围——**哪些是明确不做的**，别去补 |

---

## 三、已完成的三大块（不要重做）

1. **基线**（`238c372`）：归档/编目/借阅（多级审批+逾期催还）/统计/首页质控/上报（本地 XML 桩）/JWT/操作审计；20 张表、71 条路由
2. **功能扩展批次 1**（`6ba4bd3` + `6273902`）：10 项能力
3. **本轮（2026-10-09，`9c868e4` → `b1326b8`）**：
   - **D1** `API.md` 重写：补 22 条批次 1 接口 + 1 条历史遗漏（`/healthz`），修正 5 处失实描述；新增契约守卫测试
   - **D2** `SPEC.md` V1.1：收录批次 1，新增 §3.5 界面落点与 §5.3 验证边界
   - **D3** 条件必填改真字段（首页 4 列）+ 前端录入与「修正首页数据」真闭环
   - **D4** 质控退回补医生端（`my-pending` + 前端页 + 权限收紧）
   - **D9** 修 `RunQc` 重跑丢状态（**数据丢失缺陷**，改为同键增量 upsert）
   - **ADR-0001/0002/0003** 落档 + `docs/adr/README.md` 索引
   - 演示数据修复：`deptuser`（陈志远）从 0 份 → 20 份可测病案
   - 前端状态投影修正（`RECYCLED` 正名为「待编目」）+ 编目完成后自动上架导致列表不显示新记录的修复

---

## 四、当前可运行状态

| 项 | 值 |
|---|---|
| 前端（演示入口） | http://127.0.0.1:8090/frontend/index.html |
| 后端健康检查 | http://127.0.0.1:8080/healthz |
| 数据库 | PostgreSQL 5432（便携版在 `tools/pg/`，**无 psql 客户端**） |
| 账号 | `admin` / `admin123`（病案室，王主任）· `deptuser` / `123456`（临床，陈志远/心血管内科） |
| 启停 | 双击 `tools/demo-start.cmd` / `tools/demo-stop.cmd` |
| Go | `C:\Program Files\Go\bin\go.exe`（**不在 PATH**） |
| Node（语法检查/模板编译验证） | `C:\Program Files\nodejs\node.exe` |
| 静态服务 | `python serve.py 8090`（带 no-cache 头，**不要用 `python -m http.server`**） |

**演示数据规模**：87 份病案、57 份首页、19 张借阅单、2 个上报批次、**ICD-10 121 条**（批次 A 扩充）、ICD-9-CM-3 14 条。
**陈志远名下 20 份**，全部在心血管内科（6 待编目 / 10 已编目 / 2 已归档 / 2 未归档）——为的是让 `deptuser` 的「我的待整改」按科室过滤时不会永远是空的。

**重建演示数据**（会重置所有改动）：
```bash
cd mrs-backend
go run ./cmd/createdb drop && go run ./cmd/createdb
# 然后重启后端，空库会自动灌全部种子（含 ICD 扩充、D3 规则、陈志远演示数据）
```

**编译与启动后端**（必须独立进程 + 正确工作目录，否则读不到 `config.yaml`）：
```bash
cd mrs-backend && go build -buildvcs=false -o ../data/mrs.exe .
# 用 Start-Process 启动，WorkingDirectory 必须是 mrs-backend
```

**每批交付前必跑**：
```bash
cd mrs-backend
go build -buildvcs=false ./...
go vet ./...
go test ./... -count=1        # ← -count=1 不可省，见第七节第 1 条
go fmt ./...                   # 应无输出
python tools/check-api-coverage.py   # 契约覆盖度人工复核
# 前端全部 JS：node --check <file>
```

---

## 五、已确认的 9 项缺口（D1–D9）

**D1 / D2 已修复**（文档同步）。**D3 后端已修、前端已录入口但未做完整编辑器**。**D4 已修复**。**D9 已修复**。

**仍待处理**（详见 `SPEC.md` §2.3 与 `docs/BACKLOG.md`）：

| # | 缺口 | 备注 |
|---|---|---|
| D5 | 变更留痕只覆盖首页 | 编目变更、`med_record` 主档变更未留痕；依据 E9 要求"归档后可追溯" |
| D6 | 催缴只有清单导出、无通知动作 | 调研核心痛点「每天电话催办 50 次」未真正解除 |
| D7 | 上报 CSV 为 18 列骨架 | 官方约 90+ 采集项，**待官方文档** |
| D8 | 两处近似实现 | 一致性核对的 `updatedAt`+24h 容忍窗口；质量评分口径为自定 |

**另有移交项（非 D 编号，但同等重要）**：

| 项 | 位置 | 说明 |
|---|---|---|
| **编目职责建模三处偏差** | **`docs/adr/0003`** | 编码员定主诊断 / 代写医生字段 / 编码与质控未分岗。**决策=记为已知边界并移交，给了三档修复选项** |
| 数据隔离未生效 | `middleware/cors.go:25` | 自述"仅做功能角色校验"；实测 `deptuser` 可见全院 87 份 |
| `sql/schema.sql` 与模型不一致 | `mrs-backend/sql/` | 手工 DDL 停在基线 20 表；**运行时真实来源是 `main.go` 的 AutoMigrate**（`cmd/createdb` 只 `CREATE DATABASE`） |
| 上报 payload 未含 D3 新字段 | `service/reportclient.go` | 该文件有独立白名单 |
| `frontPageFieldString` 未映射新字段 | `service/qc_condition.go` | 故新字段**不可用于 `conditionExpr`**（写了会取空串→条件不成立→规则静默跳过）；已有测试固化 |
| T 编码漏报外部原因必填 | `service/qc_condition.go` | 条件引擎只支持单前缀 `LIKE`；写 `OR` 会退化为"恒成立" |
| 编码库维护界面缺失 | `frontend/` | `POST /icd-dicts/import` 后端完备，**前端无入口** |
| `/icd-dicts/search` 前端零调用 | `frontend/js/api-store.js` | 后端按生产规模设计了检索（Redis+限 20 条），前端走"启动全量拉取+内存过滤"；码表到数千条会失效 |

---

## 六、两份 ADR 的落地进度（接手人最容易搞错的地方）

### ADR-0001（本地 AI 编码）—— **尚未开工**
- 决策已定：**本地部署**，不用外部 API；产品定位是**推荐**不是自动编码
- 实测：便携版 PG **无 pgvector**，Docker 也不在 PATH → pgvector 降级为规模驱动项
- `.env` 里的 `OPENCODE_ZEN_*` 三个键**已作废**（后端从未读过）
- **待定**：embedding 模型型号、本地推理服务形态、TopK、字典向量化时机

### ADR-0002（DRG 严重度预警）—— **批次 A/B 已做，C/D 未做**
- ✅ **批次 A**：ICD-10 24 → 121 条（MCC 31 / CC 43 / 非 CC 23）；`SeedIcdExt` 幂等
- ✅ **批次 B（部分）**：`otherDiagnosisFlags` 字段 + 解析层（`service/other_diagnosis.go`）+ 判读层（`service/ccmcc.go`，16 条 **Source=demo** 规则）
- ❌ **未接线**：`frontpage.Save` **没有**调用 `ParseOtherDiagnoses`，所以"重复编码/长度不等拒绝"**尚未在 HTTP 路径生效**
- ❌ **批次 C**：低靠（漏编）/高靠（套高）检测**未接入 `qc_rule` 质控闭环**
- ❌ **批次 D**：前端严重度分层预览未做
- ❌ `cc_mcc_rule` 仍是 **Go 内存规则集**，未落表（ADR 要求"可替换+版本化"）
- **待确认（阻塞"用官方表"）**：设计单位是否持有医保局 2.0 版 CHS-DRG 分组方案 / CC-MCC 表

---

## 七、陷阱清单（本轮新踩的坑，按"省时间"排序）

1. **验证命令必须带 `-count=1`**：`frontend/` 在 Go module 之外，而 Go 的测试缓存**不跟踪 module 外的文件**。实测：故意注入一个不存在的接口路径后，不加 `-count=1` 显示 `ok (cached)`（**假绿灯**），加上才 FAIL。凡涉及前端的验证一律 `go test ./... -count=1`。
2. **每个守卫性测试必须做负控制**：故意注入错误确认它会红并**精确指认位置**，再逐字节还原（附 sha256 佐证）。**不能红的测试等于没写**。
3. **数量类断言一律程序化核对，不靠估算**：本轮用正则试算"前端调用数"得到 **46 / 31 / 27** 三个互相矛盾的结果，真值 51（由扫描器给出）。凡"多少条"都跑测试。
4. **禁止用 `Get-Content -Raw` / `Set-Content` 读写仓库源码**：实测会静默吃掉换行（某文件 852 → 842 行），导致行号偏移与内容损坏。用就地编辑或字节级读写。
5. **文档里的数量改完要回扫一遍**：本轮 `API.md` 头部的"路由总量"因一次并发写失败而漏改，**是提交后逐文件回扫才发现的**——而那个文件正是"契约唯一依据"。
6. **PowerShell 5.1 不支持三元运算符**（`? :`）——改用 `if/else`，或直接用 Python 做接口断言。
7. **没有 `psql`**：要执行 SQL 就复用仓库已有的 `pgx` 驱动写一次性 `cmd/xxx`，**用完即删**。
8. **`PowerShell` 的工作目录陷阱**：工具可能把工作目录设为 `mrs-backend`，脚本里再 `cd` 无效；用绝对路径最稳（本轮多次因此跑错路径）。
9. **并发按「文件所有权」拆，不按功能拆**：热点文件 `qcengine.go` / `router.go` / `api-store.js` / `layout.js` / `index.html` / `catalog.js`。
10. **注册函数不做 AutoMigrate**：迁移统一在 `main.go` 列表（否则路由冒烟测试传 nil db 会 panic）。
11. **中文 payload 不能用 Git Bash 的 `curl -d`**（会按 GBK 发送变乱码）；cmd 批处理必须 GBK 编码。
12. **`.gitignore` 不支持行尾注释**。

---

## 八、验证现状与边界（**接手人必须知道**）

| 项 | 状态 |
|---|---|
| `go build` / `go vet` / `go test ./... -count=1` | ✅ 全绿 |
| 契约覆盖度 | ✅ `TestApiCoverage` 双向核对 **94/94 = 100%** |
| 前端调用 ↔ 路由核对 | ✅ `TestFrontendCallsResolveToRegisteredRoutes`：51 条请求全匹配 |
| 前端 15 个 JS 语法 | ✅ `node --check` 全通过 |
| **浏览器视觉验证** | ⚠️ **做得很有限**。候选工具在 Node 里用真实 Vue 运行时做了"模板编译 + `setup()` 逻辑断言"（114 项），但**没有渲染像素** |
| **真库端到端** | ✅ 做过关键路径：D4 闭环（退回→临床可见→补填→质控通过→移出清单）、D9（重跑质控后痕迹逐字段一致）、归档锁定 409、陈志远编目后自动上架 |

**接手人第一件该做的事**：起服务，用两个账号各点一遍，重点看**本轮未做浏览器验证的**部分（前端角色门控的视觉表现、严重度分层弹窗、编目表单 4 个新字段的录入体验）。

---

## 九、敏感信息

API key（opencode zen，**已作废但仍存于** `.env`）在 `.env`（已被 `.gitignore` 忽略，未入库）。
**不要**把它写进任何文档、提交、或复述到对话里。若确定不再使用，可直接删除该文件中的三个键。

---

## 十、产物索引

| 类别 | 路径 |
|---|---|
| 待办与方案 | `docs/BACKLOG.md` |
| **架构决策** | `docs/adr/README.md`（索引）+ 0001/0002/0003 |
| 规格书（答辩用） | `SPEC.md` V1.1、预览 `spec-preview.html`（`python tools/render-spec.py` 生成） |
| 功能说明与操作指南 | `docs/extension-guide.html` |
| 接口契约 | `mrs-backend/docs/API.md`（94 条，含变更记录与勘误） |
| 契约核对证据 | `mrs-backend/routes.diff.md`、核对脚本 `mrs-backend/tools/check-api-coverage.py` |
| 需求依据 | `docs/research/A1-病案科岗位与痛点.md`、`A2-政策绩效驱动需求.md` |
| 需求匹配度分析 | `docs/research/user-match.html` |
| 原型基线与决策 | `prototype/BASELINE.md`、`prototype/review.html`、`prototype/STORIES.md` |
| 截图素材 | `prototype/screenshots/`（`final/` 定稿七屏、`ext/` 扩展功能、`e2e/` 早期联调） |
| 后端源码 | `mrs-backend/`（`internal/{handler,service,model,router,seed}`） |
| **守卫性测试** | `internal/router/{contract_test.go,frontend_calls_test.go}`、`internal/service/{other_diagnosis_test.go,ccmcc_test.go,qc_condition_d3_test.go,qcengine_upsert_d9_test.go}`、`internal/handler/qc_ext_d4_test.go`、`internal/seed/icd_ext_test.go` |
| 前端源码 | `frontend/`（`js/api-store.js` 适配层 + `js/components/*.js`） |
| 启停脚本 | `tools/demo-start.cmd`、`tools/demo-stop.cmd` |

---

## 十一、建议调用的技能

| 技能 | 何时用 |
|---|---|
| `dispatching-parallel-agents` | 拆并发任务时——**按文件所有权拆**，并注意 D3/D4 这类必须串行的组合 |
| `verification-before-completion` | 每批交付前——本项目的验证口径见第四节末 |
| `tdd` | 新增规则引擎或校验逻辑时（D9 的 `mergeQcResults` 就是先写纯函数再写测试） |
| `code-review` | 改动合并前审一遍（尤其涉及热点文件时） |
| `handoff` | 再次交接时 |
