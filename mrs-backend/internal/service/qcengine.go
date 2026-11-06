package service

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"mrs/internal/model"
)

// RunQc 质控规则引擎（调研节 8）：必填校验（含条件必填）、逻辑校验、编码存在性校验、
// 主诊断-主手术匹配（DRG）与主要诊断选择提示。
// 结果逐字段落 front_page_qc_result，采用**增量 upsert**（缺口 D9，2026-10-09）：
// 同键（record_id + rule_id + field_name）的历史行只刷新自动判定字段（error_level/error_msg），
// 保留全部人工处理痕迹（handle_status 与退回/整改/复核/fixed 痕迹），不再整批删除重建——
// 旧实现的全删重建会让已退回/已整改/已闭环的项回退为 OPEN，处理痕迹静默丢失。
// 合并策略的完整说明与取舍见 mergeQcResults。
// ERROR 命中 → QC_REJECTED，仅 WARN → QC_PASSED（判定逻辑保持不变，扩展规则不回归）。
// 同时计算质量评分与等级并落 front_page_quality（任务16）。
// 规则清单由 qc_rule 表配置（来源：调研阶段三交付物），不硬编码。
func RunQc(db *gorm.DB, fp *model.MedFrontPage) ([]model.QcResult, *model.FrontPageQuality, error) {
	var rules []model.QcRule
	if err := db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return nil, nil, err
	}

	// ADR-0002 批次 C：CC/MCC 判读规则集。落表后优先读库（规则即数据，换表不改代码）；
	// 读不到或库中没有该版本数据时降级到内置演示表（fail-soft，保证预警不断供）。
	// 演示规则一律 Source=demo，提示文案自带「演示口径」标注，界面另有 disclaimer 兜底（ADR §6）。
	ccmccRules := CcMccDemoRules
	if loaded, err := LoadCcMccRules(db, CcMccDemoVersion); err == nil && len(loaded) > 0 {
		ccmccRules = loaded
	} else if err != nil {
		log.Printf("[qc] cc_mcc_rule 读取失败，本次判读降级使用内置演示表: %v", err)
	} else {
		// err==nil 但查不到数据 = 种子没跑 / 版本对不上 / 表是空的。静默降级会让
		// 「库里有表却没数据」这类配置错误长期隐身，故显式留痕（施工单元 A1 提出）。
		log.Printf("[qc] cc_mcc_rule 在版本 %s 下无规则数据，本次判读降级使用内置演示表", CcMccDemoVersion)
	}

	v := reflect.ValueOf(*fp)
	var results []model.QcResult
	errCount := 0
	for _, rule := range rules {
		// 任务4 条件必填：REQUIRED 规则配置了 condition_expr 时，条件不成立则跳过本规则。
		if rule.RuleType == model.QcRequired &&
			strings.TrimSpace(rule.ConditionExpr) != "" &&
			!evalCondition(rule.ConditionExpr, fp) {
			continue
		}

		// 任务6 扩展规则类型：不依赖单一字段反射，直接以整份首页数据判定。
		// ADR-0002 批次 C：低靠(CC_MCC_LOW) / 高靠(CC_MCC_HIGH) 与既有两类共用本分支，
		// 沿用「空串=未命中」的约定与「仅 ERROR 计 errCount」的拦截口径（批次 C 必须 WARN，不拦截）。
		if rule.RuleType == model.QcDiagOpMatch || rule.RuleType == model.QcMainDiagRule ||
			rule.RuleType == model.QcCcMccLow || rule.RuleType == model.QcCcMccHigh {
			var msg string
			switch rule.RuleType {
			case model.QcDiagOpMatch:
				msg = checkDiagOpMatch(fp)
			case model.QcMainDiagRule:
				msg = checkMainDiag(fp)
			case model.QcCcMccLow:
				msg = CheckCcMccLow(fp, ccmccRules)
			case model.QcCcMccHigh:
				msg = CheckCcMccHigh(fp, ccmccRules)
			}
			if msg == "" {
				continue
			}
			if rule.ErrorLevel == "ERROR" {
				errCount++
			}
			results = append(results, model.QcResult{
				RecordID: fp.RecordID, RuleID: rule.ID, FieldName: rule.TargetField,
				ErrorLevel: rule.ErrorLevel, ErrorMsg: msg, HandleStatus: model.HandleOpen,
			})
			continue
		}

		field := v.FieldByName(rule.TargetField)
		var msg string
		switch {
		case !field.IsValid():
			msg = "规则配置错误：字段不存在（检查 qc_rule.target_field）"
		case rule.RuleType == model.QcRequired && isBlank(field):
			msg = "必填字段为空"
		case rule.RuleType == model.QcCodeExist && !isBlank(field):
			ok, err := codeExists(db, rule.Expression, field.String())
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				msg = "编码 " + field.String() + " 不存在于 " + rule.Expression + " 编码库"
			}
		case rule.RuleType == model.QcLogic:
			msg = checkLogic(rule.Expression, fp)
		}
		if msg == "" {
			continue
		}
		if rule.ErrorLevel == "ERROR" {
			errCount++
		}
		results = append(results, model.QcResult{
			RecordID: fp.RecordID, RuleID: rule.ID, FieldName: rule.TargetField,
			ErrorLevel: rule.ErrorLevel, ErrorMsg: msg, HandleStatus: model.HandleOpen,
		})
	}

	// 任务16：质量评分 + 等级 + 单项否决，随质控结果一并落库。
	score, gradeCode, gradeName, vetos := ScoreQc(results)
	warnCount := len(results) - errCount
	quality := &model.FrontPageQuality{
		RecordID: fp.RecordID, Score: score, GradeCode: gradeCode, GradeName: gradeName,
		ErrorCount: errCount, WarnCount: warnCount,
		Vetoed: len(vetos) > 0, VetoReason: strings.Join(vetos, "；"),
		ScoredAt: time.Now(),
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		// 缺口 D9（2026-10-09）：改为增量 upsert。
		// 旧实现 `DELETE WHERE record_id = ?` + `Create(全部新行)` 会把 handle_status 重置为 OPEN，
		// 并清空 return_opinion/returned_by/returned_at/rectified_*/review_*/fixed* 等人工处理痕迹，
		// 直接打断任务5/D4 的质控退回返修闭环（病案室看不到自己发的退回意见、医生端「我的待整改」项凭空消失）。
		// 现行做法：先取该病案历史行，与本次命中做同键合并，仅需删除的行先留痕再删。
		var existing []model.QcResult
		if err := tx.Where("record_id = ?", fp.RecordID).Order("id ASC").Find(&existing).Error; err != nil {
			return err
		}
		// 纯函数决策：不碰数据库，可在无库环境下用表驱动测试完整覆盖（qcengine_upsert_d9_test.go）。
		// 判重键 (record_id, rule_id, field_name) 的依据见 mergeQcResults 注释。
		finalRows, deleteIDs, err := mergeQcResults(existing, results)
		if err != nil {
			return err
		}
		// 记录「本次不再命中 → 将被删除」的行：这是有意的取舍（保留删除语义，见 mergeQcResults 第 3 点），
		// 故删除前把人工处理痕迹汇总进日志，至少留下可追溯的痕迹，而不是静默丢弃。
		// 依据《电子病历应用管理规范（试行）》的修改留痕要求；更完整的审计（落 front_page_revision 表）
		// 需要 service 层直接读写留痕表，超出本次修复范围，故只做日志留痕。
		for _, row := range vanityRows(existing, deleteIDs) {
			// 人工痕迹标记：has_trace=false 说明这条只是自动判定项（没有人工介入），
			// has_trace=true 的才是有价值的审计线索（退回/整改/复核/fixed 痕迹就此从库中消失）。
			log.Printf("[qc] D9 质控项消失（已修正 / 规则停用 / 规则改 target_field），删除前留痕: has_trace=%t %s",
				qcRowHasManualTrace(row), qcRowAuditBrief(row))
		}
		if len(deleteIDs) > 0 {
			if err := tx.Where("id IN ?", deleteIDs).Delete(&model.QcResult{}).Error; err != nil {
				return err
			}
		}
		// Save 语义：有 ID 的历史行走 UPDATE（保留人工处理痕迹，只刷新自动判定字段），
		// 无 ID 的新命中走 INSERT（handle_status=OPEN，保持现行语义）。同一事务内，重复执行结果一致。
		if len(finalRows) > 0 {
			if err := tx.Save(&finalRows).Error; err != nil {
				return err
			}
		}
		// 质量评分按 record_id 覆盖（先删后建，保证一条最新记录）。
		// 评分口径说明：分数仍只依据**本次命中**计算（ScoreQc(results)），不含历史未命中的已闭环项，
		// 与修复前一致，前端展示无变化。
		if err := tx.Where("record_id = ?", fp.RecordID).Delete(&model.FrontPageQuality{}).Error; err != nil {
			return err
		}
		if err := tx.Create(quality).Error; err != nil {
			return err
		}
		status := model.PageQcPassed
		if errCount > 0 {
			status = model.PageQcRejected
		}
		fp.Status = status
		return tx.Model(fp).Update("status", status).Error
	})
	if err != nil {
		return nil, nil, err
	}
	return results, quality, nil
}

