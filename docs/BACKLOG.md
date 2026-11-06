# 后续计划（BACKLOG）

> 记录「已定方案待开工」与「待确认的候补项」。确认完毕后按批次并发执行，每批完成即提交。
> **当前基线：`a01fff1` + 工作区未提交改动**（`git log --oneline -5` 实测最新提交为 `a01fff1 截图：智能推荐入口位置…`；其后无新提交）。
> 未提交改动至少分两摊：① **D1/D2 文档同步** —— `SPEC.md`、`mrs-backend/docs/API.md`；② **D3 实现（另一路并行进行中，写入时尚未完成/提交）** —— `mrs-backend/internal/model/record.go`、`internal/handler/frontpage.go`、`internal/handler/qc_ext.go`、`main.go`（仅加注释）；另有 `internal/model/intelligence.go` 一处注释修正，归属待确认。
> 上述改动均未 commit，此处**不写提交号**。工作区正被并行修改，使用本文件时请以 `git status` / `git log` 当场复核。
> 现役状态：功能扩展批次 1 的 10 项能力已交付（刚性加权匹配度 78.2%，10 项中 6 项已补齐该项缺口）。

---

## ✅ 已完成：D1/D2 文档同步（2026-10-09，未提交）

| # | 原缺口 | 本轮补了什么 |
|---|---|---|
| D1 | 接口契约滞后（旧 `API.md` 缺批次 1 的 22 条新接口，且有 5 处描述与实现不符） | `mrs-backend/docs/API.md` 重写：新增 §11–§16 共 22 条接口文档（智能编码辅助 5 / DRG 预检 5 / 质量评分与退回返修 7 / 归档及时率与催缴 2 / 一致性核对 2 / 上报导出 2），另补 1 条历史遗漏 `GET /healthz`；现覆盖全部 **94 条**已注册路由（`TestApiCoverage` 双向核对 94/94 = 100%）；并修正 5 处失实描述（归档路径参数 `:id`、编目列表回复形态、人工催还无通知动作、首页保存与批次创建的 409 与留痕、目录条数） |
| D2 | SPEC 未收录批次 1 | `SPEC.md` 升级 V1.1：收录十项能力的功能域、状态机、数据模型（24 表）、页面落点、验证与边界 |

**逐条明细见 `mrs-backend/docs/API.md` §「变更记录」（2026-10-09 条目）**，本文不再复述。
该节还留了 2 条「实现与文档/设计不一致」的标注，均已转为下方候补项 D4、D5。

> ✅ **已核实并修正（2026-10-09）**：本轮草稿曾称 `sys_dept` / `sys_doctor` / `sys_dict` / `sys_role` 四表**未纳入** `main.go` 的 `AutoMigrate`。经复核当前文件与 `git show HEAD:mrs-backend/main.go`，**四表均在**迁移列表中（合计 24 张）——该说法**系误判**，`SPEC.md` §2.4 与 `API.md` §10 中的相关表述已删除并留勘误记录。**本轮新路由数为 93 条（不是草稿里的 105 条）**，经 `tools/check-api-coverage.py` 程序化核对为 100% 覆盖。

---

## 待确认的候补项：对齐 SPEC §2.3「已知缺口」D3–D8

编号与 `SPEC.md` §2.3「⚠️ 已知缺口」一致（D1/D2 已完成，不再列为候补）。工作量沿用 `SPEC.md` §「批次 1 遗留缺口的演进路径」的估算。

### 建议优先级

**D9（修数据丢失，最高）→ D3 前端收尾 → D5 / D6 / D8 → 批次 3**；D7 等官方文档到位再动。D3 后端与 D4 已落地（见下）。

### 已落地（2026-10-09，未提交）

| # | 状态 | 交付 |
|---|---|---|
| **D3（后端）** | ✅ 完成 | 首页新增 4 列（`external_cause_code` / `external_cause_name` / `newborn_birth_weight` / `newborn_gestational_weeks`），条件必填规则改指真字段；新生儿拆成「体重」「孕周」两条独立规则；`Save` 白名单透传；新增 `internal/service/qc_condition_d3_test.go`（9 个测试函数 / 37 子用例）。**前端输入项未做** |
| **D4** | ✅ 完成 | 新增 `GET /api/qc-results/my-pending`（临床侧按本科室、病案室全院督办）；`rectify` 权限收紧为 `DEPT_USER`+`ADMIN` 并对临床侧追加本科室校验；新增前端页 `frontend/js/components/rectify.js`（我的待整改 / 整改督办，按角色切换文案）；`layout.js` 新增角色门控；新增 `internal/handler/qc_ext_d4_test.go` |
| **D9（新发现）** | ✅ 已修 | `RunQc` 改为同键增量 upsert，重跑质控不再清空退回/整改/复核痕迹；新增 `internal/service/qcengine_upsert_d9_test.go`（16 个用例，含幂等与痕迹零漂移断言）。遗留：并发两次质控无唯一约束（重复键只在下次质控收敛）；已修正项仍删除（痕迹只进日志）；CLOSED 项再次命中不重开、回归信号偏弱 |

### 已落地（2026-10-16，未提交）

