package model

import "time"

// 本文件为质控域扩展模型（任务16 病历质量评分、任务17 归档变更留痕），
// 与既有 record.go 解耦，便于后续按需扩展。

// ---------- 病历质量等级（任务16） ----------
// 行业通用口径：甲级 ≥90 分、乙级 75–89 分、丙级 <75 分；
// 国家《三级医院评审标准》要求甲级病历率 ≥90%、杜绝丙级病历。
const (
	GradeCodeA = "A"
	GradeCodeB = "B"
	GradeCodeC = "C"

	GradeNameA = "甲级"
	GradeNameB = "乙级"
	GradeNameC = "丙级"
)

// FrontPageQuality 病案首页质量评分（每次质控后按 record_id 覆盖一条）。
// 依据国卫办医发〔2016〕24号《关于印发住院病案首页数据填写质量规范（暂行）
// 和住院病案首页数据质量管理与控制指标（2016版）的通知》及三级医院评审要求。
type FrontPageQuality struct {
	ID       int64 `json:"id" gorm:"primaryKey"`
	RecordID int64 `json:"recordId" gorm:"index"`

	Score     int    `json:"score"`     // 0–100 分
	GradeCode string `json:"gradeCode"` // A/B/C
	GradeName string `json:"gradeName"` // 甲级/乙级/丙级

	ErrorCount int `json:"errorCount"` // ERROR 级问题数
	WarnCount  int `json:"warnCount"`  // WARN 级问题数

	// Vetoed 是否触发单项否决（丙级直判）；VetoReason 记录否决原因（多条用； 连接）。
	Vetoed     bool   `json:"vetoed"`
	VetoReason string `json:"vetoReason"`

	ScoredAt  time.Time `json:"scoredAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (FrontPageQuality) TableName() string { return "front_page_quality" }

// FrontPageRevision 首页数据变更留痕（任务17）。
// 依据《电子病历应用管理规范（试行）》（国卫办医发〔2017〕8号）：
// 归档后原则上不得修改，确需修改的应经审批并保留修改痕迹。
type FrontPageRevision struct {
	ID       int64 `json:"id" gorm:"primaryKey"`
	RecordID int64 `json:"recordId" gorm:"index"`

	BeforeJSON string    `json:"beforeJson" gorm:"type:text"` // 修改前快照
	AfterJSON  string    `json:"afterJson" gorm:"type:text"`  // 修改后快照
	ChangedBy  string    `json:"changedBy"`                   // 修改人
	Reason     string    `json:"reason"`                      // 修改事由（可选）
	ChangedAt  time.Time `json:"changedAt"`

	CreatedAt time.Time `json:"createdAt"`
}

func (FrontPageRevision) TableName() string { return "front_page_revision" }
