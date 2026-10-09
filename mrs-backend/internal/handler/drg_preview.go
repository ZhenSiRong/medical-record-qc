package handler

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
	"mrs/pkg/resp"
)

// =====================================================================
// 任务 7：DRG/DIP 预分组（查表版）
//
// 背景：DRG/DIP 支付方式下，编码直接决定支付结果；入组错误会导致拒付
// （歧义组 QY 按 50%、0000 组按 40% 结算）。本系统没有官方分组器，改做
// 「简化查表版」：依据主诊断/主手术编码前缀匹配示例规则，给出分组提示与
// 参考权重，用于编码质控阶段的风险发现。
//
// 重要：本功能为查表提示，非官方 DRG/DIP 分组器，仅供编码质控参考，
// 不作为医保结算依据。正式入组须使用国家医保局发布的官方分组器与权重表。
// =====================================================================
type DrgHandler struct{ DB *gorm.DB }

// drgDisclaimer 免责声明：所有分组结果均需携带，避免被误认为真实分组结果。
const drgDisclaimer = "本功能为查表提示，非官方 DRG/DIP 分组器，仅供编码质控参考，不作为医保结算依据。"

// chronicDiagPrefixes 慢性病/内科类主诊断前缀（简化判断用）：此类主诊断若同时存在
// 主要手术，常提示主诊断与手术不匹配，存在入歧义组(QY)风险。
var chronicDiagPrefixes = []string{"I10", "I25", "I50", "E11", "J44", "G40", "I63"}

func isChronicDiag(code string) bool {
	for _, p := range chronicDiagPrefixes {
		if strings.HasPrefix(code, p) {
			return true
		}
	}
	return false
}

type drgResult struct {
	DiagCode   string  `json:"diagCode"`
	OpCode     string  `json:"opCode"`
	GroupCode  string  `json:"groupCode"`
	GroupName  string  `json:"groupName"`
	Weight     float64 `json:"weight"`
	Matched    bool    `json:"matched"`
	Hint       string  `json:"hint"`
	Disclaimer string  `json:"disclaimer"`
}

// Preview POST /api/drg-preview（登录）
// body: {"recordId":123} 或 {"diagCode":"K35.801","opCode":"47.0900"}
func (h *DrgHandler) Preview(c *gin.Context) {
	var req struct {
		RecordID int64  `json:"recordId"`
		DiagCode string `json:"diagCode"`
		OpCode   string `json:"opCode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	diag := strings.TrimSpace(req.DiagCode)
	op := strings.TrimSpace(req.OpCode)
	if diag == "" && req.RecordID > 0 {
		var fp model.MedFrontPage
		if err := h.DB.Where("record_id = ?", req.RecordID).First(&fp).Error; err != nil {
			resp.NotFound(c, "该病案暂无首页数据，无法进行分组提示")
			return
		}
		diag, op = fp.MainDiagnosisCode, fp.MainOperationCode
	}
	if diag == "" {
		resp.BadRequest(c, "请传 recordId 或 diagCode")
		return
	}
	// ICD-9 字典中 "00.0000 无手术操作" 为占位码，视为无手术。
	if op == "00.0000" {
		op = ""
	}

	var rules []model.DrgGroupRule
	if err := h.DB.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}

	// 选最优匹配：优先「限定了手术前缀」的规则，其次前缀更长的（更具体）。
	var best *model.DrgGroupRule
	bestScore := -1
	for i := range rules {
		r := &rules[i]
		if r.DiagPrefix == "" || !strings.HasPrefix(diag, r.DiagPrefix) {
			continue
		}
		if r.OpPrefix != "" {
			if op == "" || !strings.HasPrefix(op, r.OpPrefix) {
				continue
			}
		}
		score := len(r.DiagPrefix) + len(r.OpPrefix)
		if r.OpPrefix != "" {
			score += 100
		}
		if score > bestScore {
			bestScore = score
			best = r
		}
	}

	out := drgResult{DiagCode: diag, OpCode: op, Disclaimer: drgDisclaimer}
	risk := isChronicDiag(diag) && op != "" && (best == nil || best.OpPrefix == "")

	if best == nil {
		// 匹配不到规则本身就是有价值的信号：可能意味着编码不完整或主诊断/主手术填写缺失。
		out.Matched = false
		out.Hint = "未匹配到分组规则，请核对主诊断/主手术填写（编码不完整或填写缺失会导致无法入组）。"
	} else {
		out.Matched = true
		out.GroupCode = best.GroupCode
		out.GroupName = best.GroupName
		out.Weight = best.Weight
		out.Hint = fmt.Sprintf("匹配到示例分组规则：%s %s（主诊断前缀 %s%s），参考权重 %.2f。示例数据仅作提示。",
			best.GroupCode, best.GroupName, best.DiagPrefix, opPart(best.OpPrefix), best.Weight)
	}
	if risk {
		out.Hint += " 注意：主诊断为慢性病编码但存在主要手术，主诊断与手术可能不匹配，存在入歧义组(QY)风险，请复核主诊断/主手术。"
	}
	resp.OK(c, out)
}

func opPart(opPrefix string) string {
	if opPrefix == "" {
		return ""
	}
	return "+手术前缀 " + opPrefix
}

// ---- 分组规则 CRUD：登录读、ADMIN 写 ----

// RulesPage GET /api/drg-group-rules/page（登录）
func (h *DrgHandler) RulesPage(c *gin.Context) {
	q := h.DB.Model(&model.DrgGroupRule{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("diag_prefix ILIKE ? OR op_prefix ILIKE ? OR group_code ILIKE ? OR group_name ILIKE ?",
			like(kw), like(kw), like(kw), like(kw))
	}
	resp.OK(c, resp.Page[model.DrgGroupRule](c, q))
}

// RuleCreate POST /api/drg-group-rules（ADMIN）
func (h *DrgHandler) RuleCreate(c *gin.Context) {
	var r model.DrgGroupRule
	if err := c.ShouldBindJSON(&r); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if r.DiagPrefix == "" || r.GroupCode == "" {
		resp.BadRequest(c, "diagPrefix / groupCode 必填")
		return
	}
	if r.Remark == "" {
		r.Remark = "示例数据，非官方分组器"
	}
	if err := h.DB.Create(&r).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, r)
}

// RuleUpdate PUT /api/drg-group-rules/:id（ADMIN）
func (h *DrgHandler) RuleUpdate(c *gin.Context) {
	var r model.DrgGroupRule
	if err := c.ShouldBindJSON(&r); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	res := h.DB.Model(&model.DrgGroupRule{}).Where("id = ?", c.Param("id")).Updates(map[string]any{
		"diag_prefix": r.DiagPrefix,
		"op_prefix":   r.OpPrefix,
		"group_code":  r.GroupCode,
		"group_name":  r.GroupName,
		"weight":      r.Weight,
		"remark":      r.Remark,
		"enabled":     r.Enabled,
	})
	if res.Error != nil {
		resp.ServerErr(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		resp.NotFound(c, "规则不存在")
		return
	}
	resp.OK(c, nil)
}

// RuleDelete DELETE /api/drg-group-rules/:id（ADMIN）
func (h *DrgHandler) RuleDelete(c *gin.Context) {
	if err := h.DB.Delete(&model.DrgGroupRule{}, c.Param("id")).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, nil)
}
