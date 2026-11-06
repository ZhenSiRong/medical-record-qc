# mrs —— 病案管理系统 · 首页质控与 CC/MCC 严重度预警

以**病案首页**为核心，跑通「登记 → 回收上架 → ICD 编目 → 首页质控 → 退回整改 → 上报 → 统计」全链路的教学/演示型系统：**Go + Gin + GORM + PostgreSQL** 后端、**Vue 3 全局版零构建**前端、**95 条 REST 接口**、三条状态机、规则驱动的质控引擎。

最值得看的一块是 **ADR-0002 的 CC/MCC 严重度预警**：在**不出具 DRG/DIP 分组码与权重**的前提下，用一份**可替换、版本化的演示判读表**识别「漏编并发症（低靠）」与「套高（高靠）」，产出 **WARN 级、不拦截**的质控提示，并在**界面与接口双重标注**「非官方、不得作为结算依据」。

> **⚠️ 边界（务必先读）**：仓库内所有患者、病案、编码、医师数据均为**虚构演示数据**；`cc_mcc_rule` 表的 16 条判读规则 `Source=demo`，**不是国家医保局官方 CC/MCC 表**。系统只做院内编码质控提示，**不出具 DRG/DIP 分组结果、不计算权重、不作为医保结算依据**。生产落地须先取得属地医保官方文件并替换规则表（广东 DIP 与 CHS-DRG 逻辑不同，口径不可混用）。

---

## 演示

本仓库有**两套实机录屏**，互补而非重复：

| 录屏 | 时长 | 覆盖 | 适合谁看 |
|---|---|---|---|
| **① 全系统总览**（`tour/demo.webm`） | 78.7 秒 | **8 个菜单页全部走一遍**，每页展示核心内容 + 打开该域关键弹窗 | 想先知道「这系统长什么样」的人 |
| **② CC/MCC 深度演示**（`ccmcc-demo-20261106.webm`） | 48.5 秒 | 首页质控与 CC/MCC 严重度预警的**完整操作闭环** | 想学「CC/MCC 预警怎么用」的人 |
| **③ 各功能域深度演示**（`archive/ borrow/ stats/ quality/ report/ rectify/ ai-assist/`） | 各 30–90 秒 | 单域**操作闭环**（真点按钮、真跑流程、真落库） | 想逐个功能看操作过程的人 |

### ① 全系统总览（78.7 秒 · 8 个菜单页）

> 下面是**实机录屏**（Playwright 驱动本机 Chrome 真实点击，非模拟、非拼接）：登录 → 工作台 → 病案归档 → 归档详情 → 病案编目 → 质控工作台 → 病案借阅 → 借阅申请 → 数据统计 → 首页数据上报 → 质量看板 → 我的待整改 → CC/MCC 严重度预警。
>
> 画面带**人工化操作标注**：橙色鼠标光标、点击涟漪、被点元素高亮，以及顶部「步骤 N/12」进度条 —— 由录屏脚本**注入页面**，**不是产品 UI**，目的是让观众不用倒带也能看清操作顺序。

<img src="https://gcore.jsdelivr.net/gh/ZhenSiRong/medical-record-qc@main/docs/screenshots/tour/demo.gif" width="620" alt="全系统总览：8 个菜单页依次走一遍的操作动图">

