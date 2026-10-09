package service

import (
	"fmt"
	"slices"
	"strings"
)

// =====================================================================
// CC/MCC 判读表（ADR-0002 §4，批次 B）
//
// 定位：只回答「某个其他诊断在本院口径下像 CC 还是 MCC」，供严重度分层预览、
// 低靠（漏编）与高靠（套高）预警使用（ADR §2）。**不出具分组码与权重**——那不是
// 实现工作量问题，而是授权件问题（ADR §3）：凡依赖外部授权件的部分一律不做假实现。
//
// ADR §4 的三条硬要求全部落在本文件：
//  1. 可替换 + 版本化：规则是数据（CcMccRule），换官方表只换数据不改代码；
//     每条规则带 Version，判读只应用指定版本（"按当前启用版本查询"）。
//  2. 来源标注：Source 取 "demo" / "official"；demo 数据界面必须显著标注。
//     判读结果用 Sources / DemoOnly 把这个事实传给调用方（ADR §6 合规红线：
//     用自造表出具支付等级判断，风险高于技术实现问题）。
//  3. 免责声明内置：任何返回严重度判断的接口/结果都必须带 disclaimer，
//     故 Disclaimer 是 CcMccJudgement 的字段，由本文件填充，调用方不自己拼。
//     文案沿用 handler/drg_preview.go 的 drgDisclaimer 风格。
//
// 本期内置的 CcMccDemoRules 全部为 Source=demo 的**演示条目**（自造，非官方数据，
// 见 ADR §6：官方 2.0 版 CC/MCC 表尚未取得），官方表到位后按行替换即可。
//
// 与 other_diagnosis.go 的分工（同包，两侧互不修改对方文件）：
// 那边管「字符串 ↔ 编码/标记列表」的对齐与校验；这边管「编码 → 严重度」查表。
// 批次 C 的低靠/高靠检测拿"解析层的已录标记"与"本层的判读严重度"逐位比对即可。
//
// 术语提醒（两个"无"不要混）：本文件的 SeverityNone = "0"，与 OtherDiagnosisFlags
// 的取值口径一致（ADR §5 锁定，同一份 JSON 里只出现一种"无"）；而
// other_diagnosis.go 的 SeverityLevelNone = "NONE" 是"本例严重度分层"的展示口径。
// 两者语义相同、字面不同，跨层拼接时需显式映射（见 JudgeSeverity 注释）。
// =====================================================================

// ---------- 严重度等级（ADR §4：MCC / CC / 无）----------

const (
	SeverityMCC  = "MCC" // MCC：主要并发症/合并症
	SeverityCC   = "CC"  // CC：并发症/合并症
	SeverityNone = "0"   // 无严重度。取 "0" 而非空串，是为了与 OtherDiagnosisFlags 的
	// '0' / 'C' / 'M' 口径对齐（ADR §5）：同一份响应里出现两种"无"，
	// 前端就得写映射表，而映射表正是错位与漏判的温床。
)

// 规则数据来源（ADR §4：每条记录标明 official / demo；demo 必须在界面显著标注）。
const (
	CcMccSourceDemo     = "demo"     // 演示数据：自造条目，非官方口径
	CcMccSourceOfficial = "official" // 官方表数据（本期尚未取得，见 ADR §6）
)

// CcMccDemoVersion 内置演示规则集的版本号。
// 版本号刻意带日期：换了演示数据就换版本号，事后能追溯"当时用的是哪一版表"
// （ADR §4「不同版本可共存」；官方表到位后另起版本号，与本版本并存）。
const CcMccDemoVersion = "demo-2026.10"

// CcMccDisclaimer 免责声明：它是接口契约的一部分，不是注释（ADR §4 强约束）。
// 任何返回严重度判断的接口/结果都必须原样携带，调用方不要自行改写或省略。
const CcMccDisclaimer = "本判断基于院内演示用 CC/MCC 判读表（Source=demo，非国家医保局官方表），" +
	"仅供编码质控参考，不作为医保结算或 DRG/DIP 分组依据；正式严重度分层须使用官方发布的 CC/MCC 表与分组器。"

