package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"mrs/internal/middleware"
)

// =====================================================================
// 病案首页其他诊断（ADR-0002 §5，批次 B）写路径接线测试
//
// 目的：证明 FrontPageHandler.Save 在**碰数据库之前**会先校验其他诊断，
// 从而把"重复编码 / 长度不等 / 非法标记"这类脏数据挡在库外。
//
// 约束：本项目使用 PostgreSQL，未引入 sqlite 等内存驱动，测试无法真连库。
// 本文件用一条"nil DB"技巧把"校验是否生效"变成可断言的事实：
//
//   测试装配的处理器 h.DB == nil。若请求**越过**其他诊断校验、继续执行，
//   必然走到 h.DB.Transaction(...)，对 nil *gorm.DB 调用方法会 panic；
//   而路由树挂了 CustomRecoveryWithWriter 把 panic 转成 **500**。
//
//   于是：
//     - 非法载荷 → **400**：说明校验在碰数据库之前就拦下了（否则会是 500）；
//     - 合法载荷 → **500**：说明校验已放行、走到了 nil DB。
//         ⚠️ 这个 500 **不是 bug**，恰恰是"校验已放行"的证据。
//
// 若将来把校验挪到事务内（DB 非 nil 的真实环境），本文件的 400 断言仍成立，
// 但 500 证据会换成真实库行为；因此这里附上注释，避免后人误读。
// =====================================================================

const otherDiagTestSecret = "frontpage-otherdiag-test-secret"

// newOtherDiagTestRouter 装配只含首页保存路由的最小路由树（db=nil）。
// 路由口径与 internal/router/router.go 一致：PUT /api/front-pages/:recordId 挂在 ADMIN 之下。
func newOtherDiagTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 静默 Recovery：nil DB 触发的 panic 转 500，且不打印堆栈，保持测试输出干净。
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	api := r.Group("/api")
	auth := api.Group("")
	auth.Use(middleware.Auth(otherDiagTestSecret))
	admin := auth.Group("")
	admin.Use(middleware.RequireRole(roleAdmin))
	admin.PUT("/front-pages/:recordId", (&FrontPageHandler{DB: nil}).Save)
	return r
}

func otherDiagToken(t *testing.T, role string) string {
	t.Helper()
	tk, err := middleware.SignToken(otherDiagTestSecret, 1, 1, "tester", role)
	if err != nil {
		t.Fatalf("签发测试 JWT 失败: %v", err)
	}
	return tk
}

// otherDiagPut 发送 PUT 请求（注意是 Put 而非 Post）并返回响应记录器。
func otherDiagPut(r *gin.Engine, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestFrontPageSaveRejectsInvalidOtherDiagnosis 是"接线生效"的核心证据：
// 非法其他诊断必须在 400 被拒，且响应体带出错原因——这证明校验发生在动数据库之前
// （若校验缺失，请求会继续执行并因 nil DB panic → 500）。
func TestFrontPageSaveRejectsInvalidOtherDiagnosis(t *testing.T) {
	r := newOtherDiagTestRouter(t)
	tk := otherDiagToken(t, roleAdmin)

	cases := []struct {
		name string
		body string
	}{
		{
			name: "重复编码（ADR §5 硬约束）",
			body: `{"otherDiagnosisCodes":"I10.x00,I10.x00","otherDiagnosisFlags":"0,C"}`,
		},
		{
			name: "长度不等：codes 3 项 / flags 2 项",
			body: `{"otherDiagnosisCodes":"I10.x00,E11.901,N18.500","otherDiagnosisFlags":"0,C"}`,
		},
		{
			name: "非法标记：X",
			body: `{"otherDiagnosisCodes":"I10.x00","otherDiagnosisFlags":"X"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := otherDiagPut(r, "/api/front-pages/1", tc.body, tk)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("期望 400（校验拦截，未碰数据库），实际 %d（body=%s）", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "其他诊断数据不合法") {
				t.Errorf("响应体未包含错误说明 %q，实际 body=%s", "其他诊断数据不合法", w.Body.String())
			}
		})
	}
}

// TestFrontPageSavePassesValidationThenHitsNilDB 用"合法载荷 → 500"证明校验已放行：
// 请求越过校验、执行到 h.DB.Transaction(nil DB) 才 panic，被 Recovery 转 500。
// 这条 500 **不是 bug**，而是"校验未误杀合法数据"的证据。
func TestFrontPageSavePassesValidationThenHitsNilDB(t *testing.T) {
	r := newOtherDiagTestRouter(t)
	tk := otherDiagToken(t, roleAdmin)

	cases := []struct {
		name string
		body string
	}{
		{name: "两侧皆空", body: `{}`},
		{name: "等长合法（含 CC）", body: `{"otherDiagnosisCodes":"I10.x00,E11.901","otherDiagnosisFlags":"0,C"}`},
		{name: "codes 非空 + flags 整串为空（历史数据形态）", body: `{"otherDiagnosisCodes":"I10.x00,E11.901","otherDiagnosisFlags":""}`},
		{name: "脏但可规范化（编码大小写与空格）", body: `{"otherDiagnosisCodes":" i10.x00 , e11.901 ","otherDiagnosisFlags":" 0 , C "}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := otherDiagPut(r, "/api/front-pages/1", tc.body, tk)
			if w.Code != http.StatusInternalServerError {
				// 这里刻意把 400 与 500 分开报告，便于分辨"误杀合法数据"与"未越过校验"。
				t.Fatalf("期望 500（校验放行后撞上 nil DB，非 bug），实际 %d（body=%s）", w.Code, w.Body.String())
			}
		})
	}
}

// TestFrontPageSaveInvalidFlagLowercaseCase 小写标记 'c' 属于"错误断言"而非等价写法，
// 必须 400（与解析层第 4 条一致），不能因为它在"看起来合法"的载荷里就被放过。
func TestFrontPageSaveInvalidFlagLowercaseCase(t *testing.T) {
	r := newOtherDiagTestRouter(t)
	tk := otherDiagToken(t, roleAdmin)
	w := otherDiagPut(r, "/api/front-pages/1",
		`{"otherDiagnosisCodes":" i10.x00 , e11.901 ","otherDiagnosisFlags":" 0 , c "}`, tk)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("小写标记 c 应 400，实际 %d（body=%s）", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "其他诊断数据不合法") {
		t.Errorf("响应体未包含错误说明，实际 body=%s", w.Body.String())
	}
}

// TestFrontPageSaveRoleGate 权限闸门 sanity：路由与生产一致挂在 ADMIN 之下。
// 未带 token → 401；DEPT_USER → 403；ADMIN → 越过闸门（撞 nil DB → 500）。
func TestFrontPageSaveRoleGate(t *testing.T) {
	r := newOtherDiagTestRouter(t)

	if w := otherDiagPut(r, "/api/front-pages/1", `{}`, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("未带 token 期望 401，实际 %d", w.Code)
	}

	if w := otherDiagPut(r, "/api/front-pages/1", `{}`, otherDiagToken(t, roleDeptUser)); w.Code != http.StatusForbidden {
		t.Errorf("DEPT_USER 期望 403，实际 %d（body=%s）", w.Code, w.Body.String())
	}

	if w := otherDiagPut(r, "/api/front-pages/1", `{}`, otherDiagToken(t, roleAdmin)); w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("ADMIN 不应被权限闸门拒绝，实际 %d（body=%s）", w.Code, w.Body.String())
	}
}
