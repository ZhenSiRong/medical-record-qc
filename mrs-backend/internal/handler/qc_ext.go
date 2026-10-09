package handler

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

// 本文件为质控域扩展（任务4/5/6/16/17）的独立实现：
//   - 任务4  条件必填（规则引擎在 service 层实现，这里只补示例规则种子）
//   - 任务5  质控退回返修闭环（return / rectify / review + 质控工作台）
//   - 任务6  DRG 主诊断-主手术匹配、主要诊断选择提示（示例规则种子）
//   - 任务16 病历质量评分与等级查询
//   - 任务17 归档锁定辅助 + 首页变更留痕与历史查询
// 所有新增路由通过 RegisterQcExt 统一注册（router.go 由主干统一调用），
// 新增数据表统一由 main.go 的 AutoMigrate 负责。

// QcExtHandler 质控域扩展处理器
type QcExtHandler struct {
	DB *gorm.DB
}

// RegisterQcExt 注册质控域扩展路由（纯注册，不做迁移）。
// 约定：由 router.Register 在既有首页路由之后调用，避免与主干 router.go 冲突。
func RegisterQcExt(auth *gin.RouterGroup, admin *gin.RouterGroup, db *gorm.DB) {
	h := &QcExtHandler{DB: db}

	// 任务16：质量评分查询
	auth.GET("/front-pages/:recordId/quality", h.Quality)
	// 任务5：质控工作台视图
	auth.GET("/front-pages/:recordId/qc-workbench", h.QcWorkbench)
	// 任务17：首页变更历史
	auth.GET("/front-pages/:recordId/revisions", h.Revisions)

	// 任务5：退回返修闭环（退回/复核为病案室操作，整改为临床用户操作）
	admin.POST("/qc-results/:id/return", h.Return)
	auth.POST("/qc-results/:id/rectify", h.Rectify)
	admin.POST("/qc-results/:id/review", h.Review)
}

// ---------- 任务16：质量评分查询 ----------

// Quality 查询某病案首页最新质量评分与等级。
func (h *QcExtHandler) Quality(c *gin.Context) {
	var q model.FrontPageQuality
	if err := h.DB.Where("record_id = ?", c.Param("recordId")).First(&q).Error; err != nil {
		resp.NotFound(c, "尚无质量评分，请先执行质控")
		return
	}
	resp.OK(c, q)
}

// ---------- 任务5：质控工作台视图 ----------

// QcWorkbench 质控工作台：返回该病案首页数据 + 全部质控项（含处理状态与评审意见）+ 质量评分等级。
func (h *QcExtHandler) QcWorkbench(c *gin.Context) {
	var fp model.MedFrontPage
	if err := h.DB.Where("record_id = ?", c.Param("recordId")).First(&fp).Error; err != nil {
		resp.NotFound(c, "该病案尚无首页数据")
		return
	}
	var results []model.QcResult
	h.DB.Where("record_id = ?", fp.RecordID).Order("id ASC").Find(&results)

	var quality *model.FrontPageQuality
	var q model.FrontPageQuality
	if err := h.DB.Where("record_id = ?", fp.RecordID).First(&q).Error; err == nil {
		quality = &q
	}
	resp.OK(c, gin.H{
		"frontPage":       fp,
		"qcResults":       results,
		"quality":         quality,
		"frontPageStatus": fp.Status,
	})
}

// ---------- 任务5：退回 / 整改 / 复核 ----------

type reviewReq struct {
	Result  string `json:"result"` // PASS | FAIL
	Opinion string `json:"opinion"`
}

// Return 退回该项质控给临床（置 RETURNED，记录退回时间/操作人/意见）。
func (h *QcExtHandler) Return(c *gin.Context) {
	var req struct {
		Opinion string `json:"opinion"`
	}
	_ = c.ShouldBindJSON(&req)
	_, username, _ := middleware.CurrentUser(c)

	item, err := h.loadResult(c)
	if err != nil {
		return
	}
	if item.HandleStatus == model.HandleClosed {
		resp.Conflict(c, "该质控项已闭环，无需退回")
		return
	}
	now := time.Now()
	res := h.DB.Model(&model.QcResult{}).Where("id = ?", item.ID).Updates(map[string]any{
		"handle_status":  model.HandleReturned,
		"return_opinion": req.Opinion,
		"returned_by":    username,
		"returned_at":    &now,
	})
	if res.Error != nil {
		resp.ServerErr(c, res.Error)
		return
	}
	resp.OK(c, gin.H{"msg": "已退回临床整改"})
}

