package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

// CatalogHandler 病案编目（节 4）：诊断 ICD-10 / 手术 ICD-9-CM3 编目
type CatalogHandler struct {
	DB *gorm.DB
}

func (h *CatalogHandler) List(c *gin.Context) {
	q := h.DB.Model(&model.CatalogEntry{}).Where("record_id = ?", c.Query("recordId"))
	var list []model.CatalogEntry
	if err := q.Order("catalog_type, sort_order, id").Find(&list).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, list)
}

// Create 新增编目：编码必须存在于 ICD 字典；首条编目驱动病案 RECYCLED → CATALOGED
func (h *CatalogHandler) Create(c *gin.Context) {
	var req model.CatalogEntry
	if err := c.ShouldBindJSON(&req); err != nil || req.RecordID == 0 || req.IcdCode == "" {
		resp.BadRequest(c, "请传 recordId、catalogType、icdCode")
		return
	}
	_, username, _ := middleware.CurrentUser(c)
	req.CoderName = username

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// 归档锁定（任务 17，依据《电子病历应用管理规范》国卫办医发〔2017〕8号）：
		// 已归档/借出/封存的病案禁止再编目，避免归档后数据被改动
		var rec model.MedRecord
		if err := tx.First(&rec, req.RecordID).Error; err != nil {
			return errBiz("病案不存在")
		}
		if isArchivedLocked(rec.Status) {
			return errBiz("该病案已归档，编目已锁定（如需修正请走变更审批并留痕）")
		}
		// 编目类型 → 字典类型映射，编码存在性校验与质控 CODE_EXIST 同口径
		dictType := model.Icd10
		switch req.CatalogType {
		case model.CatalogDiagnosis:
			dictType = model.Icd10
		case model.CatalogOperation:
			dictType = model.Icd9Cm3
		default:
			return errBiz("catalogType 必须为 DIAGNOSIS 或 OPERATION")
		}
		var cnt int64
		if err := tx.Model(&model.IcdDict{}).
			Where("dict_type = ? AND code = ? AND enabled = ?", dictType, req.IcdCode, true).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			return errBiz("ICD 编码在字典中不存在，请从编码库检索选择")
		}
		if err := tx.Create(&req).Error; err != nil {
			return err
		}
		// 首条编目驱动状态流转（条件更新，天然幂等）
		return tx.Model(&model.MedRecord{}).
			Where("id = ? AND status = ?", req.RecordID, model.RecRecycled).
			Updates(map[string]any{"status": model.RecCataloged, "cataloged_at": time.Now()}).Error
	})
	if err != nil {
		if be, ok := err.(bizErr); ok {
			resp.Conflict(c, be.msg)
			return
		}
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, req)
}

func (h *CatalogHandler) Delete(c *gin.Context) {
	// 归档锁定（任务 17）：已归档/借出/封存病案的编目不可删除
	var entry model.CatalogEntry
	if err := h.DB.First(&entry, c.Param("id")).Error; err != nil {
		resp.NotFound(c, "编目记录不存在")
		return
	}
	var rec model.MedRecord
	if err := h.DB.First(&rec, entry.RecordID).Error; err == nil && isArchivedLocked(rec.Status) {
		resp.Conflict(c, "该病案已归档，编目已锁定，不可删除")
		return
	}
	if err := h.DB.Delete(&model.CatalogEntry{}, c.Param("id")).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, nil)
}

// bizErr 轻量业务错误：事务里返回、外层转 409
type bizErr struct{ msg string }

func (e bizErr) Error() string { return e.msg }

func errBiz(msg string) error { return bizErr{msg: msg} }