// ---------- 缺口 D9：质控结果增量 upsert（保留人工处理痕迹） ----------

// qcKey 质控结果的识别键：一条质控「项」由 病案 + 规则 + 被校验字段 共同确定。
//
// 为什么用 (record_id, rule_id, field_name) 这个三元组：
//  1. front_page_qc_result 没有天然唯一键（模型 QcResult 无 uniqueIndex，schema.sql 只在 record_id 上建普通索引）；
//  2. RunQc 的规则循环对每条启用规则**最多产出 1 条结果**：REQUIRED/LOGIC/CODE_EXIST 走 reflect.FieldByName(rule.TargetField)，
//     DIAG_OP_MATCH/MAIN_DIAG_RULE 两类扩展规则不反射，其 FieldName 也直接取自 rule.TargetField（qcengine.go 中
//     `FieldName: rule.TargetField`），因此同一规则在一次质控中对同一字段不可能产出两条结果；
//  3. 规则的 target_field 在规则被修改前是固定的，故同一规则+字段在本函数的一次调用内与历史行可稳定对齐；
//  4. 只用 field_name 不够：同一字段可以被多条规则校验（如主诊断同时被 DIAG_OP_MATCH 与 MAIN_DIAG_RULE 盯上），
//     那样会把两条不同项的处理痕迹串在一起；只用 rule_id 也不够（规则改 target_field 后语义已变，需要重新命中）。
//  5. 已知边界：rule_id 被删除/停用或规则改过 target_field 时，历史行无法再与新命中对上，
//     仍会走「消失」分支（保留删除语义）；找不到同键行是新命中，按 OPEN 落库。
type qcKey struct {
	RecordID  int64
	RuleID    int64
	FieldName string
}

