package model

import "time"

// =====================================================================
// CC/MCC 判读规则表（ADR-0002 §4，批次 B：判读层落库）
//
// 本表是 service.CcMccRule（判读用的内存结构体）的**持久化形态**：字段一一对应，
// 由 service/ccmcc_store.go 读出后转成 service.CcMccRule 交给 JudgeSeverity。
// 换官方表 = 换本表的行数据（另起 Version），判读逻辑不改——这是 ADR §4
// 「可替换 + 版本化 / 换数据不改代码」的落地。
//
// ⚠️ 合规红线（ADR §3/§6，改本文件前必读）：
//  1. 本表**绝不允许**出现分组码、权重、金额之类的列。那不是实现工作量问题，
//     而是授权件问题：CHS-DRG/DIP 的分组码与权重属官方授权件，本院未取得，
//     自造即为出具支付等级判断。本表只回答「像 CC 还是 MCC」，不出具分组与权重。
//  2. 每行必须带 Source（"demo" / "official"）：demo 行是非官方自造数据，
//     对外展示时必须显著标注（service.CcMccDisclaimer 已内置该声明）。
//  3. Version 是判读的开关：只有与请求版本相等的行才会参与判读，
//     version 为空时一律不加载（fail-closed，见 service.LoadCcMccRules）。
//
// 建表由 main.go 的 AutoMigrate 列表统一负责（与 intelligence.go 同一约定）。
// =====================================================================

// CcMccRule 一条 CC/MCC 判读规则（表名 cc_mcc_rule）。
//
// 字段口径与 service.CcMccRule 保持一致（DiagPrefix 大写 ICD-10 前缀、不含通配符；
// Severity 只允许 MCC / CC；Version 与 Source 见上）。不设 json tag：本结构体是库表
// 行，不直接参与接口序列化——对外的 JSON 形态是 service.CcMccItem / CcMccJudgement，
// 那里才带 Disclaimer 与来源标注（ADR §4 强约束）。
//
// ---------- 关于 (version, diag_prefix) 唯一索引：本期故意不建 ----------
//
// 事实上的判重键是 **(version, diag_prefix)**：seed.SeedCcMccRules（internal/seed/ccmcc_ext.go）
// 按它判重灌入，换版本时旧版本行原样保留（ADR §4「不同版本可共存」）。
// 但本模型**没有**给这两列加 uniqueIndex，这是刻意决定，不是遗漏——
// 看到 seed 里的 `clause.OnConflict{DoNothing: true}` 却找不到对应约束时，别顺手补索引：
//
//  1. 已有库（已跑过种子）若存在重复行，AutoMigrate 建唯一索引会失败，
//     而 main.go 的 AutoMigrate 失败即 log.Fatalf ⇒ 演示库启动不起来。
//     演示系统"起不来"的代价远大于重复行本身。
//  2. 重复行不构成功能缺陷：判读取"最长前缀命中"（service.JudgeSeverity），
//     同前缀重复行不改变判读结果，也不改变 sources / demoOnly；
//     种子的 seen 预检已挡住单进程重复执行，演示场景是单实例，并发路径走不到。
//
// 将来确需上唯一索引（例如改成多实例部署）时，**顺序不可颠倒**：
// 先跑去重脚本清理既有重复行 → 再加 uniqueIndex → 最后才允许 AutoMigrate 执行。
// 顺序反了，已存在重复行的库会在启动阶段直接 Fatal。
type CcMccRule struct {
	ID                    int64  `gorm:"primaryKey"`
	DiagPrefix            string `gorm:"size:32;not null"`
	Severity              string `gorm:"size:8;not null"`
	Version               string `gorm:"size:32;not null"`
	Source                string `gorm:"size:16;not null"`
	ExcludeWhenMainPrefix string `gorm:"size:32"`
	Remark                string `gorm:"size:512"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// TableName 表名固定为 cc_mcc_rule（GORM 默认会把 CcMccRule 转成 cc_mcc_rules，
// 与既有的 cc_mcc_rule 约定不符，故显式指定）。
func (CcMccRule) TableName() string { return "cc_mcc_rule" }
