package handler

import (
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
)

// =====================================================================
// 旁支扩展功能统一装配（旁支实现者交付物）
//
// 约定（与主线解耦，避免改动热点文件）：
//   - 新增能力一律挂在本函数内，由 router.go 统一调用 RegisterBranchExt(auth, admin, db)；
//   - 新表统一由 main.go 的 AutoMigrate 负责（本函数保持纯注册）；
//   - 示例种子由 SeedBranchExt(db) 幂等灌入，不修改 seed.go。
//
// 涵盖任务：3 智能编码辅助、7 DRG/DIP 预分组（查表版）、8 上报导出、
//
//	10 上报一致性核对、23 归档及时率与超期催缴。
//
// =====================================================================
func RegisterBranchExt(auth, admin *gin.RouterGroup, db *gorm.DB) {
	// ---------- 任务 3：智能编码辅助（规则版预编码） ----------
	cs := &CodeSuggestHandler{DB: db}
	auth.POST("/code-suggest", cs.Suggest)                    // 登录：编码推荐
	auth.GET("/code-suggestion-rules/page", cs.RulesPage)     // 登录：规则查询
	admin.POST("/code-suggestion-rules", cs.RuleCreate)       // ADMIN：新增规则
	admin.PUT("/code-suggestion-rules/:id", cs.RuleUpdate)    // ADMIN：修改规则
	admin.DELETE("/code-suggestion-rules/:id", cs.RuleDelete) // ADMIN：删除规则

	// ---------- 任务 7：DRG/DIP 预分组（查表版） ----------
	dg := &DrgHandler{DB: db}
	auth.POST("/drg-preview", dg.Preview)               // 登录：分组提示
	auth.GET("/drg-group-rules/page", dg.RulesPage)     // 登录：规则查询
	admin.POST("/drg-group-rules", dg.RuleCreate)       // ADMIN
	admin.PUT("/drg-group-rules/:id", dg.RuleUpdate)    // ADMIN
	admin.DELETE("/drg-group-rules/:id", dg.RuleDelete) // ADMIN

	// ---------- 任务 10：上报数据与院内原始数据一致性核对 ----------
	cc := &ConsistencyHandler{DB: db}
	auth.GET("/report-consistency", cc.Report)          // 登录：单周期核对
	auth.GET("/report-consistency/summary", cc.Summary) // 登录：近 6 月概览

	// ---------- 任务 23：归档及时率 / 超期催缴清单 ----------
	as := &ArchiveStatsHandler{DB: db}
	auth.GET("/archive-timeliness", as.Timeliness) // 登录：归档及时率
	auth.GET("/archive-overdue", as.Overdue)       // 登录：超期催缴清单

	// ---------- 任务 8：上报数据导出（符合国家接口标准的 CSV） ----------
	ex := &ReportExportHandler{DB: db}
	admin.GET("/report-batches/:id/export", ex.Export)           // ADMIN：导出 CSV
	auth.GET("/report-batches/:id/export-check", ex.ExportCheck) // 登录：导出前检查
}

// SeedBranchExt 幂等灌入旁支扩展的示例数据。
// 编码辅助规则的关键词→编码映射，其编码均取自系统内置 icd_dict（见 seed.go），
// 保证推荐编码可通过编目/质控的存在性校验。
func SeedBranchExt(db *gorm.DB) error {
	var cnt int64
	if err := db.Model(&model.CodeSuggestionRule{}).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		rules := seedCodeSuggestionRules()
		if err := db.Create(&rules).Error; err != nil {
			return err
		}
		log.Printf("[seed] 编码辅助规则 %d 条已灌入", len(rules))
	}

	if err := db.Model(&model.DrgGroupRule{}).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		rules := seedDrgGroupRules()
		if err := db.Create(&rules).Error; err != nil {
			return err
		}
		log.Printf("[seed] DRG 预分组示例规则 %d 条已灌入", len(rules))
	}
	return nil
}