[打开 GIF 原图](docs/screenshots/tour/demo.gif) · [全屏播放完整录屏（WEBM，78.7 秒，10.3 MB）](https://gcore.jsdelivr.net/gh/ZhenSiRong/medical-record-qc@main/docs/screenshots/tour/demo.webm) · [录屏脚本](tools/shot-tour.js)

### ② CC/MCC 深度演示（48.5 秒）

> 下面这段动图取自 **48.5 秒实机录屏**（Playwright 驱动本机 Chrome、2 fps 抽帧，无模拟数据、无截图拼接）：编目 → 选病案 → 质控工作台 → CC/MCC 严重度预览 → 填报其他诊断 → 保存重判 → 跑质控 → 回看两条 WARN。
>
> GitHub 不支持在 README 内嵌视频；且 GitHub 原生相对路径会走 `raw.githubusercontent.com`，国内网络常加载失败，因此图片统一走 **jsDelivr CDN**，并在每张图下给出仓库内原图兜底链接。

<img src="https://gcore.jsdelivr.net/gh/ZhenSiRong/medical-record-qc@main/docs/screenshots/demo.gif" width="660" alt="病案编目 → CC/MCC 严重度预览 → 两条 WARN 质控项 的完整操作动图">

[打开 GIF 原图](docs/screenshots/demo.gif) · [下载 / 全屏播放完整录屏（WEBM，48.5 秒，5.9 MB）](https://gcore.jsdelivr.net/gh/ZhenSiRong/medical-record-qc@main/docs/screenshots/ccmcc-demo-20261106.webm) · [录屏驱动脚本](tools/shot-ccmcc.js)

> **光标是怎么进画面的**：Playwright 录屏默认**不录系统指针**（headless 下根本没有指针），所以脚本往页面里注入了一个跟随真实 `mousemove` 的虚拟光标；同理 `mouse.move(..., {steps})` 走完只要几毫秒、25fps 只会拍到「瞬移」，因此鼠标轨迹是脚本自己**分步插值 + 逐步延时**画出来的。两处都是「让录屏可读」的技术处理，与产品功能无关。

### ③ 各功能域深度演示

> 每个功能域一段独立录屏，各自跑完整操作闭环（真点按钮、真跑流程、真落库）。
> 录制脚本全部在 `tools/` 下，可复现；每段都有程序化断言（`console.log` 输出计数与状态），**不靠肉眼看图判绿**。
>
> 所有录屏共享 `tools/shot-lib.js` 里的**「人类化操作层」**：虚拟光标、点击涟漪、被点元素高亮、顶部步骤条、拟人点击与逐字打字。标注是**注入页面的**，不是产品 UI。

| 功能域 | 录屏 | 演示脚本 | 演示内容 |
|---|---|---|---|
| 全系统总览 | `docs/screenshots/tour/demo.webm` | `tools/shot-tour.js` | **12 步走完 8 个菜单页**：工作台 → 归档 → 归档详情 → 编目 → 质控工作台 → 借阅 → 借阅申请 → 统计 → 上报 → 质量看板 → 待整改 → CC/MCC 预览（78.7 秒） |
| 病案归档 | `docs/screenshots/archive/demo.webm` | `tools/shot-archive-borrow.js` | 归档列表（10 行 / 共 87 份）、详情抽屉（19 个字段）、登记表单（15 项，**填了不提交**）、批量归档入口、导出清单（50.3 秒） |
| 病案借阅 | `docs/screenshots/borrow/demo.webm` | `tools/shot-archive-borrow.js` | 申请 → 审批 → 借出 → **续借 7 天** → 归还核销 → 台账导出 → 逾期标红（63.3 秒） |
| 数据统计 | `docs/screenshots/stats/demo.webm` | `tools/shot-stats-quality.js` | 8 张图表（状态环图 / 疾病分类 / 质量等级 / 趋势 / 住院日与费用分布）+ 科室明细表 + 导出（56.3 秒） |
| 质量看板 | `docs/screenshots/quality/demo.webm` | `tools/shot-stats-quality.js` | 2 日归档率 **16.1%** / 3 日 **28.7%** / 平均归档天数 **9.87** / 月度趋势 / 科室排名 / 超期催缴清单（50.6 秒） |
| 首页上报 | `docs/screenshots/report/demo.webm` | `tools/shot-report.js` | 批次管理 → 新建批次 → 上报前校验 → **一致性核对** → 导出前检查 → 导出 CSV（88.3 秒） |
| 我的待整改（病案室） | `docs/screenshots/rectify/demo.webm` | `tools/shot-rectify-ai.js` | 整改督办清单、修正首页数据弹窗（带退回意见原文）、刷新（30.6 秒） |
| 我的待整改（临床） | `docs/screenshots/rectify-clinical/demo-clinical.webm` | `tools/shot-rectify-ai.js` | **科室医师视角**：待整改清单 → 提交整改 → 填写整改说明 → 提交成功（27.2 秒） |
| 智能辅助 | `docs/screenshots/ai-assist/demo.webm` | `tools/shot-rectify-ai.js` | 智能预编码推荐（`K35.801 急性阑尾炎 命中「阑尾炎」· 权重 10`）、DRG/DIP 预检、免责措辞逐字（36.7 秒） |

**怎么自己重录一遍**（仓库根目录）：

```bash
# 本机沙箱会在每次工具调用结束时回收子进程 ⇒ 用包装器把「起服务 + 跑脚本」放进同一个调用
bash tools/with-services.sh tools/shot-tour.js                  # 全系统总览
SHOT_VIDEO=1 bash tools/with-services.sh tools/shot-tour.js    # 同上（录视频）

bash tools/with-services.sh tools/shot-archive-borrow.js       # 归档 + 借阅
bash tools/with-services.sh tools/shot-stats-quality.js        # 统计 + 质量看板
bash tools/with-services.sh tools/shot-report.js               # 上报
bash tools/with-services.sh tools/shot-rectify-ai.js           # 待整改 + 智能辅助
bash tools/with-services.sh tools/shot-ccmcc.js               # CC/MCC 深度演示

bash tools/compress-videos.sh 3        # 批量压缩（VP9/CRF，README 挂 CDN 需要）
```

> 依赖 `playwright-core`（**不下载浏览器内核**，直接驱动本机已装的 Chrome）+ 本机已启动的后端 :8080 / 前端 :8090 / PostgreSQL。
> 动图合成：`python tools/make-gif.py <帧目录> <输出.gif>`（Pillow 统一调色板，避免帧间色带；自动剔除开头的空白起幅帧）。
>
> **踩过的坑**（写在这里省后来人时间）：
> - `S.click()` 对**视口外**的元素会点空（`boundingBox()` 是页面坐标，`mouse.move` 是视口坐标）→ 库里已内置 `scrollIntoViewIfNeeded`。
> - 原生 `confirm()` 会被 Playwright 默认 dismiss **静默中止** → 必须 `page.on('dialog', d => d.accept())`。
> - 注入的步骤条 `#vb-steps` 会盖住 `.toasts` 提示 → 需要 `.toasts{top:74px !important;}`。
> - Playwright 自带 ffmpeg 是**精简版**：有 vp9 编码器但**没有 concat 滤镜 / concat demuxer / gif 编码器** ⇒ webm 拼接只能改走「重新录一条」，GIF 只能走「抽帧 → Pillow」。

### 演示动线：9 步，每步都有可验证的断言

> 下表编号与录屏顶部「步骤 N/9」进度条**一一对应**，可以对着进度条看视频。

| 步 | 操作 | 系统行为 | 实测结果 |
|---|---|---|---|
| 1 | 打开系统并登录（`admin / admin123`） | JWT 签发、按角色装配菜单 | 工作台显示 **87 份**在院病案、归档完成率 **81.6%**、编目完成率 **74.7%** |
| 2 | 侧栏「病案编目」 | 拉待编目队列 + ICD 字典 | 队列 **16 份**，ICD-10 / ICD-9-CM-3 检索可用 |
| 3 | 选中第一份病案 `BAZ0200009` | 首页快照载入编目工作台 | 患者蒋淑桂 · 心血管内科 · 主诊断 **I61.901**（脑出血）· 费用 102,523.69 元 |
| 4 | 点「质控工作台」 | 聚合首页 + 质控结果 + 处理按钮 | **90 分 / 甲级 / 错误 0 / 警报 2 / `QC_PASSED`** |
| 5 | 点「CC/MCC 严重度预览」 | `POST /api/ccmcc-preview`，库优先读规则、读不到降级演示表 | 本例总分层 **MCC**；`I50.900 → MCC（命中前缀 I50）`、`K80.201 → 无`；`version=demo-2026.10`、`demoOnly=true`、`disclaimer` 非空 |
| 6 | 其他诊断填 `I50.900,K80.201`、标记填 `0,C` | 编码与标记**按逗号位置一一对应** | 视频里逐字输入，不是整段赋值 |
| 7 | 点「保存并重新判读」 | PUT 首页 → **GET 刷新快照** → 重新判读 | 弹窗回填 `0,C`（不是保存前的旧值） |
| 8 | `POST /api/front-pages/9/qc` | 规则驱动质控、按 `(recordId, ruleId, fieldName)` 增量 upsert | 落库 **2 条 WARN**，`results` 非空 |
| 9 | 回看质控工作台 | 两条 WARN 各自独立成行 | `ruleId=15` 低靠 / `ruleId=16` 高靠；**评分仍 90 甲级、错误 0、状态仍 `QC_PASSED`** |

> 第 9 步是整段演示的落点：**预警出现，但病案没有被判为退回**——这正是「WARN 提示核对、不拦截」的设计意图，而不是漏判。

### 关键画面

**① 工作台**：三条状态机的汇总视图（月度出院归档趋势、病案流转状态环图、科室病案量排名、待办事项）。

<img src="https://gcore.jsdelivr.net/gh/ZhenSiRong/medical-record-qc@main/docs/screenshots/02-dashboard.png" width="760" alt="工作台总览">

[原图](docs/screenshots/02-dashboard.png)

**② 严重度分层预览**：逐条其他诊断给出严重度、命中前缀、来源与排除原因；红色免责块取自**接口返回的 `disclaimer`**，前端不自造文案。副标题「演示口径 · 非官方判定」是常态而非警告色装饰。

<img src="https://gcore.jsdelivr.net/gh/ZhenSiRong/medical-record-qc@main/docs/screenshots/06-ccmcc-preview.png" width="760" alt="CC/MCC 严重度分层预览弹窗">

[原图](docs/screenshots/06-ccmcc-preview.png)

**③ 保存并重新判读**：编辑框回填的是**刷新后**的 `0,C`，判读明细同步更新——这一屏专门用来证明「保存 → 刷新快照 → 重判」没有断链。

<img src="https://gcore.jsdelivr.net/gh/ZhenSiRong/medical-record-qc@main/docs/screenshots/07-ccmcc-after-save.png" width="760" alt="保存并重新判读后的预览">

[原图](docs/screenshots/07-ccmcc-after-save.png)

**④ 两条 WARN 上屏**（核心）：低靠与高靠按不同 `fieldName` 独立成行，不会被 `mergeQcResults` 合并成一条。

<img src="https://gcore.jsdelivr.net/gh/ZhenSiRong/medical-record-qc@main/docs/screenshots/08-qc-with-ccmcc.png" width="760" alt="质控工作台里的低靠与高靠两条 WARN">

[原图](docs/screenshots/08-qc-with-ccmcc.png)

### 两条 WARN 逐字长什么样

| 规则 | 归属字段 | 级别 | 文案（实测落库） | 判定依据 |
|---|---|---|---|---|
| `CC_MCC_LOW`（id=15） | `OtherDiagnosisCodes` | `WARN` | 其他诊断【I50.900】在演示判定表中为 MCC，但未标记为并发症/合并症，建议核对是否漏编；**如属陈旧性或既往史，可忽略本提示**（演示口径，非官方判定，不作结算依据） | `I50.900` 判读 MCC，标记却是 `0` |
| `CC_MCC_HIGH`（id=16） | `OtherDiagnosisFlags` | `WARN` | 其他诊断【K80.201】被标记为 CC，但在演示判定表中不构成 CC/MCC（表内无对应条目），建议核对高靠风险与病案依据（演示口径，非官方判定，不作结算依据） | `K80.201` 标记 `C`，演示表内无对应条目 |

两条必须**各自独立成规则**：质控结果的判重键是 `(record_id, rule_id, field_name)`，同一条规则产出两条同字段消息会被合并成一条。低靠文案里那段「可忽略出口」来自专家回执的**降噪清单**（陈旧性/既往史、入院前已存在且无影响的慢性病、仅为排除而检查后排除的诊断、与主诊断同一疾病过程、产科/新生儿生理性条目），避免把正当不作为并发症的情形误报成问题。

### 为什么是 WARN 而不是 ERROR

| 维度 | 取值 | 理由 |
|---|---|---|
| 判读依据 | 院内演示表 `Source=demo` | 16 条前缀规则远非官方表，判错率不低 |
| 级别 | `WARN` | `RunQc` 只对 `ERROR` 计 `errCount` 并置 `QC_REJECTED`；写成 ERROR 会把一批合规病案批量判为退回，属回归事故 |
| 评分影响 | −5 分/条 | 100 → 90，等级仍**甲级**，单项否决不触发 |
| 流程影响 | 无 | 状态仍 `QC_PASSED`，不进退回返修闭环 |
| 责任人 | 编码员 / 病案室（第一），质控办 / 医保办（第二） | 提示核对，不替代人工判断 |

---

## 1. 功能域一览

需求来自题目四域 + 调研文档 12 节，归并为 10 个功能域；菜单顺序与课件顺序一致（便于边开发边录课）。

| # | 功能域 | 核心能力 | 界面入口 |
|---|---|---|---|
| 1 | 登录与权限 | JWT 认证、双角色（病案管理员 / 科室医师）、操作审计 | 登录页 / 侧边栏角色门控 |
| 2 | 病案归档 | 病案登记、批量回收、归档上架（架位容量校验）、封存、流转履历 | 病案建档 |
| 3 | 病案编目 | 待编目队列、ICD-10 / ICD-9-CM-3 选码、编码存在性校验、质量分级 | 病案编目 |
| 4 | 病案借阅 | 申请 → 审批 → 借出 → 归还核销、续借、逾期催还、台账追溯 | 病案借阅 / 整改督办 |
| 5 | 数据统计 | 状态分布、归档率、疾病谱、科室排名、费用与住院日分布 | 数据统计 / 质量看板 |
| 6 | **首页质控** | 五类规则（必填 / 条件必填 / 逻辑 / 编码存在性 / 主诊断选择）、逐字段异常标记、评分与甲乙丙等级、**退回-整改-复核闭环**、归档锁定与变更留痕 | 编目页「质控工作台」 |
| 7 | 首页上报 | 批次管理、上报前校验、XML 组包、回执状态、**接口标准 CSV 导出**、**上报一致性核对** | 首页数据上报 |
| 8 | 基础数据 | 科室、医师、ICD 编码库（版本化）、架位、通用字典 | 后端 API（无独立界面） |
| 9 | 合规安全 | 操作日志审计、数据权限范围、病案禁删、封存冻结、归档后变更留痕 | 工作台动态 / 后端约束 |
| 10 | 智能与决策辅助 | 规则版预编码推荐、DRG/DIP 入组预检、归档及时率与超期催缴、**CC/MCC 严重度预警** | 编目页弹窗 / 质量看板 / 质控工作台 |

> **完整功能清单（含逐条功能的界面入口、实现状态、演示覆盖标记）见 [`docs/功能清单.md`](docs/功能清单.md)** —— 那份文档按 10 个功能域逐条列到「用户可感知的操作粒度」，并标注每条是「有界面」「仅后端 API」还是「规则驱动无专属界面」。

### 1.1 功能演示覆盖矩阵

把「哪些功能有视频」摊开看，避免「README 说有、点进去没有」：

| 功能域 | 有界面 | 有视频 | 仅后端 API（无界面） |
|---|---|---|---|
| 登录与权限 | ✅ | ✅ 总览片 + 主线片 | ADMIN 操作拦截、当前用户信息 |
| 病案归档 | ✅ | ✅ `archive/` | 封存、流转履历、架位 CRUD |
| 病案编目 | ✅ | ✅ 主线片 + `ai-assist/` | 编目变更留痕（`revisions` 已封装未调用） |
| 病案借阅 | ✅ | ✅ `borrow/` | 审批链追溯、人工催还 |
| 数据统计 | ✅ | ✅ `stats/` | — |
| 首页质控 | ✅ | ✅ 主线片 + `rectify/` | 归档锁定、首页变更留痕、条件必填 4 字段（前端无输入项） |
| 首页上报 | ✅ | ✅ `report/` | XML 组包回执、编码辅助规则 CRUD |
| 质量看板 | ✅ | ✅ `quality/` | — |
| 智能与决策辅助 | ✅ | ✅ `ai-assist/` | 编码辅助 / DRG 分组规则 CRUD |
| 基础数据 | ❌ | ❌ | 科室、医师、字典、角色、用户、架位、ICD 整版导入（**后端完备，前端未建页**） |

**「仅后端 API」不是遗漏，是原型阶段的显式边界**（`SPEC.md` §2.3 明确冻结）——接口可用、可测、可调，只是没建前端页。README 不把它们说成「已实现」。

---

## 2. 快速开始

### 2.1 Docker Compose（后端 + PostgreSQL 16 + Redis 7）

```bash
docker compose up -d --build
curl http://localhost:8080/healthz
```

表结构由 `mrs-backend/sql/schema.sql` 首次初始化；空库启动时后端自动灌种子（管理员、ICD 编码库、质控规则、16 条 CC/MCC 演示规则、三份演示病案）。

### 2.2 本地免 Docker（前端零构建，无需 `npm install`）

```bash
# 1) 数据库：本机 PostgreSQL 建库 mrs / 用户 mrs，或直接用仓库内的便携 PG 脚本
cd mrs-backend && go run .                 # 后端 :8080，Redis 缺失自动降级直查

# 2) 前端静态服务（适配层会自动指向 127.0.0.1:8080）
cd .. && python -m http.server 8090 --bind 127.0.0.1
# 浏览器打开 http://127.0.0.1:8090/frontend/index.html
```

> 前端是 **Vue 3 全局版 + 原生 JS 组件**，靠 `index.html` 里的 `<script>` 顺序装配，**没有打包器**。新增组件必须同时在该文件加 `<script>`（且早于 `layout.js`），并 bump `?v=` 版本号，否则浏览器缓存会让你怀疑人生。

### 2.3 演示账号

| 账号 | 口令 | 角色 | 能看到什么 |
|---|---|---|---|
| `admin` | `admin123` | 病案管理员 | 全部菜单；可编目、改首页、跑质控、退回/复核、上报 |
| `deptuser` | `123456` | 科室医师 | 本科室待整改、借阅申请等受限视图 |

> 这两个账号、`mrs-backend/config.yaml` 里的数据库口令与 `jwt.secret` 都是**本地演示默认值**，公开仓库里故意保留以便一键跑通；**任何生产/公网部署都必须用 `MRS_*` 环境变量覆盖**。

种子数据约 87 份病案（近 6 个月、状态加权）、12 科室、12 医师、35 条 ICD 码表、19 张借阅单、2 个历史上报批次。重灌：`go run ./cmd/createdb drop && go run ./cmd/createdb` 后重启后端。

---

## 3. 功能演示逐项

下面按「**入口 → 操作 → 观察点**」列出每个模块怎么亲手验，均可在本机跑通。

### 3.1 登录与工作台

- **入口**：`/frontend/index.html`（未登录自动落到登录页）。
- **操作**：用 `admin` / `deptuser` 分别登录，对比侧边栏菜单差异。
- **观察点**：四张概览卡（在院病案总数 / 归档完成率 / 编目完成率 / 本月出院病案）、月度出院病案归档趋势折线、病案流转状态环图、科室病案量 Top 7、待办事项；写操作会自动落操作日志并在工作台动态里出现。

### 3.2 病案归档（状态机 ①）

- **入口**：侧栏「病案建档」。
- **操作**：登记新病案 → 批量回收 → 归档上架（选架位）→ 查看流转履历。
- **观察点**：主状态机 `REGISTERED → RECYCLED → ARCHIVED → SEALED`；上架时做**架位行锁 + 容量校验**，超量自动截断；归档后禁止改首页与增删编目（返回 409）。

### 3.3 病案编目（含智能辅助）

- **入口**：侧栏「病案编目」。
- **操作**：左侧待编目队列选一份 → 右侧填主诊断 / 主手术 ICD 编码 → 保存编目。
- **观察点**：
  - 编码必须命中 ICD 字典，否则 `CODE_EXIST` 规则报错；
  - 「**智能推荐**」按关键词规则给出候选编码（辅助而非替代编码员，去重回查字典、按权重排序）；
  - 「**DRG 预检**」给查表版入组提示 + QY 风险，**响应带免责声明**；
  - 首条编目自动推进病案状态；编目动作链内自动触发首页质控。

### 3.4 借阅全链路（状态机 ②）

- **入口**：「病案借阅」→ 申请；「整改督办」相关视图看台账。
- **操作**：申请 → 审批 → 借出 → 归还核销；可续借、可人工催还。
- **观察点**：子状态机 `APPLIED → APPROVED → LENT → RETURNED`；台账支持 `overdueOnly` / `urgent` 过滤，前端动态计算逾期天数并标红；审批链可追溯。

### 3.5 统计与质量看板

- **入口**：「数据统计」「质量看板」。
- **观察点**：状态分布、归档率、疾病谱、趋势；质量看板专项展示 **2 日 / 3 日归档率 + 月度趋势 + 科室排名 + 超期未归档清单**。

### 3.6 首页质控（状态机 ③，核心闭环）

- **入口**：编目工作台里的「质控工作台」按钮。
- **规则五类**：`REQUIRED`（含条件必填 `conditionExpr`）、`LOGIC`、`CODE_EXIST`、`DIAG_OP_MATCH`、`MAIN_DIAG_RULE`，外加本项目的 `CC_MCC_LOW` / `CC_MCC_HIGH`。规则是 **`qc_rule` 表配置数据，不是硬编码**，改规则不改代码。
- **操作**：改首页 → 保存 → 跑质控 → 看评分与质控项 → 退回 → 临床整改 → 病案室复核。
- **观察点**：
  - 每次质控产出 0–100 分 + 甲乙丙等级 + 单项否决，落 `front_page_quality`；
  - `RunQc` 按 `(recordId, ruleId, fieldName)` **增量 upsert**，重跑不清空 `RETURNED` / `RECTIFIED` 等人工处理痕迹；
  - 归档锁定后改首页返回 409；每次改首页写前后快照 + 修改人 + 事由（`revisions` 接口已封装，界面待补）。

### 3.7 CC/MCC 严重度分层（本项目重点）

- **入口**：质控工作台里的「CC/MCC 严重度预览」按钮。
- **操作**：看逐条判读 → 在弹窗底部改「其他诊断编码 / 标记」→「保存并重新判读」。
- **关键契约**：`other_diagnosis_flags` 与 `other_diagnosis_codes` **按逗号位置一一对应**（`0` 无 / `C` CC / `M` MCC）；重复编码、长度不等、非法标记在解析层直接 400，不让错位数据参与判读。
- **接口示例**（响应体必带 `disclaimer`）：

```bash
curl -X POST http://localhost:8080/api/ccmcc-preview \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900,K80.201","otherDiagnosisFlags":"M,0"}'
```

```jsonc
{
  "code": 0, "msg": "ok",
  "data": {
    "version": "demo-2026.10",
    "overallSeverity": "MCC",
    "mccCount": 1, "ccCount": 0,
    "items": [
      { "code": "I50.900", "severity": "MCC", "matchedPrefix": "I50", "source": "demo" },
      { "code": "K80.201", "severity": "0",   "matchedPrefix": "",    "source": "demo" }
    ],
    "sources": ["demo"], "demoOnly": true,
    "disclaimer": "本判断基于院内演示用 CC/MCC 判读表（非国家医保局官方表）……"
  }
}
```

- **规则即数据**：`cc_mcc_rule` 表按 `version + diag_prefix` 共存多版本，`LoadCcMccRules(db, version)` 读库；读不到或该版本 0 条 → **降级内置演示表并打日志**（fail-soft），但空版本直接拒绝判读（fail-closed）。表内**没有分组码 / 权重 / 金额列**——这是 ADR-0002 的硬约束，由单测 `TestCcMccRuleNoPaymentColumns` 守住。

### 3.8 首页上报与一致性核对

- **入口**：「首页数据上报」。
- **操作**：建批次 → 提交（本地 XML 桩，防腐层可替换）→ 看回执状态 → 「导出前检查」→ 导出 18 列 CSV（UTF-8 BOM）。
- **观察点**：「一致性核对」对比院内应有 vs 实际上报，输出漏报 `MISSING` 与上报后被改 `MODIFIED` 明细。18 列是骨架（官方约 90+ 采集项），见第 9 节边界。

---

## 4. CC/MCC 合规边界（ADR-0002）

| 约束 | 实现方式 |
|---|---|
| 只做**严重度预警**，不出分组结果 | 判读层只返回 `overallSeverity` / 逐条 `severity` / `matchedPrefix` / `excluded`，**无 `group_code` / `weight` / `amount`** |
| 演示数据显著标注 | 表 `Source=demo`；响应 `demoOnly=true` + `sources`；前端红色免责块 + 弹窗副标题「演示口径 · 非官方判定」 |
| 免责声明是**接口契约** | `disclaimer` 由判读层填充，handler 不改写不兜底（避免两处可写导致口径漂移）；前端用后端文案，缺失时用兜底提示但绝不显示空白 |
| 规则可替换、版本化 | `cc_mcc_rule` 表按 `(version, diag_prefix)` 共存；`main.go` 只登记 AutoMigrate，种子按判重键幂等灌入 |
| 不拦截业务 | 两条规则恒为 `WARN`，种子里 `ccMccWarnLevel` 常量 + 注释写明不得改 `ERROR` |
| 结算口径 ≠ 院内质控口径 | 广东 DIP 与 CHS-DRG 逻辑不同；落地须先确认属地医保文件，见 `docs/专家评审包/批次C-CC-MCC口径-回执-v1.md` |

---

## 5. 架构与技术选型

| 层 | 选型 | 说明 |
|---|---|---|
| 后端 | Go + Gin + GORM | 统一响应 `{code,msg,data}`、JWT + 角色中间件、操作日志审计 |
| 数据 | PostgreSQL 16（主库）+ Redis 7（热点缓存） | Redis 不可用自动降级直查；表结构唯一事实来源 `mrs-backend/sql/schema.sql` |
| 前端 | Vue 3 全局版 + 原生 JS 组件 | 零构建、`<script>` 顺序装配；`api-store.js` 是唯一 REST 适配层，页面组件只认同构的 `window.MRMS` |
| 部署 | Docker Compose 三容器 / 本地免 Docker 双轨 | `MRS_*` 环境变量覆盖 `config.yaml` |
| 规范 | 三条状态机 + 规则驱动质控 + BDD 可执行引用 | 权威层级：`SPEC.md §2.2`（领域）→ `API.md`（接口）→ `features/*.feature`（引用，不重定义规则） |

**三条状态机**：病案主状态机（全系统锚点）、借阅子状态机、首页质控与上报子状态机。详见 `SPEC.md §2.2`。

---

## 6. 接口与契约

- **95 条已注册路由**，逐条文档见 [`mrs-backend/docs/API.md`](mrs-backend/docs/API.md)（含账号、菜单→模块对照、请求/响应示例）。
- `TestApiCoverage` 双向核对文档与真实路由，**95/95 = 100%**；`frontend_calls_test.go` 反向扫描前端源码里的请求，确保没有指向不存在的接口。
- 契约测试读取 `mrs-backend` module 之外的 `frontend/`，Go 缓存不跟踪它，**改前端后必须用 `go test -count=1`**（不能吃缓存）。

---

## 7. 测试与门禁

```bash
cd mrs-backend
go build ./...
go vet ./...
gofmt -l .                    # 应为空
go test ./... -count=1        # 必须带 -count=1
```

覆盖：`internal/bdd`（Gherkin 场景，自足、不需要 PG 与活进程）、`handler`（含 nil-DB 技巧验证「校验先于库」）、`router`（契约覆盖 + 权限闸门）、`seed`（幂等与列约束）、`service`（质控引擎、其他诊断解析、CC/MCC 检测 30 用例 / 96.5% 覆盖）。前端另有 16 个 JS 文件走 `node --check`。

**端到端实机脚本**（`tools/shot-ccmcc.js`，Playwright 驱动本机 Chrome，不需要下载浏览器内核）：

```bash
NODE_PATH="<playwright-core 所在 node_modules>" node tools/shot-ccmcc.js
SHOT_VIDEO=1 NODE_PATH="..." node tools/shot-ccmcc.js   # 同时录屏
```

脚本内置三个**程序化断言**，不靠肉眼看图判绿：`CCMCC_RENDERED`（判读结果真的渲染出来，而不是静默落到「暂无判读结果」）、`SNAPSHOT_REFRESHED`（两轮保存负控制：`M,M` → `0,C`，证明保存后刷新了本地快照）、`QC_CCMCC_HITS`（低靠 + 高靠两条都落库）。产物落到 `docs/screenshots/`。

录屏那一轮额外注入三层**可读性标注**（虚拟光标 / 点击涟漪 + 被点元素高亮 / 顶部 9 步进度条），注入方式用 `context.addInitScript` —— 若改成 `goto` 之后再挂标注，录屏开头会留一段约 2 秒「页面已显示、标注还没挂上」的裸画面，首帧就丢了步骤条。

> 工程纪律：守卫测试必做**负控制**（注入错误 → 必须变红且精确指位 → 逐字节还原 + sha256 佐证）；`go test` 一律 `-count=1`；改文案必须同步改**硬编码**的测试期望串（否则失去「捕获文案意外变化」的能力）。

---

## 8. 目录

```
├── README.md                    本文件（功能演示 + 运行说明）
├── SPEC.md / DESIGN.md          系统规格说明书 / 骨架设计 V2.1
├── docker-compose.yml           postgres + redis + backend 三容器
├── serve.py                     静态联调服务器
├── frontend/
│   ├── index.html               零构建入口（script 顺序 = 装配顺序）
│   ├── js/api-store.js          REST 适配层（唯一出口，req() 返回 json.data）
│   ├── js/components/           12 个页面组件（dashboard/archive/catalog/borrow/
│   │                            statistics/report/quality/rectify/ccmcc/...）
│   ├── js/theme-xiayi.js        视觉对标（取色板，不取工位）
│   └── css/                     主题样式
├── mrs-backend/
│   ├── main.go / config.yaml    入口（AutoMigrate + 种子登记）/ 本地配置
│   ├── sql/schema.sql           PostgreSQL 表结构（唯一事实来源）
│   ├── docs/API.md              95 条接口契约
│   ├── features/                Gherkin 场景（只引用不定义规则）
│   └── internal/
│       ├── config/ cache/ seed/      配置 / 缓存降级 / 幂等种子
│       ├── model/                    GORM 实体 + 状态机常量 + cc_mcc_rule 表模型
│       ├── middleware/               CORS / JWT / 角色校验 / 审计
│       ├── handler/                  各业务模块接口（含 ccmcc_preview.go）
│       ├── service/                  qcengine / ccmcc / ccmcc_qc / other_diagnosis
│       ├── router/                   路由装配 + 契约守卫测试
│       └── bdd/                      godog 驱动套件
├── docs/
│   ├── screenshots/              演示截图 01–08、demo.gif、完整录屏 webm
│   ├── adr/                      ADR-0002（DRG 严重度预警边界）等决策记录
│   ├── 专家评审包/               批次 C 送审包 + 傅医保回执 v1
│   ├── BACKLOG.md                缺口清单与执行约定
│   └── 自迭代开发循环-v0.1.md    协作模式（六拍环 / 三条不变量 / 转译铁律）
├── prototype/                   原型与视觉对标参考图
├── tools/shot-ccmcc.js          端到端截图 / 录屏脚本（带光标与步骤条标注）
└── tools/make-gif.py            录屏帧 → README 演示动图（统一调色板合成）
```

---

## 9. 已知边界与非目标

### 明确不做

| 事项 | 理由 |
|---|---|
| **官方** DRG/DIP 分组与结算 | 官方分组器与各省权重表需授权获取；本系统只做编码质控提示，**不作为结算依据** |
| 无纸化病案 / HIS-EMR 自动采集 | 不接院内临床系统，首页数据由本系统维护 |
| 条码 / RFID 病案示踪 | 仅留架位与流转流水，不做硬件示踪 |
| 服务端分页 | 前端全量加载 + 内存分页（演示规模 ≤ 数百条） |
| 真实 LLM 智能编码 | 本期以「规则版预编码」交付，LLM 方案留在 `BACKLOG.md` |

### 已实测确认的缺口（不是推测，逐条登记在 `docs/BACKLOG.md`）

| # | 缺口 | 影响 |
|---|---|---|
| D3 | 条件必填的 4 个真字段前端无处可填 | 后端已支持，界面待补输入项 |
| D5 | 变更留痕只覆盖首页 | 编目变更、病案主档变更未留痕 |
| D6 | 催缴只是清单导出，无通知动作 | 与「每天电话催办」的真实痛点仍有距离 |
| D7 | 上报 CSV 为 18 列骨架 | 官方约 90+ 采集项 |
| D8 | 一致性核对用 `updatedAt` + 24h 窗口近似；评分口径自定 | 均为演示近似，非官方细则 |
| — | 其他诊断错位脏数据在 CC/MCC 检测里静默 | 已 fail-closed 不判读，但暂无 WARN 提示 |
| — | 演示表仅 16 条前缀 | E11.2/.4/.5/.7、E86、N18.4、T80–T88 等漏检面较大 |

---

## 10. 文档索引

| 想看什么 | 去哪 |
|---|---|
| **全部功能清单（逐条功能 + 界面入口 + 演示覆盖）** | [`docs/功能清单.md`](docs/功能清单.md) |
| 完整功能规格、三条状态机、界面设计、验收口径 | [`SPEC.md`](SPEC.md) |
| 骨架设计、表结构、菜单与课件对照 | [`DESIGN.md`](DESIGN.md) |
| 95 条接口的请求/响应示例 | [`mrs-backend/docs/API.md`](mrs-backend/docs/API.md) |
| 为什么不做官方分组、CC/MCC 表怎么设计 | [`docs/adr/0002-DRG严重度预警边界与CC-MCC表设计.md`](docs/adr/0002-DRG严重度预警边界与CC-MCC表设计.md) |
| 专家怎么判「低靠/高靠」口径、降噪清单、补表优先级 | [`docs/专家评审包/批次C-CC-MCC口径-回执-v1.md`](docs/专家评审包/批次C-CC-MCC口径-回执-v1.md) |
| 缺口、执行约定、每批落地记录 | [`docs/BACKLOG.md`](docs/BACKLOG.md) |
| 自迭代开发循环（六拍环 / 转译铁律 / 元评审） | [`docs/自迭代开发循环-v0.1.md`](docs/自迭代开发循环-v0.1.md) |
| 批次 1 十项能力的操作路径与截图 | [`docs/extension-guide.html`](docs/extension-guide.html) |
| **录屏脚本（含共享的「人类化操作层」）** | [`tools/shot-lib.js`](tools/shot-lib.js) · [`tools/shot-tour.js`](tools/shot-tour.js) |

---

> 本项目为**教学 / 演示用途**。所有演示数据均为虚构，CC/MCC 判读表为院内演示口径；**不得用于真实病案质控结论或医保结算**。
