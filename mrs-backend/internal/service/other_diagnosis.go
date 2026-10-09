package service

import (
	"errors"
	"fmt"
	"strings"
)

// 本文件实现 ADR-0002 §5「其他诊断的主次标记」的解析与对齐层（批次 B）。
//
// 数据模型（ADR 锁定）：
//   - MedFrontPage.OtherDiagnosisCodes：逗号分隔的编码串（既有字段）
//   - MedFrontPage.OtherDiagnosisFlags：**按逗号位置等长对齐**的严重度标记串
//     取值 '0' 无 / 'C' CC / 'M' MCC
//
// 等长对齐是 ADR 自己承认的「隐式契约」，故解析层是唯一的对齐权威：
// 所有读写 otherDiagnosisCodes/otherDiagnosisFlags 的代码都应当经由此处，
// 不要在各处再写一次 strings.Split。
//
// 刻意只依赖两个字符串入参、不依赖结构体字段：一是让本层能被纯内存测试
// 完全覆盖（不连库、不建服务），二是 model.MedFrontPage 的新列由其他批次
// 并行添加，本层与字段解耦后互不阻塞。
//
// 硬约束（ADR §5）：OtherDiagnosisCodes 内不允许重复编码——重复会让索引
// 对齐失去意义（同一编码对应两个 flag，谁说了算？），故解析层直接拒绝。

// 严重度标记取值（ADR §5 锁定，勿扩展：新增取值会让历史数据语义漂移）。
const (
	SeverityFlagNone = "0" // 无严重度（既非 CC 也非 MCC）
	SeverityFlagCC   = "C" // CC：并发症/合并症
	SeverityFlagMCC  = "M" // MCC：主要并发症/合并症
)

// 严重度分层结果（SeverityLevel 的返回值，ADR §2「严重度分层预览」）。
const (
	SeverityLevelMCC  = "MCC"
	SeverityLevelCC   = "CC"
	SeverityLevelNone = "NONE"
)

// otherDiagnosisSeparator 其他诊断编码/标记的分隔符，与既有 otherDiagnosisCodes 口径一致。
const otherDiagnosisSeparator = ","

// 解析层错误。全部为哨兵错误，调用方用 errors.Is 判定即可区分处置，
// 同时错误文本包含出错位置便于定位脏数据。
var (
	// ErrOtherDiagnosisLengthMismatch：codes 与 flags 的槽位数不一致（错位风险的核心防线）。
	ErrOtherDiagnosisLengthMismatch = errors.New("其他诊断编码与严重度标记长度不等")
	// ErrOtherDiagnosisDuplicate：存在重复编码（去空格、转大写后判定），违反 ADR §5 硬约束。
	ErrOtherDiagnosisDuplicate = errors.New("其他诊断编码存在重复")
	// ErrOtherDiagnosisInvalidFlag：标记不是有效的严重度等级。
	ErrOtherDiagnosisInvalidFlag = errors.New("其他诊断严重度标记非法")
)

// OtherDiagnosisItem 一条其他诊断的解析结果。
type OtherDiagnosisItem struct {
	Code  string // 规范化后的编码：去首尾空格 + 转大写（与 drg_rules.go / qc_condition.go 的编码比较口径一致）
	Flag  string // 严重度标记："0" / "C" / "M"
	Index int    // **原始槽位序号**（从 0 开始）；空槽被丢弃后序号不重排，便于回溯到录入原文
}

