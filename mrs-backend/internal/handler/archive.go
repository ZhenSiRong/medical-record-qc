package handler

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

// ArchiveHandler 归档管理（节 3）：批量回收、批量归档上架、封存、流转履历
type ArchiveHandler struct {
	DB *gorm.DB
}

type recycleReq struct {
	RecordIDs []int64 `json:"recordIds" binding:"required,min=1"`
}

// Recycle 批量回收：NOT_RECYCLED → RECYCLED，逐条写流水（同一批次号）
func (h *ArchiveHandler) Recycle(c *gin.Context) {
	var req recycleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "请传 recordIds 数组")
		return
	}
	_, operator, _ := middleware.CurrentUser(c)
	batchNo := batchNo("RC")

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.MedRecord{}).
			Where("id IN ? AND status = ?", req.RecordIDs, model.RecNotRecycled).
			Update("status", model.RecRecycled)
		if res.Error != nil {
			return res.Error
		}
		rows := make([]model.ArchiveRecord, 0, len(req.RecordIDs))
		for _, id := range req.RecordIDs {
			rows = append(rows, model.ArchiveRecord{
				RecordID: id, Action: model.ArRecycle, BatchNo: batchNo, OperatorName: operator,
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, gin.H{"batchNo": batchNo})
}

type shelveReq struct {
	RecordIDs []int64 `json:"recordIds" binding:"required,min=1"`
	ShelfID   int64   `json:"shelfId" binding:"required"`
}

// Shelve 批量归档上架：RECYCLED/CATALOGED → ARCHIVED 并绑定架位；架位容量校验
func (h *ArchiveHandler) Shelve(c *gin.Context) {
	var req shelveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "请传 recordIds 与 shelfId")
		return
	}
	_, operator, _ := middleware.CurrentUser(c)
	batchNo := batchNo("AR")

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// 行锁防并发上架超容
		var shelf model.Shelf
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&shelf, req.ShelfID).Error; err != nil {
			return fmt.Errorf("架位不存在")
		}
		free := shelf.Capacity - shelf.UsedCount
		if free <= 0 {
			return fmt.Errorf("架位 %s 已满", shelf.ShelfCode)
		}
		ids := req.RecordIDs
		if len(ids) > free {
			ids = ids[:free]
		}
		res := tx.Model(&model.MedRecord{}).
			Where("id IN ? AND status IN ?", ids, []string{model.RecRecycled, model.RecCataloged}).
			Updates(map[string]any{"status": model.RecArchived, "shelf_id": req.ShelfID, "archived_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			if err := tx.Model(&shelf).Update("used_count", gorm.Expr("used_count + ?", res.RowsAffected)).Error; err != nil {
				return err
			}
			rows := make([]model.ArchiveRecord, 0, res.RowsAffected)
			for _, id := range ids {
				rows = append(rows, model.ArchiveRecord{
					RecordID: id, Action: model.ArArchive, BatchNo: batchNo,
					ShelfID: req.ShelfID, OperatorName: operator,
				})
			}
			return tx.Create(&rows).Error
		}
		return nil
	})
	if err != nil {
		resp.Conflict(c, err.Error())
		return
	}
	resp.OK(c, gin.H{"batchNo": batchNo})
}

// Seal 封存（合规要求）：ARCHIVED → SEALED，封存后禁止一切借阅
func (h *ArchiveHandler) Seal(c *gin.Context) {
	_, operator, _ := middleware.CurrentUser(c)
	res := h.DB.Model(&model.MedRecord{}).
		Where("id = ? AND status = ?", c.Param("id"), model.RecArchived).
		Update("status", model.RecSealed)
	if res.Error != nil {
		resp.ServerErr(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		resp.Conflict(c, "病案不存在或当前状态不允许封存")
		return
	}
	var rec model.MedRecord
	h.DB.First(&rec, c.Param("id"))
	h.DB.Create(&model.ArchiveRecord{RecordID: rec.ID, Action: model.ArSeal, OperatorName: operator})
	resp.OK(c, nil)
}

// Flow 归档流转履历
func (h *ArchiveHandler) Flow(c *gin.Context) {
	var rows []model.ArchiveRecord
	if err := h.DB.Where("record_id = ?", c.Param("id")).Order("id ASC").Find(&rows).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, rows)
}

func batchNo(prefix string) string {
	return prefix + time.Now().Format("20060102150405")
}
