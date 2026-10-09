package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

// 角色码常量：取值与 sys_user.role_code、JWT 的 role 载荷一致（见 internal/seed/seed.go）。
// 缺口 D4 用到：整改提交只对「临床侧」放行（DEPT_USER），并保留 ADMIN（病案室）兼容入口。
const (
	roleAdmin    = "ADMIN"     // 病案室管理员
	roleDeptUser = "DEPT_USER" // 科室医师（临床侧）
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

	// 任务5：退回返修闭环（退回/复核为病案室操作）
	admin.POST("/qc-results/:id/return", h.Return)
	admin.POST("/qc-results/:id/review", h.Review)

	// 缺口 D4（临床端整改）：待整改清单（只读，按角色限定数据范围，见 MyPending）。
	auth.GET("/qc-results/my-pending", h.MyPending)

	// 缺口 D4（权限收紧）：rectify 原挂在 auth 组（任意登录用户），现收敛为「临床侧」子组。
	// 口径 = DEPT_USER（真实整改人）+ ADMIN（病案室），理由：
	//   1) DEPT_USER 是本接口的目标用户（医生端「我的待整改」提交整改）；
	//   2) 必须保留 ADMIN —— frontend/js/components/catalog.js 的质控工作台里，
	//      病案室角色通过 qcRectifyItem 调本接口做「模拟整改」演示（该文件属他人维护范围，
	//      本任务不改动）。若只放行 DEPT_USER，该按钮会立刻 403，形成"改了后端导致现有按钮失效"的坑。
	//      语义上 ADMIN 也只是「代临床登记整改结果」，写入的是整改人与时间，不越权改质控结论。
	//   3) 其它角色码（后续批次若引入 AUDITOR 等）一律 403 —— 比原来的"任意登录用户"确有收紧。
	// 除角色外，Rectify 处理函数内还对 DEPT_USER 追加「本科室病案」归属校验（数据范围 DEPT）。
	// 使用 auth.Group("") + Use(RequireRole(...)) 而非改 router.go，保持本文件自包含。
	clinical := auth.Group("")
	clinical.Use(middleware.RequireRole(roleDeptUser, roleAdmin))
	clinical.POST("/qc-results/:id/rectify", h.Rectify)
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

// rectifyReq 整改提交请求体。Opinion 为可选整改说明（缺口 D4）；
// 保持"空请求体 / {} 也能调用"的既有契约，故不设 binding:"required"。
type rectifyReq struct {
	Opinion string `json:"opinion"` // 可选：整改说明（临床已补充/修正的内容）
}

// Rectify 临床整改提交：置 RECTIFIED，记录整改人与时间。
// 权限（缺口 D4 收紧）：路由层 RequireRole(DEPT_USER, ADMIN)（见 RegisterQcExt）；
// 处理函数内对 DEPT_USER 追加「本科室病案」归属校验——DEPT_USER 的数据范围是本科室
// （sys_role.data_scope=DEPT），不允许跨科室提交他科病案的整改。
// ADMIN 为病案室督办/模拟入口，不做科室限制。
func (h *QcExtHandler) Rectify(c *gin.Context) {
	var req rectifyReq
	_ = c.ShouldBindJSON(&req) // 允许无请求体或 {}：整改说明为可选字段

	uid, username, role := middleware.CurrentUser(c)

	item, err := h.loadResult(c)
	if err != nil {
		return
	}
	if item.HandleStatus == model.HandleClosed {
		resp.Conflict(c, "该质控项已闭环")
		return
	}
	if role != roleAdmin {
		var user model.SysUser
		if err := h.DB.First(&user, uid).Error; err != nil {
			resp.NotFound(c, "登录用户不存在，请重新登录")
			return
		}
		var rec model.MedRecord
		if err := h.DB.First(&rec, item.RecordID).Error; err != nil {
			resp.NotFound(c, "该质控项关联的病案不存在")
			return
		}
		if !deptScopeMatches(user.DeptName, user.DeptID, &rec) {
			resp.Forbidden(c, "该病案不属于您所在科室，无法提交整改")
			return
		}
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

	// 整改说明留痕（可选）：front_page_qc_result 没有「整改说明」列，而本任务不改动
	// internal/model/record.go（文件所有权约定，避免与并行批次冲突），故复用既有的
	// front_page_revision 变更留痕表把医生填写的说明落库（reason 承载），
	// 使病案室复核时可按时间线看到"临床改了什么"。Before/After 快照置 null：
	// 本接口不改首页数据快照（首页数据修正仍走既有的 PUT /front-pages/:recordId）。
	// 后续若允许改模型，建议在 front_page_qc_result 增加 rectify_opinion 列并在质控工作台展示。
	if opinion := strings.TrimSpace(req.Opinion); opinion != "" {
		reason := fmt.Sprintf("【临床整改说明】质控项 #%d（%s）：%s", item.ID, item.FieldName, opinion)
		if err := recordRevision(h.DB, item.RecordID, nil, nil, username, reason); err != nil {
			// 留痕失败不阻断整改主流程（主状态已落库），仅记录日志便于排查。
			log.Printf("[qc-ext] 整改说明留痕写入失败 record=%d qc=%d: %v", item.RecordID, item.ID, err)
		}
	}

	resp.OK(c, gin.H{
		"msg":          "已提交整改，待病案室复核",
		"handleStatus": model.HandleRectified,
		"recordId":     item.RecordID,
		"rectifiedBy":  username,
	})
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

// ---------- 缺口 D4：临床端「我的待整改」 ----------

// pendingClinicalStatuses 「需临床处理」的质控项处理状态集合。
//   - RETURNED       病案室已退回，等待临床整改；
//   - REJECTED_BY_QC 病案室复核不通过，再次退回临床。
//
// 刻意不含：OPEN（尚未退回，球在病案室）、RECTIFIED（已提交整改，球在病案室）、
// CLOSED（已闭环）。与 frontend/js/components/catalog.js 的质控工作台状态机一致。
func pendingClinicalStatuses() []string {
	return []string{model.HandleReturned, model.HandleRejectedByQc}
}

// needsClinicalAction 判断某处理状态是否属于「需临床处理」。
func needsClinicalAction(status string) bool {
	return status == model.HandleReturned || status == model.HandleRejectedByQc
}

// myPendingQcItem 待整改列表项：质控项字段 + 病案展示字段（一次查全，前端不再 N+1 回查）。
type myPendingQcItem struct {
	ID          int64  `json:"id"`
	RecordID    int64  `json:"recordId"`
	RecordNo    string `json:"recordNo"`
	PatientName string `json:"patientName"`
	Gender      string `json:"gender"`
	DeptName    string `json:"deptName"`
	DoctorName  string `json:"doctorName"`
	// 出院时间用于临床排优先级（越久越急）。
	DischargeTime *time.Time `json:"dischargeTime"`

	FieldName     string     `json:"fieldName"`     // 问题所在字段（首页字段名）
	RuleName      string     `json:"ruleName"`      // 命中的质控规则名（qc_rule.rule_name）
	ErrorLevel    string     `json:"errorLevel"`    // ERROR / WARN
	ErrorMsg      string     `json:"errorMsg"`      // 问题描述
	HandleStatus  string     `json:"handleStatus"`  // RETURNED / REJECTED_BY_QC
	ReturnOpinion string     `json:"returnOpinion"` // 病案室退回意见
	ReturnedBy    string     `json:"returnedBy"`
	ReturnedAt    *time.Time `json:"returnedAt"`
	// 复核信息：handleStatus=REJECTED_BY_QC 时最有价值的是「复核不通过原因」——
	// 注意 Review(FAIL) 只写 review_opinion/reviewed_at，不会刷新 returned_at，
	// 故这类项的"最近一次反馈时间"应取 reviewed_at（前端据此计算已等待天数）。
	ReviewOpinion string     `json:"reviewOpinion"`
	ReviewedBy    string     `json:"reviewedBy"`
	ReviewedAt    *time.Time `json:"reviewedAt"`
}

// MyPending GET /api/qc-results/my-pending（登录）临床端「我的待整改」。
//
// 返回本科室病案上处于「需临床处理」状态（RETURNED / REJECTED_BY_QC）的质控项，
// 并带上病案号/患者/科室/字段/问题描述/退回意见/退回时间等列表展示字段。
//
// 科室匹配口径（依据，重要）：
//  1. 主口径为 med_record.dept_name = sys_user.dept_name 的**文本匹配**。
//     不用 dept_id 的原因：种子库中 deptuser 的 sys_user.dept_id 为 0（seed.go 未回填），
//     而病案主档/首页/借阅的科室口径统一为 deptName，文本匹配是当前数据下唯一稳定可用的口径。
//  2. 兜底口径：登录用户 dept_name 为空但 dept_id > 0 时，退化为 med_record.dept_id 匹配
//     （覆盖"只填了科室 ID"的账号）。
//  3. 两者皆空：视为无科室归属，返回空清单 + hint，**不返回全院数据**（最小权限，fail-closed）。
//
// ADMIN 的处理方式（取舍）：ADMIN（病案室）返回全院待整改清单，scope=ALL。
//   - 理由：病案室是整改督办方，需要看到全院待办才能按科室催办（本页对 ADMIN 即"整改督办"视角）；
//   - 安全性：本接口只读、不改任何状态，放行 ADMIN 不产生越权写；
//   - 临床侧（DEPT_USER）严格限定本科室，避免跨科查看。
func (h *QcExtHandler) MyPending(c *gin.Context) {
	uid, _, role := middleware.CurrentUser(c)

	var user model.SysUser
	if err := h.DB.First(&user, uid).Error; err != nil {
		resp.NotFound(c, "登录用户不存在，请重新登录")
		return
	}

	q := h.DB.Model(&model.QcResult{}).Where("handle_status IN ?", pendingClinicalStatuses())
	scope := "DEPT" // DEPT 本科室 / ALL 全院（ADMIN 督办）
	switch {
	case role == roleAdmin:
		scope = "ALL"
	case user.DeptName != "":
		sub := h.DB.Model(&model.MedRecord{}).Select("id").Where("dept_name = ?", user.DeptName)
		q = q.Where("record_id IN (?)", sub)
	case user.DeptID > 0:
		sub := h.DB.Model(&model.MedRecord{}).Select("id").Where("dept_id = ?", user.DeptID)
		q = q.Where("record_id IN (?)", sub)
	default:
		resp.OK(c, gin.H{
			"scope": "NONE", "deptName": "", "count": 0,
			"handleStatuses": pendingClinicalStatuses(),
			"list":           []myPendingQcItem{},
			"hint":           "当前账号未设置所属科室，无法定位待整改病案，请联系病案室维护账号科室信息",
		})
		return
	}

	var items []model.QcResult
	// 退回时间升序：最早退回的排最前（待办优先级）；PG 中 ASC 的 NULL 排在末尾。
	if err := q.Order("returned_at ASC, id ASC").Find(&items).Error; err != nil {
		resp.ServerErr(c, err)
		return
	}
	// 防御性过滤：用状态机口径（needsClinicalAction）再核一遍，
	// 保证只把「球在临床一侧」的项交给医生，不混入球在病案室一侧的项。
	kept := items[:0]
	for _, it := range items {
		if needsClinicalAction(it.HandleStatus) {
			kept = append(kept, it)
		}
	}
	items = kept

	// 批量补齐病案展示字段与规则名（各 1 次 IN 查询，避免逐条回查）。
	recIDs := make([]int64, 0, len(items))
	ruleIDs := make([]int64, 0, len(items))
	for _, it := range items {
		recIDs = append(recIDs, it.RecordID)
		if it.RuleID > 0 {
			ruleIDs = append(ruleIDs, it.RuleID)
		}
	}
	recByID := make(map[int64]model.MedRecord, len(recIDs))
	if len(recIDs) > 0 {
		var recs []model.MedRecord
		if err := h.DB.Where("id IN ?", recIDs).Find(&recs).Error; err != nil {
			resp.ServerErr(c, err)
			return
		}
		for _, r := range recs {
			recByID[r.ID] = r
		}
	}
	ruleNameByID := make(map[int64]string, len(ruleIDs))
	if len(ruleIDs) > 0 {
		var rules []model.QcRule
		if err := h.DB.Where("id IN ?", ruleIDs).Find(&rules).Error; err != nil {
			resp.ServerErr(c, err)
			return
		}
		for _, r := range rules {
			ruleNameByID[r.ID] = r.RuleName
		}
	}

	list := make([]myPendingQcItem, 0, len(items))
	for _, it := range items {
		rec := recByID[it.RecordID] // 理论必有；数据异常时展示字段留空，不丢质控项
		list = append(list, myPendingQcItem{
			ID: it.ID, RecordID: it.RecordID, RecordNo: rec.RecordNo,
			PatientName: rec.PatientName, Gender: rec.Gender,
			DeptName: rec.DeptName, DoctorName: rec.DoctorName,
			DischargeTime: rec.DischargeTime,
			FieldName:     it.FieldName, RuleName: ruleNameByID[it.RuleID],
			ErrorLevel: it.ErrorLevel, ErrorMsg: it.ErrorMsg,
			HandleStatus: it.HandleStatus, ReturnOpinion: it.ReturnOpinion,
			ReturnedBy: it.ReturnedBy, ReturnedAt: it.ReturnedAt,
			ReviewOpinion: it.ReviewOpinion, ReviewedBy: it.ReviewedBy, ReviewedAt: it.ReviewedAt,
		})
	}

	resp.OK(c, gin.H{
		"scope":          scope,
		"deptName":       user.DeptName,
		"count":          len(list),
		"handleStatuses": pendingClinicalStatuses(),
		"list":           list,
	})
}

// deptScopeMatches 病案是否属于登录用户的科室（口径与 MyPending 注释一致）。
// dept_name 非空按科室名文本匹配；否则退化为 dept_id；两者皆空视为不匹配（fail-closed）。
func deptScopeMatches(deptName string, deptID int64, rec *model.MedRecord) bool {
	if rec == nil {
		return false
	}
	if deptName != "" {
		return rec.DeptName == deptName
	}
	if deptID > 0 {
		return rec.DeptID == deptID
	}
	return false
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
//
// ⚠ 升级说明（缺口 D3，2026-10-09）：
// 判重键是 rule_name，因此**对已经跑过种子的库，本函数不会更新既有规则行**——
// 历史库里那两条条件必填规则仍是占位版（target_field 为 other_diagnosis_codes / ext_json）。
// 需要手工执行（或删库重建）：
//
//	UPDATE qc_rule SET target_field='ExternalCauseCode', condition_expr='main_diagnosis_code LIKE ''S%''',
//	  remark='依据《住院病案首页数据采集质量与接口标准（2020版）》：损伤中毒的外部原因（第二十章 V01–Y98）为条件必填，主诊断命中 S 编码时须填写外部原因编码。'
//	  WHERE rule_name='损伤中毒类主诊断需填外部原因（条件必填）';
//	UPDATE qc_rule SET target_field='NewbornBirthWeight', condition_expr='main_diagnosis_code LIKE ''P%''',
//	  remark='依据《住院病案首页数据采集质量与接口标准（2020版）》：新生儿（P 编码）出生体重为条件必填，主诊断命中 P 编码时须填写新生儿出生体重（克）。'
//	  WHERE rule_name='新生儿需填出生体重/孕周（条件必填）';
//
// 新增的「新生儿需填孕周（条件必填）」规则在历史库中会按 rule_name 自动补入，无需手工处理；
// 上表两条历史行若不更新，其占位校验（校验 other_diagnosis_codes / ext_json 非空）仍会照旧生效。
func SeedQcExt(db *gorm.DB) error {
	rules := []model.QcRule{
		// 任务4：条件必填（D3 改为指向真实字段）
		{
			RuleName: "损伤中毒类主诊断需填外部原因（条件必填）", RuleType: model.QcRequired,
			TargetField: "ExternalCauseCode", ConditionExpr: "main_diagnosis_code LIKE 'S%'",
			ErrorLevel: "WARN", Enabled: true,
			Remark: "依据《住院病案首页数据采集质量与接口标准（2020版）》：损伤中毒的外部原因（ICD-10 第二十章 V01–Y98）为条件必填，主诊断编码命中 S 时须填写 external_cause_code（外部原因编码；external_cause_name 为配对名称，不作为校验目标）。" +
				"标准要求 S 或 T 编码，但条件表达式引擎（service/qc_condition.go 的 likeCondRe）仅支持单前缀 LIKE，无 OR/多前缀能力；" +
				"为避免为本条规则扩展表达式引擎，暂只匹配 S%，T 编码的覆盖待后续批次为条件表达式引入 OR 支持后再补。",
		},
		{
			RuleName: "新生儿需填出生体重/孕周（条件必填）", RuleType: model.QcRequired,
			TargetField: "NewbornBirthWeight", ConditionExpr: "main_diagnosis_code LIKE 'P%'",
			ErrorLevel: "WARN", Enabled: true,
			Remark: "依据《住院病案首页数据采集质量与接口标准（2020版）》：新生儿（P 编码）须填出生体重与孕周。本规则校验出生体重（newborn_birth_weight，单位克）；" +
				"孕周由并行规则「新生儿需填孕周（条件必填）」校验（引擎一条 REQUIRED 规则只能反射单个目标字段）。",
		},
		{
			RuleName: "新生儿需填孕周（条件必填）", RuleType: model.QcRequired,
			TargetField: "NewbornGestationalWeeks", ConditionExpr: "main_diagnosis_code LIKE 'P%'",
			ErrorLevel: "WARN", Enabled: true,
			Remark: "依据《住院病案首页数据采集质量与接口标准（2020版）》：新生儿（P 编码）须填出生体重与孕周。本规则校验孕周（newborn_gestational_weeks，单位周），与出生体重规则成对。",
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