// ParseOtherDiagnoses 解析并对齐其他诊断编码与严重度标记。
//
// 各种边界情形的行为（这些是「隐式契约」的落地点，改动前请先看 other_diagnosis_test.go）：
//
//  1. 编码规范化：逐项去首尾空格 + 转大写。故 "I10.x00" 与 " i10.X00 " 视为同一编码。
//     依据：drg_rules.go 的 hasDiagPrefix 与 qc_condition.go 的 LIKE 前缀匹配
//     都用 strings.ToUpper(strings.TrimSpace(...))，解析层必须同口径，否则
//     "字典里有、对齐判定说没有"这类分裂会出现。
//
//  2. 长度校验在**去空之前**做，且按「槽位数」而非「非空项数」比较：
//     codes 的 3 个槽位碰上 flags 的 2 个槽位 → ErrOtherDiagnosisLengthMismatch。
//     注意 ",,A" 是 3 个槽位（2 空 + 1 有值），不等于 1 个槽位——若先滤掉空项再对齐，
//     ",,A" + "C" 会静默地把 C 配给 A，这正是本函数要拦下的错位。
//
//  3. 空槽成对丢弃：codes 中某个槽位去空格后为空时，**连同其同序号的 flag 一起丢弃**，
//     不因为「有 flag 没编码」而报「逻辑矛盾」。依据：编码缺失时其严重度无从判定，
//     且 "" 与 "0" 在语义上都表示「无严重度」，强行报错只会逼调用方造一个假编码。
//     丢弃后 Index 不重排，保留原始槽位号。
//
//  4. 非法 flag（如 "X"、"CC"、"c"）→ ErrOtherDiagnosisInvalidFlag，**不**降级为 "0"。
//     依据：静默降级会把一个 CC/MCC 标记悄悄抹掉，让严重度分层偏向"无"，
//     风险方向是漏报而非多报——等长对齐的契约要求解析层宁可拒绝也不猜。
//     大小写不做宽容：写 'c' 的是错数据，不是"等价写法"。
//     注意与"空槽"的区别：空串是**缺失**（按 "0"，见第 5 条），"X" 是**错误断言**。
//     缺失没有声明任何东西，取默认值合理；错误断言必须报错，否则等于替用户猜。
//
//  5. flags 整体为空但 codes 非空 → 全部按 "0"（无严重度）处理，**不报错**；
//     个别空槽（如 "C,,M"）同样按 "0" 处理。
//     依据：这是本轮实测的历史数据形态（57 份首页 otherDiagnosisCodes 全空，
//     将来补录编码时 flags 列必然是空串）。向后兼容优先于严格性。
//     另一条依据：otherDiagnosisCodes 目前没有结构化录入界面
//     （rectify.js / api-store.js 都只是整串回传的自由文本），尾随逗号是真实存在的
//     脏数据形态，据此拒绝整份病案会挡住合法编码。
//
//  6. 两侧 trim 后均为空 → 返回 nil 切片 + nil 错误（"没有其他诊断"，不是错误）。
//
//  7. 重复编码（规范化后判定，如 "I10.x00, i10.X00 "）→ ErrOtherDiagnosisDuplicate。
//     多个重复会在错误里逐个列出编码与槽位，便于退回返修时定位。
//
//  8. 分隔符只认半角逗号：全角逗号 "，"、分号 ";" 不会被当作分隔符，
//     会被当成编码正文的一部分（转大写后成为 "，"/";"），由后续编码字典校验拦截。
//     本层不做字符集猜测，避免"一个字符判错就错位整串"。
//
// 已知残余风险（本层单方面修不掉，已在 ADR 复核建议中列明）：
// **编码槽为空而 flag 非空时，flag 随空槽丢弃**，因此若 codes 串尾部多了一个逗号
// 而 flags 串恰好同位置有 "C"/"M"，该标记会静默消失（分层偏低，风险方向是漏报）。
// 长度相等使本层无法区分"这个 C 属于被删掉的空编码"与"属于下一个编码"。
// 根治手段是改用结构化数组（或 JSON）承载其他诊断，见 ADR §5 的升级触发条件。
func ParseOtherDiagnoses(codes, flags string) ([]OtherDiagnosisItem, error) {
	codes = strings.TrimSpace(codes)
	flags = strings.TrimSpace(flags)

	if codes == "" && flags == "" {
		// 相邻无其他诊断：空切片而非错误。
		return nil, nil
	}
	if codes == "" {
		// 有标记却没有任何编码：标记无所依附，长度必然不等（0 槽 vs ≥1 槽）。
		return nil, fmt.Errorf("%w：其他诊断编码为空，但严重度标记为 %q", ErrOtherDiagnosisLengthMismatch, flags)
	}

	codeSlots := strings.Split(codes, otherDiagnosisSeparator)
	// 历史数据兼容（第 5 条）：整串为空 ⇒ 每个槽位都是"无严重度"。
	flagSlots := make([]string, len(codeSlots))
	if flags != "" {
		flagSlots = strings.Split(flags, otherDiagnosisSeparator)
	}
	if len(codeSlots) != len(flagSlots) {
		// 长度不等 ⇒ 无法建立索引对齐关系，直接拒绝（第 2 条）。
		return nil, fmt.Errorf("%w：编码 %d 项，标记 %d 项（codes=%q flags=%q）",
			ErrOtherDiagnosisLengthMismatch, len(codeSlots), len(flagSlots), codes, flags)
	}

	items := make([]OtherDiagnosisItem, 0, len(codeSlots))
	seen := make(map[string]int, len(codeSlots)) // 规范化编码 → 首次出现的槽位
	var duplicates []string

	for i, rawCode := range codeSlots {
		code := normalizeDiagnosisCode(rawCode)
		if code == "" {
			// 空槽：与其 flag 一起丢弃（第 3 条），Index 不重排。
			continue
		}
		if firstAt, ok := seen[code]; ok {
			duplicates = append(duplicates,
				fmt.Sprintf("%s（第 %d 项与第 %d 项）", code, firstAt+1, i+1))
			continue
		}
		seen[code] = i
		items = append(items, OtherDiagnosisItem{
			Code:  code,
			Flag:  normalizeFlagToken(flagSlots[i]),
			Index: i,
		})
	}

	if len(duplicates) > 0 {
		return nil, fmt.Errorf("%w：%s", ErrOtherDiagnosisDuplicate, strings.Join(duplicates, "、"))
	}

	// flag 校验放在最后：让「长度不等」「重复编码」这两个更根本的问题先报出来，
	// 否则错位场景下的 flag 报错会掩盖真正的病因（脏 flag 可能只是错位的副产品）。
	for _, it := range items {
		if !ValidSeverityFlag(it.Flag) {
			return nil, fmt.Errorf("%w：第 %d 项（编码 %s）标记为 %q，仅允许 %q / %q / %q",
				ErrOtherDiagnosisInvalidFlag, it.Index+1, it.Code, it.Flag,
				SeverityFlagNone, SeverityFlagCC, SeverityFlagMCC)
		}
	}

	return items, nil
}

