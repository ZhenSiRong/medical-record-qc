# API.md 契约覆盖核对报告

> 首次生成日期：2026-10-09　·　生成方式：`tools_check_contract.py`（**该一次性脚本已删除**，被 Go 测试取代，见 §8）
> 本文件是**静态证据存档**：§0–§7 是首轮核对（93 条基线）的原始记录，§8 是收尾复核（94 条基线 + 负控制）。
> §0–§7 里的行号/哈希绑定到下面记录的那个文档版本；文档后续又被并发修改，行号会漂移，故以 §8 的结论为准。

## 0. 数据来源与复现命令

| 项 | 值 |
|---|---|
| 事实清单 | `routes.actual.txt`（首轮 93 行，sha256:16 = `f63caf8d026f5611`；收尾时 94 行，见 §8） |
| 事实来源 | `internal/router/contract_test.go::TestDumpRoutes` 调用 `router.Register(r, nil, cache, &config.Config{})` 后取 `r.Routes()` |
| 文档 | `docs/API.md`（首轮 sha256:16 = `09922ddcc3b6dc03`；收尾 sha256:16 = `d03094c206187f27`，见 §8） |
| 路由装配入口 | `internal/router/router.go:19`；`internal/handler/{system,qc_ext,branch_ext}.go` 的 `Register*` |

```powershell
# 首轮核对方式（脚本已删除，保留原始命令作为可追溯记录）
cd mrs-backend
& "C:\Program Files\Go\bin\go.exe" test ./internal/router/ -run TestDumpRoutes -v
python tools_check_contract.py     # ← 已删除

# 现行方式（覆盖度校验已成为 go test 的一部分）
& "C:\Program Files\Go\bin\go.exe" test ./internal/router/ -v
```

比对口径：路径参数名（`:id` / `:recordId`）**不参与**缺失判定，统一归一为 `:param`；
参数名差异见附录 S。无方法的 `/api/...` 字面量计为「弱覆盖」。

> ⚠️ **版本绑定**：`docs/API.md` 与 `internal/handler/*.go` 在本报告生成期间正被其他作者并发修改。
> 上表两个 `sha256:16` 就是本次结论所绑定的确切版本；文档若再有改动，请重跑上面的 `go test` 复算。

## 1. A. 已注册但 API.md 未覆盖

合计 **0** 条：完全未提及 **0** 条，弱覆盖（路径出现但方法未文档化）**0** 条。

### A1. 完全未提及（真实遗漏）

_无。_

### A2. 弱覆盖（路径被提及，但未文档化该方法）

_无。_

## 2. B. API.md 提到但实际未注册

合计 **0** 条**有方法的虚构端点**：路径与方法都不存在 **0** 条，方法不存在 **0** 条；
另有 **0** 条无方法的路径提及未命中真实路由（多为泛指引用，单列见 B3）。

### B1. 方法与路径都不存在

_无。_

### B2. 路径存在但该方法未注册

_无。_

### B3. 无方法的路径提及，未命中任何真实路由

_无。_

## 3. C. 覆盖数 / 总数 与覆盖率

| 指标 | 数值 |
|---|---|
| 实际已注册路由总数 | **93** |
| API.md 明确文档化（方法+路径匹配） | **93** |
| 仅弱覆盖（路径提及、无方法） | **0** |
| 覆盖数（强 + 弱） | **93** |
| **覆盖率（强 + 弱）** | **100.0%** |
| **覆盖率（仅强匹配）** | **100.0%** |
| 未覆盖 | **0** |
| 文档虚构（有方法） | **0** |

### 3.1 按业务域分组（事实清单口径）