| # | 状态 | 交付 |
|---|---|---|
| **ADR-0002 批次 B 接线** | ✅ 完成 | 新增写路径入口 `service.CanonicalizeOtherDiagnoses`（校验+规范化，出错原样回显入参），`frontpage.Save` 在**事务前**调用：「重复编码 / 长度不等 / 非法标记」→ **400 拒绝**；`other_diagnosis_flags` 显式入 `Save` 白名单；前端 `api-store.js` 的 `FP_TEXT` 补入 `otherDiagnosisFlags`（此前漏传 → 后端全量赋值会清空该列，与 D3 同类数据丢失）。守卫测试 `internal/handler/frontpage_otherdiag_test.go`（nil-DB 技巧证「校验先于库」）+ `service/other_diagnosis_test.go` 三组用例；**负控制已做**（注入旁路→变红并精确指位→逐字节还原，sha256 一致）。契约总数不变（仍 94/94 = 100%） |
| **ADR-0002 批次 C/D + `cc_mcc_rule` 落表** | ✅ 完成（2026-10-27，详见下方「已落地」小节；本行保留评估摘要作历史记录） | 三项判定与实施序列见 `docs/adr/0002-批次C-D与落表评估.md`。摘要：**批次 C（低靠/高靠接入 `qc_rule`）= 首选**，不需官方表、不需新表，只加两个 WARN 级规则类型即可复用 `RunQc` 闭环；`cc_mcc_rule` 落表建议与 C 同批（单独落表目前只是把死代码挪进库，`JudgeSeverity` 尚无生产调用方）；批次 D 有前置（后端无返回严重度的接口、前端无 flags 录入 UI、种子首页其他诊断全空） |
| **行为规范可执行化（BDD 层）** | ✅ 完成（新增能力，非既有待办） | 新增 `mrs-backend/features/*.feature`（Gherkin，**只做引用不定义规则**，权威源＝SPEC §2.2 状态机 + API.md）+ 驱动套件 `mrs-backend/internal/bdd/`（godog，留在 `go test` 门禁内，不新增语言/CI）。当前覆盖「首页其他诊断写路径校验」与「接口权限闸门」两域，**11 场景 / 36 步全过**；真实全量路由 + db=nil 自足。已做负控制。**未做**：状态机流转类场景（需真库）、活服务 e2e（可按需另起 pytest-bdd 引用同一批 `.feature`） |

### 进行中（2026-11-06 复核）

| # | 状态 | 内容 |
|---|---|---|
| **自迭代开发循环 v0.1 · 试运行** | 🔄 首轮链路已跑通，**复盘 → v0.2 定稿未做** | 四项拍板（命名 = 自迭代开发循环；专家先启用 1 位；MedWork = 京医千询基座垂直领域 Agent · 远期插槽；元评审 = 下一议题）见 `docs/自迭代开发循环-v0.1.md` §6.0。首轮评审包 `批次C-CC-MCC口径-评审包-v1.md` 已送审并收 `回执-v1.md`（傅医保，2026-10-27，§8 列 7 条转译去向）；批次 C/D 已实现、门禁全绿、截图 + 录屏实机验证完成（见下方 2026-10-27 小节）。**剩余**：首轮复盘与 v0.2 定稿、元评审五机制落地（种子埋点 / 评分卡字段 / 转译率实测）、真人终审与 MedWork 远期接入、回执 §8 两条待补转译（ADR §6 边界 / Gherkin 正负例） |

### 已落地（2026-10-27，未提交）

| # | 状态 | 交付 |
|---|---|---|
| **ADR-0002 批次 C（低靠 / 高靠接入质控闭环）** | ✅ 完成 | 检测纯函数 `service/ccmcc_qc.go`（30 测试 / 96.5% 覆盖，负控制已做）；`qcengine.go` 扩展分支接入 `CC_MCC_LOW` / `CC_MCC_HIGH`，规则集**库优先、读不到降级演示表**（fail-soft，且空规则集不再静默）；`model/record.go` 加两个规则类型常量；种子 `seed/ccmcc_ext.go` 灌两条 **WARN** 规则 + 三份演示病案（低靠 / 高靠 / 负控制） |
| **`cc_mcc_rule` 落表** | ✅ 完成 | `model/ccmcc.go` + `service/ccmcc_store.go`（`LoadCcMccRules(db, version)`，空版本 fail-closed）；`main.go` AutoMigrate 登记 + 种子幂等灌 16 条 demo 规则。**故意不建唯一索引**（理由见模型注释：避免已有库 AutoMigrate 建索引失败 → 启动 Fatal） |
| **批次 D 后端出口** | ✅ 完成 | `handler/ccmcc_preview.go`：`POST /api/ccmcc-preview`（支持 `recordId` 与直接传码两种形态），响应**必带** `disclaimer`；契约 94 → **95/95 = 100%** |
| **批次 D 前端** | ✅ 完成 | 新组件 `frontend/js/components/ccmcc.js`；`catalog.js` 加「CC/MCC 严重度预览」入口与弹窗（含其他诊断填报）；`api-store.js` 加 `ccmccPreview`；`index.html` 加 script |
| **专家回执** | ✅ 完成 | `docs/专家评审包/批次C-CC-MCC口径-回执-v1.md`（傅医保，答送审包 7 问） |

**门禁**：build / vet / `go test ./... -count=1` 全绿；`gofmt -l` 空；15 个前端 JS `node --check` 全过；契约 95/95。

