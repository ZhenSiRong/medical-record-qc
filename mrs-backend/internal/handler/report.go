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

// ReportHandler 病案首页上报（节 9）：批次创建 → XML 组包 → 回执 → 异常排查
type ReportHandler struct {
	DB     *gorm.DB
	Client service.ReportClient
}

func (h *ReportHandler) Page(c *gin.Context) {
	q := h.DB.Model(&model.ReportBatch{})
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if p := c.Query("period"); p != "" {
		q = q.Where("period = ?", p)
	}
	resp.OK(c, resp.Page[model.ReportBatch](c, q))
}

// Create 创建上报批次：默认纳入全部质控通过的首页数据；也可指定 recordIds
func (h *ReportHandler) Create(c *gin.Context) {
	var req struct {
		Period    string  `json:"period" binding:"required"` // 如 2026-08
		RecordIDs []int64 `json:"recordIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "请传 period（如 2026-08）")
		return
	}
	_, username, _ := middleware.CurrentUser(c)

	var fps []model.MedFrontPage
	q := h.DB.Where("status = ?", model.PageQcPassed)
	if len(req.RecordIDs) > 0 {
		q = h.DB.Where("status = ? AND record_id IN ?", model.PageQcPassed, req.RecordIDs)
	}
	if err := q.Find(&fps).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	if len(fps) == 0 {
		resp.Conflict(c, "没有质控通过(QC_PASSED)的首页数据可上报")
		return
	}

	batch := model.ReportBatch{
		BatchNo: "SB" + time.Now().Format("20060102150405"), Period: req.Period,
		Status: model.BatPending, TotalCount: len(fps), OperatorName: username,
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		items := make([]model.ReportItem, 0, len(fps))
		for _, fp := range fps {
			items = append(items, model.ReportItem{BatchID: batch.ID, RecordID: fp.RecordID})
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, batch)
}

// Submit 提交批次：逐条走上报防腐层（骨架为本地 XML 落盘桩），回执落库供排查
func (h *ReportHandler) Submit(c *gin.Context) {
	var batch model.ReportBatch
	if err := h.DB.First(&batch, c.Param("id")).Error; err != nil {
		resp.NotFound(c, "批次不存在")
		return
	}
	if batch.Status != model.BatPending {
		resp.Conflict(c, "批次已提交过，请勿重复上报")
		return
	}

	var items []model.ReportItem
	h.DB.Where("batch_id = ?", batch.ID).Find(&items)

	success, fail := 0, 0
	now := time.Now()
	for i := range items {
		item := &items[i]
		var fp model.MedFrontPage
		if err := h.DB.Where("record_id = ?", item.RecordID).First(&fp).Error; err != nil {
			item.ReceiptStatus, item.ReceiptMsg, item.ReceiptTime = model.RcFail, "首页数据缺失: "+err.Error(), ptrTime(now)
			fail++
			continue
		}
		status, msg, err := h.Client.Submit(&batch, item, &fp)
		if err != nil {
			status, msg = model.RcFail, "上报通道异常: "+err.Error()
		}
		item.ReceiptStatus, item.ReceiptMsg, item.ReceiptTime = status, msg, ptrTime(time.Now())
		if status == model.RcSuccess {
			success++
			h.DB.Model(&model.MedFrontPage{}).Where("record_id = ?", item.RecordID).
				Update("status", model.PageReported)
		} else {
			fail++
		}
		h.DB.Save(item)
	}

	batch.SuccessCount, batch.FailCount = success, fail
	batch.SubmitTime = ptrTime(time.Now())
	switch {
	case fail == 0:
		batch.Status = model.BatSuccess
	case success == 0:
		batch.Status = model.BatFailed
	default:
		batch.Status = model.BatPartial
	}
	h.DB.Save(&batch)
	resp.OK(c, batch)
}

// Items 批次明细与回执（异常排查入口：失败原因在 receipt_msg）
func (h *ReportHandler) Items(c *gin.Context) {
	var items []model.ReportItem
	if err := h.DB.Where("batch_id = ?", c.Param("id")).Order("id ASC").Find(&items).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, items)
}