// seedCodeSuggestionRules 内置常见编码辅助规则（约 24 条）。
// 编码严格取自 seed.go 内置 ICD 字典，确保与系统码表一致。
func seedCodeSuggestionRules() []model.CodeSuggestionRule {
	type def struct {
		keyword  string
		dictType string
		code     string
		weight   int
	}
	defs := []def{
		// ---------- ICD-10 诊断 ----------
		{"高血压", model.Icd10, "I10.x00", 10},
		{"冠心病", model.Icd10, "I25.101", 10},
		{"心力衰竭", model.Icd10, "I50.901", 10},
		{"脑梗死", model.Icd10, "I63.901", 10},
		{"脑出血", model.Icd10, "I61.901", 10},
		{"肺炎", model.Icd10, "J18.901", 10},
		{"慢性阻塞性肺疾病", model.Icd10, "J44.101", 10},
		{"上呼吸道感染", model.Icd10, "J06.900", 9},
		{"阑尾炎", model.Icd10, "K35.801", 10},
		{"胆囊结石", model.Icd10, "K80.201", 10},
		{"胆结石", model.Icd10, "K80.201", 8},
		{"胃溃疡", model.Icd10, "K25.901", 9},
		{"股骨颈骨折", model.Icd10, "S72.001", 10},
		{"骨折", model.Icd10, "S72.001", 5}, // 泛指关键词，权重低，仅作提示
		{"肺癌", model.Icd10, "C34.901", 10},
		{"乳腺癌", model.Icd10, "C50.901", 10},
		{"糖尿病", model.Icd10, "E11.901", 10},
		{"甲亢", model.Icd10, "E05.901", 9},
		{"肾结石", model.Icd10, "N20.001", 10},
		{"前列腺增生", model.Icd10, "N40.001", 9},
		{"顺产", model.Icd10, "O80.001", 10},
		{"剖宫产", model.Icd10, "O82.001", 10},
		{"脓毒症", model.Icd10, "A41.901", 9},
		// ---------- ICD-9-CM3 手术操作 ----------
		{"阑尾切除", model.Icd9Cm3, "47.0900", 10},
		{"胆囊切除", model.Icd9Cm3, "51.2300", 10},
		{"支架植入", model.Icd9Cm3, "36.0600", 9},
		{"髋关节置换", model.Icd9Cm3, "81.5100", 10},
		{"剖宫产术", model.Icd9Cm3, "74.1000", 10},
		{"肺叶切除", model.Icd9Cm3, "32.4901", 10},
		{"血液透析", model.Icd9Cm3, "39.9500", 10},
		{"前列腺电切", model.Icd9Cm3, "60.2901", 10},
	}
	out := make([]model.CodeSuggestionRule, 0, len(defs))
	for _, d := range defs {
		out = append(out, model.CodeSuggestionRule{
			Keyword: d.keyword, DictType: d.dictType, IcdCode: d.code,
			Weight: d.weight, Enabled: true,
			Remark: "内置规则：关键词命中即推荐，供编码员参考选择（辅助而非替代人工编码）",
		})
	}
	return out
}

// seedDrgGroupRules 内置 DRG 预分组示例规则（约 14 条）。
// 说明：均为自造示例，非官方分组器，仅用于编码质控阶段的入组提示与风险发现。
func seedDrgGroupRules() []model.DrgGroupRule {
	const note = "示例数据，非官方 DRG/DIP 分组器，仅供编码质控参考"
	type def struct {
		diag, op, code, name string
		weight               float64
	}
	defs := []def{
		{"K35", "47.09", "GB25", "阑尾切除术", 0.85},
		{"K80", "51.23", "HC29", "胆囊切除术", 1.05},
		{"I25", "36.06", "FM19", "经皮冠状动脉支架植入", 2.10},
		{"S72", "81.51", "IB29", "髋关节置换术", 1.60},
		{"N20", "55.51", "MD29", "肾结石手术", 0.95},
		{"O82", "74.10", "OB09", "剖宫产", 0.80},
		{"C34", "32.49", "EE29", "肺叶切除术", 1.90},
		{"N40", "60.29", "MC19", "前列腺手术", 0.88},
		{"I10", "", "FR29", "高血压", 0.55},
		{"J18", "", "ES31", "呼吸系统感染", 0.92},
		{"J44", "", "EX15", "慢性阻塞性肺疾病", 0.70},
		{"I63", "", "BR23", "脑缺血性疾病", 0.78},
		{"E11", "", "KS19", "糖尿病", 0.62},
		{"G40", "", "BR33", "癫痫", 0.68},
		{"A41", "", "AG19", "脓毒症", 1.30},
	}
	out := make([]model.DrgGroupRule, 0, len(defs))
	for _, d := range defs {
		out = append(out, model.DrgGroupRule{
			DiagPrefix: d.diag, OpPrefix: d.op, GroupCode: d.code, GroupName: d.name,
			Weight: d.weight, Enabled: true, Remark: note,
		})
	}
	return out
}