**本轮登记的缺口**：
- 错位脏数据（`other_diagnosis_flags` 与 `codes` 长度不等）在 CC/MCC 检测里是**静默**的（合法脏数据不做 WARN）→ 需新增规则类型；
- 「跨 MDC 且标 MCC」的高靠粗筛（需 MDC 分组数据）；
- 质控预警的「按科室月度视角」；
- 演示表条目补全（回执 §5 优先级：E11.2/.4/.5/.7 漏检面最大 → E86 / T80–T88 / N18.4 / J96.0/.9 / G30/G31 / K70/K74）；
- 「结算口径 vs 院内质控口径」的边界说明补进 ADR-0002 §6（回执 §7，**待补**：现 §6 只写「待确认的前提」，无两口径对比与「不可混用」结论）；
- **Z50 康复医疗场景**：本次住院针对脑梗后遗症做康复时，I69.3 的「主诊断 I63\* 时不计」判定会变化，演示表未覆盖（回执 §1，**本轮补登记**：该条此前只写在回执里，未落进缺口表）；
- 低靠 / 高靠正负例的 **Gherkin 场景**（回执 §8，**待补**：`features/` 目前只有 `front_page_other_diagnosis.feature` 注释引用 ADR，无可执行场景；单测 `ccmcc_qc_test.go` 已覆盖，但 BDD 层缺位）；
- 「跨 MDC 且标 MCC」的高靠粗筛信号（回执 §4 建议的最保守补充；与上列跨 MDC 项合并，触发条件 = 拿到 MDC 分组数据）。

#### 端到端实机验证（2026-10-27，截图 + 录屏）

**产物**：`docs/screenshots/01-login`…`08-qc-with-ccmcc`（1680×1050 静态图）、`docs/screenshots/ccmcc-demo-20261027.webm`（3.7M 录屏）、驱动脚本 `tools/shot-ccmcc.js`（`SHOT_VIDEO=1` 开录屏，可复跑）。

**动线**：登录 → 病案编目 → 选病案（`recordId=9`）→ 质控工作台 → CC/MCC 严重度预览 → 填报其他诊断 → 保存并重新判读 → 跑质控 → 回看质控项。

**实测结论**：两条 WARN 都落库并上屏——`ruleId=15 CC_MCC_LOW`（`OtherDiagnosisCodes`，I50.900 判读 MCC 却标 0）、`ruleId=16 CC_MCC_HIGH`（`OtherDiagnosisFlags`，K80.201 演示表无条目却标 C）；质量评分 **90 分 / 甲级 / 错误 0 / 警报 2**、首页状态 `QC_PASSED`。⇒ **「WARN 提示核对、不拦截」这条设计意图在真实界面上成立**，不是纸上结论。

#### 验证过程中修掉的三个实机缺陷（单测全绿也拦不住的那类）

| # | 症状 | 根因 | 修法 |
|---|---|---|---|
| 1 | 预览弹窗恒显「暂无判读结果」，控制台一个错都不报 | `api-store.js` 的 `req()` 已经 `return json.data`（第 51 行），`E.call` 又原样透传，而 `catalog.js` 再取了一层 `r.data` ⇒ `undefined` | `catalog.js` 改为 `(r && typeof r === 'object') ? r : null`，并把层级约定写进注释 |
| 2 | 「保存并重新判读」后编辑框仍显示保存前的值，看着像保存失败 | 复用的是**打开工作台那一刻**的 `qcData.frontPage` 旧快照，保存后没重拉 | `saveCcmccOtherDiag` 保存后 `GET /front-pages/:rid` 刷新 `qcData.frontPage` 再 `runCcmcc`；脚本里改成「先存干扰值 M,M、再存目标值 0,C」的两轮负控制，否则库值与快照相同、缺陷会被完全掩盖 |
| 3 | 静态服务 `favicon.ico` 404（控制台噪音一条） | 8090 未放置该文件 | 无害，**登记不修** |

另：CC/MCC 预览 + 其他诊断填报此前只被单测覆盖，缺端到端断言 —— 现已由 `tools/shot-ccmcc.js` 承担（含 `CCMCC_RENDERED` / `SNAPSHOT_REFRESHED` / `QC_CCMCC_HITS` 三个程序化断言，不靠肉眼看图判绿）。**遗留**：该脚本尚未进 `go test` 门禁（建议由 pytest-bdd 引用同一批 `.feature`，或直接固化为 CI 步骤）。

#### 本轮三条行为复盘（比起缺陷本身更值钱）

1. **基于旧快照判别人的活没干**：duplicate case 与测试期望串两次被判「未修」，均被施工单元的时间线证据推翻。已立规矩：**以 `go test` exit code 为唯一判据，回执只作补充**。
2. **演示数据会骗人**：`other_diagnosis_flags` 与 `codes` **按位一一对应**，本轮先填成 `C,0`（本意要 `0,C`），结果 I50/K80 都因「判读与标记一致」被**正确沉默**——症状与「规则没落库」一模一样，我差点又去查库。⇒ 端到端脚本里的断言数据必须先理清语义，且要让成功路径与失败路径产生**可区分的**输出。
3. **日志统计口径不能拍脑袋**：按 `r.ruleType` 统计命中数，可响应里根本没有这个字段，低靠那条因此被漏数成「命中 1 条」（实际 2 条），再一次把「我数错了」演成「功能少了」。⇒ **日志里凡依赖响应字段的统计，先把原始结构打印一份再写过滤器**。

