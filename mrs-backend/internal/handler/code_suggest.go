package handler

import (
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
	"mrs/pkg/resp"
)

// =====================================================================
// 任务 3：智能编码辅助（规则版预编码）
//
// 背景：编码员日均需处理 44~56 份病案，人力缺口数十倍，"AI 预编码"是行业公认的
// 减负方向。本系统不具备 AI 能力，改做「规则版推荐」：依据病历文本关键词命中规则，
// 推荐候选 ICD 编码，供编码员选择而非替代。
//
// 设计意图（务必保留）：这是「编码辅助」而非「自动编码」。国家规范要求编码员依据
// 病案内容自主判断并承担编码责任，系统只做推荐提示。
// =====================================================================
type CodeSuggestHandler struct{ DB *gorm.DB }

type codeSuggestItem struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	MatchedKeyword string `json:"matchedKeyword"`
	Weight         int    `json:"weight"`
}

// Suggest POST /api/code-suggest（登录）
// body: {"text":"病历文本或诊断名称","type":"ICD10|ICD9CM3(可选)"}
// 返回按权重降序排序的推荐列表（最多 10 条）。
func (h *CodeSuggestHandler) Suggest(c *gin.Context) {
	var req struct {
		Text string `json:"text" binding:"required"`
		Type string `json:"type"` // 可选：限定编码类型
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "请传 text（病历文本或诊断名称）")
		return
	}

	q := h.DB.Where("enabled = ?", true)
	if req.Type != "" {
		q = q.Where("dict_type = ?", req.Type)
	}
	var rules []model.CodeSuggestionRule
	if err := q.Order("weight DESC").Find(&rules).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}

	// 关键词命中即推荐；同一编码去重并取最大权重。
	type cand struct {
		code     string
		dictType string
		keyword  string
		weight   int
	}
	seen := make(map[string]int) // dictType|code -> cands 下标
	var cands []cand
	for _, r := range rules {
		if !strings.Contains(req.Text, r.Keyword) {
			continue
		}
		key := r.DictType + "|" + r.IcdCode
		if i, ok := seen[key]; ok {
			if r.Weight > cands[i].weight {
				cands[i].weight = r.Weight
			}
			continue
		}
		seen[key] = len(cands)
		cands = append(cands, cand{code: r.IcdCode, dictType: r.DictType, keyword: r.Keyword, weight: r.Weight})
	}

	// 从 icd_dict 反查编码名称（保证推荐编码与系统字典一致）。
	nameMap := make(map[string]string) // dictType|code -> name
	if len(cands) > 0 {
		codes := make([]string, 0, len(cands))
		for _, cd := range cands {
			codes = append(codes, cd.code)
		}
		var dicts []model.IcdDict
		_ = h.DB.Where("code IN ?", codes).Find(&dicts).Error
		for _, d := range dicts {
			nameMap[d.DictType+"|"+d.Code] = d.Name
		}
	}

	out := make([]codeSuggestItem, 0, len(cands))
	for _, cd := range cands {
		out = append(out, codeSuggestItem{
			Code:           cd.code,
			Name:           nameMap[cd.dictType+"|"+cd.code],
			MatchedKeyword: cd.keyword,
			Weight:         cd.weight,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Weight > out[j].Weight })
	if len(out) > 10 {
		out = out[:10]
	}
	resp.OK(c, out)
}

// RulesPage GET /api/code-suggestion-rules/page（登录）
func (h *CodeSuggestHandler) RulesPage(c *gin.Context) {
	q := h.DB.Model(&model.CodeSuggestionRule{})
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("keyword ILIKE ? OR icd_code ILIKE ?", like(kw), like(kw))
	}
	if t := c.Query("type"); t != "" {
		q = q.Where("dict_type = ?", t)
	}
	resp.OK(c, resp.Page[model.CodeSuggestionRule](c, q))
}

// RuleCreate POST /api/code-suggestion-rules（ADMIN）
func (h *CodeSuggestHandler) RuleCreate(c *gin.Context) {
	var r model.CodeSuggestionRule
	if err := c.ShouldBindJSON(&r); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if r.Keyword == "" || r.DictType == "" || r.IcdCode == "" {
		resp.BadRequest(c, "keyword / dictType / icdCode 必填")
		return
	}
	if r.Weight == 0 {
		r.Weight = 10
	}
	if err := h.DB.Create(&r).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, r)
}

// RuleUpdate PUT /api/code-suggestion-rules/:id（ADMIN）
// 采用 map 更新，保证 enabled=false、weight=0 等零值也能被写入。
func (h *CodeSuggestHandler) RuleUpdate(c *gin.Context) {
	var r model.CodeSuggestionRule
	if err := c.ShouldBindJSON(&r); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	res := h.DB.Model(&model.CodeSuggestionRule{}).Where("id = ?", c.Param("id")).Updates(map[string]any{
		"keyword":   r.Keyword,
		"dict_type": r.DictType,
		"icd_code":  r.IcdCode,
		"weight":    r.Weight,
		"enabled":   r.Enabled,
		"remark":    r.Remark,
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

// RuleDelete DELETE /api/code-suggestion-rules/:id（ADMIN）
func (h *CodeSuggestHandler) RuleDelete(c *gin.Context) {
	if err := h.DB.Delete(&model.CodeSuggestionRule{}, c.Param("id")).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, nil)
}