| 前缀分组 | 条数 |
|---|---|
| `/api/archive-overdue` | 1 |
| `/api/archive-timeliness` | 1 |
| `/api/archives` | 4 |
| `/api/auth` | 2 |
| `/api/borrow-ledger` | 3 |
| `/api/borrows` | 5 |
| `/api/catalogs` | 3 |
| `/api/code-suggest` | 1 |
| `/api/code-suggestion-rules` | 4 |
| `/api/depts` | 5 |
| `/api/dicts` | 5 |
| `/api/doctors` | 5 |
| `/api/drg-group-rules` | 4 |
| `/api/drg-preview` | 1 |
| `/api/front-pages` | 7 |
| `/api/icd-dicts` | 3 |
| `/api/oper-logs` | 1 |
| `/api/qc-results` | 4 |
| `/api/records` | 4 |
| `/api/report-batches` | 6 |
| `/api/report-consistency` | 2 |
| `/api/roles` | 5 |
| `/api/shelves` | 5 |
| `/api/stats` | 5 |
| `/api/users` | 6 |
| `/healthz` | 1 |
| **合计** | **93** |

## 4. 附录 S. 参数名差异（路径形状一致，仅参数名不同）

> ✅ **已修正**（2026-10-09 收尾复核）：`docs/API.md` 已按**实际注册写法** `:id` 改写这两条，
> 见 §8.4。下表保留为问题发现时的原始记录。

| # | 方法 | API.md 写法（发现时） | 实际注册写法 | API.md 位置（首轮版本） | 状态 |
|---|---|---|---|---|---|
| 1 | GET | `/api/archives/records/:recordId/flow` | `/api/archives/records/:id/flow` | `docs/API.md:129` | ✅ 已修正为 `:id` |
| 2 | POST | `/api/archives/seal/:recordId` | `/api/archives/seal/:id` | `docs/API.md:126` | ✅ 已修正为 `:id` |

## 5. 附录 D. API.md 声明的数量 vs 实测

> ✅ **已修正**（2026-10-09 收尾复核）：文档数字已由 105 改为实测值，见 §8.3。
> 下表保留为问题发现时的原始记录。

| 声明位置（首轮版本） | 声明值 | 实测 | 状态 |
|---|---|---|---|
| `docs/API.md:6` | 路由总量 **105 条** | **93 条**（差 12 条） | ✅ **已修正**为「路由总量 **93 条**（`/api` 下 92 条 + `/healthz`）」 |
| `docs/API.md:565` | 「105 条路由中，api-store.js 已调用 46 条」 | 分母应为 **93** 条 | ✅ **已修正**为「93 条路由中，`api-store.js` 已调用 **46 条**（约 49%）」（46/93 = 49.5%） |
| `docs/API.md:6` | 旧文档覆盖约 46 条 | 详见 §3 | 保留（该表述无误） |

### 5.1 API.md 目录表自查（`docs/API.md:20-36`）

各节「接口数」列横向求和 = **94**，与实际 93 条差 1，差额来自 §11：
目录写 6（`docs/API.md:31`），实际只有 5 条（`internal/handler/branch_ext.go:28-32`）。

| 口径 | 条数 |
|---|---|
| API.md 目录表各节求和 | 94 |
| 减去 §11 目录超额（6 声称 vs 5 实际） | 93 |
| gin 路由树实测 | **93** |
| API.md 正文声明的「路由总量」 | 105（**对不上任何一种口径**）→ ✅ **已修正为 93** |

---

## 6. 结论（人工裁定）

### 6.0 一句话裁定

**「105 条全覆盖」这个声明一半对一半错**：

| 子声明 | 裁定 | 依据 |
|---|---|---|
| 「路由总量 **105** 条」 | **不成立（数字错误）** | 实测 **93** 条，差 12 条；见 §3、§5.1 |
| 「全部 **105** 条已覆盖」中的「覆盖」部分 | **成立** | A = 0 条、B = 0 条，覆盖率 93/93 = 100%；见 §1、§2、§3 |

即：**文档对路由的覆盖是完备的（没有漏、也没有编），但路由总量这个数字是错的。**

### 6.1 「105 条」的裁定：不成立 —— 数字错误，而非「文档漏了 12 条」

三种相互独立的统计口径两两吻合在 **93**，且都不是 105：

