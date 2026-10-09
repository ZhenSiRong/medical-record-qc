package service

import (
	"strings"

	"mrs/internal/model"
)

// 本文件实现病历质量评分（0–100 分）与单项否决识别（任务16）。
// 评分规则（自定，需合理）：
//   - 基础分 100 分；
//   - 每命中 1 条 ERROR 级质控问题扣 15 分；
//   - 每命中 1 条 WARN 级质控问题扣 5 分；
//   - 最低 0 分。
// 等级口径（行业通用，对齐三级医院评审"甲级病历率≥90%、杜绝丙级"）：
//   - score >= 90 → 甲级(A)；
//   - 75 <= score <= 89 → 乙级(B)；
//   - score < 75 → 丙级(C)。
// 单项否决：命中严重缺陷（如姓名缺失、主要诊断编码缺失、出院早于入院）直接判丙级，
// 即使总分达标。

const (
	scoreBase    = 100
	errorDeduct  = 15
	warnDeduct   = 5
	gradeAThresh = 90
	gradeBThresh = 75
)

// vetoFieldRules 单项否决——按质控项字段名识别（字段为空命中该否决）。
// key=QcResult.FieldName，value=否决原因描述。
var vetoFieldRules = map[string]string{
	"PatientName":       "患者姓名缺失",
	"MainDiagnosisCode": "主要诊断编码缺失",
}

// vetoMsgKeywords 单项否决——按问题描述关键字识别（覆盖逻辑类严重缺陷）。
var vetoMsgKeywords = []string{
	"出院时间必须晚于入院时间", // 出院早于/等于入院
}

// DetectVeto 识别单项否决项，返回全部否决原因（可能多条）。
func DetectVeto(results []model.QcResult) []string {
	var reasons []string
	for _, r := range results {
		// 仅 ERROR 级才可能触发单项否决
		if r.ErrorLevel != "ERROR" {
			continue
		}
		if reason, ok := vetoFieldRules[r.FieldName]; ok && isBlankMsg(r.ErrorMsg) {
			reasons = append(reasons, reason+"（字段："+r.FieldName+"）")
			continue
		}
		for _, kw := range vetoMsgKeywords {
			if strings.Contains(r.ErrorMsg, kw) {
				reasons = append(reasons, kw)
				break
			}
		}
	}
	return reasons
}

// ScoreQc 依据质控结果计算分数、等级与单项否决原因。
// 返回值：score（0–100）、gradeCode（A/B/C）、gradeName（甲级/乙级/丙级）、vetoReasons。
func ScoreQc(results []model.QcResult) (int, string, string, []string) {
	errCount, warnCount := 0, 0
	for _, r := range results {
		switch r.ErrorLevel {
		case "ERROR":
			errCount++
		default:
			warnCount++
		}
	}

	score := scoreBase - errCount*errorDeduct - warnCount*warnDeduct
	if score < 0 {
		score = 0
	}

	vetos := DetectVeto(results)

	code, name := gradeOf(score)
	// 单项否决：直接判丙级
	if len(vetos) > 0 {
		code, name = model.GradeCodeC, model.GradeNameC
	}
	return score, code, name, vetos
}

// gradeOf 按分数映射等级。
func gradeOf(score int) (string, string) {
	switch {
	case score >= gradeAThresh:
		return model.GradeCodeA, model.GradeNameA
	case score >= gradeBThresh:
		return model.GradeCodeB, model.GradeNameB
	default:
		return model.GradeCodeC, model.GradeNameC
	}
}

// isBlankMsg 判断问题描述是否为"必填字段为空"类（用于单项否决的字段级匹配）。
func isBlankMsg(msg string) bool {
	return strings.Contains(msg, "必填字段为空") || strings.Contains(msg, "为空") || strings.Contains(msg, "缺失")
}
