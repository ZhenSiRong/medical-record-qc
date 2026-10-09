package model

import "time"

// =====================================================================
// 旁支扩展功能数据模型（新增文件，不影响既有模型）
// 本文件承载「编码辅助 / DRG 预分组」两张新表。
// 建表由 main.go 的 AutoMigrate 列表统一负责（与 handler/qc_ext.go 同一约定：
// 注册函数保持"纯注册"，不做迁移，否则路由冒烟测试传 nil db 会 panic）。
// =====================================================================

// ---------- 任务 3：智能编码辅助（规则版预编码） ----------
//
// 设计意图：本功能是「编码辅助」而非「自动编码」。
// 国家病案质量与编码管理规范要求编码员依据病案内容自主判断并承担编码责任，
// 系统只能基于关键词给出候选编码「推荐提示」，最终编码由编码员确认选择，
// 绝不替代人工编码判断。
type CodeSuggestionRule struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	Keyword   string    `json:"keyword" gorm:"size:128;index"` // 病历文本/诊断名称关键词（命中即推荐）
	DictType  string    `json:"dictType"`                      // 推荐编码类型：ICD10 / ICD9CM3
	IcdCode   string    `json:"icdCode" gorm:"size:32"`        // 推荐编码（须存在于 icd_dict 字典）
	Weight    int       `json:"weight" gorm:"default:10"`      // 权重：越大排序越靠前
	Enabled   bool      `json:"enabled" gorm:"default:true"`   // 是否启用
	Remark    string    `json:"remark"`                        // 备注（来源/依据）
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (CodeSuggestionRule) TableName() string { return "code_suggestion_rule" }

// ---------- 任务 7：DRG/DIP 预分组（查表版） ----------
//
// 重要声明：本表为「查表提示规则」，非官方 DRG/DIP 分组器。
// 本系统不具备官方分组器与权重表，正式入组/结算必须以国家医保局发布的
// 官方分组器（CHS-DRG / DIP 目录）为准。此表仅用于编码质控阶段的入组「提示」，
// 帮助发现编码不完整、主诊断与手术不匹配等高风险情形，不得作为结算依据。
type DrgGroupRule struct {
	ID         int64     `json:"id" gorm:"primaryKey"`
	DiagPrefix string    `json:"diagPrefix" gorm:"size:32;index"` // 主诊断编码前缀
	OpPrefix   string    `json:"opPrefix" gorm:"size:32"`         // 主手术编码前缀（可空=不限定手术）
	GroupCode  string    `json:"groupCode" gorm:"size:32"`        // 分组代码（示例）
	GroupName  string    `json:"groupName"`                       // 分组名称（示例）
	Weight     float64   `json:"weight"`                          // 参考权重（示例）
	Remark     string    `json:"remark"`                          // 说明（示例数据，非官方分组器）
	Enabled    bool      `json:"enabled" gorm:"default:true"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (DrgGroupRule) TableName() string { return "drg_group_rule" }
