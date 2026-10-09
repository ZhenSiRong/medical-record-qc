package handler

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/config"
	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

// BorrowHandler 借阅申请与审批（节 5）：申请 → 多级审批 → 借出 → 归还核销
type BorrowHandler struct {
	DB  *gorm.DB
	CFG *config.Config
}

type applyReq struct {
	RecordID int64      `json:"recordId" binding:"required"`
	Reason   string     `json:"reason" binding:"required"`
	DueTime  *time.Time `json:"dueTime"` // 应还时间，留空用默认借阅期限
	Urgent   bool       `json:"urgent"`
}

// Apply 借阅申请：只有已归档(在架)的病案可借，任何登录用户可申请
func (h *BorrowHandler) Apply(c *gin.Context) {
	var req applyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "请传 recordId 与 reason")
		return
	}
	_, username, _ := middleware.CurrentUser(c)

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var rec model.MedRecord
		if err := tx.First(&rec, req.RecordID).Error; err != nil {
			return errBiz("病案不存在")
		}
		if rec.Status != model.RecArchived {
			return errBiz("仅已归档(在架)的病案可申请借阅，当前状态 " + rec.Status)
		}
		due := time.Now().AddDate(0, 0, h.CFG.Borrow.BorrowDays)
		if req.DueTime != nil {
			due = *req.DueTime
		}
		order := model.BorrowOrder{
			OrderNo:      "JY" + time.Now().Format("20060102150405"),
			RecordID:     rec.ID,
			RecordNo:     rec.RecordNo,
			BorrowerName: username,
			BorrowerDept: rec.DeptName,
			Reason:       req.Reason,
			Urgent:       req.Urgent,
			Status:       model.BorApplying,
			ApplyTime:    ptrTime(time.Now()),
			DueTime:      ptrTime(due),
		}
		return tx.Create(&order).Error
	})
	if err != nil {
		replyBiz(c, err)
		return
	}
	resp.OK(c, gin.H{"msg": "申请已提交"})
}

// Approve 审批（病案室）：按配置的层级数逐级审批，全部通过 → APPROVED；任一级驳回 → REJECTED
func (h *BorrowHandler) Approve(c *gin.Context) {
	var req struct {
		Result  string `json:"result" binding:"required"` // APPROVED / REJECTED
		Opinion string `json:"opinion"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Result != "APPROVED" && req.Result != "REJECTED") {
		resp.BadRequest(c, "result 必须为 APPROVED 或 REJECTED")
		return
	}
	_, username, _ := middleware.CurrentUser(c)

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var order model.BorrowOrder
		if err := tx.First(&order, c.Param("id")).Error; err != nil {
			return errBiz("借阅单不存在")
		}
		if order.Status != model.BorApplying {
			return errBiz("借阅单当前状态不可审批")
		}
		var level int64
		tx.Model(&model.BorrowApproval{}).Where("order_id = ?", order.ID).Count(&level)
		if err := tx.Create(&model.BorrowApproval{
			OrderID: order.ID, Level: int(level) + 1,
			ApproverName: username, Result: req.Result, Opinion: req.Opinion,
		}).Error; err != nil {
			return err
		}
		if req.Result == "REJECTED" {
			return tx.Model(&order).Update("status", model.BorRejected).Error
		}
		if int(level)+1 >= h.CFG.Borrow.ApproveLevels {
			return tx.Model(&order).Update("status", model.BorApproved).Error
		}
		return nil // 还有更高级别审批，保持 APPLYING
	})
	if err != nil {
		replyBiz(c, err)
		return
	}
	resp.OK(c, gin.H{"msg": "审批完成"})
}

// Lend 借出：APPROVED → BORROWED；病案 ARCHIVED → BORROWED（条件更新防并发双借）
func (h *BorrowHandler) Lend(c *gin.Context) {
	_, _, _ = middleware.CurrentUser(c)
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var order model.BorrowOrder
		if err := tx.First(&order, c.Param("id")).Error; err != nil {
			return errBiz("借阅单不存在")
		}
		if order.Status != model.BorApproved {
			return errBiz("仅审批通过的借阅单可借出")
		}
		res := tx.Model(&model.MedRecord{}).
			Where("id = ? AND status = ?", order.RecordID, model.RecArchived).
			Update("status", model.RecBorrowed)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errBiz("病案当前不在架，无法借出")
		}
		now := time.Now()
		return tx.Model(&order).Updates(map[string]any{
			"status": model.BorBorrowed, "lend_time": now, "handler_name": operatorName(c),
		}).Error
	})
	if err != nil {
		replyBiz(c, err)
		return
	}
	resp.OK(c, gin.H{"msg": "借出完成"})
}

// Return 归还核销：BORROWED/OVERDUE → RETURNED；病案回到在架
func (h *BorrowHandler) Return(c *gin.Context) {
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var order model.BorrowOrder
		if err := tx.First(&order, c.Param("id")).Error; err != nil {
			return errBiz("借阅单不存在")
		}
		if order.Status != model.BorBorrowed && order.Status != model.BorOverdue {
			return errBiz("借阅单当前状态不可归还")
		}
		res := tx.Model(&model.MedRecord{}).
			Where("id = ? AND status = ?", order.RecordID, model.RecBorrowed).
			Update("status", model.RecArchived)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errBiz("病案状态异常，请联系管理员核查")
		}
		return tx.Model(&order).Updates(map[string]any{
			"status": model.BorReturned, "return_time": time.Now(), "handler_name": operatorName(c),
		}).Error
	})
	if err != nil {
		replyBiz(c, err)
		return
	}
	resp.OK(c, gin.H{"msg": "归还核销完成"})
}

// Renew 续借（D4）：仅借出中的借阅单可续，应还时间顺延；逾期单先归还
func (h *BorrowHandler) Renew(c *gin.Context) {
	var req struct {
		Days int `json:"days" binding:"required,min=1,max=60"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "days 必须为 1-60 的整数")
		return
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var order model.BorrowOrder
		if err := tx.First(&order, c.Param("id")).Error; err != nil {
			return errBiz("借阅单不存在")
		}
		if order.Status != model.BorBorrowed {
			return errBiz("仅借出中的借阅单可续借，逾期借阅请先归还")
		}
		if order.DueTime == nil {
			return errBiz("借阅单缺少应还时间，请联系管理员")
		}
		return tx.Model(&order).Update("due_time", order.DueTime.AddDate(0, 0, req.Days)).Error
	})
	if err != nil {
		replyBiz(c, err)
		return
	}
	resp.OK(c, gin.H{"msg": fmt.Sprintf("已续借 %d 天", req.Days)})
}

func operatorName(c *gin.Context) string {
	_, username, _ := middleware.CurrentUser(c)
	return username
}

func ptrTime(t time.Time) *time.Time { return &t }

func replyBiz(c *gin.Context, err error) {
	if be, ok := err.(bizErr); ok {
		resp.Conflict(c, be.msg)
		return
	}
	resp.ServerErr(c, err)
}