// qcKeyOf 取识别键。RuleID<=0（规则已删除的遗留脏数据）或 FieldName 为空的行视为无有效键，
// 不参与同键合并（qcKeyValid 判定），只按 record_id 兜底归属，避免把不同病案的奇数行串到一起。
func qcKeyOf(r model.QcResult) qcKey {
	return qcKey{RecordID: r.RecordID, RuleID: r.RuleID, FieldName: r.FieldName}
}

// qcKeyValid 判断一行是否具备可用的识别键（RuleID 与 FieldName 都必须有效）。
// RunQc 构造的本次命中行恒满足；历史脏行（rule_id=0 / field_name 空）不参与同键合并。
func qcKeyValid(r model.QcResult) bool {
	return r.RuleID > 0 && strings.TrimSpace(r.FieldName) != ""
}

// hasReturnedTrace / hasRectifiedTrace / hasReviewedTrace / hasFixedTrace
// 判断一行是否带有人工处理痕迹（用于删除前日志留痕汇总，不参与合并决策）。
func hasReturnedTrace(r model.QcResult) bool {
	return r.HandleStatus == model.HandleReturned ||
		r.ReturnOpinion != "" || r.ReturnedBy != "" || r.ReturnedAt != nil
}

func hasRectifiedTrace(r model.QcResult) bool {
	return r.HandleStatus == model.HandleRectified ||
		r.RectifiedBy != "" || r.RectifiedAt != nil
}