// CcMccRule 一条判读规则（ADR §4「表的设计要点」的落地形态）。
// 现在它是内存里的结构体；将来落库为 cc_mcc_rule 表时字段一一对应，
// 判读逻辑无需改动（这正是"只换数据不改代码"的含义）。
type CcMccRule struct {
	DiagPrefix string // 诊断编码前缀（ICD-10，大写，不含通配符；空前缀视为无效规则）
	Severity   string // MCC / CC（不得为 SeverityNone：没有严重度的诊断不需要规则）
	Version    string // 所属版本：判读时只应用与请求版本相等的规则
	Source     string // "demo" / "official" —— 来源标注，界面须能显著区分
	// ExcludeWhenMainPrefix 排除规则留位（官方排除表的核心复杂度，ADR §4/§7）：
	// 主诊断命中该前缀时，本规则**不适用**，该诊断不计为 CC/MCC。
	// 本期实现的是"最简可用"形态：单前缀 + 前缀匹配，见 JudgeSeverity 与
	// CcMccItem.Excluded 的说明；官方排除表的完整形态（多对多排除、
	// 按 MDC/ADRG 分组排除、版本间差异）未实现，不得据此认为已达结算级准确度。
	ExcludeWhenMainPrefix string
	Remark                string // 备注：演示条目的临床含义 / 排除说明（会随判读结果回传）
}

// ---------- 判读结果（对外 JSON 结构）----------

// CcMccItem 单个其他诊断的判读结果。Items 与入参 otherCodes **按位置一一对应**
// （包括空编码占位），调用方可直接与解析层的已录标记串逐位比对。
type CcMccItem struct {
	Code          string `json:"code"`                    // 规范化后的编码（去空格 + 转大写）
	Severity      string `json:"severity"`                // MCC / CC / 0
	MatchedPrefix string `json:"matchedPrefix,omitempty"` // 命中的规则前缀（未命中为空）
	Excluded      bool   `json:"excluded"`                // 命中的规则被排除规则挡下 ⇒ 不计 CC/MCC
	ExcludeReason string `json:"excludeReason,omitempty"` // 排除原因（回传给人看的解释）
	Source        string `json:"source,omitempty"`        // 命中规则的来源：demo / official
	Remark        string `json:"remark,omitempty"`        // 命中规则的备注（含演示标注）
}

// CcMccJudgement 判读结果。Disclaimer 由本层填充（ADR §4 强约束）。
type CcMccJudgement struct {
	MainDiagnosisCode string      `json:"mainDiagnosisCode"`
	Version           string      `json:"version"`         // 实际应用的规则版本（空版本 ⇒ 无规则生效）
	Items             []CcMccItem `json:"items"`           // 与 otherCodes 等长（空输入为 []，非 null）
	OverallSeverity   string      `json:"overallSeverity"` // 总体分层：MCC > CC > 0
	MccCount          int         `json:"mccCount"`
	CcCount           int         `json:"ccCount"`
	Sources           []string    `json:"sources"`  // 生效规则的来源集合（升序去重）
	DemoOnly          bool        `json:"demoOnly"` // 生效规则全为演示数据 ⇒ 界面必须显著标注"非官方"
	Disclaimer        string      `json:"disclaimer"`
}