#### 执行约定补充（第 10–12 条）

- **第 10 条**：改文案必须同步改测试期望串，且期望串**保持硬编码**（不许改成引用实现常量，否则失去「捕获文案意外变化」的能力）。
- **第 11 条**：前端 RPC 响应层级 —— `req()` 返回的是 `json.data`，调用方**不得**再解一层（见缺陷 1）。
- **第 12 条**：改任何 `catalog.js` / `layout.js` / `ccmcc.js` 后必须 bump `index.html` 里的 `?v=` 版本号（本轮 `catalog.js` 由 `20260921d` → `20261027`），否则浏览器缓存会让改动对用户不可见。

### 清单

| # | 缺口（一句话） | 涉及文件 / 模块 | 预估工作量 | 依据 |
|---|---|---|---|---|
| **D9** | **重跑质控会清空退回返修的处理状态**：`RunQc` 是「先删该病案全部质控结果、再重建」，重建时 `handle_status` 一律置 `OPEN`。对已退回/已整改的病案重跑质控（含编目动作链里的自动质控）会**丢掉 `RETURNED`/`RECTIFIED` 状态、退回意见与整改留痕**，医生端待办随之消失且无提示——**直接削弱 D4 闭环** | 后端 `internal/service/qcengine.go`（`RunQc` 的 `Delete` + `Create` 改为按规则**增量 upsert**，并保留 `handle_status` 及退回/整改字段） | 0.5–1 天（**建议最高优先**，是数据丢失而非观感问题） | D4 收尾时实测发现（`qcengine.go:100`）；SPEC §2.3 D9 |
| **D3（前端收尾）** | 条件必填的 4 个真字段**前端无处可填**，规则在界面上无法满足 | 前端 `frontend/js/components/archive.js`（首页录入表单）、`frontend/js/components/catalog.js`、`frontend/js/api-store.js`（`PUT /front-pages/:id` 载荷透传 4 键；未填须传 `null`，**不能用 0 占位**——0 会被质控判为空）；建议在「我的待整改」页加「修正首页数据」入口，与 D4 形成真闭环 | 1 天 | SPEC §2.3 D3；D3 交付报告「前端后续需要加什么」 |
| **D4（收尾建议）** | 两处低成本加固：① `Rectify` 未校验源状态（`OPEN` 项被调用也会变 `RECTIFIED`），建议只允许 `RETURNED`/`REJECTED_BY_QC`；② `front_page_qc_result` 无「整改说明」列，现借用 `front_page_revision.reason` 承载，建议加 `rectify_opinion` 列并在质控工作台展示 | 后端 `internal/handler/qc_ext.go`、`internal/model/record.go`（加列，AutoMigrate 自动补） | 0.5 天 | D4 交付报告存疑项 2、5 |
| **D5** | 变更留痕只覆盖首页：编目变更、病案主档（`med_record`）变更未留痕，《电子病历应用管理规范》要求的「归档后可追溯」只做到一半 | 后端 `internal/handler/qc_ext.go`（`recordRevision` 泛化）、`internal/handler/catalog.go` 与 `internal/handler/record.go`（写留痕）、`internal/model/qc_ext_models.go`（`FrontPageRevision` 泛化或新增表）；前端 `frontend/js/components/catalog.js`（`revisions` 接口已封装但**零组件调用**） | 1 天 | SPEC §2.3 D5；《电子病历应用管理规范（试行）》（国卫办医发〔2017〕8号）：归档后原则上不得修改、修改须留痕 |
| **D6** | 催缴只是「清单导出」：`GET /archive-overdue` 只输出清单，**无通知动作**（无站内信/短信/对接医生站） | 后端 `internal/service/overdue.go`（复用 `borrow_reminder` 模式做站内信）、`internal/handler/archive_stats.go`；前端 `frontend/js/components/quality.js`（看板加「催缴动作」入口） | 2–3 天 | SPEC §2.3 D6；A1 调研「每天电话催办 50 次」为核心痛点 |
| **D7** | 上报 CSV 为 18 列骨架：官方约有 90+ 采集项 | 后端 `internal/handler/report_export.go`（列集与导出前校验）、`internal/model/report.go` | **待官方文档**（当前无法估） | SPEC §2.3 D7；官方《住院病案首页数据采集质量与接口标准》 |
| **D8** | 两处近似实现：① 一致性核对用 `updatedAt` + 24h 容忍窗口近似判断「上报后被改」（`report_consistency.go` 中 `consistencyTolerance = 24 * time.Hour`），可能误判；② 质量评分口径（100 / ERROR −15 / WARN −5）为自定，非官方细则 | 后端 `internal/handler/report_consistency.go`（改用变更留痕时间戳，**依赖 D5**）、`internal/service/qc_score.go`（换官方细则） | 1 天（② 需先拿到官方细则，**待确认**，可能顺延） | SPEC §2.3 D8；`API.md`「待确认」第 5、6 条 |

### D3 收尾时必须一并处理的三件事（实测发现，勿遗漏）