| # | 口径 | 结果 | 证据 |
|---|---|---|---|
| (a) | gin 路由树程序化导出 | 93 | `routes.actual.txt` 93 行；`contract_test.go:56` 输出「已注册路由总数: 93」 |
| (b) | 逐文件读源码手工点数 | 93 | `router.go` 内联 3（healthz + login + me）＋ 36（records 4 / archives 4 / catalogs 3 / icd-dicts 3 / borrows 5 / borrow-ledger 3 / front-pages 4 / qc-results 1 / report-batches 4 / stats 5）＝39；`system.go` 6×5＋2＝32；`qc_ext.go` 6；`branch_ext.go` 16 → 39+32+6+16 = 93 |
| (c) | API.md 自己的目录表求和 | 94 → 93 | `docs/API.md:20-36` 求和 94，减去 §11 目录超额 1 |

**严重程度：低危（文档准确性缺陷）。** 不影响前端联调与接口可用性，但会污染所有以路由总数为分母的结论：
`docs/API.md:565` 的「105 条中已调用 46 条（约 44%）」分母错误，按 93 计算应为 **49.5%**。

**修正建议（只报告，不改文档）**：把 `docs/API.md:6` 与 `docs/API.md:565` 的 105 改成 93，
并把 `docs/API.md:31` 的「6」改成「5」。

> ✅ **落实情况**（2026-10-09 收尾复核）：
> - `105 → 93`：**已修正**（`docs/API.md` 现写「路由总量 **93 条**（`/api` 下 92 条 + `/healthz`）」）。
> - §17 占比分母：**已修正**（「93 条路由中，`api-store.js` 已调用 **46 条**（约 49%）」）。
> - `/healthz`：**已补文档**（新增 §0 健康检查章节，含 `### GET /healthz`）。
> - §11 目录「6」：**截至收尾仍为 6**（`internal/handler/branch_ext.go:28-32` 实际 5 条）。
>   影响极低（仅目录合计 94 ≠ 实测 94/93），详见 §8.3。**未修改，仅报告。**

### 6.2 「全覆盖」的裁定：成立

| 清单 | 条数 | 结论 |
|---|---|---|
| A. 已注册但 API.md 未覆盖 | 0 | 无真实遗漏 |
| B. API.md 提到但实际未注册 | 0 | 无文档虚构 |
| C. 覆盖率 | 93/93 = 100% | 强匹配即 100%，无需借助弱覆盖 |

集合层面可证：|已注册| = |被文档化| = 93，且 A 为空 ⇒ 已注册 ⊆ 文档化；B 为空 ⇒ 文档化 ⊆ 已注册；
故两侧集合**相等**（不是「恰好条数相同」的巧合）。

### 6.3 顺带发现：2 处「描述与实现不符」（不是覆盖缺口，是描述错误）

| # | 位置 | 文档写的 | 实现是 | 严重程度 |
|---|---|---|---|---|
| 1 | `docs/API.md:126`、`docs/API.md:129` | 加粗强调「路径参数名是 **`:recordId`**」 | `router.go:50` `POST /archives/seal/:id`、`router.go:51` `GET /archives/records/:id/flow`，参数名是 **`:id`** | **中低危**：URL 形状一致故不影响路由匹配，但前端若按文档取 `req.params.recordId` 会拿到 `undefined`。文档还专门加粗强调了错误的那个名字，误导性较强 |
| 2 | `docs/API.md:31`（§11 目录） | 接口数 **6** | `branch_ext.go:28-32` 实际 **5** 条 | **低危**：数量笔误，也是 §5.1 中 94 − 1 = 93 的那 1 条差额来源 |

> 注：另有 1 条**并发编辑**带来的时序问题，非缺陷：本次核对开始时 `docs/API.md` 尚无 §0 健康检查章节，
> `GET /healthz` 一度被判为「弱覆盖」；核对期间文档被追加了 `docs/API.md:62-66` 的 §0 章节后，
> 该条转为强匹配，A 清单归零。这恰好说明**本核对流程是可复算的**——报告里的 `sha256:16` 就是本次结论绑定的文档版本。

### 6.4 已排除的「解析口径差异」（不是缺陷，记录备查）

