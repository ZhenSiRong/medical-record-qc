package handler

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
	"mrs/pkg/resp"
)

// =====================================================================
// 任务 8：上报数据导出（符合国家接口标准的 CSV）
//
// 背景：真实上报需通过官方通道（HQMS/NCIS/医保结算清单），而官方接口文档本项目
// 无法获取（调研文档明确「禁止以口头需求作为设计依据」）。因此本模块做到
// 「符合接口标准的文件导出 + 导出前校验」为止，官方通道本身仍留防腐层。
//
// 依据：《住院病案首页数据采集质量与接口标准》的文件命名与「无数据填 -」要求。
// 重要：下列字段集为骨架版核心 18 列；《标准》完整采集项 90+，其准确字段编号
// 与完整字段集以官方最新标准为准，后续对齐官方文档后应替换。
// =====================================================================
type ReportExportHandler struct{ DB *gorm.DB }

// exportHeader 导出列名（英文采集项编号，骨架版）。
// 列含义对照：
//
//	P900 医疗机构代码  P1 住院号      P3 病案号        P4 患者姓名
//	P5   性别代码      P6 出生日期    P22 入院日期     P23 入院科别
//	P25  出院日期      P26 出院科别   P27 实际住院天数 P30 入院诊断
//	P31  出院诊断      P32 主要诊断编码(ICD-10)        P33 主要诊断名称
//	P36  主要手术操作编码(ICD-9-CM3)  P37 主要手术操作名称  P40 总费用
var exportHeader = []string{
	"P900", "P1", "P3", "P4", "P5", "P6", "P22", "P23", "P25", "P26",
	"P27", "P30", "P31", "P32", "P33", "P36", "P37", "P40",
}

// Export GET /api/report-batches/:id/export（ADMIN）
// 生成该批次内所有病案首页的 CSV 文件（UTF-8 BOM，Excel 可直接打开）。
// 文件命名以 hqmsts_ 开头（对齐接口标准要求），如 hqmsts_202609.csv。
func (h *ReportExportHandler) Export(c *gin.Context) {
	var batch model.ReportBatch
	if err := h.DB.First(&batch, c.Param("id")).Error; err != nil {
		resp.NotFound(c, "批次不存在")
		return
	}
	var items []model.ReportItem
	if err := h.DB.Where("batch_id = ?", batch.ID).Order("id ASC").Find(&items).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}

	buf := &bytes.Buffer{}
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM：便于 Excel 正确识别中文
	w := csv.NewWriter(buf)
	_ = w.Write(exportHeader)

	for _, it := range items {
		var fp model.MedFrontPage
		if err := h.DB.Where("record_id = ?", it.RecordID).First(&fp).Error; err != nil {
			continue // 首页缺失的明细跳过（导出前检查会提示）
		}
		var rec model.MedRecord
		_ = h.DB.First(&rec, fp.RecordID).Error

		row := []string{
			"-", // P900 医疗机构代码（机构级配置，骨架未存储，按标准无数据填 -）
			cell(fp.HospitalNo),
			cell(rec.RecordNo),
			cell(fp.PatientName),
			cell(genderCode(fp.Gender)),
			cell(dateStr(fp.BirthDate)),
			cell(dateStr(fp.AdmitTime)),
			cell(fp.DeptName),
			cell(dateStr(fp.DischargeTime)),
			cell(fp.DeptName),
			cell(daysStr(fp.ActualDays)),
			cell(fp.AdmitDiagnosis),
			cell(fp.DischargeDiagnosis),
			cell(fp.MainDiagnosisCode),
			cell(icdName(h.DB, model.Icd10, fp.MainDiagnosisCode)),
			cell(fp.MainOperationCode),
			cell(icdName(h.DB, model.Icd9Cm3, fp.MainOperationCode)),
			cell(costStr(fp.TotalCost)),
		}
		_ = w.Write(row)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		resp.ServerErr(c, err)
		return
	}

	filename := fmt.Sprintf("hqmsts_%s.csv", strings.ReplaceAll(batch.Period, "-", ""))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

