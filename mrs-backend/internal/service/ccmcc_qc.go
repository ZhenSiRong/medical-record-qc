package service

import (
	"fmt"
	"strings"

	"mrs/internal/model"
)

// =====================================================================
// CC/MCC 编码质控：低靠（漏编）与高靠（套高）检测（ADR-0002，批次 C）
//
// 分工（两侧互不修改对方文件）：
//   - other_diagnosis.go：字符串 ↔ 编码/标记列表的对齐与校验，产出 []OtherDiagnosisItem；
//   - ccmcc.go：编码 → 严重度查表，Items 与入参 otherCodes **按位置一一对应**；
//   - 本文件：把两侧同一个位置的「已录标记」与「判读严重度」拿出来比对，产出 WARN 文案。
//
// 两条检测的口径（本文件的全部业务含义就在这两行）：
//
//	低靠（漏编）：判读是 CC/MCC，但编码员把标记填成了「无」→ 可能少编了一个并发症；
//	高靠（套高）：标记是 C/M，但判读不是 CC/MCC（不在表内，或被排除规则挡下）→ 可能在套高。
//
// 均为 **WARN 级提示，不拦截**（只有 ERROR 才会让病案落到 QC_REJECTED）。
// 本层只负责"不写出拦截语义"：文案用"建议核对"/"可忽略"这类降级措辞，
// 不出现「必须修改」「不予通过」「拦截」「退回」等判定句——
// ERROR/WARN 由 qc_rule.error_level 决定（种子数据的责任），本文案在两种级别下都得站得住。
//
// 合规红线（ADR §6）：判读表当前是 Source=demo 的**演示数据**，不是国家医保局官方
// CC/MCC 表。故本文件产出的每条文案都**必须**带「演示」字样并明示不作结算依据——
// 这是文案的硬性成分，不是措辞偏好，改写时不得删掉。
//
// 纯函数：不碰数据库、不碰全局状态、不依赖时间，输入相同则输出相同，
// 便于零依赖的表驱动测试（对齐 other_diagnosis_test.go / ccmcc_test.go 的风格）。
// =====================================================================

// ccMccQcMsgSep 多条提示的连接符。
const ccMccQcMsgSep = "；"

// CheckCcMccLow 低靠（漏编）检测：其他诊断在 CC/MCC 判定表中构成 CC/MCC，
// 但与其**同一槽位**的严重度标记为「无」→ 提示核对是否漏编。
//
// 未命中（含首页为空 / 无其他诊断 / 解析出错 / 无规则生效）返回空串——
// 质控提示的风险方向必须偏向"不多说"：一个凭空出现的 WARN 比漏一条 WARN 更难被发现，
// 且会稀释真实提示的信噪比（ADR §6 的红线正是"用自造表出具支付等级判断"）。
//
// rules 可混含多个版本，实际只应用 Version == CcMccDemoVersion 的规则（见 ccmcc.go：
// 版本不符的规则不参与判读）。若该版本下没有任何规则，本函数直接返回空串。
func CheckCcMccLow(fp *model.MedFrontPage, rules []CcMccRule) string {
	pairs, ok := ccAlignedCcMccPairs(fp, rules)
	if !ok {
		return ""
	}

	msgs := make([]string, 0, len(pairs))
	for _, p := range pairs {
		// 判读侧必须真的构成 CC/MCC（被排除者的 Severity 已是「无」，这里再挡一层）。
		if p.excluded || (p.judged != SeverityMCC && p.judged != SeverityCC) {
			continue
		}
		if !ccIsNoneSeverity(p.flag) {
			continue // 编码员已经标记了 CC/MCC ⇒ 不漏编
		}
		// 措辞口径（专家回执）：这是**提示核对**而非拦截判定，故用"建议核对"而非命令式，
		// 并给出明确的"可忽略"出口（陈旧性 / 既往史是漏编误报的主要来源）——
		// 一条没有出口的机器提示，只会被当成噪音忽略或被当成命令执行。
		msgs = append(msgs, fmt.Sprintf(
			"其他诊断【%s】在演示判定表中为 %s，但未标记为并发症/合并症，建议核对是否漏编；如属陈旧性或既往史，可忽略本提示（演示口径，非官方判定，不作结算依据）",
			p.code, p.judged))
	}
	return strings.Join(msgs, ccMccQcMsgSep)
}

