package model

import "time"

// ---------- 上报子状态机 ----------
// 批次：PENDING 待上报 → SUCCESS / FAILED / PARTIAL(部分成功)
// 明细：WAITING → SUCCESS / FAIL（回执留痕，供异常排查）
const (
	BatPending = "PENDING"
	BatSuccess = "SUCCESS"
	BatFailed  = "FAILED"
	BatPartial = "PARTIAL"

	RcWaiting = "WAITING"
	RcSuccess = "SUCCESS"
	RcFail    = "FAIL"
)

type ReportBatch struct {
	ID           int64      `json:"id" gorm:"primaryKey"`
	BatchNo      string     `json:"batchNo"`
	Period       string     `json:"period"` // 如 2026-08
	Status       string     `json:"status" gorm:"default:PENDING"`
	TotalCount   int        `json:"totalCount"`
	SuccessCount int        `json:"successCount"`
	FailCount    int        `json:"failCount"`
	SubmitTime   *time.Time `json:"submitTime"`
	OperatorName string     `json:"operatorName"`
	Remark       string     `json:"remark"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (ReportBatch) TableName() string { return "report_batch" }

type ReportItem struct {
	ID            int64      `json:"id" gorm:"primaryKey"`
	BatchID       int64      `json:"batchId"`
	RecordID      int64      `json:"recordId"`
	ReceiptStatus string     `json:"receiptStatus" gorm:"default:WAITING"`
	ReceiptMsg    string     `json:"receiptMsg"`
	ReceiptTime   *time.Time `json:"receiptTime"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (ReportItem) TableName() string { return "report_item" }