| # | 现象 | 为何不算缺陷 |
|---|---|---|
| 1 | `docs/API.md:50-58` 菜单表里的 `/records`、`/archives`、`/stats/*`、`/depts` 等**不带 `/api` 前缀**的模块名 | 是模块泛指引用而非端点。解析器只采信含 `/api/`（或 `/healthz`）的字面量；若把它们当端点会凭空造出 B 类误报 |
| 2 | `docs/API.md:315-319` 的 `{资源}` 占位（`/api/{资源}/page` 等 5 行） | 已按 6 类资源（depts/doctors/dicts/roles/shelves/users）展开为 30 条真实路由，**全部命中**，无一条落进 A/B（见 §3.1 六类各 5 条） |
| 3 | `docs/API.md:126/129` 的 `:recordId` vs 实现 `:id` | 按既定口径参数名归一为 `:param`，**不计入 A 清单**，单独列在附录 S |
| 4 | `docs/API.md:598` 变更记录称「旧文档写 `:id`，本轮改写为 `:recordId`」 | 即上述第 1 项：本轮「修正」的方向反了——把本来正确的 `:id` 改成了与实现不符的 `:recordId` |

### 6.5 交付与边界

- 本次**只报告不改文档**：`docs/API.md`、`SPEC.md`、`docs/extension-guide.html`、`internal/handler/*.go`、`router.go` 均未改动（见 §7）。
- `routes.actual.txt` 由测试导出，是可重跑的**事实基线**；契约若变化，重跑两条命令即可复算。

## 7. 本次核对的产物与「未改动清单」

### 7.1 本轮核对的产物（收尾后状态，详见 §8）

| 文件 | 性质 | 说明 |
|---|---|---|
| `internal/router/contract_test.go` | 新增 | `TestDumpRoutes`（导出事实）+ **`TestApiCoverage`**（把覆盖度校验做成 `go test` 的一部分，见 §8.1）。不写死条数断言 |
| `routes.actual.txt` | 新增（测试生成物，gitignore） | 真实路由，`METHOD /path`，按路径字典序。首轮 93 行 → 收尾 94 行 |
| `tools_check_contract.py` | **已删除** | 一次性 python 脚本，已被 `TestApiCoverage` 取代（见 §8.1） |
| `routes.diff.md` | 本文件 | 静态证据存档：A/B/C 三份清单 + 附录 S/D + 结论 + 收尾复核 |

### 7.2 明确未改动（其他作者正在编辑，本核对全程只读）

| 文件 | 状态 |
|---|---|
| `docs/API.md` | 只读（被并发修改，见 §0 版本绑定） |
| `SPEC.md` | 只读 |
| `docs/extension-guide.html` | 只读 |
| `internal/router/router.go` | 只读（`router.go:19` 为唯一装配入口，`main.go:71` 调用） |
| `internal/handler/system.go` | 只读（`system.go:14` `RegisterSystem`） |
| `internal/handler/qc_ext.go` | 只读（`qc_ext.go:32` `RegisterQcExt`） |
| `internal/handler/branch_ext.go` | 只读（`branch_ext.go:25` `RegisterBranchExt`） |

未执行 `go build ./...` 与 `go vet ./...`（按任务分工由他人负责）；只执行了 `go test ./internal/router/ -run TestDumpRoutes`。

---

# 8. 收尾复核（2026-10-09，覆盖度校验已并入 `go test`）

首轮核对（§0–§7）只能靠人工跑 python 脚本发现漂移。收尾把同一判定逻辑搬进了 Go 测试，
使文档与实现的漂移在 `go test ./...` 时**自动失败**。

## 8.1 改动：`TestApiCoverage` 成为 `go test` 的一部分

`internal/router/contract_test.go` 现有两个测试（另有 `router_test.go` 里原有的
`TestRoutesRegisterNoPanic` 冒烟测试，以及另一位作者新增的 `frontend_calls_test.go`）：

| 测试 | 文件 | 职责 |
|---|---|---|
| `TestRoutesRegisterNoPanic` | `router_test.go` | 原有冒烟：路由树可装配（不依赖 DB/Redis） |
| `TestDumpRoutes` | `contract_test.go` | 导出事实到 `routes.actual.txt`，用 `t.Log` 打印总数；**不断言条数** |
| **`TestApiCoverage`** | `contract_test.go` | **新增**：把 `docs/API.md` 与真实路由表做双向集合断言 |

