package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/cache"
	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

// IcdHandler 编码库（节 4）：检索（Redis 缓存）、分页、整版导入
type IcdHandler struct {
	DB *gorm.DB
	Ca *cache.Cache
}

// Search 编码检索：编目/首页录入时关键字选码（热数据 Redis 10 分钟）
func (h *IcdHandler) Search(c *gin.Context) {
	dictType := c.DefaultQuery("type", model.Icd10)
	kw := c.Query("keyword")
	type hit struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	var hits []hit
	cacheKey := "icd:search:" + dictType + ":" + kw
	if h.Ca.GetJSON(cacheKey, &hits) {
		resp.OK(c, hits)
		return
	}
	q := h.DB.Model(&model.IcdDict{}).Where("dict_type = ? AND enabled = ?", dictType, true)
	if kw != "" {
		kw := like(kw)
		q = q.Where("code ILIKE ? OR name ILIKE ?", kw, kw)
	}
	if err := q.Limit(20).Find(&hits).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	h.Ca.SetJSON(cacheKey, hits, 10*time.Minute)
	resp.OK(c, hits)
}

func (h *IcdHandler) Page(c *gin.Context) {
	q := h.DB.Model(&model.IcdDict{}).Where("enabled = ?", true)
	if t := c.Query("type"); t != "" {
		q = q.Where("dict_type = ?", t)
	}
	if kw := c.Query("keyword"); kw != "" {
		kw := like(kw)
		q = q.Where("code ILIKE ? OR name ILIKE ?", kw, kw)
	}
	resp.OK(c, resp.Page[model.IcdDict](c, q))
}

type importReq struct {
	DictType string     `json:"dictType" binding:"required"`
	Version  string     `json:"version" binding:"required"`
	Entries  []icdEntry `json:"entries" binding:"required,min=1"`
}
type icdEntry struct {
	Code string `json:"code" binding:"required"`
	Name string `json:"name" binding:"required"`
}

// Import 编码库整版导入（调研节 4：编码库更新维护）。TODO 官方码表文件格式确认后
// 改为解析 CSV/Excel 文件上传；当前骨架接收 JSON 列表。
func (h *IcdHandler) Import(c *gin.Context) {
	var req importReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	_, username, _ := middleware.CurrentUser(c)

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		ver := model.IcdDictVersion{
			Version: req.Version, DictType: req.DictType,
			ImporterName: username, TotalCount: len(req.Entries), Status: "ACTIVE",
		}
		if err := tx.Create(&ver).Error; err != nil {
			return err
		}
		// 旧版本转历史，不物理删除（编目记录需可追溯）
		if err := tx.Model(&model.IcdDictVersion{}).
			Where("dict_type = ? AND id <> ?", req.DictType, ver.ID).
			Update("status", "HISTORY").Error; err != nil {
			return err
		}
		rows := make([]model.IcdDict, 0, len(req.Entries))
		for _, e := range req.Entries {
			rows = append(rows, model.IcdDict{
				DictType: req.DictType, Code: e.Code, Name: e.Name, VersionID: ver.ID, Enabled: true,
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, gin.H{"count": len(req.Entries), "version": req.Version})
}