1. **历史库不会被自动升级**：`SeedQcExt` 判重键是 `rule_name`，已跑过种子的库里那两条条件必填规则仍是占位版（`target_field` = `other_diagnosis_codes` / `ext_json`）。需手工 `UPDATE qc_rule …` 或删库重建——SQL 已写在 `internal/handler/qc_ext.go` 的 `SeedQcExt` 注释里。
2. **S/T 只覆盖了 S**：条件表达式引擎（`internal/service/qc_condition.go` 的 `likeCondRe`）只支持单前缀 `LIKE`，暂无 OR / 多前缀能力，故 `main_diagnosis_code LIKE 'S%'` 覆盖不到 T 编码。要么补条件引擎的 OR 支持，要么明确记为遗留（**待确认**取哪条路）。
3. **文档要同步**：D3 落地后 `SPEC.md` §2.3 的 D3 行、§2.4 数据模型（字段数）与 `mrs-backend/docs/API.md` 的首页字段说明都要更新——按下面「执行约定」第 5 条与代码同批提交。

### 关于 `main.go` 的 `AutoMigrate` 列表（D3/D4 都要碰它的说法需修正）

实测 `main.go` 的 `AutoMigrate` 调用（写入时位于第 42–52 行，该文件正被并行修改，行号会漂）已登记 24 张表，其中**含** `MedFrontPage`：

- **D3（已实测确认）**：给已在册的 `MedFrontPage` 加列，GORM `AutoMigrate` 会自动补列，**不需要**改这个列表——工作区里 D3 那一路只往 `main.go` 加了一段说明性注释，列表本身未动；
- **D4 的「我的待整改」若从既有 `front_page_qc_result` 派生**，也不需要新表（**待确认**，取决于实现是否引入新表）；
- 只有**新增独立表**时才必须往列表里登记（该文件正被他人修改注释，约定为「注册函数保持纯注册、迁移统一由 `main.go` 负责」）。

结论：「D3/D4 都必须改 `main.go` 的 `AutoMigrate` 列表」**不成立**——D3 已证实不需要，D4 视方案而定。但 **D3 与 D4 必须串行的结论不变**，真正的争用点是 `api-store.js` 与 `catalog.js`（见下节）。

---

## D3–D8 之外的候补（非 SPEC 缺口编号，保留原有依据）

### 性价比高、成本低（建议在 D5/D6/D8 同期或之后做）

| 项 | 说明 | 依据 |
|---|---|---|
| **编码员负荷看板** | 2021 版人力资源配置指标；数据已具备（`coder_name` + 编目时间戳） | 匹配度矩阵第 20 项（缺失） |
| **病案示踪时间轴** | 把现有 `archive_record` 流水聚合成时间轴视图；演示观感好 | 第 15 项（部分） |
| **统计服务端导出** | `/stats/export` 现返回 501，口径确认即可实现 | 第 19 项（部分） |

### 文档/结构一致性（本轮实测发现，成本极低）

| 项 | 说明 | 依据 |
|---|---|---|
| **`sql/schema.sql` 与模型对齐** | 手工 DDL 只覆盖**基线 20 表**，缺 `med_record.archived_at`/`cataloged_at`、D3 新增的 4 个首页列、批次 1 的 4 张扩展表；而 `README.md`/`DESIGN.md` 称它是「表结构唯一事实来源」——**实际事实来源是 `main.go` 的 AutoMigrate**（`cmd/createdb` 只 `CREATE DATABASE`，不建表）。两种修法：① 把 schema.sql 补成与 AutoMigrate 一致；② 明确降级 schema.sql 为「历史骨架参考」并改掉文档措辞。建议 ②，成本更低且不再产生"第二事实来源" | 本轮 D1/D2 同步时实测（`SPEC.md` §2.4 已加说明） |
| **`service/reportclient.go` 上报 payload 未含 D3 新字段** | 该文件有自己的字段白名单，新增的 `external_cause_code` 等不会进入上报 XML/CSV。若上报需要这些数据元需补白名单 | D3 交付报告的存疑项 |
| **`service/qc_condition.go` 的 `frontPageFieldString` 未映射 D3 新字段** | 导致新字段**不可用于 `conditionExpr`**（写了会取空串 → 条件不成立 → 规则静默跳过）。已用测试固化该限制，但映射表应补上以消除陷阱 | D3 交付报告的存疑项 |

### 合规类（中低成本）

| 项 | 说明 |
|---|---|
| 病历保存年限管理 | 住院 30 年 / 门诊 15 年，加字段 + 到期预警 |
| 病案复印服务 | 复用借阅子状态机（原型决策 D7 已留 API 位） |
| 数据一致性核对扩展 | 已做上报侧，可扩展到首页 vs 月报 |

### 需外部条件（暂缓）

| 项 | 阻塞原因 |
|---|---|
| DRG 真实分组器 | 需授权分组规则与各省权重数据 |
| 官方上报通道 | 需官方接口文档（调研文档明确「禁止以口头需求为设计依据」）；与 D7 同一阻塞 |
| 无纸化 / HIS-EMR 采集 | 基线明确不做 |

---

## 批次 3：智能编码升级（**方案已定：本地部署，见 [ADR-0001](../adr/0001-本地向量召回与LLM精排-for-ICD编码辅助.md)**）

> **优先级定位**：本批次排在 **D3/D4（功能闭环补完）→ D5/D6/D8（低成本候补项）** 之后。
> 理由：D3/D4 补的是**已交付能力里的语义/闭环缺陷**（占位字段、缺医生端），属"把已承诺的事做实"；批次 3 是**新增能力**，需引入 pgvector 与本地模型，成本与不确定性都更高。