type exportProblem struct {
	RecordID    int64    `json:"recordId"`
	RecordNo    string   `json:"recordNo"`
	PatientName string   `json:"patientName"`
	Issues      []string `json:"issues"`
}

// ExportCheck GET /api/report-batches/:id/export-check（登录）
// 导出前检查：扫描批次内病案首页的必填字段完整性问题，提示可能被平台退回。
func (h *ReportExportHandler) ExportCheck(c *gin.Context) {
	var batch model.ReportBatch
	if err := h.DB.First(&batch, c.Param("id")).Error; err != nil {
		resp.NotFound(c, "批次不存在")
		return
	}
	var items []model.ReportItem
	h.DB.Where("batch_id = ?", batch.ID).Order("id ASC").Find(&items)

	problems := make([]exportProblem, 0)
	problemCount := 0
	for _, it := range items {
		var fp model.MedFrontPage
		if err := h.DB.Where("record_id = ?", it.RecordID).First(&fp).Error; err != nil {
			problems = append(problems, exportProblem{RecordID: it.RecordID, Issues: []string{"首页数据缺失，无法导出"}})
			problemCount++
			continue
		}
		var rec model.MedRecord
		_ = h.DB.First(&rec, fp.RecordID).Error

		var issues []string
		if strings.TrimSpace(fp.PatientName) == "" {
			issues = append(issues, "患者姓名(P4)为空")
		}
		if strings.TrimSpace(fp.Gender) == "" {
			issues = append(issues, "性别(P5)为空")
		}
		if strings.TrimSpace(fp.HospitalNo) == "" {
			issues = append(issues, "住院号(P1)为空")
		}
		if strings.TrimSpace(fp.DeptName) == "" {
			issues = append(issues, "出院科别(P26)为空")
		}
		if fp.AdmitTime == nil {
			issues = append(issues, "入院日期(P22)为空")
		}
		if fp.DischargeTime == nil {
			issues = append(issues, "出院日期(P25)为空")
		}
		if fp.ActualDays <= 0 {
			issues = append(issues, "实际住院天数(P27)无效")
		}
		if strings.TrimSpace(fp.MainDiagnosisCode) == "" {
			issues = append(issues, "主要诊断编码(P32)为空")
		}
		if fp.TotalCost <= 0 {
			issues = append(issues, "总费用(P40)为空或为 0")
		}
		if len(issues) > 0 {
			problemCount += len(issues)
			problems = append(problems, exportProblem{
				RecordID: fp.RecordID, RecordNo: rec.RecordNo, PatientName: fp.PatientName, Issues: issues,
			})
		}
	}

	message := "字段完整，可正常导出"
	if problemCount > 0 {
		message = fmt.Sprintf("存在 %d 项问题，导出后可能被平台退回", problemCount)
	}
	resp.OK(c, gin.H{
		"batchId": batch.ID, "batchNo": batch.BatchNo, "period": batch.Period,
		"total": len(items), "problemItems": len(problems), "problemCount": problemCount,
		"items": problems, "message": message,
	})
}

// ---------- 导出字段辅助函数 ----------

// cell 无数据一律填 "-"（符合接口标准要求）。
func cell(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	return s
}

func dateStr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// genderCode 性别编码按国标：1=男、2=女。
func genderCode(g string) string {
	switch g {
	case "男", "1":
		return "1"
	case "女", "2":
		return "2"
	}
	return ""
}

func daysStr(d int) string {
	if d <= 0 {
		return ""
	}
	return strconv.Itoa(d)
}

func costStr(f float64) string {
	if f <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", f)
}

// icdName 依据字典类型与编码从 icd_dict 反查编码名称。
func icdName(db *gorm.DB, dictType, code string) string {
	if code == "" {
		return ""
	}
	var d model.IcdDict
	db.Where("dict_type = ? AND code = ?", dictType, code).First(&d)
	return d.Name
}