// CheckCcMccHigh 高靠（套高）检测：其他诊断被标记为 C（CC）/ M（MCC），
// 但同一槽位的编码在 CC/MCC 判定表中**不构成** CC/MCC
// （表内无对应条目，或命中了排除规则）→ 提示核对高靠风险。
//
// 未命中返回空串，理由与 CheckCcMccLow 相同（绝不误报）。
//
// 标记串的字面取值以 other_diagnosis.go 的解析层为准（'M' MCC / 'C' CC / '0' 无），
// 不要拿 ccmcc.go 的 SeverityMCC("MCC")/SeverityCC("CC") 去跟它比——那是判读结果的口径。
// 本函数内部做了这层映射（ccFlagLabel），调用方不必关心。
func CheckCcMccHigh(fp *model.MedFrontPage, rules []CcMccRule) string {
	pairs, ok := ccAlignedCcMccPairs(fp, rules)
	if !ok {
		return ""
	}

	msgs := make([]string, 0, len(pairs))
	for _, p := range pairs {
		label := ccFlagLabel(p.flag)
		if label == "" {
			continue // 标记为「无」⇒ 没有套高的立足点
		}
		switch {
		case p.excluded:
			// 命中了规则但被排除规则挡下：p.reason 由 ccmcc.go 填充，已含主诊断与排除前缀。
			msgs = append(msgs, fmt.Sprintf(
				"其他诊断【%s】被标记为 %s，但在演示判定表中不构成 CC/MCC（已被排除：%s），建议核对病案实际诊断依据并复核该标记（演示口径，非官方判定，不作结算依据）",
				p.code, label, p.reason))
		case p.judged != SeverityMCC && p.judged != SeverityCC:
			msgs = append(msgs, fmt.Sprintf(
				"其他诊断【%s】被标记为 %s，但在演示判定表中不构成 CC/MCC（表内无对应条目），建议核对高靠风险与病案依据（演示口径，非官方判定，不作结算依据）",
				p.code, label))
		default:
			// 判读确实是 CC/MCC ⇒ 标记正确，不提示。
		}
	}
	return strings.Join(msgs, ccMccQcMsgSep)
}

// ccAlignedPair 一个槽位上的「已录标记」与「判读严重度」的对齐结果。
// 两个检测函数只消费这一种结构，避免各写一次 strings.Split 造成口径分裂
// （索引对齐的唯一权威是 other_diagnosis.go 的解析层）。
//
// 之所以按**解析后的列表**逐位比对、而不是按原始逗号槽位：空编码槽位会在解析层
// 与其标记一起被丢弃，强行按原始槽位对齐反而会把两个不同架构的位置对到一起。
// 这是标的错了比没标更糟的典型场景，故一律走解析层。
type ccAlignedPair struct {
	code     string // 规范化后的编码（解析层口径：去空格 + 转大写）
	flag     string // 已录严重度标记："0" / "C" / "M"（可能含非法值，靠 defensively 兜住）
	judged   string // 判读严重度：MCC / CC / 0
	excluded bool   // 命中规则但被排除规则挡下
	reason   string // 排除原因（ccmcc.go 原样填充）
}