> ⚠️ **本批次方案已由 [ADR-0001](../adr/0001-本地向量召回与LLM精排-for-ICD编码辅助.md) 重新决策**（2026-10-09）：
> **改为本地部署，不用外部 API**；原「opencode zen + 三阶段」方案**作废**。以下按 ADR 重写。

**目标**：把现有「规则版预编码」（31 条关键词规则 + `POST /code-suggest`）升级为「**语义召回 + LLM 精排**」，从"关键词字面命中"变为"理解语义 + 给出理由"。

**产品定位（不可动摇）**：本能力是**模型辅助编码 = 推荐**，不是模型直接编码。模型给候选 + 理由 + 证据片段，**编码员点击确认才填入**，决定权与责任在人。

### 已确定的技术选型（ADR-0001）

| 项 | 取值 | 备注 |
|---|---|---|
| **部署形态** | **全部本地部署** | 外部 API 方案作废：患者数据不出域；且终态本就是本地，先外部后本地等于写两遍 |
| 召回层 | **本地 embedding 模型** | 把病历文本与 ICD 字典条目编码到同一向量空间，取相似度 TopK。**具体型号待定**（ADR §8） |
| 精排层 | **本地生成式 LLM** | 从 TopK 候选中挑 1~3 条**推荐**给编码员 + 理由 + 证据片段 |
| 向量存储 | **pgvector** | 复用现有 PostgreSQL 16，不引 Milvus/Qdrant |
| 推理服务 | **待定**（Ollama 候选） | 需同时提供 embedding 接口与 chat 接口（ADR §8） |
| ~~LLM 服务~~ | ~~opencode zen 外部 API~~ | **已作废**，见 ADR-0001 |
| ~~Embedding~~ | ~~Qwen3-Embedding~~ | 原定型号，**待重新评估**（本地部署下需考虑体积与推理硬件） |

### 架构（三段式，防幻觉）

```
病历文本
  ↓ ① 召回：本地 embedding → pgvector 相似度检索 TopK 候选
  ↓ ② 精排：本地 LLM 从候选中挑 1~3 条 + 证据片段 + 理由   ← 关键：限制输出空间，不自由生成
  ↓ ③ 校验：编码回查 icd_dict（复用现有 CODE_EXIST 机制）
  ↓ ④ 展示：候选 + 理由 + 证据片段 → 编码员点击确认（界面「智能推荐」已就位）
```

**核心约束**：LLM **只能从候选清单里选**，不得自行生成编码——编码错误直接关联 DRG 拒付，幻觉不可接受。
**降级路径（硬要求）**：本地推理服务不可用时，**自动回落到现行 31 条规则版**，界面明确提示"当前为规则版推荐"，不伪装成语义结果。

### 交付范围与暂缓项

- **本次交付**：本地召回 + 本地精排 + 校验 + 降级路径，接入现有「智能推荐」入口
- **刻意暂缓**（见 ADR §3）：召回率基线摸底、外部/本地 A/B 对比。**口径是"能用优先，指标后补"**
- **指标补测**（不阻塞开工，见 ADR §9）：正确编码是否进 Top3/Top10；仅召回 vs 召回+精排的增益；单次推荐 P50/P95 延迟

### 依据（行业实证）

- 中山八院+哈工大「智码系统」：两阶段标注 + 思维链微调，**显式呈现编码证据片段**（可解释性是采纳前提）
- 杭州市妇产科医院：Ollama + RAG + 私有向量库，**离线、数据零外泄** ← 与本 ADR 的本地部署决策同路
- 智慧眼「砭石大模型」：编码约 10 秒，精准率 91.7%，含**主诊断与主手术关系校验**（我们已有 `DIAG_OP_MATCH`）
- 某三级医院（中华医院管理杂志 2025）：漏写率 31.31%→12.34%，质控时间 20→5 分钟
- medRxiv 2026（德国肿瘤）：Qwen3-Embedding 最佳，RAG 把 partial-match 提到 80.6% ← 但该结论的前提是**有评测集**；我们暂缓基线测量，故此条仅作选型参考，不作效果承诺
- JMIR 2025 / MIMIC-IV 多阶段：混合架构（编码器抽取 + LLM + RAG 校验）为主流

### 我们已有的优势（降低实现成本）

- PostgreSQL → **直接上 pgvector**，零新组件
- `CODE_EXIST` 校验机制 → ③校验层复用
- `DIAG_OP_MATCH` 规则 → 与行业"主诊断-主手术校验"同思路
- 前端「智能推荐」入口与"编码员负责"文案 → **界面无需改动**（本次只换候选来源）
- `code_suggest.go` 的 `POST /code-suggest` 契约可保持不变 → 前端零改动即可在规则版/语义版之间切换
- `.env` 加载机制 → 后端读环境变量即可

---

## 执行约定

