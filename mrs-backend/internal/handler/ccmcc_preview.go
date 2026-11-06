package handler

import (
	"errors"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
	"mrs/internal/service"
	"mrs/pkg/resp"
)

// =====================================================================
// CC/MCC 严重度判读预览（ADR-0002 §2 / §4 / §6，批次 D）
//
// 定位：给前端的「严重度分层预览」与「CC/MCC 命中清单」提供后端出口。判读本身
// 在 service.JudgeSeverity（internal/service/ccmcc.go），本文件只做三件事：
//  1. 入参整形：既支持 {"recordId":123} 从库里读首页，也支持直接传主诊断与其他
//     诊断编码串 —— 后者是给编目阶段首页尚未落库的场景留的口子（先判读、后落库）；
//  2. 其他诊断一律经 service.ParseOtherDiagnoses 解析（ADR §5 的唯一对齐权威）：
//     重复编码 / 长度不等 / 非法标记直接 400，不让错位数据参与判读；
//  3. 原样回传 service.CcMccJudgement（含 items / overallSeverity / mccCount /
//     ccCount / sources / demoOnly / disclaimer）。
//
// 合规红线（ADR §4 / §6）：响应体必须带 disclaimer，且 demoOnly=true 时前端据此
// 显示「非官方、不得作为结算依据」的显著标注。disclaimer 由判读层填充，本文件
// 既不改写也不兜底 —— 在 handler 里再造一份免责声明，等于把"谁对文案负责"搞成
// 两处可写，任一侧悄悄改短都会让合规口径漂移。
//
// 规则表来源：cc_mcc_rule 已落表（service/ccmcc_store.go），故优先按版本读库
// （规则即数据，换表不改代码，ADR §4）；读不到或该版本无数据时降级内置演示表
// service.CcMccDemoRules，与 qcengine.go 的 RunQc 同口径——宁可降级也不让判读断供，
// 且降级用的演示数据一律 Source=demo，响应里的 demoOnly / disclaimer 照样如实标注。
// =====================================================================
type CcMccHandler struct{ DB *gorm.DB }

func NewCcMccHandler(db *gorm.DB) *CcMccHandler { return &CcMccHandler{DB: db} }

// ccMccPreviewRequest 判读预览入参：两种形态二选一，二者都给时以 mainDiagnosisCode 为准
// （显式传码是更强的意图，与 drg_preview.go 的处理一致）。
type ccMccPreviewRequest struct {
	RecordID            int64  `json:"recordId"`            // 病案 ID：从首页取主诊断与其他诊断
	MainDiagnosisCode   string `json:"mainDiagnosisCode"`   // 主诊断编码（ICD-10）
	OtherDiagnosisCodes string `json:"otherDiagnosisCodes"` // 其他诊断编码串，逗号分隔
	OtherDiagnosisFlags string `json:"otherDiagnosisFlags"` // 其他诊断严重度标记串，按逗号位置与编码一一对应
}

// Preview POST /api/ccmcc-preview（登录）
//
// body 形态一：{"recordId":123} —— 从病案首页取主诊断与其他诊断编码；
// body 形态二：{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900,K80.201","otherDiagnosisFlags":"M,0"}。
//
// 响应：resp 外壳下的 service.CcMccJudgement，disclaimer 必带。
func (h *CcMccHandler) Preview(c *gin.Context) {
	var req ccMccPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.RecordID < 0 {
		resp.BadRequest(c, "参数错误: recordId 必须为正整数")
		return
	}

	mainCode := strings.TrimSpace(req.MainDiagnosisCode)
	otherCodes := req.OtherDiagnosisCodes
	otherFlags := req.OtherDiagnosisFlags

	if mainCode == "" && req.RecordID > 0 {
		var fp model.MedFrontPage
		if err := h.DB.Where("record_id = ?", req.RecordID).First(&fp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				resp.NotFound(c, "该病案暂无首页数据，无法进行 CC/MCC 判读")
				return
			}
			resp.ServerErr(c, err)
			return
		}
		mainCode, otherCodes, otherFlags = fp.MainDiagnosisCode, fp.OtherDiagnosisCodes, fp.OtherDiagnosisFlags
		mainCode = strings.TrimSpace(mainCode)
	}
	if mainCode == "" {
		resp.BadRequest(c, "请传 recordId 或 mainDiagnosisCode")
		return
	}

	j, err := judgeCcMccFromCodes(h.DB, mainCode, otherCodes, otherFlags)
	if err != nil {
		resp.BadRequest(c, "其他诊断数据不合法："+err.Error())
		return
	}
	resp.OK(c, j)
}

// judgeCcMccFromCodes 解析其他诊断并对本例做出严重度判读。
//
// recordId 形态与直接传码形态共用此函数，保证两种入参走的是同一条口径
// （一致性由 ccmcc_preview_test.go 的 TestCcMccPreview_RecordIdFormEqualsDirectForm 固化）。
// 这里不碰 disclaimer：它由判读层填，本函数只负责把判读结果原样往上递。
func judgeCcMccFromCodes(db *gorm.DB, mainCode, otherCodes, otherFlags string) (service.CcMccJudgement, error) {
	items, err := service.ParseOtherDiagnoses(otherCodes, otherFlags)
	if err != nil {
		return service.CcMccJudgement{}, err
	}
	return service.JudgeSeverity(
		service.CcMccDemoVersion,
		mainCode,
		service.OtherDiagnosisCodes(items),
		ccMccRulesFor(db, service.CcMccDemoVersion),
	), nil
}

// ccMccRulesFor 取本次判读要用的规则表：优先读 cc_mcc_rule，读不到降级内置演示表
// （与 qcengine.go RunQc 的第 36-41 行同口径，两处降级行为必须一致，否则同一份病案
// 在质控与预览两个入口会给出两个分层）。
//
// db == nil 时直接用演示表：本仓的 handler 单测就是用 nil DB 装配的（见
// ccmcc_preview_test.go 的 nil-DB 技巧），此时库无从读取，不该为了拿规则去碰一个
// 不存在的连接。生产路径 db 恒非 nil，故这条分支只服务于无库装配场景。
func ccMccRulesFor(db *gorm.DB, version string) []service.CcMccRule {
	if db == nil {
		return service.CcMccDemoRules
	}
	loaded, err := service.LoadCcMccRules(db, version)
	if err != nil {
		log.Printf("[ccmcc] cc_mcc_rule 读取失败，本次判读降级使用内置演示表: %v", err)
		return service.CcMccDemoRules
	}
	if len(loaded) == 0 {
		// 表建了但本版本没数据（冷启动 / 还没跑 SeedCcMccRules）：降级，不静默判"无严重度"。
		// 与上面的「报错降级」分开记日志：这条指向"没灌数据"，那条指向"库/表有问题"，
		// 判读数值虽然一样，排障方向完全不同。
		log.Printf("[ccmcc] cc_mcc_rule 版本 %q 查得 0 条，本次判读降级使用内置演示表", version)
		return service.CcMccDemoRules
	}
	return loaded
}