func hasReviewedTrace(r model.QcResult) bool {
	return r.HandleStatus == model.HandleClosed || r.HandleStatus == model.HandleRejectedByQc ||
		r.ReviewOpinion != "" || r.ReviewedBy != "" || r.ReviewedAt != nil
}

func hasFixedTrace(r model.QcResult) bool {
	return r.Fixed || r.FixedBy != "" || r.FixedAt != nil
}

// autoStateUnchanged 判断本次自动判定结果与历史行的自动判定字段是否一致
// （一致时不重复覆盖，避免无意义的 updated_at 抖动；error_msg 文案变化时才更新）。
func autoStateUnchanged(old, fresh model.QcResult) bool {
	return old.ErrorLevel == fresh.ErrorLevel && old.ErrorMsg == fresh.ErrorMsg
}

// qcRowHasManualTrace 判断一行是否存在任何人工处理痕迹（决定删除时是否值得留痕）。
func qcRowHasManualTrace(r model.QcResult) bool {
	return hasReturnedTrace(r) || hasRectifiedTrace(r) || hasReviewedTrace(r) || hasFixedTrace(r)
}

// qcDeleteAudit 待删除行的人工处理痕迹快照：删除前写日志用，日志输出稳定、可检索。
type qcDeleteAudit struct {
	HandleStatus  string
	ReturnOpinion string
	ReturnedBy    string
	ReturnedAt    string
	RectifiedBy   string
	RectifiedAt   string
	ReviewOpinion string
	ReviewedBy    string
	ReviewedAt    string
	Fixed         bool
	FixedBy       string
	FixedAt       string
}

// newQcDeleteAudit 从质控结果行生成留痕快照（时间统一 RFC3339，nil 显示 -）。
func newQcDeleteAudit(r model.QcResult) qcDeleteAudit {
	ts := func(t *time.Time) string {
		if t == nil {
			return "-"
		}
		return t.Format(time.RFC3339)
	}
	orDash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	return qcDeleteAudit{
		HandleStatus: r.HandleStatus, ReturnOpinion: orDash(r.ReturnOpinion),
		ReturnedBy: orDash(r.ReturnedBy), ReturnedAt: ts(r.ReturnedAt),
		RectifiedBy: orDash(r.RectifiedBy), RectifiedAt: ts(r.RectifiedAt),
		ReviewOpinion: orDash(r.ReviewOpinion), ReviewedBy: orDash(r.ReviewedBy), ReviewedAt: ts(r.ReviewedAt),
		Fixed: r.Fixed, FixedBy: orDash(r.FixedBy), FixedAt: ts(r.FixedAt),
	}
}

// String 输出稳定的 k=v 形式，便于日志检索与人工比对。
func (a qcDeleteAudit) String() string {
	return fmt.Sprintf(
		"handle_status=%s fixed=%t return_opinion=%q returned_by=%s returned_at=%s rectified_by=%s rectified_at=%s review_opinion=%q reviewed_by=%s reviewed_at=%s fixed_by=%s fixed_at=%s",
		a.HandleStatus, a.Fixed, a.ReturnOpinion, a.ReturnedBy, a.ReturnedAt,
		a.RectifiedBy, a.RectifiedAt, a.ReviewOpinion, a.ReviewedBy, a.ReviewedAt,
		a.FixedBy, a.FixedAt)
}

// qcRowAuditBrief 生成单行留痕摘要，供 RunQc 删除「消失项」前汇总打进日志。
func qcRowAuditBrief(r model.QcResult) string {
	return fmt.Sprintf("record=%d rule=%d field=%s id=%d %s",
		r.RecordID, r.RuleID, r.FieldName, r.ID, newQcDeleteAudit(r))
}

