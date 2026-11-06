package seed

// =====================================================================
// CC/MCC 落表种子（ADR-0002 §9 批次 C，2026-10-09）
//
// 背景：低靠（漏编 CC/MCC）/ 高靠（套高）两条质控提示要在界面上看得见，
// 库里必须同时具备三样东西：
//
//	① cc_mcc_rule 表里的 16 条演示判读规则（判读表本身）；
//	② qc_rule 表里两条 WARN 级质控规则（新规则类型 CC_MCC_LOW / CC_MCC_HIGH）；
//	③ 若干份"其他诊断有内容"的演示病案（现状：seed.go 构造的 MedFrontPage
//	   从未设置 other_diagnosis_codes / other_diagnosis_flags，演示库里其他诊断全空，
//	   端到端只能看到 0 命中）。
//
// 本文件一次交付这三样，三个导出函数互相独立、均可单独调用。
//
// 数据来源与合规红线（ADR §6）：
//   - 判读规则 100% 来自 service.CcMccDemoRules（Source 恒为 demo，版本
//     service.CcMccDemoVersion = "demo-2026.10"），**不是**国家医保局官方表；
//   - 两条质控规则一律 WARN 级——WARN 不拦截、不计入 errCount，不会把合规病案
//     批量判为退回（写成 ERROR 即属回归事故）；
//   - 表里不出现分组码 / 权重 / 金额列（ADR §2/§6 明确禁止）。
//
// 幂等：三个函数都可重复执行，判重键见各函数注释；重复执行不再产生写操作。
//
// 文件所有权：本文件只写这三张表的数据，不调用 AutoMigrate（迁移登记统一在
// main.go，见 BACKLOG「注册函数不做 AutoMigrate」），也不改 cc_mcc_rule 的表模型。
// =====================================================================

import (
	"log"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mrs/internal/model"
	"mrs/internal/service"
)

// SeedCcMccRules 幂等灌入 16 条演示判读规则到 cc_mcc_rule。
//
// 判重键：**(version, diag_prefix)**。选它的理由（ADR 批次评估 §2.1 风险 3）：
// cc_mcc_rule 没有 rule_name 这类业务唯一名，而"某编码前缀在某版本下的定级"
// 天然唯一；换版本时旧版本行原样保留（ADR §4「不同版本可共存」），
// 新版本行按新 (version, prefix) 补入，互不覆盖。
//
// 只补不覆盖：已存在的 (version, diag_prefix) 一律跳过，不更新 severity、
// 不删除、不改 source。因此对已跑过种子的库是纯增量、可重复执行。
//
// 数据源只有一处：service.CcMccDemoRules（ADR §2.1 风险 1 的反向约束——
// 内存切片在这里的角色是**种子数据源**，判读必须走库，不允许两边各存一份事实）。
func SeedCcMccRules(db *gorm.DB) error {
	version := service.CcMccDemoVersion

	// 先捞该版本已有前缀（整行读是省事的写法；真到几千行时这里应退化成 Pluck）。
	var existing []model.CcMccRule
	if err := db.Where("version = ?", version).Find(&existing).Error; err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(existing))
	for _, r := range existing {
		seen[r.DiagPrefix] = struct{}{}
	}

	rows := make([]model.CcMccRule, 0, len(service.CcMccDemoRules))
	for _, r := range service.CcMccDemoRules {
		if _, ok := seen[r.DiagPrefix]; ok {
			continue // 已存在：跳过（幂等，不覆盖）
		}
		seen[r.DiagPrefix] = struct{}{} // 同一批次内的重复前缀也只插一条
		rows = append(rows, model.CcMccRule{
			DiagPrefix:            r.DiagPrefix,
			Severity:              r.Severity,
			Version:               r.Version,
			Source:                r.Source,
			ExcludeWhenMainPrefix: r.ExcludeWhenMainPrefix,
			Remark:                r.Remark,
		})
	}
	if len(rows) == 0 {
		log.Printf("[seed] cc_mcc_rule 无需补充（判重键 version+diag_prefix，当前版本 %s）", version)
		return nil
	}
	// 并发/重复启动保护（与 SeedIcdExt 同套路）：唯一冲突时不做任何事。
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		return err
	}
	log.Printf("[seed] cc_mcc_rule 灌入 %d 条演示判读规则（version=%s，source=demo，非官方表）",
		len(rows), version)
	return nil
}

// ---------- qc_rule：低靠 / 高靠两条 WARN 规则 ----------

// 两条规则的规则名（判重键，故必须是稳定字面量）。
//
// 规则类型直接引用 model.QcCcMccLow / model.QcCcMccHigh（record.go，取值为
// "CC_MCC_LOW" / "CC_MCC_HIGH"）——**不在本文件另起一份类型常量**：
// 本文件写进库的 rule_type 必须与 qcengine 的分支判断同源，两边各写一份
// 字面量迟早会漂移（漂移的表现很难查：规则存在、永远不命中）。
const (
	ccMccLowRuleName  = "CC/MCC 低靠预警（疑似漏编）"
	ccMccHighRuleName = "CC/MCC 高靠预警（疑似套高）"
)