// FormatOtherDiagnoses 是 ParseOtherDiagnoses 的反向操作，产出规范化的双串。
//
// 用于：写库前把录入原文规范化（" I10.x00 , e11.901 " → "I10.X00,E11.901" / "0,C"）；
// 或在质控提示里重建"应当长什么样"的对照串。
//
// 与 Parse 的关系：Parse(Format(items)) 与 items 在规范化后逐项相等（见
// TestOtherDiagnosisFormatParseRoundTrip）。Format ∘ Parse 是幂等的，但
// Parse(Format(x)) == 字面 x 只对已经规范化的输入成立（大小写会被改成大写）。
//
// 同样校验重复编码与非法 flag：反向函数不该成为绕过契约的后门。
func FormatOtherDiagnoses(items []OtherDiagnosisItem) (codes, flags string, err error) {
	codeParts := make([]string, 0, len(items))
	flagParts := make([]string, 0, len(items))
	seen := make(map[string]int, len(items))
	var duplicates []string

	for i, it := range items {
		code := normalizeDiagnosisCode(it.Code)
		if code == "" {
			return "", "", fmt.Errorf("%w：第 %d 项编码为空", ErrOtherDiagnosisInvalidFlag, i+1)
		}
		if firstAt, ok := seen[code]; ok {
			duplicates = append(duplicates,
				fmt.Sprintf("%s（第 %d 项与第 %d 项）", code, firstAt+1, i+1))
			continue
		}
		seen[code] = i

		flag := normalizeFlagToken(it.Flag)
		if !ValidSeverityFlag(flag) {
			return "", "", fmt.Errorf("%w：第 %d 项（编码 %s）标记为 %q，仅允许 %q / %q / %q",
				ErrOtherDiagnosisInvalidFlag, i+1, code, it.Flag,
				SeverityFlagNone, SeverityFlagCC, SeverityFlagMCC)
		}
		codeParts = append(codeParts, code)
		flagParts = append(flagParts, flag)
	}

	if len(duplicates) > 0 {
		return "", "", fmt.Errorf("%w：%s", ErrOtherDiagnosisDuplicate, strings.Join(duplicates, "、"))
	}

	return strings.Join(codeParts, otherDiagnosisSeparator),
		strings.Join(flagParts, otherDiagnosisSeparator), nil
}

// ValidSeverityFlag 判断标记是否为合法严重度取值（注意：不做大小写宽容，"c" 非法）。
func ValidSeverityFlag(flag string) bool {
	switch flag {
	case SeverityFlagNone, SeverityFlagCC, SeverityFlagMCC:
		return true
	default:
		return false
	}
}

// SeverityLevel 依据解析结果给出本例的严重度分层（ADR §2 的「严重度分层预览」）。
//
// 规则：有任一 M → "MCC"；否则有任一 C → "CC"；否则 "NONE"（含空列表）。
//
// 范围声明：这是**院内口径**的取最高级汇总，只回答"本例是否带 CC/MCC 标记"，
// 不含官方排除表逻辑（"主诊断为 X 类时 Y 不算 CC/MCC"），**不得作为结算依据**。
// 按 ADR §4 的强约束，任何对外接口在返回本结果时必须附带 disclaimer 字段。
func SeverityLevel(items []OtherDiagnosisItem) string {
	hasCC := false
	for _, it := range items {
		switch normalizeFlagToken(it.Flag) {
		case SeverityFlagMCC:
			return SeverityLevelMCC
		case SeverityFlagCC:
			hasCC = true
		}
	}
	if hasCC {
		return SeverityLevelCC
	}
	return SeverityLevelNone
}

// OtherDiagnosisCodes 取出规范化后的编码列表，保持索引对齐顺序。
// 供批次 C 的低靠（漏编）检测查 CC/MCC 表使用。
func OtherDiagnosisCodes(items []OtherDiagnosisItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Code)
	}
	return out
}

// normalizeDiagnosisCode 编码规范化：去首尾空格 + 转大写。
// 与 drg_rules.go 的编码比较、qc_condition.go 的 LIKE 前缀匹配保持同一口径。
func normalizeDiagnosisCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// normalizeFlagToken 标记规范化：去首尾空格，空串归一为"无严重度"。
//
// 空串按 "0" 处理（而不是报错）是与第 5 条同源的取舍：**缺失不是断言**。
// flag 串里的空槽只说明"没填"，没有任何"这里不是 CC/MCC"的声明，因此取系统
// 默认值（无严重度）比拒绝整份病案更合理；而 "X"/"c"/"CC" 是**明确写错的值**，
// 属于错误断言，必须报错（见第 4 条）。两者不是同一种数据质量问题。
//
// 不做大小写转换：大小写是错误断言，转大写会让 'c' 被静默接受。
func normalizeFlagToken(flag string) string {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return SeverityFlagNone
	}
	return flag
}
