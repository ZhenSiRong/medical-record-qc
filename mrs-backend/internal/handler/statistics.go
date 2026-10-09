package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/cache"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

const minute = time.Minute

// StatsHandler 统计分析（节 7）。统计口径为骨架默认值，待调研阶段三输出后调整。
type StatsHandler struct {
	DB *gorm.DB
	Ca *cache.Cache
}

type kv struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// Overview 病案各状态分布（Redis 缓存 5 分钟）
func (h *StatsHandler) Overview(c *gin.Context) {
	const key = "stats:overview"
	var out []kv
	if h.Ca.GetJSON(key, &out) {
		resp.OK(c, out)
		return
	}
	rows, err := h.DB.Model(&model.MedRecord{}).
		Select("status AS key, count(*) AS count").Group("status").Rows()
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var r kv
		_ = h.DB.ScanRows(rows, &r)
		out = append(out, r)
	}
	h.Ca.SetJSON(key, out, 5*minute)
	resp.OK(c, out)
}

// ArchiveRate 归档率：口径=（已归档在架+借出）/ 全部病案（待调研确认口径）
func (h *StatsHandler) ArchiveRate(c *gin.Context) {
	var total, archived int64
	h.DB.Model(&model.MedRecord{}).Count(&total)
	h.DB.Model(&model.MedRecord{}).
		Where("status IN ?", []string{model.RecArchived, model.RecBorrowed}).
		Count(&archived)
	rate := 0.0
	if total > 0 {
		rate = float64(archived) / float64(total)
	}
	resp.OK(c, gin.H{"total": total, "archived": archived, "rate": rate})
}

// DiseaseTop 疾病谱 Top10：编目诊断计数（Redis 缓存 10 分钟）
func (h *StatsHandler) DiseaseTop(c *gin.Context) {
	const key = "stats:disease-top"
	var out []kv
	if h.Ca.GetJSON(key, &out) {
		resp.OK(c, out)
		return
	}
	rows, err := h.DB.Model(&model.CatalogEntry{}).
		Select("icd_code || ' ' || coalesce(icd_name,'') AS key, count(*) AS count").
		Where("catalog_type = ?", model.CatalogDiagnosis).
		Group("icd_code, icd_name").Order("count DESC").Limit(10).Rows()
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var r kv
		_ = h.DB.ScanRows(rows, &r)
		out = append(out, r)
	}
	h.Ca.SetJSON(key, out, 10*minute)
	resp.OK(c, out)
}

// BorrowMonthly 近 12 个月借出量趋势
func (h *StatsHandler) BorrowMonthly(c *gin.Context) {
	const key = "stats:borrow-monthly"
	var out []kv
	if h.Ca.GetJSON(key, &out) {
		resp.OK(c, out)
		return
	}
	rows, err := h.DB.Model(&model.BorrowOrder{}).
		Select("to_char(lend_time, 'YYYY-MM') AS key, count(*) AS count").
		Where("lend_time IS NOT NULL").Group("key").
		Order("key DESC").Limit(12).Rows()
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var r kv
		_ = h.DB.ScanRows(rows, &r)
		out = append(out, r)
	}
	h.Ca.SetJSON(key, out, 10*minute)
	resp.OK(c, out)
}

// Export 报表导出（节 7）：格式与口径待调研输出后实现（Excel/CSV）
func (h *StatsHandler) Export(c *gin.Context) {
	resp.Fail(c, 501, "导出功能待统计口径确认后实现（调研阶段三交付物）")
}