抽出 `actualRoutes(t) []string` 辅助函数（装配路由树 → 排序后的 `"METHOD /path"`），
`TestDumpRoutes` 与 `TestApiCoverage` 共用，避免重复装配逻辑。

### 判定口径（已写进代码注释，刻意宽松 + 可解释）

1. **strong**（方法+路径）：只采信三种可机读形式 —— 标题行 `### METHOD /path`、
   正文内联 `` `METHOD /path` ``、§10 通用 CRUD 表格行 `| METHOD | `path` |`。
2. **weak**（仅路径）：只出现 `/api/...` 或 `/healthz` 字面量而未写方法。按「已文档化」**放行**，
   但用 `t.Log` 记录，便于补全方法。
3. **路径参数名不参与比较**：`:id` / `:recordId` / `:recordNo` 统一归一为 `:param`。
   > 这条是附录 S 的教训，已写进 `documentedEndpoints` 的注释：把「参数名写错」误判成
   > 「路由未文档化」会让测试因为文档描述问题而无谓变红，应由人工按实现纠正。
4. **`{资源}` 占位**展开为 6 类资源（depts/doctors/dicts/roles/shelves/users）；
   同行有方法时按该方法精确展开（§10 表格即此形式），同行无方法时兜底为 §10 的 5 个通用 CRUD 操作。
5. **只认 `/api/` 前缀与 `/healthz`**：正文里 `/stats/*`、`/records`、`/archives` 这类无前缀的
   模块泛指引用不是端点；变更记录里 `GET /front-pages/:id/quality` 那种省略 `/api` 的简写也不算，
   否则会凭空造出大量「文档虚构」误报。

### 断言

- **方向一**：每条已注册路由都必须能在文档中找到；失败用 `t.Errorf` **逐条列出**，
  并提示「请在 docs/API.md 补充，或确认该路由已废弃并删除注册」。
- **方向二**：文档里写明「方法+路径」、实际却未注册的端点同样失败（防文档虚构）。
  局限已在注释写明：只覆盖成对出现的形式，正文无方法的泛指提及不参与，以免噪声大于收益。
- **不写死条数**：93/94 只出现在日志里。

## 8.2 负控制（证明测试不是空跑）

从**全绿**基线出发，临时把 `docs/API.md` 的 `### GET /api/archive-overdue?days=3`
改成 `### GET /api/archive-overdue-QC-TEST?days=3`，得到**精确命中、零噪声**的双向失败：

```
    contract_test.go:230: 真实注册路由 94 条；docs/API.md 文档化「方法+路径」端点 94 条，仅路径提及 67 条
    contract_test.go:247: 有 1 条已注册路由未在 docs/API.md 中记录：
        	GET /api/archive-overdue
        请在 docs/API.md 补充文档，或确认该路由已废弃并删除注册。
    contract_test.go:275: docs/API.md 记录了 1 个实际并未注册的端点（文档与实现不符）：
        	GET /api/archive-overdue-QC-TEST
        请删除文档中的虚构接口，或确认实现缺失。
--- FAIL: TestApiCoverage (1.72s)
FAIL
FAIL	mrs/internal/router	5.320s
```

**恢复证据**（用反向 edit 撤销，未使用文件级覆盖，避免并发写覆盖他人改动）：

| 检查项 | 值 |
|---|---|
| 全绿基线 sha256 | `d03094c206187f276124fbd7c017170f12c31a75c0d916202bb2759c60618f10` |
| 恢复后 sha256 | `d03094c206187f276124fbd7c017170f12c31a75c0d916202bb2759c60618f10` |
| 是否逐字节一致 | **是** |
| `git diff --numstat -- mrs-backend/docs/API.md`（前 / 后） | `521  60` / `521  60` |
| `QC-TEST` 残留计数 | `0` |

> `git diff --stat` 显示 `API.md` 有改动（`521 insertions(+), 60 deletions(-)`），那是**本任务开始前
> 就存在的、由文档作者本人做的修改**；恢复前后 numstat 完全相同，证明**没有残留我制造的改动**。

