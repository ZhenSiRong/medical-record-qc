package bdd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/gin-gonic/gin"

	"mrs/internal/cache"
	"mrs/internal/config"
	"mrs/internal/middleware"
	"mrs/internal/router"
)

// bddJWTSecret 是本套件签发/校验 JWT 所用密钥，仅存在于测试进程内，与生产密钥无关。
const bddJWTSecret = "bdd-behavior-suite-secret"

// 角色字面量：必须与 internal/handler/qc_ext.go 的 roleAdmin / roleDeptUser 保持一致。
//
// 跨包无法引用未导出常量，故此处以字面量固化。二者的一致性由**行为自证**：
// 若取值发生漂移，「病案室管理员可通过权限闸门」场景会因被 403 拦下而变红。
const (
	roleAdmin    = "ADMIN"
	roleDeptUser = "DEPT_USER"
)

// bddFrontPagePath 是场景统一操作的目标端点。
//
// 固定使用 recordId=1：其他诊断校验发生在「碰数据库之前」，
// 故该 id 是否真实存在不影响断言——这也正是本套件无需连库的原因。
const bddFrontPagePath = "/api/front-pages/1"

// bddRouter 是整套场景共享的真实路由树。
// 它是无状态的（db=nil，仅做请求分发与中间件链执行），故可安全共享。
var bddRouter *gin.Engine

// stateKey 用于把场景状态挂到 godog 的 context 上——每个场景一份，天然相互隔离。
type stateKey struct{}

type scenarioState struct {
	token string
	resp  *httptest.ResponseRecorder
}

func stateOf(ctx context.Context) *scenarioState {
	s, _ := ctx.Value(stateKey{}).(*scenarioState)
	return s
}

// TestFeatures 是套件入口。以 godog 官方推荐方式挂到 go test 之下：
// 不依赖 godog 命令行，随 `go test ./... -count=1` 一起跑。
func TestFeatures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bddRouter = newBDDRouter()

	suite := godog.TestSuite{
		Name:                "mrs 行为规范",
		ScenarioInitializer: initializeScenario,
		Options: &godog.Options{
			Format: "pretty",
			// 关闭 ANSI 颜色：CI 与日志里出现转义码会干扰检索（本套件曾被误读为输出异常）。
			NoColors: true,
			// Paths 相对于本测试包的目录（go test 的工作目录即包目录）。
			Paths:    []string{"../../features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("行为套件未通过")
	}
}

// newBDDRouter 装配真实全量路由树，但 db 传 nil（不连库）。
//
// 关键点：router.Register 自身**不挂 Recovery**（生产形态由 gin.Default() 提供）。
// 本套件 db=nil，合法请求会在 h.DB.Transaction 处 panic；
// 若不先挂静默 Recovery，panic 会冒泡并直接崩掉测试进程。
// 因此这里必须在 Register 之前挂上，且用 io.Discard 保持测试输出干净。
func newBDDRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		c.AbortWithStatus(http.StatusInternalServerError)
	}))

	cfg := &config.Config{}
	cfg.JWT.Secret = bddJWTSecret

	// Redis 地址指向必不存在的端口：本套件用不到缓存，命中失败会自动降级直查。
	router.Register(r, nil, cache.New("127.0.0.1:1", "", 0), cfg)
	return r
}

func initializeScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, stateKey{}, &scenarioState{}), nil
	})

	sc.Step(`^病案室管理员已登录$`, adminLoggedIn)
	sc.Step(`^科室医师已登录$`, deptUserLoggedIn)
	sc.Step(`^未携带令牌保存病案首页$`, saveWithoutToken)
	sc.Step(`^保存病案首页$`, saveFrontPage)
	sc.Step(`^以其他诊断编码 "([^"]*)" 与严重度标记 "([^"]*)" 保存病案首页$`, saveFrontPageWithOtherDiagnosis)

	sc.Step(`^响应状态码应为 (\d+)$`, responseStatusCodeShouldBe)
	sc.Step(`^响应应说明 "([^"]*)"$`, responseShouldContain)
	sc.Step(`^请求应越过校验并触达数据库层$`, shouldPassValidationAndReachDB)
}

