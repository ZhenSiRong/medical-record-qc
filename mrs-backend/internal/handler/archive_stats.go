package handler

import (
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
	"mrs/pkg/resp"
)

// =====================================================================
// 任务 23：归档及时率统计与超期催缴清单
//
// 背景：依据《病案管理质量控制指标（2021 版）》，出院病案 2 日归档率、归档及时率
// （≥90%）均为硬考核指标；而「催办低效」（每天电话催 50 次）是调研中最扎心的痛点。
// 本模块提供指标统计与「超期未归档催缴清单」，供病案科按科室责任制催办。
//
// 统计口径（重要）：
//
//	分母 = 已出院病案总数（discharge_time IS NOT NULL）
//	分子 = 其中已归档（archived_at IS NOT NULL）且归档耗时 ≤ N 天的病案
//	归档耗时 = archived_at - discharge_time（天）；未归档记录计入分母、不计入分子
//
// 只读实现，不新增表。
// =====================================================================
type ArchiveStatsHandler struct{ DB *gorm.DB }

// archiveTarget 归档及时率目标值（依据病案管理质量控制指标 2021 版，≥90%）。
const archiveTarget = 0.90

type archiveOverall struct {
	Total    int64   `json:"total"`    // 已出院病案总数（分母）
	Archived int64   `json:"archived"` // 已归档数
	Within2  int64   `json:"within2"`  // 2 日内归档数（分子）
	Within3  int64   `json:"within3"`
	Rate2    float64 `json:"rate2"`   // 2 日归档率
	Rate3    float64 `json:"rate3"`   // 3 日归档率
	AvgDays  float64 `json:"avgDays"` // 平均归档天数（仅已归档）
	Target   float64 `json:"target"`  // 目标值 0.90
	Met      bool    `json:"met"`     // 2 日归档率是否达标
}

type archiveMonthRow struct {
	Month   string  `json:"month"`
	Total   int64   `json:"total"`
	Within2 int64   `json:"within2"`
	Within3 int64   `json:"within3"`
	Rate2   float64 `json:"rate2"`
	Rate3   float64 `json:"rate3"`
}

type archiveDeptRow struct {
	DeptName string  `json:"deptName"`
	Total    int64   `json:"total"`
	Archived int64   `json:"archived"`
	Within2  int64   `json:"within2"`
	Rate2    float64 `json:"rate2"`
	AvgDays  float64 `json:"avgDays"`
}

type archiveTimeliness struct {
	Overall archiveOverall    `json:"overall"`
	Trend   []archiveMonthRow `json:"trend"`
	ByDept  []archiveDeptRow  `json:"byDept"`
}