// JudgeSeverity 判读严重度：给定版本、主诊断编码与一组其他诊断编码，逐项查表给出
// 严重度，并汇总本例的总体分层。
//
// 签名说明：任务书给的形态是 (mainDiag, otherCodes, rules)；这里把 version 提到第一位，
// 因为 ADR §4 要求「表记录归属某个 version，按当前启用版本查询」——不给版本参数，
// 「只应用指定版本的规则」就无从谈起（把版本写死在函数里则换版本要改代码，与
// 「换数据不改代码」的决策冲突）。演示期调用方传 (CcMccDemoVersion, fp.MainDiagnosisCode, codes, CcMccDemoRules)。
//
// version 为空 ⇒ 没有任何规则生效（fail-closed）：宁可判"无严重度"，也不让未声明版本的
// 规则悄悄参与判断——静默使用演示数据正是 ADR §6 的合规红线。
//
// 入参口径：
//   - mainDiag / otherCodes 允许大小写与首尾空格，内部按 normalizeDiagnosisCode
//     （other_diagnosis.go，与 drg_rules.go / qc_condition.go 同口径）规范化；
//     otherCodes 通常来自 ParseOtherDiagnoses → OtherDiagnosisCodes；
//   - rules 可混含多个版本，本函数自行按 Version 过滤，调用方不必预筛。
//
// 判读规则（与 handler/drg_preview.go 的匹配口径一致：简单前缀匹配，无通配符）：
//  1. 命中 = code 以规则 DiagPrefix 开头（规范化后比较）；空 DiagPrefix 跳过
//     （空前缀会匹配一切，与 drg_preview.go 的处理一致）；
//  2. 多条命中取**前缀最长者**（更具体者优先，同 drg_preview.go 的 bestScore 思路）；
//     等长前缀不可能同时命中同一编码，故结果确定；
//  3. 取中的规则若 ExcludeWhenMainPrefix 非空且主诊断命中该前缀 ⇒ 该诊断
//     **不计**为 CC/MCC（Severity=None、Excluded=true），且**不再回退**到较宽泛的
//     其他规则：排除规则针对的是"该编码在本次主诊断下算不算"，最具体的条目说了算，
//     回退等于用一个更泛的条目绕开排除。
//
// 范围声明：本函数不含官方分组码、权重与费用折算，结果不得作为结算依据。
func JudgeSeverity(version, mainDiag string, otherCodes []string, rules []CcMccRule) CcMccJudgement {
	main := normalizeDiagnosisCode(mainDiag)
	ver := strings.TrimSpace(version)

	out := CcMccJudgement{
		MainDiagnosisCode: main,
		Version:           ver,
		Items:             make([]CcMccItem, 0, len(otherCodes)), // 非 nil：JSON 出 [] 而不是 null
		OverallSeverity:   SeverityNone,
		Sources:           []string{},
		Disclaimer:        CcMccDisclaimer,
	}

	hasOfficial := false
	for _, code := range otherCodes {
		item := judgeCcMccOne(main, code, ver, rules)
		switch item.Severity {
		case SeverityMCC:
			out.MccCount++
		case SeverityCC:
			out.CcCount++
		}
		if item.Source != "" {
			if !slices.Contains(out.Sources, item.Source) {
				out.Sources = append(out.Sources, item.Source)
			}
			if item.Source == CcMccSourceOfficial {
				hasOfficial = true
			}
		}
		out.Items = append(out.Items, item)
	}

	switch {
	case out.MccCount > 0:
		out.OverallSeverity = SeverityMCC
	case out.CcCount > 0:
		out.OverallSeverity = SeverityCC
	}

	slices.Sort(out.Sources)
	// 命中过规则、且命中的规则里没有 official ⇒ 本结论完全建立在演示数据上，
	// 界面必须显著标注（ADR §4/§6）。一条都没命中时为 false：没有依据可言，
	// 不冒充"演示结论"。
	out.DemoOnly = len(out.Sources) > 0 && !hasOfficial
	return out
}

// judgeCcMccOne 判读单个其他诊断。code 为空时原样返回"无严重度"，保证与入参等长对齐。
func judgeCcMccOne(main, code, version string, rules []CcMccRule) CcMccItem {
	c := normalizeDiagnosisCode(code)
	item := CcMccItem{Code: c, Severity: SeverityNone}

	rule := matchCcMccRule(c, version, rules)
	if rule == nil {
		return item
	}

	item.MatchedPrefix = rule.DiagPrefix
	item.Source = rule.Source
	item.Remark = rule.Remark
	if rule.excludesMain(main) {
		item.Excluded = true
		item.ExcludeReason = fmt.Sprintf(
			"主诊断【%s】命中排除前缀【%s】，按演示表本条不计为 %s",
			main, rule.ExcludeWhenMainPrefix, rule.Severity)
		return item
	}
	item.Severity = rule.Severity
	return item
}

// matchCcMccRule 在指定版本内找该编码最具体（前缀最长）的规则；无命中返回 nil。
// 版本不等（含请求版本为空）、前缀为空、严重度非 MCC/CC 的规则一律跳过。
func matchCcMccRule(code, version string, rules []CcMccRule) *CcMccRule {
	if code == "" || version == "" {
		return nil
	}
	var best *CcMccRule
	for i := range rules {
		r := &rules[i]
		if strings.TrimSpace(r.Version) != version {
			continue
		}
		if r.Severity != SeverityMCC && r.Severity != SeverityCC {
			continue // 无严重度的规则没有意义，宁可判"无"也不猜
		}
		prefix := normalizeDiagnosisCode(r.DiagPrefix)
		if prefix == "" || !strings.HasPrefix(code, prefix) {
			continue
		}
		if best == nil || len(prefix) > len(normalizeDiagnosisCode(best.DiagPrefix)) {
			best = r
		}
	}
	return best
}

// excludesMain 排除规则判定：主诊断以 ExcludeWhenMainPrefix 开头时不适用本规则。
// 前缀为空 = 无排除条件。主诊断为空时不触发排除（缺主诊断另有必填规则去管，
// 这里不越界报错）。
func (r CcMccRule) excludesMain(main string) bool {
	prefix := normalizeDiagnosisCode(r.ExcludeWhenMainPrefix)
	return prefix != "" && strings.HasPrefix(main, prefix)
}

