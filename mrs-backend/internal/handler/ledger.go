package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

// LedgerHandler 借阅台账（节 6）：全量借阅历史、逾期筛选、审批链追溯、催还登记
type LedgerHandler struct {
	DB *gorm.DB
}

func (h *LedgerHandler) Page(c *gin.Context) {
	q := h.DB.Model(&model.BorrowOrder{})
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if kw := c.Query("keyword"); kw != "" {
		kw := like(kw)
		q = q.Where("order_no ILIKE ? OR record_no ILIKE ? OR borrower_name ILIKE ?", kw, kw, kw)
	}
	if c.Query("overdueOnly") == "true" {
		q = q.Where("status = ? OR (status = ? AND due_time < ?)", model.BorOverdue, model.BorBorrowed, time.Now())
	}
	if c.Query("urgent") == "true" {
		q = q.Where("urgent = ?", true)
	}
	resp.OK(c, resp.Page[model.BorrowOrder](c, q))
}

// Approvals 审批链追溯
func (h *LedgerHandler) Approvals(c *gin.Context) {
	var rows []model.BorrowApproval
	if err := h.DB.Where("order_id = ?", c.Param("id")).Order("level ASC").Find(&rows).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, rows)
}

// Remind 人工发起催还（自动催还由定时任务负责）
func (h *LedgerHandler) Remind(c *gin.Context) {
	var order model.BorrowOrder
	if err := h.DB.First(&order, c.Param("id")).Error; err != nil {
		resp.NotFound(c, "借阅单不存在")
		return
	}
	_, username, _ := middleware.CurrentUser(c)
	reminder := model.BorrowReminder{
		OrderID: order.ID, Channel: "SYSTEM",
		Result: "已向借阅人 " + order.BorrowerName + " 发出催还提醒（发起人 " + username + "）",
	}
	if err := h.DB.Create(&reminder).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, reminder)
}