// Rectify 临床整改（模拟）：置 RECTIFIED，记录整改人与时间。
func (h *QcExtHandler) Rectify(c *gin.Context) {
	_, username, _ := middleware.CurrentUser(c)

	item, err := h.loadResult(c)
	if err != nil {
		return
	}
	if item.HandleStatus == model.HandleClosed {
		resp.Conflict(c, "该质控项已闭环")
		return
	}
	now := time.Now()
	res := h.DB.Model(&model.QcResult{}).Where("id = ?", item.ID).Updates(map[string]any{
		"handle_status": model.HandleRectified,
		"rectified_by":  username,
		"rectified_at":  &now,
	})
	if res.Error != nil {
		resp.ServerErr(c, res.Error)
		return
	}
	resp.OK(c, gin.H{"msg": "已提交整改，待病案室复核"})
}

// Review 病案室复核：PASS → CLOSED，FAIL → REJECTED_BY_QC（退回临床），记录复核意见。
func (h *QcExtHandler) Review(c *gin.Context) {
	var req reviewReq
	if err := c.ShouldBindJSON(&req); err != nil || (req.Result != "PASS" && req.Result != "FAIL") {
		resp.BadRequest(c, "请传 result（PASS 或 FAIL）")
		return
	}
	_, username, _ := middleware.CurrentUser(c)

	item, err := h.loadResult(c)
	if err != nil {
		return
	}
	if item.HandleStatus == model.HandleClosed {
		resp.Conflict(c, "该质控项已复核闭合，无需重复复核")
		return
	}
	status := model.HandleClosed
	if req.Result == "FAIL" {
		status = model.HandleRejectedByQc
	}
	now := time.Now()
	res := h.DB.Model(&model.QcResult{}).Where("id = ?", item.ID).Updates(map[string]any{
		"handle_status":  status,
		"review_opinion": req.Opinion,
		"reviewed_by":    username,
		"reviewed_at":    &now,
	})
	if res.Error != nil {
		resp.ServerErr(c, res.Error)
		return
	}
	resp.OK(c, gin.H{"handleStatus": status})
}

// loadResult 按 :id 加载质控项，失败时已写出响应，返回 err 供调用方直接 return。
func (h *QcExtHandler) loadResult(c *gin.Context) (*model.QcResult, error) {
	var item model.QcResult
	if err := h.DB.First(&item, c.Param("id")).Error; err != nil {
		resp.NotFound(c, "质控结果不存在")
		return nil, err
	}
	return &item, nil
}

// ---------- 任务17：首页变更历史 ----------

// Revisions 查看某病案首页的变更历史（分页）。
func (h *QcExtHandler) Revisions(c *gin.Context) {
	q := h.DB.Model(&model.FrontPageRevision{}).Where("record_id = ?", c.Param("recordId"))
	resp.OK(c, resp.Page[model.FrontPageRevision](c, q))
}

// recordRevision 写入一条首页变更留痕（在首页保存事务内调用）。
// before/after 为首页数据快照；依据《电子病历应用管理规范（试行）》保留修改痕迹。
func recordRevision(tx *gorm.DB, recordID int64, before, after *model.MedFrontPage, changedBy, reason string) error {
	bj, _ := json.Marshal(before)
	aj, _ := json.Marshal(after)
	return tx.Create(&model.FrontPageRevision{
		RecordID:   recordID,
		BeforeJSON: string(bj),
		AfterJSON:  string(aj),
		ChangedBy:  changedBy,
		Reason:     reason,
		ChangedAt:  time.Now(),
	}).Error
}

// changeReason 从请求头或查询参数读取变更事由（可选）。
func changeReason(c *gin.Context) string {
	if r := c.GetHeader("X-Change-Reason"); r != "" {
		return r
	}
	if r := c.Query("reason"); r != "" {
		return r
	}
	return "首页数据修改"
}