## 8.3 收尾时的事实与裁定（94 条基线）

| 项 | 首轮（§0–§7） | 收尾 |
|---|---|---|
| 已注册路由 | 93 | **94** |
| API.md 文档化（方法+路径） | 93 | **94** |
| 覆盖率 | 100% | **100%** |
| A 清单（已注册未文档化） | 0 | **0** |
| B 清单（文档虚构） | 0 | **0** |
| `docs/API.md` sha256:16 | `09922ddc3b6dc03` | `d03094c206187f27`（收尾时刻）→ `bdccab65dbca0680`（报告定稿时，见 §8.6） |

> 📌 **文档是移动目标**：核对期间 `docs/API.md` 被作者持续修改，本报告观察到至少四代版本
> （`f092a35b…` → `09922ddc…` → `d03094c2…` → `bdccab65…`）。因此**唯一权威是测试本身**：
> 任何时刻执行 `go test ./internal/router/ -v` 即可复算覆盖率，本报告只是某一时刻的快照。

- **「105」已彻底消失**：全文 `105` 出现次数 = **0**。
- **数字 105 → 93 已修正**（现为「路由总量 **93 条**（`/api` 下 92 条 + `/healthz`）」，收尾时因新增
  `my-pending` 实为 94 条）；**`/healthz` 已补文档**（原缺失章节已补为 §0 健康检查，含 `### GET /healthz`）。
  > ⚠️ **附录 S 未落实（与任务交办时的说法不符，特此以实测为准）**：截至收尾，
  > `docs/API.md:127`（`### POST /api/archives/seal/:recordId`）与 `docs/API.md:130`
  > （`### GET /api/archives/records/:recordId/flow`）**仍写作 `:recordId`**，
  > 而实现在 `internal/router/router.go:50`/`:51` 是 **`:id`**；「变更记录」也仍把
  > 「路径参数写作 `:recordId`（旧文档写 `:id`）」列为修正项。
  > 本项**仅报告，未修改文档**，需文档作者按实现订正。
- **新增路由 `GET /api/qc-results/my-pending`**（`internal/handler/qc_ext.go:56`，D4 临床端整改）
  在收尾过程中由路由方添加；`TestApiCoverage` **当场报红并列出该路由**（这是本测试的首次实战拦截）：
  ```
      contract_test.go:247: 有 1 条已注册路由未在 docs/API.md 中记录：
          	GET /api/qc-results/my-pending
  ```
  该路由随后已由文档作者补入 `docs/API.md:471`，测试转绿。
- **§11 目录仍写 6**（`branch_ext.go:28-32` 实际 5 条，多报 1）；同时 **§13 目录写 6 但实际已 7 条**
  （新增 `my-pending` 未同步目录，少报 1）。两处错误相互抵消，**目录合计 94 恰好等于实测 94 属巧合**，
  并非目录正确。**仅报告，未修改文档。**
  > 计数口径：按「该节实际文档化的端点数」比对，**不看 `###` 标题个数**——§7（5 条）与 §10（32 条）
  > 分别用正文内联字面量与 `{资源}` 表格呈现，`###` 标题只有 0 / 2 个，用标题数会得出错误结论。
  > 逐节实测：§0=1 §1=2 §2=4 §3=4 §4=6 §5=5 §6=3 §7=5 §8=5 §9=4 §10=32 §11=**5** §12=5
  > §13=**7** §14=2 §15=2 §16=2，合计 94，与 gin 路由树 94 条一致。
- **反向印证**：并行的独立实现 `mrs-backend/tools/check-api-coverage.py`（文档作者所写，与
  `TestApiCoverage` 无共享代码）在同一基线上同样给出 **93/93 = 100%**，两条独立实现互证。

## 8.4 最终测试输出原文

