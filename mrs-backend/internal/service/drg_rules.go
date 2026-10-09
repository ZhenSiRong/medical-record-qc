package service

import (
	"fmt"
	"strings"

	"mrs/internal/model"
)

// 本文件为 DRG/DIP 付费改革相关的病案首页质控规则数据与判定逻辑（任务6）。
// 政策依据：国家医保局《关于印发按病组和病种分值付费2.0版分组方案并深入推进相关工作的通知》
// 及医保办发〔2024〕9号相关要求——主要诊断与主要手术操作不匹配会落入 QY 歧义组，
// 导致医保按 50% 折付，需在病案首页质控阶段予以提示。

// DrgMatchRule 主诊断→主手术匹配简表的一行。
// DiagPrefix 为主诊断编码前缀；OpPrefixes 为该诊断下合理的（主要）手术编码前缀集合。
// AllowNoOp 表示该诊断本就允许无手术操作（如顺产）。
type DrgMatchRule struct {
	DiagPrefix string   `json:"diagPrefix"`
	DiagName   string   `json:"diagName"`
	OpPrefixes []string `json:"opPrefixes"`
	AllowNoOp  bool     `json:"allowNoOp"`
	Note       string   `json:"note"`
}

// DrgMatchTable DRG 主诊断-主手术匹配简表（可扩展：后续按院内病种结构补充）。
// 编码前缀取 ICD-10（诊断）/ ICD-9-CM3（手术）大类前缀。
var DrgMatchTable = []DrgMatchRule{
	{DiagPrefix: "K35", DiagName: "急性阑尾炎", OpPrefixes: []string{"47.0"}, Note: "阑尾炎通常实施阑尾切除术"},
	{DiagPrefix: "K80", DiagName: "胆囊结石", OpPrefixes: []string{"51.2"}, Note: "胆囊结石通常实施胆囊切除术"},
	{DiagPrefix: "K81", DiagName: "胆囊炎", OpPrefixes: []string{"51.2"}, Note: "胆囊炎通常实施胆囊切除术"},
	{DiagPrefix: "S72", DiagName: "股骨骨折", OpPrefixes: []string{"79.", "81.5"}, Note: "股骨骨折常行内固定或关节置换术"},
	{DiagPrefix: "S82", DiagName: "小腿（含踝）骨折", OpPrefixes: []string{"79.", "81.5"}, Note: "骨折常行内固定术"},
	{DiagPrefix: "N20", DiagName: "肾结石", OpPrefixes: []string{"55.5", "98.5"}, Note: "肾结石常行取石/碎石术"},
	{DiagPrefix: "N40", DiagName: "前列腺增生", OpPrefixes: []string{"60.2"}, Note: "前列腺增生常行经尿道前列腺电切术"},
	{DiagPrefix: "I25", DiagName: "冠心病", OpPrefixes: []string{"36.0"}, Note: "冠心病常行支架植入/搭桥术"},
	{DiagPrefix: "C34", DiagName: "支气管肺癌", OpPrefixes: []string{"32.4", "32.3"}, Note: "肺癌常行肺叶切除等手术"},
	{DiagPrefix: "O82", DiagName: "剖宫产分娩", OpPrefixes: []string{"74.1"}, Note: "剖宫产分娩应实施剖宫产术"},
	{DiagPrefix: "O80", DiagName: "顺产", AllowNoOp: true, OpPrefixes: []string{"73.5", "74.1"}, Note: "顺产通常无手术操作，允许无手术"},
}

// checkDiagOpMatch 校验主诊断与主要手术操作是否匹配（规则类型 DIAG_OP_MATCH）。
// 未命中匹配表时返回空串（不产生提示），避免误报。
func checkDiagOpMatch(fp *model.MedFrontPage) string {
	diag := strings.ToUpper(strings.TrimSpace(fp.MainDiagnosisCode))
	op := strings.TrimSpace(fp.MainOperationCode)
	if diag == "" {
		return "" // 主诊断缺失由必填规则处理，这里不重复报
	}
	for _, r := range DrgMatchTable {
		if !strings.HasPrefix(diag, strings.ToUpper(r.DiagPrefix)) {
			continue
		}
		if op == "" {
			if r.AllowNoOp {
				return ""
			}
			return fmt.Sprintf("主诊断【%s %s】通常需实施手术（%s），但主要手术编码为空，存在落入 DRG 歧义组(QY)按 50%% 折付的风险，请核对是否漏填主要手术操作",
				r.DiagPrefix, r.DiagName, strings.Join(r.OpPrefixes, "/"))
		}
		if !matchAnyPrefix(op, r.OpPrefixes) {
			return fmt.Sprintf("主诊断【%s %s】与主要手术编码【%s】不匹配（合理手术前缀：%s），依据医保办发〔2024〕9号可能落入 DRG 歧义组(QY)导致 50%% 折付，请临床核对主诊断/主手术选择",
				r.DiagPrefix, r.DiagName, op, strings.Join(r.OpPrefixes, "/"))
		}
		return ""
	}
	return ""
}

func matchAnyPrefix(code string, prefixes []string) bool {
	c := strings.ToUpper(strings.TrimSpace(code))
	for _, p := range prefixes {
		if strings.HasPrefix(c, strings.ToUpper(p)) {
			return true
		}
	}
	return false
}

// ---------- MAIN_DIAG_RULE：主要诊断选择合理性提示（WARN 级，不拦截） ----------

// chronicDiagPrefixes 慢性病主诊断编码前缀：本次住院若实施手术操作，主诊断可能应为手术相关诊断。
var chronicDiagPrefixes = []string{"I10", "E11", "E05", "I25", "J44", "N40", "I50", "K25"}

// sequelaDiagPrefixes 后遗症/恢复期类编码前缀：需核对其是否适合作为本次住院主要诊断。
// 依据《住院病案首页数据填写质量规范》主要诊断选择原则：本次住院治疗的主要疾病、且消耗医疗资源最多者。
var sequelaDiagPrefixes = []string{"T90", "T91", "T92", "T93", "T94", "T95", "T96", "T97", "T98", "I69", "Z50", "Z42"}

// checkMainDiag 主要诊断选择合理性提示（规则类型 MAIN_DIAG_RULE，WARN 级）。
func checkMainDiag(fp *model.MedFrontPage) string {
	diag := strings.ToUpper(strings.TrimSpace(fp.MainDiagnosisCode))
	op := strings.TrimSpace(fp.MainOperationCode)
	if diag == "" {
		return ""
	}
	for _, p := range sequelaDiagPrefixes {
		if strings.HasPrefix(diag, p) {
			return fmt.Sprintf("主要诊断编码【%s】疑似后遗症/恢复期类编码，请核对是否符合主要诊断选择原则（宜以本次住院治疗的主要疾病作为主要诊断）", fp.MainDiagnosisCode)
		}
	}
	if op != "" {
		for _, p := range chronicDiagPrefixes {
			if strings.HasPrefix(diag, p) {
				return fmt.Sprintf("主要诊断为慢性病相关编码【%s】，但本次住院实施了手术操作【%s】，请核对是否应选择与手术相关的疾病作为主要诊断", fp.MainDiagnosisCode, op)
			}
		}
	}
	return ""
}