// ccAlignedCcMccPairs 把「解析层的已录标记」与「判读层的严重度」按位置配成对。
//
// 返回 ok=false 表示"判断不了"，调用方一律按空串处理（不提示）。具体包括：
//   - fp 为 nil；
//   - 该版本下没有任何可用规则（无依据 ⇒ 不给出任何结论，这是 fail-closed，不是偷懒）；
//   - 其他诊断为空、或解析出错（长度不等 / 重复编码 / 非法标记）；
//   - 判读结果长度与解析结果长度不等（理论上不可能；真出现说明对齐假设已破，宁可不提示）。
func ccAlignedCcMccPairs(fp *model.MedFrontPage, rules []CcMccRule) ([]ccAlignedPair, bool) {
	if fp == nil {
		return nil, false
	}
	if len(ccRulesOfVersion(CcMccDemoVersion, rules)) == 0 {
		return nil, false
	}

	items, err := ParseOtherDiagnoses(fp.OtherDiagnosisCodes, fp.OtherDiagnosisFlags)
	if err != nil || len(items) == 0 {
		// 解析出错 ⇒ 数据本身错位，此时任何"位置 i 的判读 vs 位置 i 的标记"都不可信。
		return nil, false
	}

	codes := OtherDiagnosisCodes(items)
	judgement := JudgeSeverity(CcMccDemoVersion, fp.MainDiagnosisCode, codes, rules)
	if len(judgement.Items) != len(items) {
		return nil, false
	}

	out := make([]ccAlignedPair, 0, len(items))
	for i, it := range items {
		j := judgement.Items[i]
		out = append(out, ccAlignedPair{
			code:     it.Code,
			flag:     normalizeFlagToken(it.Flag), // "" → "0"，与 Low 的判空口径一致
			judged:   j.Severity,
			excluded: j.Excluded,
			reason:   j.ExcludeReason,
		})
	}
	return out, true
}

// ccRulesOfVersion 取出指定版本下的有效规则（前缀为空的规则无判别力，不算）。
// 只用于判断"有没有依据"，不替代 JudgeSeverity 的匹配逻辑。
func ccRulesOfVersion(version string, rules []CcMccRule) []CcMccRule {
	out := make([]CcMccRule, 0, len(rules))
	for _, r := range rules {
		if strings.TrimSpace(r.Version) != version {
			continue
		}
		if normalizeDiagnosisCode(r.DiagPrefix) == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// ccIsNoneSeverity 判断一个「无」是不是无严重度。
//
// ⚠️ 坑（踩过一次，整仓编译是被这一行卡住的）：SeverityNone（ccmcc.go）与
// SeverityFlagNone（other_diagnosis.go）同在 service 包，且**字面值都是 "0"**。
// 因此在同一个 switch 里为它们各写一个 case 是**非法**的——值为常量且重复，
// 编译器报 "duplicate case SeverityFlagNone (constant "0") in expression switch"，
// 而报错点在写 case 的那一行、症状却是"service 包整体编译不过"，不易一眼看出是两个常量撞了字面值。
// 故这里退化为 if + 单个常量（取 SeverityNone），两者谁是"主"交给下面的测试守卫。
//
// 两侧"无"是否仍然同源（字面相等）由 ccmcc_qc_test.go 的 TestCcMccNoneLiteralsAgree 守住：
// 一旦有人改掉任一侧的字面值，索引对齐的判定口径就分家了，那里会立刻变红。
//
// 另一头的尺度：非法值（"X"/"c"/"CC"）**不**按"无"处理——它们到达本函数前应已被解析层
// 拒绝；真到这里说明调用路径绕过了解析层，此时保持沉默比猜一个语义更安全。
// 空串则相反，它是"没填"（缺失不是断言），按 ParseOtherDiagnoses 第 5 条的口径视作「无」。
func ccIsNoneSeverity(flag string) bool {
	return flag == "" || flag == SeverityNone
}

// ccFlagLabel 把已录标记（'M'/'C'）映射回严重程度名（"MCC"/"CC"）用于文案。
// 标记不是 C/M（含「无」与非法值）时返回空串。
func ccFlagLabel(flag string) string {
	switch flag {
	case SeverityFlagMCC:
		return SeverityMCC
	case SeverityFlagCC:
		return SeverityCC
	default:
		return ""
	}
}
