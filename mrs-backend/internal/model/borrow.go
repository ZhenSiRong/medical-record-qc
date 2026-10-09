package model

import "time"

// ---------- 借阅子状态机 ----------
// APPLYING 审批中(多级) → APPROVED 待借出 → BORROWED 借出 → RETURNED 已归还
// 任一审批级驳回 → REJECTED；BORROWED 超应还时间由定时任务标记 OVERDUE
const (
	BorApplying = "APPLYING"
	BorApproved = "APPROVED"
	BorRejected = "REJECTED"
	BorBorrowed = "BORROWED"
	BorOverdue  = "OVERDUE"
	BorReturned = "RETURNED"
)

type BorrowOrder struct {
	ID           int64      `json:"id" gorm:"primaryKey"`
	OrderNo      string     `json:"orderNo"`
	RecordID     int64      `json:"recordId"`
	RecordNo     string     `json:"recordNo"`
	BorrowerName string     `json:"borrowerName"`
	BorrowerDept string     `json:"borrowerDept"`
	Reason       string     `json:"reason"`
	Urgent       bool       `json:"urgent"`
	Status       string     `json:"status" gorm:"default:APPLYING"`
	ApplyTime    *time.Time `json:"applyTime"`
	LendTime     *time.Time `json:"lendTime"`
	DueTime      *time.Time `json:"dueTime"`
	ReturnTime   *time.Time `json:"returnTime"`
	HandlerName  string     `json:"handlerName"`
	Remark       string     `json:"remark"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (BorrowOrder) TableName() string { return "borrow_order" }

type BorrowApproval struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	OrderID      int64     `json:"orderId"`
	Level        int       `json:"level"`
	ApproverName string    `json:"approverName"`
	Result       string    `json:"result"` // APPROVED / REJECTED
	Opinion      string    `json:"opinion"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (BorrowApproval) TableName() string { return "borrow_approval" }

type BorrowReminder struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	OrderID   int64     `json:"orderId"`
	Channel   string    `json:"channel" gorm:"default:SYSTEM"`
	Result    string    `json:"result"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (BorrowReminder) TableName() string { return "borrow_reminder" }
