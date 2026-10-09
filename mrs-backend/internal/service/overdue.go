package service

import (
	"log"
	"time"

	"gorm.io/gorm"

	"mrs/internal/model"
)

// StartOverdueScanner 逾期催还定时任务（调研节 6）：每 scanMinutes 扫描一次，
// 借出且超应还时间的借阅单标记 OVERDUE 并生成催还记录。
// TODO 骨架期催还仅落库记录；通知渠道（短信/站内信）调研确认后接入。
func StartOverdueScanner(db *gorm.DB, scanMinutes int) {
	if scanMinutes <= 0 {
		scanMinutes = 60
	}
	go func() {
		ticker := time.NewTicker(time.Duration(scanMinutes) * time.Minute)
		defer ticker.Stop()
		scanOnce(db) // 启动时先扫一次
		for range ticker.C {
			scanOnce(db)
		}
	}()
	log.Printf("[overdue] 逾期扫描已启动，间隔 %d 分钟", scanMinutes)
}

func scanOnce(db *gorm.DB) {
	var orders []model.BorrowOrder
	if err := db.Where("status = ? AND due_time < ?", model.BorBorrowed, time.Now()).Find(&orders).Error; err != nil {
		log.Printf("[overdue] 扫描失败: %v", err)
		return
	}
	for _, order := range orders {
		err := db.Transaction(func(tx *gorm.DB) error {
			// 条件更新保证并发下只标记一次
			res := tx.Model(&model.BorrowOrder{}).
				Where("id = ? AND status = ?", order.ID, model.BorBorrowed).
				Update("status", model.BorOverdue)
			if res.Error != nil || res.RowsAffected == 0 {
				return res.Error
			}
			return tx.Create(&model.BorrowReminder{
				OrderID: order.ID, Channel: "SYSTEM",
				Result: "超过应还时间，系统自动标记逾期",
			}).Error
		})
		if err != nil {
			log.Printf("[overdue] 借阅单 %s 标记失败: %v", order.OrderNo, err)
			continue
		}
		log.Printf("[overdue] 借阅单 %s 已标记逾期", order.OrderNo)
	}
}