// vanityRows 从历史行中筛出「本次将被删除」的行（仅用于删除前日志留痕）。
// 命名沿用 D9 修复说明的语境：这些行只会留下一条日志，随后从库中消失。
func vanityRows(existing []model.QcResult, deleteIDs []int64) []model.QcResult {
	if len(deleteIDs) == 0 {
		return nil
	}
	want := make(map[int64]bool, len(deleteIDs))
	for _, id := range deleteIDs {
		want[id] = true
	}
	out := make([]model.QcResult, 0, len(deleteIDs))
	for _, r := range existing {
		if want[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// zeroManualTrace 返回「无任何人工处理痕迹」的行（本次新命中按此语义落库：handle_status=OPEN）。
// 逐字段显式清零而不是依赖零值字面量：RunQc 传入的 fresh 行当前恒为零值，
// 但显式清零能在未来有人给 fresh 行补痕迹字段时保持「新增项无痕迹」的语义不破。
func zeroManualTrace(r model.QcResult) model.QcResult {
	r.HandleStatus = model.HandleOpen
	r.Fixed = false
	r.FixedAt = nil
	r.FixedBy = ""
	r.ReturnOpinion = ""
	r.ReturnedBy = ""
	r.ReturnedAt = nil
	r.RectifiedBy = ""
	r.RectifiedAt = nil
	r.ReviewOpinion = ""
	r.ReviewedBy = ""
	r.ReviewedAt = nil
	return r
}

// mergeQcResults 缺口 D9 的核心决策（纯函数，不碰数据库，可在无库环境下测试）：
// 输入「库中历史行 existing」+「本次质控命中的自动判定结果 fresh」，
// 输出「最终应写入的行 write」与「必须删除的历史行 ID deleteIDs」。
//
// 识别键：qcKey{RecordID, RuleID, FieldName}（依据见 qcKey 注释）。
//
// 合并规则（三条）：
//  1. 同键命中 → 历史行的**人工处理痕迹字段全部保留**（handle_status、return_opinion/returned_by/returned_at、
//     rectified_by/rectified_at、review_opinion/reviewed_by/reviewed_at、fixed/fixed_at/fixed_by），
//     只把自动判定字段 error_level / error_msg 更新为本次结果；两字段与历史一致时不改写。
//  2. **CLOSED（已闭环）项再次命中同一违规** → 保持 CLOSED，**不静默重新打开**（不做 OPEN/RETURNED 状态回退），
//     仅把 error_msg/error_level 刷新为最新描述并 log 一行便于排查。
//     理由：① 状态抖动比"看到旧问题描述"更糟——重开会把一条已复核确认闭环的项重新推回临床/病案室工作台，
//     而本函数无法判断"是修正失效还是数据又改回去了"；② 本次命中的 ERROR 仍会照常参与评分与
//     QC_PASSED/QC_REJECTED 判定（评分用 fresh 计算，不由本函数决定），所以拦截面不会因保留 CLOSED 而丢失；
//     ③ 保留 error_msg 刷新可让病案室在闭环项上看到最新的问题描述，作为"疑似回归"的可见信号。
//     取舍代价（如实记录）：前端质控工作台对 CLOSED 项只显示「已闭环」+ 最新 errorMsg，没有独立的
//     「疑似回归」标记，因此回归信号较弱；若要强提示需为 front_page_qc_result 增加标记列（本次不允许改模型，
//     故未做）。
//  3. **历史有、本次不再命中** → **保留删除语义**（该行进入 deleteIDs，删除前由 RunQc 汇总人工处理痕迹打日志）。
//     评估过的两个方案：
//     a) 保留该行并标记为"已解决"：模型 QcResult 没有可表达"已解决"的语义字段（Fixed 是「人工确认修正」的
//     痕迹，不是「本次自动判定不再命中」的自动结论，语义不同；handle_status 也没有 RESOLVED 取值）。
//     若为此外扩列，需要改 internal/model/record.go（并行批次正在改该文件，属禁止改动范围）。
//     b) 继续删除：GET /front-pages/:recordId 与 /qc-workbench 都会把该表全量返回，前端 catalog.js 的质控工作台
//     用 `qcData.qcResults.length` 作为「质控项（N）」计数并按 handleStatus 渲染待处理按钮，保留已解决行会在
//     前端制造"问题变多了"的错觉，还会混进 rectify 待整改清单的统计口径。
//     结论：选 b) 保持一致的前端口径，但**删除前用 log.Printf 把该行的人工处理痕迹（退回/整改/复核/fixed 全量）
//     汇总留痕**，避免"静默丢弃"，在注释中声明这是有意取舍（《电子病历应用管理规范（试行）》的留痕要求
//     以日志满足最低限度，未落库）。
//     另：识别键重复的历史脏行（并发产生的重复项）会被收敛为 1 行（保留最后一条，通常是最新写入的那条），
//     其余重复行同样进入 deleteIDs 并留痕，保证 upsert 后不残留重复键。
//
// 幂等性：输出完全由输入决定；把输出当作「历史行」再跑一次得到相同结果（write 顺序稳定：先按 existing 顺序
// 保留存活行，再按 fresh 顺序追加新行），因此同一请求重复执行不会累积脏数据。
func mergeQcResults(existing, fresh []model.QcResult) (write []model.QcResult, deleteIDs []int64, err error) {
	// idx：识别键 → 该键应保留的历史行（同键重复时取最后一条 = 最新写入的那条）。
	// 只登记「有主键」的历史行：ID=0 的行（数据异常）无法原地 UPDATE，不登记，
	// 从而在下面退化为「按本次新命中重新落库」，不会既不写入也不删除。
	idx := make(map[qcKey]model.QcResult, len(existing))
	for _, r := range existing {
		if !qcKeyValid(r) || r.ID == 0 {
			// 历史脏行（rule_id=0 / field_name 空 / 无主键）：不可能与新命中同键，按消失处理；
			// ID=0 的行没有主键可删，只打日志提示（RunQc 的删除按 id IN 执行，天然不会碰它）。
			if r.ID != 0 {
				deleteIDs = append(deleteIDs, r.ID)
			} else {
				log.Printf("[qc] D9 历史质控结果缺少主键 id（record=%d rule=%d field=%s），无主键可删，跳过",
					r.RecordID, r.RuleID, r.FieldName)
			}
			continue
		}
		if prev, dup := idx[qcKeyOf(r)]; dup {
			// 识别键重复（并发重复执行等造成的脏数据）：保留最后出现的那条，删除先出现的那些，
			// 避免 upsert 后仍残留重复键（否则前端工作台会出现两条同样的问题项）。
			// 取舍：只在同键重复时才有分歧——保留「最后一条」而不是「痕迹最完整的一条」，
			// 因为本次命中的自动判定字段会覆盖过去，而最后一条通常就是最近一次人工操作写入的。
			log.Printf("[qc] D9 识别键重复的质控结果：保留 id=%d，删除 id=%d（record=%d rule=%d field=%s）",
				r.ID, prev.ID, r.RecordID, r.RuleID, r.FieldName)
			if prev.ID != 0 {
				deleteIDs = append(deleteIDs, prev.ID)
			}
		}
		idx[qcKeyOf(r)] = r
	}

	// freshByKey：本次命中按识别键索引（保留首条；同键重复理论不可达，仍防御性去重）。
	freshByKey := make(map[qcKey]model.QcResult, len(fresh))
	for _, f := range fresh {
		if !qcKeyValid(f) {
			// 理论不可达（RunQc 恒带 rule_id 与 field_name）：仍按新增写入，但不参与同键合并。
			log.Printf("[qc] D9 本次命中缺少识别键（rule=%d field=%q），按新增写入且不与历史痕迹合并", f.RuleID, f.FieldName)
			continue
		}
		if _, dup := freshByKey[qcKeyOf(f)]; dup {
			log.Printf("[qc] D9 本次命中存在重复识别键（record=%d rule=%d field=%s），仅保留首条", f.RecordID, f.RuleID, f.FieldName)
			continue
		}
		freshByKey[qcKeyOf(f)] = f
	}

	// 1) 历史存活行：保留全部人工处理痕迹，只把自动判定字段刷新为本次结果。
	for _, r := range existing {
		if !qcKeyValid(r) || r.ID == 0 {
			continue // 已在上面按消失/跳过处理（ID=0 的行不能原地 UPDATE，下面按新命中重新落库，避免写重）
		}
		if idx[qcKeyOf(r)].ID != r.ID {
			continue // 同键重复行中未被保留的那条（已在上面收集待删）
		}
		f, hit := freshByKey[qcKeyOf(r)]
		if !hit {
			continue // 本次不再命中 → 走删除分支（保留删除语义）
		}
		row := r // 值拷贝：不就地改动入参
		if !autoStateUnchanged(row, f) {
			if row.HandleStatus == model.HandleClosed {
				// 已闭环项再次命中同一违规（修正未生效 / 数据又变回去）：保持 CLOSED 不重开，
				// 仅刷新描述，并打一行日志让病案室可察觉「疑似回归」。详见本函数注释第 2 点。
				log.Printf("[qc] D9 已闭环质控项再次命中（疑似回归，保持 CLOSED 不重开）: id=%d record=%d rule=%d field=%s old=[%s]%s new=[%s]%s reviewed_by=%s reviewed_at=%v",
					r.ID, r.RecordID, r.RuleID, r.FieldName, r.ErrorLevel, r.ErrorMsg,
					f.ErrorLevel, f.ErrorMsg, r.ReviewedBy, r.ReviewedAt)
			}
			row.ErrorLevel = f.ErrorLevel
			row.ErrorMsg = f.ErrorMsg
		}
		write = append(write, row)
	}

	// 2) 本次新命中（无同键历史行）：handle_status = OPEN（保持现行语义），无任何人工处理痕迹。
	for _, f := range fresh {
		if !qcKeyValid(f) {
			write = append(write, zeroManualTrace(f))
			continue
		}
		if _, hasHistory := idx[qcKeyOf(f)]; hasHistory {
			continue // 已有同键历史行，已在上一步合并
		}
		row := zeroManualTrace(f) // 值拷贝：不就地改动 RunQc 传入的 results（它还用于评分与接口返回）
		row.ID = 0                // 防御：确保走 INSERT 而不是 UPDATE 到别的行
		write = append(write, row)
	}

	// 3) 消失行：历史有、本次未命中（已修正 / 规则停用或改字段）→ 删除（保留删除语义，RunQc 删除前留痕）。
	for _, r := range existing {
		if qcKeyValid(r) && idx[qcKeyOf(r)].ID == r.ID {
			if _, hit := freshByKey[qcKeyOf(r)]; hit {
				continue // 存活行
			}
		}
		// 其余情况（无有效识别键 / 同键重复的非保留行 / 本次未命中）均待删除；上面已收集过的此处会去重。
		if r.ID != 0 {
			deleteIDs = append(deleteIDs, r.ID)
		}
	}

	deleteIDs = dedupSortedIDs(deleteIDs)
	if write == nil {
		write = []model.QcResult{}
	}
	return write, deleteIDs, nil
}

// dedupSortedIDs 对 ID 列表去重并升序排序，保证 mergeQcResults 的输出稳定（幂等 / 测试可断言）。
func dedupSortedIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := ids[:1]
	for _, id := range ids[1:] {
		if id != out[len(out)-1] {
			out = append(out, id)
		}
	}
	return out
}

func isBlank(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return strings.TrimSpace(v.String()) == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Ptr:
		if v.IsNil() {
			return true
		}
		return isBlank(v.Elem())
	case reflect.Struct:
		if t, ok := v.Interface().(time.Time); ok {
			return t.IsZero()
		}
	}
	return false
}

func codeExists(db *gorm.DB, dictType, code string) (bool, error) {
	if dictType == "" { // 规则未配 expression 时按编码前缀粗判
		dictType = model.Icd10
	}
	var cnt int64
	err := db.Model(&model.IcdDict{}).
		Where("dict_type = ? AND code = ? AND enabled = ?", dictType, code, true).
		Count(&cnt).Error
	return cnt > 0, err
}

// checkLogic 逻辑校验，骨架内置两条常用规则，其余待调研规则清单后扩展表达式引擎
func checkLogic(expression string, fp *model.MedFrontPage) string {
	switch expression {
	case "discharge_time>admit_time":
		if fp.AdmitTime != nil && fp.DischargeTime != nil && !fp.DischargeTime.After(*fp.AdmitTime) {
			return "出院时间必须晚于入院时间"
		}
	case "actual_days>0":
		if fp.ActualDays <= 0 {
			return "实际住院天数必须大于 0"
		}
	default:
		fmt.Printf("[qc] 未实现的逻辑规则表达式（已跳过）: %s\n", expression)
	}
	return ""
}

// marshalDebug 保留：排查质控字段反射问题时打开
func marshalDebug(v any) string { b, _ := json.Marshal(v); return string(b) }