// ccMccWarnLevel 两条规则的严重级别。
//
// 恒为 WARN（不得改 ERROR）：RunQc 只对 ERROR 计 errCount 并置 QC_REJECTED，
// 而本批次的判读表是**演示口径**（Source=demo、非官方表），判错率不低；
// 若按 ERROR 拦截，会把一批合规病案批量判为退回，属回归事故。
// WARN 的语义正是"提示核对、不拦截"（ADR §8：漏编/高靠本质是质量提示）。
const ccMccWarnLevel = "WARN"

// SeedCcMccQcRules 幂等灌入低靠 / 高靠两条 WARN 级质控规则到 qc_rule。
//
// 判重键：**rule_name**（与 handler.SeedQcExt 同口径，qc_rule 的业务唯一名就是规则名）。
// 已存在同名规则则跳过、不更新——因此对已经跑过种子的库不会覆盖手工调整过的行。
//
// 两条规则的 target_field 分工（批次 C 检测单元的判定输入）：
//   - 低靠 → OtherDiagnosisCodes：判定对象是"已录的其他诊断编码"里有没有
//     被判为 CC/MCC 却没在标记里体现的编码，问题出在**标记串**（漏标）；
//   - 高靠 → OtherDiagnosisFlags：判定对象是"标记串"里有没有 C/M 而判读为
//     无严重度的槽位，问题出在**标记本身**（多标）。
//
// 两者都读同一份解析结果（service.ParseOtherDiagnoses），只是报出的问题归属字段不同，
// 故 FieldName 不同、可在质控结果里并存而不互相覆盖（mergeQcResults 按
// (record_id, rule_id, field_name) 三元键 upsert）。
func SeedCcMccQcRules(db *gorm.DB) error {
	rules := []model.QcRule{
		{
			RuleName:    ccMccLowRuleName,
			RuleType:    model.QcCcMccLow,
			TargetField: "OtherDiagnosisCodes",
			ErrorLevel:  ccMccWarnLevel,
			Enabled:     true,
			Remark: "其他诊断中存在按演示判读表应为 CC/MCC 的编码，但首页严重度标记未体现（记为 0 或未标记），" +
				"提示核对是否漏编/漏标并发症合并症。**演示口径，非官方判定，WARN 不拦截**：" +
				"判读依据为院内演示用 CC/MCC 表（Source=demo，非国家医保局官方表），" + service.CcMccDisclaimer,
		},
		{
			RuleName:    ccMccHighRuleName,
			RuleType:    model.QcCcMccHigh,
			TargetField: "OtherDiagnosisFlags",
			ErrorLevel:  ccMccWarnLevel,
			Enabled:     true,
			Remark: "其他诊断的严重度标记存在 C/M，但按演示判读表该编码不构成 CC/MCC，" +
				"提示核对是否存在套高（把不构成严重度的诊断标为并发症合并症）。" +
				"**演示口径，非官方判定，WARN 不拦截**：判读依据为院内演示用 CC/MCC 表（Source=demo，非国家医保局官方表），" +
				service.CcMccDisclaimer,
		},
	}

	for _, r := range rules {
		var cnt int64
		if err := db.Model(&model.QcRule{}).Where("rule_name = ?", r.RuleName).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			continue // 已存在，跳过（幂等）
		}
		if err := db.Create(&r).Error; err != nil {
			return err
		}
		log.Printf("[seed] 已补入 CC/MCC 质控规则: %s（%s，%s，%s）", r.RuleName, r.RuleType, r.TargetField, r.ErrorLevel)
	}
	return nil
}

// ---------- 演示病案：回填"其他诊断编码 + 标记" ----------

// ccMccDemoCase 一份演示样本：其他诊断编码串 + 等长标记串（ADR §5 索引对齐契约）。
//
// 标记取值直接用 service 的常量，不写字面量——与解析层
// （service.ParseOtherDiagnoses / ValidSeverityFlag）同口径，改常量时这里跟着变。
type ccMccDemoCase struct {
	codes string // other_diagnosis_codes：逗号分隔，不允许重复编码
	flags string // other_diagnosis_flags：与 codes 按位一一对应
	use   string // 该样本要演示什么（仅日志用）
}

