package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/internal/service"
)

// =====================================================================
// CC/MCC 严重度判读预览（批次 D）接口测试
//
// 约束：本项目用 PostgreSQL，未引入内存驱动，测试无法真连库。沿用
// frontpage_otherdiag_test.go 的 nil-DB 技巧：h.DB == nil 时，凡走到
// 数据库的代码必 panic，被 CustomRecoveryWithWriter 转成 500。于是：
//   - 直接传码形态 → **200**：该形态不碰库，故 nil DB 下也能完整走通；
//   - recordId 形态 → **500**：证明 recordId 分支确实去读库了（而不是被忽略）。
//     ⚠️ 这个 500 不是 bug，是"分支已接线"的证据。
//
// recordId 形态与直接传码形态的**结果一致性**无法靠真连库端到端验证，改为
// 结构等价验证：把首页字段喂给两者共用的 judgeCcMccFromCodes，与直接传码
// 形态的 HTTP 响应逐字段比对（见 TestCcMccPreview_RecordIdFormEqualsDirectForm）。
// =====================================================================

const ccMccTestSecret = "ccmcc-preview-test-secret"

// ccMccHTTPResp 与 pkg/resp.R 同形，Data 直接落到 service.CcMccJudgement 上，
// 便于对 disclaimer / demoOnly / sources 这些合规字段做类型安全的断言。
type ccMccHTTPResp struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data service.CcMccJudgement `json:"data"`
}

// newCcMccTestRouter 装配只含判读预览路由的最小路由树（db=nil）。
// 路由口径与 internal/handler/branch_ext.go 一致：POST /api/ccmcc-preview 挂在 auth 之下。
func newCcMccTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 静默 Recovery：nil DB 触发的 panic 转 500，且不打印堆栈，保持测试输出干净。
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	api := r.Group("/api")
	auth := api.Group("")
	auth.Use(middleware.Auth(ccMccTestSecret))
	auth.POST("/ccmcc-preview", NewCcMccHandler(nil).Preview)
	return r
}

func ccMccToken(t *testing.T, role string) string {
	t.Helper()
	tk, err := middleware.SignToken(ccMccTestSecret, 1, 1, "tester", role)
	if err != nil {
		t.Fatalf("签发测试 JWT 失败: %v", err)
	}
	return tk
}

func ccMccPost(r *gin.Engine, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/ccmcc-preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func ccMccDecode(t *testing.T, w *httptest.ResponseRecorder) ccMccHTTPResp {
	t.Helper()
	var out ccMccHTTPResp
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应体不是合法 JSON: %v（body=%s）", err, w.Body.String())
	}
	return out
}

// TestCcMccPreview_DisclaimerAlwaysPresent 合规主线（ADR §4/§6）：任何一次成功判读，
// 响应体都必须带非空 disclaimer，且与判读层常量逐字相等（handler 不许改写）。
func TestCcMccPreview_DisclaimerAlwaysPresent(t *testing.T) {
	r := newCcMccTestRouter(t)
	tk := ccMccToken(t, roleAdmin)

	cases := []struct {
		name        string
		body        string
		wantOverall string
		wantMcc     int
		wantCc      int
		wantItems   int
	}{
		{
			name:        "命中 MCC + 未命中",
			body:        `{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900,K80.201","otherDiagnosisFlags":"M,0"}`,
			wantOverall: service.SeverityMCC, wantMcc: 1, wantCc: 0, wantItems: 2,
		},
		{
			name:        "命中两条 CC",
			body:        `{"mainDiagnosisCode":"K35.801","otherDiagnosisCodes":"I48.000,J44.101","otherDiagnosisFlags":"C,C"}`,
			wantOverall: service.SeverityCC, wantMcc: 0, wantCc: 2, wantItems: 2,
		},
		{
			name:        "无其他诊断（空串）",
			body:        `{"mainDiagnosisCode":"K35.801","otherDiagnosisCodes":"","otherDiagnosisFlags":""}`,
			wantOverall: service.SeverityNone, wantMcc: 0, wantCc: 0, wantItems: 0,
		},
		{
			name:        "排除规则命中（主诊断 I63 → I69.3 不计 CC）",
			body:        `{"mainDiagnosisCode":"I63.900","otherDiagnosisCodes":"I69.300","otherDiagnosisFlags":"C"}`,
			wantOverall: service.SeverityNone, wantMcc: 0, wantCc: 0, wantItems: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := ccMccPost(r, tc.body, tk)
			if w.Code != http.StatusOK {
				t.Fatalf("期望 200，实际 %d（body=%s）", w.Code, w.Body.String())
			}
			got := ccMccDecode(t, w)
			if got.Code != 0 {
				t.Fatalf("业务码期望 0，实际 %d（msg=%s）", got.Code, got.Msg)
			}
			if got.Data.Disclaimer == "" {
				t.Fatalf("响应缺少 disclaimer（ADR §4 强约束），body=%s", w.Body.String())
			}
			if got.Data.Disclaimer != service.CcMccDisclaimer {
				t.Errorf("disclaimer 被改写：%q，期望判读层常量 %q", got.Data.Disclaimer, service.CcMccDisclaimer)
			}
			if got.Data.Version != service.CcMccDemoVersion {
				t.Errorf("version = %q，期望 %q", got.Data.Version, service.CcMccDemoVersion)
			}
			if got.Data.OverallSeverity != tc.wantOverall {
				t.Errorf("overallSeverity = %q，期望 %q", got.Data.OverallSeverity, tc.wantOverall)
			}
			if got.Data.MccCount != tc.wantMcc || got.Data.CcCount != tc.wantCc {
				t.Errorf("计数 = (MCC %d, CC %d)，期望 (MCC %d, CC %d)",
					got.Data.MccCount, got.Data.CcCount, tc.wantMcc, tc.wantCc)
			}
			if len(got.Data.Items) != tc.wantItems {
				t.Errorf("items 长度 = %d，期望 %d", len(got.Data.Items), tc.wantItems)
			}
		})
	}
}

