package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
	"mrs/pkg/resp"
)

// =====================================================================
// 任务 10：上报数据与院内原始数据一致性核对
//
// 背景：三级医院评审现场核查会抽取 ≥20% 上报数据，偏差 ≥10% 判为错误数据并扣分。
// 病案科需要「上报口径 vs 院内实际」的核对工具，快速定位漏报与上报后改动。
//
// 只读实现，不新增表。
// =====================================================================
type ConsistencyHandler struct{ DB *gorm.DB }

// consistencyTolerance 上报后修改的判定容忍窗口。
// 近似判断依据：上报动作本身会更新首页状态与 updated_at，故此基准时间取「批次落库
// 时间、上报时间、回执时间」中较晚者，并留出容忍窗口；仅当首页 updated_at 落在
// 容忍窗口之后才判定为「上报后被修改」，避免把状态回写误判为数据修改。
const consistencyTolerance = 24 * time.Hour

type consistencyDetail struct {
	Kind        string `json:"kind"` // MISSING 未纳入上报 / MODIFIED 上报后被修改
	RecordID    int64  `json:"recordId"`
	RecordNo    string `json:"recordNo"`
	PatientName string `json:"patientName"`
	DeptName    string `json:"deptName"`
	Detail      string `json:"detail"`
}

type consistencyReport struct {
	Period     string              `json:"period"`
	Expected   int64               `json:"expected"`   // 该周期院内应有数（QC_PASSED/REPORTED 首页）
	Submitted  int64               `json:"submitted"`  // 该周期实际上报明细数
	Missing    int64               `json:"missing"`    // 符合条件但未纳入上报
	Mismatch   int64               `json:"mismatch"`   // 已上报但此后被修改
	Consistent bool                `json:"consistent"` // 是否一致
	Detail     []consistencyDetail `json:"detail"`
}

type consistencySummary struct {
	Period     string `json:"period"`
	Expected   int64  `json:"expected"`
	Submitted  int64  `json:"submitted"`
	Missing    int64  `json:"missing"`
	Mismatch   int64  `json:"mismatch"`
	Consistent bool   `json:"consistent"`
}

// Report GET /api/report-consistency?period=2026-09（登录）
func (h *ConsistencyHandler) Report(c *gin.Context) {
	period := c.Query("period")
	if period == "" {
		resp.BadRequest(c, "请传 period（如 2026-09）")
		return
	}
	rep, err := h.compute(period)
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, rep)
}

// Summary GET /api/report-consistency/summary（登录）
// 近 6 个月每月一致性概览，便于观察趋势。
func (h *ConsistencyHandler) Summary(c *gin.Context) {
	now := time.Now()
	out := make([]consistencySummary, 0, 6)
	for i := 5; i >= 0; i-- {
		period := now.AddDate(0, -i, 0).Format("2006-01")
		rep, err := h.compute(period)
		if err != nil {
			resp.ServerErr(c, err)
			return
		}
		out = append(out, consistencySummary{
			Period: rep.Period, Expected: rep.Expected, Submitted: rep.Submitted,
			Missing: rep.Missing, Mismatch: rep.Mismatch, Consistent: rep.Consistent,
		})
	}
	resp.OK(c, out)
}

