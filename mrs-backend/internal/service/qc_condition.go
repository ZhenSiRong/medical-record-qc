package service

import (
	"fmt"
	"regexp"
	"strings"

	"mrs/internal/model"
)

// 本文件实现"条件必填"的条件表达式求值（任务4）。
// 依据《住院病案首页数据采集质量与接口标准（2020版）》：
// 部分数据元为"条件必填"，例如损伤中毒的外部原因、新生儿体重/孕周、手术日期等，
// 只有在满足特定条件（如主诊断为损伤中毒类 S/T 编码）时才要求填写。

// likeCondRe 匹配轻量条件写法①：`字段名 LIKE '前缀%'`（大小写不敏感）。
// 例：main_diagnosis_code LIKE 'S%'、main_diagnosis_code LIKE 'K35%'
var likeCondRe = regexp.MustCompile(`(?i)^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s+like\s+'([^']*)%'\s*$`)

// evalCondition 判断规则条件是否成立：成立 → 需要对目标字段执行校验；不成立 → 跳过该规则。
// 支持两种形式，不引入表达式引擎依赖（轻量字符串解析）：
//
//	① 编码前缀匹配：`main_diagnosis_code LIKE 'S%'`
//	② 复用 checkLogic 语义的逻辑表达式：`actual_days>0`、`discharge_time>admit_time`
//	   以及扩展谓词 `has_operation`（是否存在主要手术操作）。
//
// 未识别的表达式保守处理为"条件成立"（宁可多校验不漏校验），并打印提示便于排查。
func evalCondition(expr string, fp *model.MedFrontPage) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}

	// 形式①：字段 LIKE '前缀%'
	if m := likeCondRe.FindStringSubmatch(expr); m != nil {
		field, prefix := m[1], m[2]
		val := frontPageFieldString(fp, field)
		return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(val)), strings.ToUpper(prefix))
	}

	// 形式②：逻辑表达式（与 checkLogic 对齐）
	switch strings.ToLower(expr) {
	case "actual_days>0":
		return fp.ActualDays > 0
	case "discharge_time>admit_time":
		return fp.AdmitTime != nil && fp.DischargeTime != nil && fp.DischargeTime.After(*fp.AdmitTime)
	case "has_operation": // 已填写主要手术编码即视为"有手术"
		return strings.TrimSpace(fp.MainOperationCode) != ""
	case "no_operation":
		return strings.TrimSpace(fp.MainOperationCode) == ""
	}

	fmt.Printf("[qc] 未实现的条件表达式（按条件成立处理，仍执行校验）: %s\n", expr)
	return true
}

// frontPageFieldString 将条件表达式里的下划线字段名映射到首页数据取值。
// 仅覆盖条件必填常用字段；未覆盖的返回空串（前缀匹配自然不成立）。
func frontPageFieldString(fp *model.MedFrontPage, field string) string {
	switch strings.ToLower(field) {
	case "main_diagnosis_code":
		return fp.MainDiagnosisCode
	case "main_operation_code":
		return fp.MainOperationCode
	case "other_diagnosis_codes":
		return fp.OtherDiagnosisCodes
	case "other_operation_codes":
		return fp.OtherOperationCodes
	case "admit_diagnosis":
		return fp.AdmitDiagnosis
	case "discharge_diagnosis":
		return fp.DischargeDiagnosis
	case "patient_name":
		return fp.PatientName
	case "gender":
		return fp.Gender
	case "dept_name":
		return fp.DeptName
	case "doctor_name":
		return fp.DoctorName
	case "hospital_no":
		return fp.HospitalNo
	case "ext_json":
		return fp.ExtJSON
	default:
		return ""
	}
}