// TestCcMccPreview_DemoOnlyAndSources 固化 ADR §6 的界面标注依据：
// 命中演示规则 ⇒ demoOnly=true 且 sources=["demo"]；一条都没命中 ⇒ 没有依据可言，
// demoOnly 必须为 false（否则等于拿"什么都没命中"冒充演示结论）。
func TestCcMccPreview_DemoOnlyAndSources(t *testing.T) {
	r := newCcMccTestRouter(t)
	tk := ccMccToken(t, roleAdmin)

	cases := []struct {
		name        string
		body        string
		wantDemo    bool
		wantSources []string
	}{
		{
			name:        "命中演示规则 ⇒ demoOnly",
			body:        `{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900,K80.201","otherDiagnosisFlags":"M,0"}`,
			wantDemo:    true,
			wantSources: []string{service.CcMccSourceDemo},
		},
		{
			name:        "未命中任何规则 ⇒ 非 demoOnly",
			body:        `{"mainDiagnosisCode":"K35.801","otherDiagnosisCodes":"K80.201","otherDiagnosisFlags":"0"}`,
			wantDemo:    false,
			wantSources: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := ccMccPost(r, tc.body, tk)
			if w.Code != http.StatusOK {
				t.Fatalf("期望 200，实际 %d（body=%s）", w.Code, w.Body.String())
			}
			got := ccMccDecode(t, w)
			if got.Data.DemoOnly != tc.wantDemo {
				t.Errorf("demoOnly = %v，期望 %v（body=%s）", got.Data.DemoOnly, tc.wantDemo, w.Body.String())
			}
			if !reflect.DeepEqual(got.Data.Sources, tc.wantSources) {
				t.Errorf("sources = %#v，期望 %#v", got.Data.Sources, tc.wantSources)
			}
		})
	}
}

// TestCcMccPreview_BadRequest 参数缺失/非法一律 400，且错误信息指位到具体原因。
func TestCcMccPreview_BadRequest(t *testing.T) {
	r := newCcMccTestRouter(t)
	tk := ccMccToken(t, roleAdmin)

	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{name: "空 body：既无 recordId 也无主诊断", body: `{}`, wantMsg: "请传 recordId 或 mainDiagnosisCode"},
		{name: "主诊断为空白串", body: `{"mainDiagnosisCode":"   "}`, wantMsg: "请传 recordId 或 mainDiagnosisCode"},
		{name: "recordId 为负", body: `{"recordId":-1}`, wantMsg: "recordId 必须为正整数"},
		{name: "JSON 不合法", body: `{"mainDiagnosisCode":`, wantMsg: "参数错误"},
		{
			name:    "其他诊断编码重复（ADR §5 硬约束）",
			body:    `{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900,I50.900","otherDiagnosisFlags":"M,M"}`,
			wantMsg: "其他诊断数据不合法",
		},
		{
			name:    "编码与标记长度不等",
			body:    `{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900,K80.201","otherDiagnosisFlags":"M"}`,
			wantMsg: "其他诊断数据不合法",
		},
		{
			name:    "严重度标记非法",
			body:    `{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900","otherDiagnosisFlags":"X"}`,
			wantMsg: "其他诊断数据不合法",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := ccMccPost(r, tc.body, tk)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("期望 400，实际 %d（body=%s）", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantMsg) {
				t.Errorf("响应体未包含 %q，实际 body=%s", tc.wantMsg, w.Body.String())
			}
		})
	}
}