// isArchivedLocked 判断病案是否处于"归档后禁止修改"状态。
// 依据《电子病历应用管理规范（试行）》（国卫办医发〔2017〕8号）：归档后原则上不得修改。
func isArchivedLocked(status string) bool {
	switch status {
	case model.RecArchived, model.RecBorrowed, model.RecSealed:
		return true
	default:
		return false
	}
}

// recStatusName 病案状态中文名，用于锁定提示。
func recStatusName(status string) string {
	switch status {
	case model.RecNotRecycled:
		return "未回收"
	case model.RecRecycled:
		return "已回收待编目"
	case model.RecCataloged:
		return "已编目"
	case model.RecArchived:
		return "已归档"
	case model.RecBorrowed:
		return "已借出"
	case model.RecSealed:
		return "已封存"
	default:
		return status
	}
}

// ---------- 种子数据（任务4/6 示例规则，幂等） ----------

// SeedQcExt 灌入质控扩展示例规则（按 rule_name 幂等：已存在则跳过）。
// 由主干统一调用（不改 seed.go）。
func SeedQcExt(db *gorm.DB) error {
	rules := []model.QcRule{
		// 任务4：条件必填示例
		{
			RuleName: "损伤中毒类主诊断需填外部原因（条件必填）", RuleType: model.QcRequired,
			TargetField: "OtherDiagnosisCodes", ConditionExpr: "main_diagnosis_code LIKE 'S%'",
			ErrorLevel: "WARN", Enabled: true,
			Remark: "依据《住院病案首页数据采集质量与接口标准（2020版）》：损伤中毒（S/T 编码）须填写外部原因。当前模型无独立外部原因字段，暂以 other_diagnosis_codes 作占位示例，生产环境应改为外部原因专用字段。",
		},
		{
			RuleName: "新生儿需填出生体重/孕周（条件必填）", RuleType: model.QcRequired,
			TargetField: "ExtJSON", ConditionExpr: "main_diagnosis_code LIKE 'P%'",
			ErrorLevel: "WARN", Enabled: true,
			Remark: "依据新生儿病案首页数据元要求（2020版接口标准）：新生儿（P 编码）须填出生体重/孕周。当前模型无独立字段，暂以 ext_json 承载作占位示例。",
		},
		{
			RuleName: "有住院天数则出院诊断必填（条件必填）", RuleType: model.QcRequired,
			TargetField: "DischargeDiagnosis", ConditionExpr: "actual_days>0",
			ErrorLevel: "WARN", Enabled: true,
			Remark: "条件必填示例（复用 LOGIC 表达式形式）：实际住院天数>0 时出院诊断必填。",
		},

		// 任务6：DRG 主诊断-主手术匹配（防 QY 歧义组）
		{
			RuleName: "主诊断-主手术匹配（DRG 防 QY）", RuleType: model.QcDiagOpMatch,
			TargetField: "MainDiagnosisCode", Expression: "", ErrorLevel: "WARN", Enabled: true,
			Remark: "依据医保办发〔2024〕9号：主要诊断与主要手术操作不匹配将落入 DRG 歧义组(QY)，按 50% 折付。判定简表见 service/drg_rules.go。",
		},
		// 任务6：主要诊断选择合理性提示
		{
			RuleName: "主要诊断选择合理性提示", RuleType: model.QcMainDiagRule,
			TargetField: "MainDiagnosisCode", ErrorLevel: "WARN", Enabled: true,
			Remark: "依据《住院病案首页数据填写质量规范（暂行）》（国卫办医发〔2016〕24号）主要诊断选择原则：慢性病主诊断+本次有手术、后遗症/恢复期类编码作主诊断时提示核对。",
		},
	}

	for _, r := range rules {
		var cnt int64
		if err := db.Model(&model.QcRule{}).Where("rule_name = ?", r.RuleName).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			continue // 已存在，跳过（幂等）
		}
		if err := db.Create(&r).Error; err != nil {
			return err
		}
		log.Printf("[qc-ext] 已补入质控扩展规则: %s", r.RuleName)
	}
	return nil
}
