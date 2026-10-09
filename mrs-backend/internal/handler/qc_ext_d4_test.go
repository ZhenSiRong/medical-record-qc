package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"mrs/internal/middleware"
	"mrs/internal/model"
)

// =====================================================================
// 缺口 D4（质控退回的医生端）测试
//
// 约束：本项目使用 PostgreSQL，未引入 sqlite 等内存驱动，故本包测试无法真连数据库。
// 因此这里覆盖三类**不需要数据库**的断言：
//  1) 路由装配冒烟：nil db 下 RegisterQcExt 不得 panic（项目约定"注册函数纯注册、不做迁移"，
//     主干 internal/router/router_test.go 亦以 db=nil 装配路由树）；
//  2) 权限闸门：rectify 的角色口径（DEPT_USER/ADMIN 放行，其它角色 403，未登录 401）；
//  3) 纯口径函数：需临床处理的状态集合、科室归属匹配。
//
// 未覆盖（需真实库，见交付报告）：MyPending 的联表过滤与字段拼装、Rectify 的状态写入。
// =====================================================================

const d4TestSecret = "d4-test-secret"

// newD4TestRouter 装配只含质控扩展路由的最小路由树（db=nil）。
func newD4TestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 处理器内部使用 nil DB 会 panic；静默 Recovery 把它转成 500（不打印堆栈，保持测试输出干净），
	// 便于把"被权限闸门拒绝（403/401）"与"已进入处理器（500）"区分开。
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	api := r.Group("/api")
	auth := api.Group("")
	auth.Use(middleware.Auth(d4TestSecret))
	RegisterQcExt(auth, auth.Group(""), nil)
	return r
}

func d4Token(t *testing.T, role string) string {
	t.Helper()
	tk, err := middleware.SignToken(d4TestSecret, 1, 1, "tester", role)
	if err != nil {
		t.Fatalf("签发测试 JWT 失败: %v", err)
	}
	return tk
}

func d4Do(r *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestRegisterQcExtNilDBNotPanic 纯注册约定：nil db 下装配不 panic，且 D4 新路由存在。
func TestRegisterQcExtNilDBNotPanic(t *testing.T) {
	r := newD4TestRouter(t)
	got := make(map[string]bool)
	for _, rt := range r.Routes() {
		got[rt.Method+" "+rt.Path] = true
	}
	for _, want := range []string{
		"GET /api/qc-results/my-pending",
		"POST /api/qc-results/:id/rectify",
		"POST /api/qc-results/:id/return",
		"POST /api/qc-results/:id/review",
	} {
		if !got[want] {
			t.Errorf("路由未注册: %s", want)
		}
	}
}

// TestRectifyRoleGate 权限收紧验证：rectify 仅对临床侧角色放行。
func TestRectifyRoleGate(t *testing.T) {
	r := newD4TestRouter(t)

	cases := []struct {
		name string
		role string
		deny bool
	}{
		{"DEPT_USER 放行（临床医师整改）", roleDeptUser, false},
		{"ADMIN 放行（病案室模拟整改入口，兼容 catalog.js）", roleAdmin, false},
		{"其它角色拒绝", "AUDITOR", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := d4Do(r, http.MethodPost, "/api/qc-results/1/rectify", d4Token(t, tc.role))
			if tc.deny {
				if w.Code != http.StatusForbidden {
					t.Fatalf("期望 403，实际 %d（body=%s）", w.Code, w.Body.String())
				}
				return
			}
			// 放行 = 已越过权限闸门进入处理器；nil DB 必然 panic 并被 Recovery 转 500。
			if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
				t.Fatalf("角色 %s 不应被权限闸门拒绝，实际 %d（body=%s）", tc.role, w.Code, w.Body.String())
			}
		})
	}

	if w := d4Do(r, http.MethodPost, "/api/qc-results/1/rectify", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("未带 token 期望 401，实际 %d", w.Code)
	}
}

// TestMyPendingRequiresLogin 待整改清单为登录接口（按角色再限定数据范围，见 MyPending 注释）。
func TestMyPendingRequiresLogin(t *testing.T) {
	r := newD4TestRouter(t)
	if w := d4Do(r, http.MethodGet, "/api/qc-results/my-pending", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("未带 token 期望 401，实际 %d", w.Code)
	}
	w := d4Do(r, http.MethodGet, "/api/qc-results/my-pending", d4Token(t, roleDeptUser))
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("登录用户不应被拒绝，实际 %d（body=%s）", w.Code, w.Body.String())
	}
}

// TestRectifyBodyOptional 固化"空请求体 / {} 也能调用"的既有契约（缺口 D4 要求不破坏请求体）。
func TestRectifyBodyOptional(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var gotOpinion string
	r.POST("/probe", func(c *gin.Context) {
		var req rectifyReq
		_ = c.ShouldBindJSON(&req) // 与 Rectify 完全一致的绑定方式
		gotOpinion = req.Opinion
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	cases := []struct {
		name string
		body string
		want string
	}{
		{"空请求体（后端既有调用方式）", "", ""},
		{"空对象（api-store.ext.qcRectify 现状）", "{}", ""},
		{"带整改说明", `{"opinion":"已补充外部原因编码 V01"}`, "已补充外部原因编码 V01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotOpinion = "unset"
			req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("期望 200，实际 %d", w.Code)
			}
			if gotOpinion != tc.want {
				t.Errorf("opinion 解析错误：期望 %q，实际 %q", tc.want, gotOpinion)
			}
		})
	}
}

// TestNeedsClinicalAction 「需临床处理」状态集合口径：RETURNED / REJECTED_BY_QC。
func TestNeedsClinicalAction(t *testing.T) {
	for _, s := range []string{model.HandleReturned, model.HandleRejectedByQc} {
		if !needsClinicalAction(s) {
			t.Errorf("%s 应属于需临床处理", s)
		}
	}
	for _, s := range []string{model.HandleOpen, model.HandleRectified, model.HandleClosed, ""} {
		if needsClinicalAction(s) {
			t.Errorf("%s 不应属于需临床处理（球在病案室一侧或已闭环）", s)
		}
	}
	if got := pendingClinicalStatuses(); len(got) != 2 {
		t.Errorf("需临床处理状态集合应为 2 个元素，实际 %v", got)
	}
}

// TestDeptScopeMatches 科室归属匹配口径：科室名优先，退化 deptId，皆空 fail-closed。
func TestDeptScopeMatches(t *testing.T) {
	rec := &model.MedRecord{DeptName: "心血管内科", DeptID: 7}
	cases := []struct {
		name     string
		deptName string
		deptID   int64
		want     bool
	}{
		{"科室名一致", "心血管内科", 0, true},
		{"科室名不一致", "神经内科", 0, false},
		{"科室名为空时按 deptId 命中", "", 7, true},
		{"科室名为空时按 deptId 不命中", "", 8, false},
		{"科室名优先于 deptId（名不符即不匹配）", "神经内科", 7, false},
		{"两者皆空 fail-closed", "", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deptScopeMatches(tc.deptName, tc.deptID, rec); got != tc.want {
				t.Errorf("deptScopeMatches(%q,%d) = %v，期望 %v", tc.deptName, tc.deptID, got, tc.want)
			}
		})
	}
	if deptScopeMatches("心血管内科", 0, nil) {
		t.Error("病案为空时应视为不匹配")
	}
}