// TestCcMccPreview_NilDBProvesBranchWiring 用 nil DB 证明两条分支的走向：
// recordId 形态必然去读库（撞 nil DB → 500），直接传码形态不碰库（→ 200）。
func TestCcMccPreview_NilDBProvesBranchWiring(t *testing.T) {
	r := newCcMccTestRouter(t)
	tk := ccMccToken(t, roleAdmin)

	if w := ccMccPost(r, `{"recordId":123}`, tk); w.Code != http.StatusInternalServerError {
		// 这里若变成 400，说明 recordId 分支根本没走到读库；若变成 200，说明它绕过了首页。
		t.Errorf("recordId 形态期望 500（撞 nil DB，证明该分支读库），实际 %d（body=%s）", w.Code, w.Body.String())
	}
	if w := ccMccPost(r, `{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900","otherDiagnosisFlags":"M"}`, tk); w.Code != http.StatusOK {
		t.Errorf("直接传码形态期望 200（不碰库），实际 %d（body=%s）", w.Code, w.Body.String())
	}
}

// TestCcMccPreview_RecordIdFormEqualsDirectForm 一致性：同一份首页字段经 recordId
// 形态（读库后喂给 judgeCcMccFromCodes）与直接传码形态判读，结果必须逐字段相等 ——
// 两种入参只是取值来源不同，判读口径不得分叉（否则同一份病案两种入口会给出两个分层）。
func TestCcMccPreview_RecordIdFormEqualsDirectForm(t *testing.T) {
	r := newCcMccTestRouter(t)
	tk := ccMccToken(t, roleAdmin)

	fp := model.MedFrontPage{
		MainDiagnosisCode:   "I21.900",
		OtherDiagnosisCodes: "I50.900,K80.201",
		OtherDiagnosisFlags: "M,0",
	}

	// recordId 形态：读库之后走的正是这一句（见 Preview；db 传 nil 即无库装配场景）。
	viaFrontPage, err := judgeCcMccFromCodes(nil, fp.MainDiagnosisCode, fp.OtherDiagnosisCodes, fp.OtherDiagnosisFlags)
	if err != nil {
		t.Fatalf("首页字段判读失败: %v", err)
	}

	body := `{"mainDiagnosisCode":"I21.900","otherDiagnosisCodes":"I50.900,K80.201","otherDiagnosisFlags":"M,0"}`
	w := ccMccPost(r, body, tk)
	if w.Code != http.StatusOK {
		t.Fatalf("直接传码形态期望 200，实际 %d（body=%s）", w.Code, w.Body.String())
	}
	viaDirect := ccMccDecode(t, w).Data

	if !reflect.DeepEqual(viaFrontPage, viaDirect) {
		t.Fatalf("两种形态判读结果不一致：\nrecordId 形态 = %+v\n直接传码形态 = %+v", viaFrontPage, viaDirect)
	}
	if viaDirect.Disclaimer == "" || viaDirect.OverallSeverity != service.SeverityMCC {
		t.Errorf("判读结果异常：disclaimer=%q overall=%q", viaDirect.Disclaimer, viaDirect.OverallSeverity)
	}
}

// TestCcMccRulesForNilDBFallsBackToDemo 固化规则表的取值顺序：无库装配时不查库、
// 直接用内置演示表（ccMccRulesFor 的 db==nil 分支），保证判读不因拿不到库而断供。
// 库内路径（LoadCcMccRules 有数据 / 报错 / 空表）由 service/ccmcc_store_test.go 覆盖。
func TestCcMccRulesForNilDBFallsBackToDemo(t *testing.T) {
	if got := ccMccRulesFor(nil, service.CcMccDemoVersion); !reflect.DeepEqual(got, service.CcMccDemoRules) {
		t.Errorf("nil DB 未降级到内置演示表，实际 %d 条", len(got))
	}
	if got := ccMccRulesFor(nil, ""); len(got) != len(service.CcMccDemoRules) {
		t.Errorf("空版本下期望同样降级到演示表，实际 %d 条", len(got))
	}
}