// ccMccDemoCases 三份可演示样本（顺序即优先级：先低靠、再高靠、最后负控制）。
//
// 选码依据（对照 service.CcMccDemoRules，版本 demo-2026.10）：
//  1. I50.900 心力衰竭 + 标记 '0'：I50 在演示表里是 **MCC**，标记却是"无严重度"
//     ⇒ 判读与已录标记不一致 ⇒ **低靠（漏编/漏标）**应命中；
//  2. K80.201 胆囊结石 + 标记 'C'：K80 在演示表里**没有任何规则**（非 CC 编码），
//     标记却是 CC ⇒ **高靠（套高）**应命中（该码取自 seed.go 既有 ICD-10 字典，
//     不新增可疑编码）；
//  3. I50.900 + 标记 'M'：判读 MCC、标记 MCC，一致 ⇒ **两条规则都不该命中**
//     （负控制：证明两条提示不是"有编码就报"的恒真规则）。
func ccMccDemoCases() []ccMccDemoCase {
	return []ccMccDemoCase{
		{codes: "I50.900", flags: service.SeverityFlagNone, use: "低靠演示：心衰（演示表 MCC）标记填 0，应命中低靠预警"},
		{codes: "K80.201", flags: service.SeverityFlagCC, use: "高靠演示：胆囊结石（非 CC 编码）标记填 C，应命中高靠预警"},
		{codes: "I50.900", flags: service.SeverityFlagMCC, use: "负控制：心衰标记填 M，与判读一致，两条预警都不应命中"},
	}
}

// SeedCcMccDemoCases 给已存在的演示病案回填"其他诊断编码 + 标记"，构造三种可演示样本。
//
// 定位与幂等（重要）：
//   - 先按 **(other_diagnosis_codes, other_diagnosis_flags) 精确匹配**查一遍：
//     命中说明该样本已存在，直接跳过（第二次运行零写操作，结果一致）；
//   - 未命中才挑一份候选首页：要求有主诊断编码、其他诊断**当前为空**、且
//     主诊断不等于即将写入的编码（避免"主诊断与其他诊断同一个码"的脏数据），
//     按 id 升序取第一条，并在本轮内排除已被其他样本占用的首页，保证三份样本
//     落在**三份不同**病案上。
//
// 写入范围：**只更新 other_diagnosis_codes / other_diagnosis_flags 两列**
// （Model(...).Where("id = ?").Updates(map) 定点更新）。不做整行 Save——
// 首页还有并行批次写入的列（external_cause_code / newborn_* 等），
// 整行覆盖会把他人种子写的字段抹成零值。
//
// 失败策略：找不到足够病案时只记日志告警并返回 nil（种子不该让服务起不来）。
// 必须在 seed.SeedDeptUserDemoData(db) **之后**调用：它要改的是已生成的病案首页，
// 且本函数按 id 升序挑页，前置种子改过的归属不影响这里，但顺序反了会挑到
// 尚未完成编目的病案。
func SeedCcMccDemoCases(db *gorm.DB) error {
	used := map[int64]bool{} // 本轮已占用的首页 id（避免三份样本落在同一份病案上）
	for _, c := range ccMccDemoCases() {
		// ① 已存在同款样本 ⇒ 幂等跳过。
		var done model.MedFrontPage
		err := db.Where("other_diagnosis_codes = ? AND other_diagnosis_flags = ?", c.codes, c.flags).
			Order("id ASC").First(&done).Error
		if err == nil {
			used[done.ID] = true
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}

		// ② 挑一份"有主诊断、其他诊断尚空"的首页。
		// 括号是必须的：GORM 虽会给含 OR 的裸条件补括号，但显式写出来
		// 才不会让人误读成 "… AND (is null) OR (= '')" 那种优先级陷阱。
		q := db.Where("main_diagnosis_code <> '' AND main_diagnosis_code <> ?", c.codes).
			Where("(other_diagnosis_codes IS NULL OR other_diagnosis_codes = '')").
			Order("id ASC")
		if len(used) > 0 {
			ids := make([]int64, 0, len(used))
			for id := range used {
				ids = append(ids, id)
			}
			q = q.Where("id NOT IN ?", ids)
		}
		var fp model.MedFrontPage
		if err := q.First(&fp).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				log.Printf("[seed] CC/MCC 演示病案不足：无可回填的空白首页（%s），跳过", c.use)
				continue
			}
			return err
		}

		// ③ 定点更新两列（不整行 Save，不碰他人种子写的字段）。
		if err := db.Model(&model.MedFrontPage{}).Where("id = ?", fp.ID).Updates(map[string]any{
			"other_diagnosis_codes": c.codes,
			"other_diagnosis_flags": c.flags,
		}).Error; err != nil {
			return err
		}
		used[fp.ID] = true
		log.Printf("[seed] CC/MCC 演示病案：首页 #%d（病案 %d，主诊断 %s）回填 其他诊断=%s 标记=%s —— %s",
			fp.ID, fp.RecordID, fp.MainDiagnosisCode, c.codes, c.flags, c.use)
	}
	return nil
}