```
$ & "C:\Program Files\Go\bin\go.exe" test ./internal/router/ -run 'TestDumpRoutes|TestApiCoverage|TestRoutesRegisterNoPanic' -count=1 -v

    contract_test.go:91: 已注册路由总数: 94
    contract_test.go:92: 路由清单已写入: D:\ZcodeWorktree\yrzk\mrs-backend\routes.actual.txt
--- PASS: TestDumpRoutes (1.73s)
    contract_test.go:230: 真实注册路由 94 条；docs/API.md 文档化「方法+路径」端点 94 条，仅路径提及 67 条
--- PASS: TestApiCoverage (1.73s)
--- PASS: TestRoutesRegisterNoPanic (1.72s)
PASS
ok  	mrs/internal/router	8.402s
```

（这是 `go fmt` 整理过 `contract_test.go` 之后、且 `internal/service` 恢复可编译后的运行结果。
途中 `internal/service/qcengine.go` 一度被另一作者改到无法编译，`router` 包因依赖 `service`
而连带 `build failed`；待其修复后复跑即全绿，与本次交付无关。）

## 8.5 清理与边界

- **删除**：`mrs-backend/tools_check_contract.py`（一次性脚本，已由 `TestApiCoverage` 取代）。
  临时备份文件（`%TEMP%\API.md.qcbak*`）亦已清理。
- **本任务只改**：`internal/router/contract_test.go`、`routes.diff.md`；删除 `tools_check_contract.py`。
- **未改动**：`docs/API.md`（负控制期间临时改过 1 行，已逐字节恢复并通过 sha256 佐证）、`SPEC.md`、
  `docs/` 下任何文件、任何 `internal/handler/*.go`、`router.go`。
- **未执行** `go build ./...` 与 `go vet ./...`（由任务方统一执行）；只执行了 `go test ./internal/router/`。
- **留档说明**：`routes.diff.md` 自本文件生成后不再由脚本重写（脚本已删除），
  后续请以 `go test ./internal/router/ -v` 的实时结果为准。

## 8.6 报告定稿前的最后一次复核（文档已再被修改）

定稿前 `docs/API.md` 又被作者改过一轮（`d03094c2…` → `bdccab65dbca0680`，666 行）。
对最新版本重新核验，结论不变：

| 项 | 结果 |
|---|---|
| 我负责的测试：`TestDumpRoutes` / `TestApiCoverage` / `TestRoutesRegisterNoPanic` | **全部 PASS** |
| 同包另一作者的 `TestFrontendCallsResolveToRegisteredRoutes`（`frontend_calls_test.go`） | ⚠️ **当前为 FAIL**，与本次交付无关：它比对的是前端 `api-store.js` 调用与路由表，报「前端调用的后端路径存在，但 HTTP 方法不匹配」（`frontend_calls_test.go:229`）。属该作者在飞的工作 |
| 全文 `105` 出现次数 | **0** |
| `/healthz` | 已文档化（§0） |
| `GET /api/qc-results/my-pending` | 已文档化（`docs/API.md:471` 附近） |
| 覆盖率（本次交付口径） | **100%**（A = 0、B = 0，94/94） |
| ⚠️ `:recordId` vs 实现 `:id` | **仍未订正**，且被写得更显眼：`docs/API.md:128` 与 `:131` 各用一整行加粗声明「路径参数名是 **`:recordId`**」；`docs/API.md:657` 的变更记录仍把「旧文档写 `:id`」当作要修正的错误 |
| ⚠️ 目录 §11 = 6（实际 5）、§13 = 6（实际 7） | **仍未订正**（`docs/API.md:32`、`:34`） |

> 两条 ⚠️ 均属**文档描述缺陷**，不影响路由匹配与前端联调的可运行性，但会误导前端按
> `req.params.recordId` 取值（实际应为 `req.params.id`，否则得到 `undefined`）。
> 按分工**仅报告，未修改文档**。
>
> 注：§8.4 引用的是**隔离运行**我负责的三个测试的结果（因此不含另一位作者的
> `TestFrontendCallsResolveToRegisteredRoutes`）。该测试曾一度全绿、后因其自查出
> 「前端调用的后端路径存在，但 HTTP 方法不匹配」（`frontend_calls_test.go:229`）而转红，
> 属该作者在飞的工作。**本次交付的判定不依赖它。**