// ---------- Given ----------

func adminLoggedIn(ctx context.Context) error { return loginAs(ctx, roleAdmin) }

func deptUserLoggedIn(ctx context.Context) error { return loginAs(ctx, roleDeptUser) }

func loginAs(ctx context.Context, role string) error {
	st := stateOf(ctx)
	if st == nil {
		return errors.New("场景状态缺失（Before 钩子未生效）")
	}
	tk, err := middleware.SignToken(bddJWTSecret, 1, 1, "bdd-"+role, role)
	if err != nil {
		return fmt.Errorf("签发 %s 角色令牌失败：%w", role, err)
	}
	st.token = tk
	return nil
}

// ---------- When ----------

func saveFrontPage(ctx context.Context) error {
	return putFrontPage(ctx, map[string]string{})
}

func saveWithoutToken(ctx context.Context) error {
	if st := stateOf(ctx); st != nil {
		st.token = "" // 显式不携带令牌：验证权限闸门的第一道关
	}
	return putFrontPage(ctx, map[string]string{})
}

func saveFrontPageWithOtherDiagnosis(ctx context.Context, codes, flags string) error {
	return putFrontPage(ctx, map[string]string{
		"otherDiagnosisCodes": codes,
		"otherDiagnosisFlags": flags,
	})
}

func putFrontPage(ctx context.Context, payload map[string]string) error {
	st := stateOf(ctx)
	if st == nil {
		return errors.New("场景状态缺失（Before 钩子未生效）")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("构造请求体失败：%w", err)
	}

	req := httptest.NewRequest(http.MethodPut, bddFrontPagePath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if st.token != "" {
		req.Header.Set("Authorization", "Bearer "+st.token)
	}

	w := httptest.NewRecorder()
	bddRouter.ServeHTTP(w, req)
	st.resp = w
	return nil
}

// ---------- Then ----------

func responseStatusCodeShouldBe(ctx context.Context, want int) error {
	st := stateOf(ctx)
	if st == nil || st.resp == nil {
		return errors.New("尚未发起请求，无法断言状态码")
	}
	if st.resp.Code != want {
		return fmt.Errorf("期望响应状态码 %d，实际 %d（body=%s）", want, st.resp.Code, st.resp.Body.String())
	}
	return nil
}

func responseShouldContain(ctx context.Context, want string) error {
	st := stateOf(ctx)
	if st == nil || st.resp == nil {
		return errors.New("尚未发起请求，无法断言响应体")
	}
	if !strings.Contains(st.resp.Body.String(), want) {
		return fmt.Errorf("响应体未包含 %q，实际 body=%s", want, st.resp.Body.String())
	}
	return nil
}

// shouldPassValidationAndReachDB 断言请求「越过了其他诊断校验与权限闸门」。
//
// 观测口径：本套件 db=nil，越过校验的请求会在 h.DB.Transaction 处 panic，
// 被测试的静默 Recovery 兑现为 500。这个 500 不是缺陷，而是「校验已放行」的证据——
// 详见 features/ 下各 .feature 文件头部的「无数据库桩」说明。
//
// 守卫价值：若该校验将来被误删（非法数据不再被拦）或误杀合法数据，
// 本步骤会分别以「得到 400 而非 500」的形式变红。
func shouldPassValidationAndReachDB(ctx context.Context) error {
	st := stateOf(ctx)
	if st == nil || st.resp == nil {
		return errors.New("尚未发起请求，无法断言")
	}
	const wantNoDBStub = http.StatusInternalServerError
	if st.resp.Code != wantNoDBStub {
		return fmt.Errorf(
			"期望请求越过校验并触达数据库层（本套件 db=nil，兑现为 %d），实际 %d（body=%s）",
			wantNoDBStub, st.resp.Code, st.resp.Body.String())
	}
	return nil
}
