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
// 已上报的数据禁止修改；任务17：已归档/借出/封存的病案禁止修改并留痕变更历史。
func (h *FrontPageHandler) Save(c *gin.Context) {
	var req model.MedFrontPage
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	_, username, _ := middleware.CurrentUser(c)
	reason := changeReason(c)

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var rec model.MedRecord
		if err := tx.First(&rec, c.Param("recordId")).Error; err != nil {
			return errBiz("病案不存在")
		}
		// 任务17 归档锁定：已归档/借出/封存禁止修改首页数据。
		// 依据《电子病历应用管理规范（试行）》（国卫办医发〔2017〕8号）：归档后原则上不得修改。
		if isArchivedLocked(rec.Status) {
			return errBiz("病案当前状态为「" + recStatusName(rec.Status) + "」，依据《电子病历应用管理规范（试行）》归档后原则上不得修改；如确需修改请走变更审批流程")
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
			// 任务17 变更留痕：记录修改前后快照
			if err := recordRevision(tx, rec.ID, &existing, &req, username, reason); err != nil {
				return err
			}
			return tx.Model(&existing).Omit("created_at").Updates(map[string]any{
				"patient_name": req.PatientName, "gender": req.Gender, "birth_date": req.BirthDate,
				"hospital_no": req.HospitalNo, "dept_name": req.DeptName, "doctor_name": req.DoctorName,
				"admit_time": req.AdmitTime, "discharge_time": req.DischargeTime, "actual_days": req.ActualDays,
				"admit_diagnosis": req.AdmitDiagnosis, "discharge_diagnosis": req.DischargeDiagnosis,
				"main_diagnosis_code": req.MainDiagnosisCode, "other_diagnosis_codes": req.OtherDiagnosisCodes,
				// ADR-0002 §5（批次 B）其他诊断严重度标记串：与编码串等长对齐，
				// 必须显式列入白名单，否则前端传了也写不进库（同 D3 的坑）。
				"other_diagnosis_flags": req.OtherDiagnosisFlags,
				"main_operation_code":   req.MainOperationCode, "other_operation_codes": req.OtherOperationCodes,
				"total_cost": req.TotalCost, "ext_json": req.ExtJSON, "status": model.PageWaitQc,
				// 缺口 D3（2026-10-09）条件必填真字段：必须显式列入白名单，否则前端传了也写不进库。
				"external_cause_code": req.ExternalCauseCode, "external_cause_name": req.ExternalCauseName,
				"newborn_birth_weight":      req.NewbornBirthWeight,
				"newborn_gestational_weeks": req.NewbornGestationalWeeks,
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

// Qc 执行质控：跑规则引擎，逐字段落异常标记，状态置 QC_PASSED / QC_REJECTED；
// 同时返回质量评分与等级（任务16）。
func (h *FrontPageHandler) Qc(c *gin.Context) {
	var fp model.MedFrontPage
	if err := h.DB.Where("record_id = ?", c.Param("recordId")).First(&fp).Error; err != nil {
		resp.NotFound(c, "该病案尚无首页数据，请先维护首页")
		return
	}
	results, quality, err := service.RunQc(h.DB, &fp)
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, gin.H{"status": fp.Status, "results": results, "quality": quality})
}

// MarkFixed 修正标记：异常修正后人工确认关闭该异常项。
// 语义上等同于走完退回返修闭环，故同步置 handle_status=CLOSED（任务5）。
func (h *FrontPageHandler) MarkFixed(c *gin.Context) {
	_, username, _ := middleware.CurrentUser(c)
	now := ptrTime(time.Now())
	res := h.DB.Model(&model.QcResult{}).Where("id = ?", c.Param("id")).Updates(map[string]any{
		"fixed": true, "fixed_at": now, "fixed_by": username,
		"handle_status": model.HandleClosed,
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
