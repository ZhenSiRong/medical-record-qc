package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/internal/service"
	"mrs/pkg/resp"
)

// FrontPageHandler 病案首页（节 8）：数据维护 → 质控 → 异常修正循环
type FrontPageHandler struct {
	DB *gorm.DB
}

func (h *FrontPageHandler) Page(c *gin.Context) {
	q := h.DB.Model(&model.MedFrontPage{})
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if kw := c.Query("keyword"); kw != "" {
		kw := like(kw)
		q = q.Where("patient_name ILIKE ? OR hospital_no ILIKE ?", kw, kw)
	}
	resp.OK(c, resp.Page[model.MedFrontPage](c, q))
}

// Get 首页数据 + 最近质控结果
func (h *FrontPageHandler) Get(c *gin.Context) {
	var fp model.MedFrontPage
	if err := h.DB.Where("record_id = ?", c.Param("recordId")).First(&fp).Error; err != nil {
		resp.NotFound(c, "该病案尚无首页数据")
		return
	}
	var results []model.QcResult
	h.DB.Where("record_id = ?", fp.RecordID).Order("id DESC").Find(&results)
	resp.OK(c, gin.H{"frontPage": fp, "qcResults": results})
}

// Save 首页数据维护（upsert）。修正异常后重新保存会回到待质控，形成质控闭环。
// 已上报的数据禁止修改。
func (h *FrontPageHandler) Save(c *gin.Context) {
	var req model.MedFrontPage
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	_, username, _ := middleware.CurrentUser(c)
	_ = username

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var rec model.MedRecord
		if err := tx.First(&rec, c.Param("recordId")).Error; err != nil {
			return errBiz("病案不存在")
		}
		var existing model.MedFrontPage
		has := tx.Where("record_id = ?", rec.ID).First(&existing).Error == nil
		if has {
			if existing.Status == model.PageReported {
				return errBiz("首页数据已上报，禁止修改")
			}
			req.ID = existing.ID
			req.RecordID = rec.ID
			req.Status = model.PageWaitQc
			return tx.Model(&existing).Omit("created_at").Updates(map[string]any{
				"patient_name": req.PatientName, "gender": req.Gender, "birth_date": req.BirthDate,
				"hospital_no": req.HospitalNo, "dept_name": req.DeptName, "doctor_name": req.DoctorName,
				"admit_time": req.AdmitTime, "discharge_time": req.DischargeTime, "actual_days": req.ActualDays,
				"admit_diagnosis": req.AdmitDiagnosis, "discharge_diagnosis": req.DischargeDiagnosis,
				"main_diagnosis_code": req.MainDiagnosisCode, "other_diagnosis_codes": req.OtherDiagnosisCodes,
				"main_operation_code": req.MainOperationCode, "other_operation_codes": req.OtherOperationCodes,
				"total_cost": req.TotalCost, "ext_json": req.ExtJSON, "status": model.PageWaitQc,
			}).Error
		}
		req.RecordID = rec.ID
		req.Status = model.PageWaitQc
		return tx.Create(&req).Error
	})
	if err != nil {
		replyBiz(c, err)
		return
	}
	resp.OK(c, gin.H{"msg": "已保存并进入待质控"})
}

// Qc 执行质控：跑规则引擎，逐字段落异常标记，状态置 QC_PASSED / QC_REJECTED
func (h *FrontPageHandler) Qc(c *gin.Context) {
	var fp model.MedFrontPage
	if err := h.DB.Where("record_id = ?", c.Param("recordId")).First(&fp).Error; err != nil {
		resp.NotFound(c, "该病案尚无首页数据，请先维护首页")
		return
	}
	results, err := service.RunQc(h.DB, &fp)
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, gin.H{"status": fp.Status, "results": results})
}

// MarkFixed 修正标记：异常修正后人工确认关闭该异常项
func (h *FrontPageHandler) MarkFixed(c *gin.Context) {
	_, username, _ := middleware.CurrentUser(c)
	now := ptrTime(time.Now())
	res := h.DB.Model(&model.QcResult{}).Where("id = ?", c.Param("id")).Updates(map[string]any{
		"fixed": true, "fixed_at": now, "fixed_by": username,
	})
	if res.Error != nil {
		resp.ServerErr(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		resp.NotFound(c, "质控结果不存在")
		return
	}
	resp.OK(c, nil)
}