1. **并发拆分原则**（沿用批次 1 经验）：按**文件所有权**划分，主干改现有热点文件、旁支只加新文件——避免 `qcengine.go` / `router.go` / `api-store.js` 等热点冲突
2. 每批完成后：`go build` + `go vet` + 路由冒烟测试 + 浏览器实测 + 一次 git 提交
3. 界面必须同步：功能只落后端 = 用户看不到（批次 1 的教训）
4. 提交信息写清"依据的政策/需求编号"，便于回溯
5. **文档与实现同步**（新增，针对 D1/D2 这类滞后）：接口、状态机、数据模型一有变更，就和代码**同一批次**提交，并当场补写 `SPEC.md` 对应章节与 `mrs-backend/docs/API.md` 的「变更记录」——不要攒到"下一轮文档同步"
6. **文档不得断言未实测的事实**：写"已实测确认"之前必须真的查过代码/路由。**本轮踩了两次**：① 曾断言 `sys_dept/sys_doctor/sys_dict/sys_role` 未纳入 `AutoMigrate`（实际 24 张表都在）；② 曾写"105 条路由 / 前端调用 46 条"（实测 94 条；"46"是未经核对的估计，用正则试算得到 46/31/27 三个互相矛盾的结果）。教训：**数量类断言一律程序化核对，不靠估算**。
7. **验证命令必须带 `-count=1`**（新增，踩坑发现）：`frontend/` 在 Go module（`mrs-backend/`）之外，而 Go 的测试缓存**不跟踪 module 外的文件**（`cmd/go/internal/test` 只把 module 根内的文件计入缓存 key）。因此只改前端、不改 Go 代码时，`go test` 会命中 `(cached)` **假绿灯**。实测：注入一个不存在的接口路径后，不加 `-count=1` 显示 `ok (cached)`，加上才 FAIL。**凡涉及前端的验证，一律 `go test ./... -count=1`**。
8. **测试要么能红，要么等于没写**（新增）：本轮新增的三个检查（`TestApiCoverage`、`TestFrontendCallsResolveToRegisteredRoutes`、D3/D4/D9 的单测）都做了**负控制**——故意注入错误确认测试会失败并精确指认位置，再逐字节还原（附 sha256/`git diff --numstat` 佐证）。新写守卫性测试时请照此办理。
9. **不要用 `Get-Content -Raw` / `Set-Content` 读写仓库里的源码或文档**（新增，踩坑发现）：实测会静默吃掉换行（某文件 852→842 行），导致行号偏移与内容损坏。改用字节级读写或就地编辑。
10. **并行切包必须保证文件路径唯一，且主线程不写 agent 拥有的文件**（2026-11-06 新增，踩了两次）：本轮我给 archive agent 派了「归档+借阅」两域，它在写 `tools/shot-archive-borrow.js`；我同时独立写了**同名**文件 → 两边互相覆盖，产物来回换手。同样的事在 rectify 域又发生一次。⇒ 切包时先登记「每个 agent 独占哪些路径」，主线程要么等、要么另起文件名。
11. **本机沙箱会在每次工具调用结束时回收该调用派生的所有子进程**（2026-11-06 新增，踩了两次）：用 `run_in_background` 起的后端/前端，**一到下一次工具调用就死了** —— 表现为「脚本跑到一半 `fetch failed`」「curl 返回 502」，极容易被误判成「代码坏了」。正解是 `tools/with-services.sh`：把「起 PG + 起后端 + 起前端 + 跑脚本 + 收服务」塞进**同一个 shell 调用**。另：`curl` 探活要加 `--noproxy '*'`（本机 `http_proxy` 把 127.0.0.1 也代理了，**502 ≠ 服务挂**）。

---

## 演示录制实测发现的业务缺陷（2026-11-06，均已独立复核）

> 来源：补录各功能域演示视频时，程序化断言 + 人工看画面暴露出来的**真实业务问题**。
> 全部是「功能实现与界面呈现不一致」类，**不是**环境问题。修复前先补测试，再改。

| # | 缺陷 | 证据位置 | 影响 |
|---|---|---|---|
| **B1** | **借阅期限被忽略**：表单里填的天数**根本没传给接口**，前端 `api-store.js:679` 硬编码 `+14` 天，后端 `handler/borrow.go:46` 又按配置 `CFG.Borrow.BorrowDays`（默认 30）自己算 | 三处口径互不一致 | 演示中「填 30 天、实际应还 14 天」；真实使用期限不可控 |
| **B2** | **质量看板的「状态」列显示原始枚举**：`quality.js:259` 直接渲染 `r.status`，而 `archive-overdue` 接口返回的是 `RECYCLED`/`NOT_RECYCLED` 等原始值，**没走 `REC_PROJ` 投影**（投影只在 `api-store.js:144` 的 records 列表里做） | `quality.js:259` vs `api-store.js:144` | 催缴清单的状态列显示英文枚举，界面不自洽 |
| **B3** | **统计页「病案质量等级」环图恒为 0**：`statistics.js:65` 的 `qualityDist` 按 `r.quality === '甲级'` 过滤，但 `records` 列表里**根本没有 `quality` 字段**（质量等级在 `front_page_quality` 表，需另调接口） | `statistics.js:65-75` | 图表永远三块全 0，等于没实现 |
| **B4** | **疾病谱 Top5 被空诊断挤占**：`statistics.js:101` 的 `map[r.diagMain]` 未过滤空值，未编目病案的 `diagMain` 为空 ⇒ 聚成一大坨 | `statistics.js:99-104` | Top5 排名失真，出现「未诊断/空」占位 |
| **B5** | **借阅单号列显示的是主键 id**，不是业务单号（`JY2026110615xxxx`）——后端两个字段都返回了，前端取了 id | 借阅台账表格 | 与「单号」的业务含义不符 |
| **B6** | **没有独立的「借出」按钮**：`approveBorrow` 内部链式 approve+lend，界面上只有「通过」，没有「借出」 | `borrow.js` / `api-store.js` | 与《医疗机构病案管理规定》「申请→审批→借出→归还」四态模型不符，界面少一态 |
| **B7** | **借阅页没有「待审批申请 / 台账」页签**，实际是 4 个 KPI 卡 + 单表格 + 状态下拉 | `borrow.js` | 与设计文档描述不符（文档滞后，非代码 bug） |