// Timeliness GET /api/archive-timeliness?months=6（登录）
func (h *ArchiveStatsHandler) Timeliness(c *gin.Context) {
	months, _ := strconv.Atoi(c.DefaultQuery("months", "6"))
	if months < 1 || months > 36 {
		months = 6
	}

	// 总体指标（全量口径：所有已出院病案）
	agg := struct {
		Total    int64
		Archived int64
		Within2  int64
		Within3  int64
		SumDays  float64
	}{}
	err := h.DB.Raw(`
SELECT count(*) AS total,
       count(*) FILTER (WHERE archived_at IS NOT NULL) AS archived,
       count(*) FILTER (WHERE archived_at IS NOT NULL AND archived_at - discharge_time <= interval '2 days') AS within2,
       count(*) FILTER (WHERE archived_at IS NOT NULL AND archived_at - discharge_time <= interval '3 days') AS within3,
       coalesce(sum(extract(epoch FROM (archived_at - discharge_time)) / 86400)
                FILTER (WHERE archived_at IS NOT NULL), 0) AS sum_days
FROM med_record WHERE discharge_time IS NOT NULL`).Scan(&agg).Error
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	overall := archiveOverall{
		Total: agg.Total, Archived: agg.Archived, Within2: agg.Within2, Within3: agg.Within3,
		Target: archiveTarget,
	}
	overall.Rate2 = ratio(agg.Within2, agg.Total)
	overall.Rate3 = ratio(agg.Within3, agg.Total)
	if agg.Archived > 0 {
		overall.AvgDays = round2(agg.SumDays / float64(agg.Archived))
	}
	overall.Met = overall.Rate2 >= archiveTarget

	// 近 N 个月趋势（按出院月份分组）
	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -(months - 1), 0)
	var trend []archiveMonthRow
	if err := h.DB.Raw(`
SELECT to_char(discharge_time, 'YYYY-MM') AS month,
       count(*) AS total,
       count(*) FILTER (WHERE archived_at IS NOT NULL AND archived_at - discharge_time <= interval '2 days') AS within2,
       count(*) FILTER (WHERE archived_at IS NOT NULL AND archived_at - discharge_time <= interval '3 days') AS within3
FROM med_record
WHERE discharge_time IS NOT NULL AND discharge_time >= ?
GROUP BY 1 ORDER BY 1`, cutoff).Scan(&trend).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	for i := range trend {
		trend[i].Rate2 = ratio(trend[i].Within2, trend[i].Total)
		trend[i].Rate3 = ratio(trend[i].Within3, trend[i].Total)
	}

	// 按科室排名（归档及时率，用于科室责任制管理）
	var byDept []archiveDeptRow
	if err := h.DB.Raw(`
SELECT dept_name AS dept_name,
       count(*) AS total,
       count(*) FILTER (WHERE archived_at IS NOT NULL) AS archived,
       count(*) FILTER (WHERE archived_at IS NOT NULL AND archived_at - discharge_time <= interval '2 days') AS within2,
       coalesce(avg(extract(epoch FROM (archived_at - discharge_time)) / 86400)
                FILTER (WHERE archived_at IS NOT NULL), 0) AS avg_days
FROM med_record
WHERE discharge_time IS NOT NULL AND dept_name <> ''
GROUP BY dept_name`).Scan(&byDept).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	for i := range byDept {
		byDept[i].Rate2 = ratio(byDept[i].Within2, byDept[i].Total)
		byDept[i].AvgDays = round2(byDept[i].AvgDays)
	}
	// 按 2 日归档率降序（及时率高的科室在前）
	sort.SliceStable(byDept, func(i, j int) bool {
		if byDept[i].Rate2 != byDept[j].Rate2 {
			return byDept[i].Rate2 > byDept[j].Rate2
		}
		return byDept[i].Total > byDept[j].Total
	})

	resp.OK(c, archiveTimeliness{Overall: overall, Trend: trend, ByDept: byDept})
}

type overdueItem struct {
	RecordID      int64      `json:"recordId"`
	RecordNo      string     `json:"recordNo"`
	PatientName   string     `json:"patientName"`
	DeptName      string     `json:"deptName"`
	DoctorName    string     `json:"doctorName"`
	DischargeTime *time.Time `json:"dischargeTime"`
	ElapsedDays   int        `json:"elapsedDays"` // 出院至今已过天数
	OverdueDays   int        `json:"overdueDays"` // 超出 N 天的天数
	Status        string     `json:"status"`
}

// Overdue GET /api/archive-overdue?days=3（登录）
// 超期未归档催缴清单：已出院且超过 N 天仍未归档的病案，按超期天数倒序。
func (h *ArchiveStatsHandler) Overdue(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "3"))
	if days < 1 {
		days = 3
	}
	cutoff := time.Now().AddDate(0, 0, -days)

	var recs []model.MedRecord
	if err := h.DB.Where("discharge_time IS NOT NULL AND archived_at IS NULL AND discharge_time < ?", cutoff).
		Order("discharge_time ASC").Find(&recs).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}

	now := time.Now()
	list := make([]overdueItem, 0, len(recs))
	for _, r := range recs {
		if r.DischargeTime == nil {
			continue
		}
		elapsed := int(now.Sub(*r.DischargeTime).Hours() / 24)
		over := elapsed - days
		if over < 0 {
			over = 0
		}
		list = append(list, overdueItem{
			RecordID: r.ID, RecordNo: r.RecordNo, PatientName: r.PatientName,
			DeptName: r.DeptName, DoctorName: r.DoctorName,
			DischargeTime: r.DischargeTime, ElapsedDays: elapsed, OverdueDays: over,
			Status: r.Status,
		})
	}
	resp.OK(c, gin.H{"days": days, "count": len(list), "list": list})
}

// ratio 计算分子/分母比例（分母为 0 时返回 0），并保留 4 位小数。
func ratio(num, den int64) float64 {
	if den <= 0 {
		return 0
	}
	return round4(float64(num) / float64(den))
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

func round4(f float64) float64 {
	return float64(int64(f*10000+0.5)) / 10000
}
