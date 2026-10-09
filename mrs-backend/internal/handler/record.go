package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
	"mrs/pkg/resp"
)

// RecordHandler 病案主档（节 2/3 入口）：登记、查询、详情
type RecordHandler struct {
	DB *gorm.DB
}

func (h *RecordHandler) Page(c *gin.Context) {
	q := h.DB.Model(&model.MedRecord{})
	if kw := c.Query("keyword"); kw != "" {
		kw := like(kw)
		q = q.Where("record_no ILIKE ? OR patient_name ILIKE ? OR hospital_no ILIKE ?", kw, kw, kw)
	}
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if d := c.Query("deptId"); d != "" {
		q = q.Where("dept_id = ?", d)
	}
	resp.OK(c, resp.Page[model.MedRecord](c, q))
}

// Create 病案登记（新住院病案入系统），状态置为未回收；病案号留空则自动生成
func (h *RecordHandler) Create(c *gin.Context) {
	var req model.MedRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.RecordNo == "" {
		req.RecordNo = "BA" + time.Now().Format("20060102150405")
	}
	req.Status = model.RecNotRecycled
	if err := h.DB.Create(&req).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, req)
}

// Detail 病案详情：主档 + 编目 + 首页数据聚合
func (h *RecordHandler) Detail(c *gin.Context) {
	var rec model.MedRecord
	if err := h.DB.First(&rec, c.Param("id")).Error; err != nil {
		resp.NotFound(c, "病案不存在")
		return
	}
	var catalogs []model.CatalogEntry
	h.DB.Where("record_id = ?", rec.ID).Order("catalog_type, sort_order").Find(&catalogs)
	var fp model.MedFrontPage
	h.DB.Where("record_id = ?", rec.ID).First(&fp)
	resp.OK(c, gin.H{"record": rec, "catalogs": catalogs, "frontPage": fp})
}

// Update 病案主档部分更新（未归档前可改；已归档/封存由归档模块管理）
func (h *RecordHandler) Update(c *gin.Context) {
	var req model.MedRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	res := h.DB.Model(&model.MedRecord{}).Where("id = ? AND status IN ?", c.Param("id"),
		[]string{model.RecNotRecycled, model.RecRecycled, model.RecCataloged}).Updates(&req)
	if res.Error != nil {
		resp.ServerErr(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		resp.Conflict(c, "病案不存在或当前状态不允许修改")
		return
	}
	resp.OK(c, nil)
}