> **B2/B3/B4 是同一类病根**：前端各处**各自**做聚合/投影，没走统一的口径层。
> 建议在 `api-store.js` 增设「视图模型投影」统一出口（`toViewModel(record)`），所有组件只消费投影结果 —— 这与本项目已登记的「第二事实来源」教训同源。


---

## 下一次并发拆分的具体建议

### 热点文件（会被多项任务争用，改动前先认领）

| 层 | 文件 | 为什么是热点 |
|---|---|---|
| 后端 | `mrs-backend/internal/router/router.go` | 所有新路由注册的单一入口 |
| 后端 | `mrs-backend/internal/service/qcengine.go` | 质控主链（规则执行 → 落库 → 状态推进），D3/D5/D8 都想动 |
| 后端 | `mrs-backend/main.go` | `AutoMigrate` 列表 + 种子调用；新增表必须在此登记（该文件当前正被他人改注释） |
| 后端 | `mrs-backend/internal/handler/qc_ext.go` | 退回/整改/复核/留痕全集中在同一个文件，D4 与 D5 都指向它 |
| 前端 | `frontend/js/api-store.js` | 唯一 API 适配层，任何新接口都要在这里加方法 |
| 前端 | `frontend/js/components/catalog.js` | 编目 + 首页摘要 + 质控工作台三合一，D3/D4/D5 都想改 |
| 前端 | `frontend/js/components/layout.js` | `NAV` 菜单 + `pageMap` 路由；**新增页面必改**。注意现状是**没有任何角色过滤**（`NAV` 静态、双角色看到同一套菜单），D4 若要「仅医生可见」需**新增**角色门控 |
| 前端 | `frontend/index.html` | **零构建**：新增组件文件必须加 `<script>` 标签，否则页面加载不到 |

### 推荐的任务切分方式

- **可并行组**：只加新文件、不碰上表任何一个的任务。例如
  - D6 的站内信可新建 `internal/service/notify.go`，只在 `router.go` 加**一行**注册（该行由主干任务统一收口）；
  - D7 可新建列集文件，不散改 `report_export.go`。
  - 旁支任务**不改** `api-store.js`：把需要的方法签名写在交付说明里，由主干任务一次性加齐。
- **必须串行的组合**：
  - **D3 与 D4 必须串行**（详见下）。
  - **D8 依赖 D5**：D8 要把「上报后被改」的判定从 `updatedAt` 近似换成真实的变更留痕时间戳，而留痕本身由 D5 补齐——这是**数据依赖**，不是文件冲突，串行做 D5 → D8。
- **主干任务**：每批指定一个任务独占 `router.go` / `api-store.js` / `layout.js` / `index.html` 的改动权，其余旁支只提交新文件；批次收尾由主干统一跑 `go build` + 路由冒烟 + 浏览器实测，然后**一次提交**。

### 特别提示：D3 与 D4 不能按"只加新文件"拆分，必须串行

两者都会改 **`frontend/js/api-store.js`**（D3 加首页新字段的读写、D4 加「我的待整改」查询与整改提交，都落在同一个 API 适配层）和 **`frontend/js/components/catalog.js`**（D3 要在首页数据展示/录入里体现新字段，D4 要复用编目页的质控工作台视图给病案科侧复核）。这两处都是现有文件的编辑，不是新增文件，并发派发必然互相覆盖。

**现场证据（写入本文件时实测 `git status`）**：D3 那一路已经改了后端 4 个文件（`model/record.go`、`handler/frontpage.go`、`handler/qc_ext.go`、`main.go` 注释），但 `frontend/` **一个文件都没动**——也就是说 `api-store.js` / `catalog.js` 的改动仍然挂在 D3 名下尚未落地。此刻若并发派发 D4，两边迟早要改同一批前端行，冲突几乎必然。

此外 D4 还要动 `frontend/js/components/layout.js`（新页面入 `NAV`/`pageMap`，并**新建**角色门控）与 `frontend/index.html`（零构建加 script 标签）；若 D4 的实现选择新增数据表，还会额外改 `main.go` 的 `AutoMigrate` 列表（仅插入行也会冲突）。

> 结论：**「D3 与 D4 必须串行」成立**，决定性理由是 `api-store.js` + `catalog.js`（其次 `layout.js` + `index.html`）的共享编辑，与 `main.go` 无关——`main.go` 那一条经实测**不成立**（D3 给已在册的 `MedFrontPage` 加列由 GORM 自动补列，工作区里只加了注释），D4 是否要改视其方案是否引入新表而定（**待确认**）。

**推荐排期**：`D3 → D4`（串行，同一主干任务；**D3 后端已在进行中，D4 须等 D3 的前端落点 `api-store.js` / `catalog.js` 收口后再开**）→ `D5 → D8`（串行，可另起主干）→ `D6`（可并行旁支）→ 低成本候补项 → `D7`（等官方文档）→ 批次 3。