// compute 计算指定周期的一致性核对结果。
// 口径：周期以首页出院时间所属月份为准（与上报批次 period 一致）。
func (h *ConsistencyHandler) compute(period string) (consistencyReport, error) {
	rep := consistencyReport{Period: period, Detail: make([]consistencyDetail, 0)}

	// 1) 院内应有：该周期出院、且首页状态为 QC_PASSED 或 REPORTED 的病案
	//    （QC_PASSED 可上报；REPORTED 已上报——二者共同构成"符合上报条件"全集）
	var fps []model.MedFrontPage
	if err := h.DB.Where("status IN ?", []string{model.PageQcPassed, model.PageReported}).
		Where("discharge_time IS NOT NULL AND to_char(discharge_time, 'YYYY-MM') = ?", period).
		Find(&fps).Error; err != nil {
		return rep, err
	}
	rep.Expected = int64(len(fps))

	// 2) 实际提交：该周期上报批次下的明细
	var batches []model.ReportBatch
	if err := h.DB.Where("period = ?", period).Find(&batches).Error; err != nil {
		return rep, err
	}
	batchByID := make(map[int64]model.ReportBatch, len(batches))
	batchIDs := make([]int64, 0, len(batches))
	for _, b := range batches {
		batchByID[b.ID] = b
		batchIDs = append(batchIDs, b.ID)
	}
	var items []model.ReportItem
	if len(batchIDs) > 0 {
		if err := h.DB.Where("batch_id IN ?", batchIDs).Find(&items).Error; err != nil {
			return rep, err
		}
	}
	rep.Submitted = int64(len(items))

	submittedSet := make(map[int64]bool, len(items))
	for _, it := range items {
		submittedSet[it.RecordID] = true
	}

	// 首页索引（含上报明细可能引用的、不在本期 fps 内的首页）
	fpByRecord := make(map[int64]model.MedFrontPage, len(fps))
	ids := make([]int64, 0, len(fps)+len(items))
	for _, fp := range fps {
		fpByRecord[fp.RecordID] = fp
		ids = append(ids, fp.RecordID)
	}
	for _, it := range items {
		if _, ok := fpByRecord[it.RecordID]; !ok {
			var fp model.MedFrontPage
			if err := h.DB.Where("record_id = ?", it.RecordID).First(&fp).Error; err == nil {
				fpByRecord[it.RecordID] = fp
			}
		}
		ids = append(ids, it.RecordID)
	}
	recMap := h.recordsByID(ids)

	// 3a) 漏报：符合条件但未纳入上报
	var missing, mismatch int64
	for _, fp := range fps {
		if submittedSet[fp.RecordID] {
			continue
		}
		missing++
		rep.Detail = append(rep.Detail, consistencyDetail{
			Kind:        "MISSING",
			RecordID:    fp.RecordID,
			RecordNo:    recMap[fp.RecordID].RecordNo,
			PatientName: fp.PatientName,
			DeptName:    fp.DeptName,
			Detail:      "符合上报条件但未纳入该周期上报批次",
		})
	}

	// 3b) 上报后修改：首页 updated_at 晚于基准时间 + 容忍窗口
	for _, it := range items {
		fp, ok := fpByRecord[it.RecordID]
		if !ok {
			continue
		}
		base := it.CreatedAt
		if b, ok := batchByID[it.BatchID]; ok {
			if b.CreatedAt.After(base) {
				base = b.CreatedAt
			}
			if b.SubmitTime != nil && b.SubmitTime.After(base) {
				base = *b.SubmitTime
			}
		}
		if it.ReceiptTime != nil && it.ReceiptTime.After(base) {
			base = *it.ReceiptTime
		}
		if fp.UpdatedAt.After(base.Add(consistencyTolerance)) {
			mismatch++
			rep.Detail = append(rep.Detail, consistencyDetail{
				Kind:        "MODIFIED",
				RecordID:    fp.RecordID,
				RecordNo:    recMap[fp.RecordID].RecordNo,
				PatientName: fp.PatientName,
				DeptName:    fp.DeptName,
				Detail:      "已上报，但首页数据在上报之后被修改（据此可能与上报口径不一致）",
			})
		}
	}

	rep.Missing = missing
	rep.Mismatch = mismatch
	rep.Consistent = missing == 0 && mismatch == 0
	return rep, nil
}

// recordsByID 按主键批量取病案主档（用于补全病案号），去重后查询。
func (h *ConsistencyHandler) recordsByID(ids []int64) map[int64]model.MedRecord {
	out := make(map[int64]model.MedRecord, len(ids))
	uniq := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}
	if len(uniq) == 0 {
		return out
	}
	var recs []model.MedRecord
	_ = h.DB.Where("id IN ?", uniq).Find(&recs).Error
	for _, r := range recs {
		out[r.ID] = r
	}
	return out
}
