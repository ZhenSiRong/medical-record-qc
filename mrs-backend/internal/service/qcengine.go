package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"

	"mrs/internal/model"
)

// RunQc 质控规则引擎（调研节 8）：必填校验、逻辑校验、编码存在性校验。
// 结果逐字段落 front_page_qc_result（先清空该病案旧结果再写入），
// ERROR 命中 → QC_REJECTED，仅 WARN → QC_PASSED。
// 规则清单由 qc_rule 表配置（来源：调研阶段三交付物），不硬编码。
func RunQc(db *gorm.DB, fp *model.MedFrontPage) ([]model.QcResult, error) {
	var rules []model.QcRule
	if err := db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return nil, err
	}

	v := reflect.ValueOf(*fp)
	var results []model.QcResult
	errCount := 0
	for _, rule := range rules {
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
				return nil, err
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
			ErrorLevel: rule.ErrorLevel, ErrorMsg: msg,
		})
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("record_id = ?", fp.RecordID).Delete(&model.QcResult{}).Error; err != nil {
			return err
		}
		if len(results) > 0 {
			if err := tx.Create(&results).Error; err != nil {
				return err
			}
		}
		status := model.PageQcPassed
		if errCount > 0 {
			status = model.PageQcRejected
		}
		fp.Status = status
		return tx.Model(fp).Update("status", status).Error
	})
	if err != nil {
		return nil, err
	}
	return results, nil
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