// ---------- 内置演示规则集（Source 一律 demo，非官方数据）----------

// CcMccDemoRules 演示用 CC/MCC 判读表，共 16 条（ADR §9 批次 B）。
//
// ⚠️ 这不是官方表：条目由本院按公开临床常识示例性编写，用于打通"编码 → 严重度"
// 的链路与演示，**不得作为结算依据**（ADR §6 合规红线）。官方 2.0 版 CC/MCC 表
// 到位后：新建一个 official 版本号、整体替换本切片的数据，判读逻辑不动。
//
// 覆盖前缀（演示范围，远不足以支撑真实分层，ADR §7「依赖码表规模」）：
//
//	MCC：A41 脓毒症 / I21 急性心肌梗死 / I50 心力衰竭 / J96 呼吸衰竭 /
//	     K72 肝衰竭 / N17 急性肾衰竭 / N18.5 慢性肾脏病5期 / I63 脑梗死
//	CC ：E11.6 2型糖尿病伴并发症 / I48 心房颤动 / I25.1 冠心病 /
//	     J44 慢阻肺 / D63 慢性病贫血 / F05 谵妄 / L89 压力性损伤 /
//	     I69.3 脑梗死后遗症
//
// 其中 2 条带 ExcludeWhenMainPrefix（演示"官方排除表"的形态，详见各条 Remark）。
var CcMccDemoRules = []CcMccRule{
	// ---- MCC（演示口径）----
	demoCcMccRule("A41", SeverityMCC, "", "脓毒症（败血症）：演示条目，非官方 MCC 判定"),
	demoCcMccRule("I21", SeverityMCC, "", "急性心肌梗死：演示条目，非官方 MCC 判定"),
	demoCcMccRule("I50", SeverityMCC, "", "心力衰竭：演示条目，非官方 MCC 判定"),
	demoCcMccRule("J96", SeverityMCC, "", "呼吸衰竭：演示条目，非官方 MCC 判定"),
	demoCcMccRule("K72", SeverityMCC, "", "肝衰竭（含肝性脑病）：演示条目，非官方 MCC 判定"),
	demoCcMccRule("N17", SeverityMCC, "", "急性肾衰竭：演示条目，非官方 MCC 判定"),
	demoCcMccRule("N18.5", SeverityMCC, "", "慢性肾脏病5期（终末期肾病）：演示条目，非官方 MCC 判定"),
	demoCcMccRule("I63", SeverityMCC, "", "脑梗死（急性期）：演示条目，非官方 MCC 判定"),

	// ---- CC（演示口径）----
	demoCcMccRule("E11.6", SeverityCC, "", "2型糖尿病伴其他特指的并发症：演示条目；无并发症的糖尿病（如 E11.9）不在表内"),
	demoCcMccRule("I48", SeverityCC, "", "心房颤动与心房扑动：演示条目，非官方 CC 判定"),
	demoCcMccRule("I25.1", SeverityCC, "", "动脉粥样硬化性心脏病：演示条目，非官方 CC 判定"),
	demoCcMccRule("J44", SeverityCC, "", "慢性阻塞性肺疾病：演示条目，非官方 CC 判定"),
	demoCcMccRule("F05", SeverityCC, "", "谵妄（非酒精等所致）：演示条目，非官方 CC 判定"),
	demoCcMccRule("L89", SeverityCC, "", "压力性损伤（褥疮）：演示条目，非官方 CC 判定"),
	demoCcMccRule("D63", SeverityCC, "C",
		"慢性病所致贫血：演示条目。演示排除形态——主诊断已为恶性肿瘤(C*)时该贫血多属疾病本身表现，不计为 CC；不含官方按 MDC/ADRG 分组的排除逻辑"),
	demoCcMccRule("I69.3", SeverityCC, "I63",
		"脑梗死后遗症：演示条目。演示排除形态——主诊断已为急性脑梗死(I63*)时系同一疾病过程的陈旧表现，不计为 CC"),
}

// demoCcMccRule 构造一条演示规则：版本、来源、排除前缀在构造处统一填，
// 保证内置表不可能混进非 demo 数据（这是 ADR §6 红线在代码层的兜底）。
func demoCcMccRule(prefix, severity, excludeWhenMainPrefix, remark string) CcMccRule {
	return CcMccRule{
		DiagPrefix:            prefix,
		Severity:              severity,
		Version:               CcMccDemoVersion,
		Source:                CcMccSourceDemo,
		ExcludeWhenMainPrefix: excludeWhenMainPrefix,
		Remark:                remark,
	}
}
