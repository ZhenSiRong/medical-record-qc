package router

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"mrs/internal/cache"
	"mrs/internal/config"
)

// =============================================================================
// 接口契约核对（路由表 ↔ docs/API.md）
//
// 两个测试分工：
//   - TestDumpRoutes   导出「事实」：把真实路由写成 mrs-backend/routes.actual.txt。
//   - TestApiCoverage  用同一份事实对文档做双向集合断言，让文档与实现的漂移在
//                      `go test ./...` 时自动失败，而不是只在人工跑脚本时才发现。
//
// 事实的唯一来源是 gin 路由树（`Register` 装配后 `r.Routes()`），不依赖数据库与 Redis。
// =============================================================================

// moduleRoot 返回 mrs-backend/ 的绝对路径。
// 测试的相对路径工作目录是包目录（internal/router），故用编译期源文件路径反推模块根，
// 保证产物与文档定位在任何工作目录下都一致。
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 contract_test.go 源文件路径")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// actualRoutes 装配完整路由树，返回 "METHOD /path" 形式的真实路由清单（按路径字典序）。
// db=nil、cache 指向不可达地址，保证无数据库/无 Redis 也能构建路由表。
// gin 的路径参数保持原样（如 /api/records/:id）。
func actualRoutes(t *testing.T) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, nil, cache.New("127.0.0.1:1", "", 0), &config.Config{})

	routes := make([]string, 0, len(r.Routes()))
	for _, rt := range r.Routes() {
		routes = append(routes, rt.Method+" "+rt.Path)
	}
	// 按路径字典序排序，路径相同时按方法排序（稳定且便于人工核对）。
	sort.Slice(routes, func(i, j int) bool {
		pi, pj := pathOf(routes[i]), pathOf(routes[j])
		if pi != pj {
			return pi < pj
		}
		return routes[i] < routes[j]
	})
	return routes
}

// pathOf 从 "METHOD /path" 行中取出路径部分。
func pathOf(line string) string {
	if i := strings.Index(line, " "); i >= 0 {
		return line[i+1:]
	}
	return line
}

// splitRoute 把 "METHOD /path" 拆成方法与路径。
func splitRoute(line string) (method, path string) {
	method, path, _ = strings.Cut(line, " ")
	return method, path
}

// TestDumpRoutes 导出真实注册路由清单，作为契约核对的「事实基线」。
// 职责仅为导出事实：刻意不做条数断言（路由会增减），总数只出现在日志里。
func TestDumpRoutes(t *testing.T) {
	routes := actualRoutes(t)

	out := filepath.Join(moduleRoot(t), "routes.actual.txt")
	content := strings.Join(routes, "\n") + "\n"
	if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
		t.Fatalf("写入路由清单失败: %v", err)
	}

	t.Logf("已注册路由总数: %d", len(routes))
	t.Logf("路由清单已写入: %s", out)
}

// =============================================================================
// 文档解析
// =============================================================================

// crudResources 是 API.md §10 中用 `/api/{资源}/...` 占位概括的 6 类系统管理资源。
var crudResources = []string{"depts", "doctors", "dicts", "roles", "shelves", "users"}

// crudOps 是 §10「通用 CRUD 语义」为 6 类资源定义的 5 个操作模板。
// 仅当某处出现 `{资源}` 占位、且同一行完全没有方法信息时才用它兜底展开
// （正常情况表格行自带方法，会走更精确的「同行方法」分支）。
var crudOps = []struct{ method, tmpl string }{
	{"GET", "/api/%s/page"},
	{"GET", "/api/%s/:id"},
	{"POST", "/api/%s"},
	{"PUT", "/api/%s/:id"},
	{"DELETE", "/api/%s/:id"},
}

var (
	// 标题行：`### POST /api/xxx`。
	// /healthz 不在 /api 前缀下（API.md §0 明确说明），故单独允许。
	reHeading = regexp.MustCompile("^#{2,4}\\s+(GET|POST|PUT|DELETE|PATCH)\\s+(/api/[^\\s`]+|/healthz)")
	// 正文内联字面量：`` `GET /api/xxx` ``。
	// 刻意只认 `/api/` 与 `/healthz` 前缀：变更记录里那种 `GET /front-pages/:id/quality`
	// （省略 /api 前缀的简写）不应被当成文档化端点，否则会凭空造出「文档虚构」。
	reInline = regexp.MustCompile("`\\s*(GET|POST|PUT|DELETE|PATCH)\\s+(/api/[^`\\s]+|/healthz)`")
	// §10 通用 CRUD 表格行：| GET | `/api/{资源}/page` | 登录 | ... |
	reTableRow = regexp.MustCompile("^\\|\\s*(GET|POST|PUT|DELETE|PATCH)\\s*\\|\\s*`([^`]+)`")
	// 任意路径字面量（含 `{资源}` 占位与中文，避免把 /api/{资源}/page 截断成 /api/{）。
	reAnyPath = regexp.MustCompile("(/api/[A-Za-z0-9_\\-/:{}\\p{Han}.]+|/healthz)")
	// 路径参数：`:id` / `:recordId` / `:recordNo` 等。
	reParam = regexp.MustCompile(`:[A-Za-z_][A-Za-z0-9_]*`)
)

// normalizePath 归一化路径，使文档写法与注册写法可比：
// 去掉查询串、尾部标点/引号/斜杠，并把路径参数名统一成 `:param`。
func normalizePath(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimRight(p, "`）)、。，,.;；*>\"'")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "/"
	}
	return reParam.ReplaceAllString(p, ":param")
}

// documentedEndpoints 解析 docs/API.md，返回被文档化的端点集合。
//
// 判定口径（刻意宽松，避免因排版微调而误报；两条都写清楚以便复核）：
//
//  1. strong —— 明确给出「方法 + 路径」的端点，来源限三种可机读形式：
//     标题行 `### METHOD /path`、正文内联 “ `METHOD /path` “、
//     §10 通用 CRUD 表格行 `| METHOD | `path` |`。
//  2. weak —— 只出现路径字面量 `/api/...`（或 `/healthz`）而未写方法的端点。
//     只认这两种前缀：正文里 `/stats/*`、`/records`、`/archives` 这类无 `/api/` 前缀的
//     模块泛指引用不是端点，若一并采信会凭空造出大量「文档虚构」误报。
//  3. **路径参数名不参与比较**：`:id` / `:recordId` / `:recordNo` 统一归一为 `:param`。
//     这条是实测教训——API.md 曾把 `/archives/seal/:id` 写成 `:recordId`
//     （见 routes.diff.md 附录 S），逐字比较会把「参数名写错」误判成「路由未文档化」。
//     参数名写错属文档描述问题，应由人工按实现纠正，而不是让覆盖度测试变红。
//  4. `{资源}` 占位展开为 6 类资源：同一行有方法时按该方法展开（§10 表格正是这种形式）；
//     同行完全没有方法信息时，兜底为 §10 定义的 5 个通用 CRUD 操作。
func documentedEndpoints(md string) (strong, weak map[string]bool) {
	strong = map[string]bool{}
	weak = map[string]bool{}

	type endpoint struct{ method, path string }

	for _, line := range strings.Split(md, "\n") {
		var pairs []endpoint // 「方法 + 路径」成对出现
		var bare []string    // 只有路径、没有方法

		if m := reHeading.FindStringSubmatch(line); m != nil {
			pairs = append(pairs, endpoint{m[1], m[2]})
		}
		for _, m := range reInline.FindAllStringSubmatch(line, -1) {
			pairs = append(pairs, endpoint{m[1], m[2]})
		}
		if m := reTableRow.FindStringSubmatch(line); m != nil {
			pairs = append(pairs, endpoint{m[1], m[2]})
		}
		for _, m := range reAnyPath.FindAllStringSubmatch(line, -1) {
			bare = append(bare, m[1])
		}

		// 同一行内是否已用「方法 + {资源}」精确表达过（§10 表格行即如此）。
		placeholderWithMethod := false

		for _, ep := range pairs {
			if !strings.Contains(ep.path, "{资源}") {
				strong[ep.method+" "+normalizePath(ep.path)] = true
				continue
			}
			placeholderWithMethod = true
			for _, res := range crudResources {
				p := strings.ReplaceAll(ep.path, "{资源}", res)
				strong[ep.method+" "+normalizePath(p)] = true
			}
		}

		for _, raw := range bare {
			if !strings.Contains(raw, "{资源}") {
				weak[normalizePath(raw)] = true
				continue
			}
			if placeholderWithMethod {
				continue // 已按同行方法精确展开，不再兜底
			}
			for _, res := range crudResources {
				for _, op := range crudOps {
					strong[op.method+" "+normalizePath(fmt.Sprintf(op.tmpl, res))] = true
				}
			}
		}
	}
	return strong, weak
}

// TestApiCoverage 断言 docs/API.md 与真实路由表双向一致。
//
// 方向一（防止文档落后于实现）：每条已注册路由都必须在文档中出现。
// 方向二（防止文档虚构接口）：文档写明「方法+路径」的端点必须真的已注册。
//
// 不写死路由条数：数字只出现在日志里，路由增减不会让测试失效，只有「漂移」才会。
func TestApiCoverage(t *testing.T) {
	mdPath := filepath.Join(moduleRoot(t), "docs", "API.md")
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("读取接口契约文档失败（%s）: %v", mdPath, err)
	}
	strong, weak := documentedEndpoints(string(raw))

	routes := actualRoutes(t)
	t.Logf("真实注册路由 %d 条；docs/API.md 文档化「方法+路径」端点 %d 条，仅路径提及 %d 条",
		len(routes), len(strong), len(weak))

	// ---- 方向一：已注册 → 必须已文档化 ----
	var missing, weakOnly []string
	for _, line := range routes {
		method, path := splitRoute(line)
		switch {
		case strong[method+" "+normalizePath(path)]:
			// 强覆盖：方法与路径都在文档中
		case weak[normalizePath(path)]:
			weakOnly = append(weakOnly, line)
		default:
			missing = append(missing, line)
		}
	}
	if len(missing) > 0 {
		t.Errorf("有 %d 条已注册路由未在 docs/API.md 中记录：\n\t%s\n"+
			"请在 docs/API.md 补充文档，或确认该路由已废弃并删除注册。",
			len(missing), strings.Join(missing, "\n\t"))
	}
	if len(weakOnly) > 0 {
		// 口径说明：只出现路径、未写方法，按「已文档化」放行（避免误报），
		// 但记录下来，便于把 HTTP 方法补全成强覆盖。
		t.Logf("以下 %d 条路由仅以「路径」形式出现，文档未写明 HTTP 方法（按放行处理）：\n\t%s",
			len(weakOnly), strings.Join(weakOnly, "\n\t"))
	}

	// ---- 方向二：文档化的有方法端点 → 必须已注册 ----
	// 局限（刻意为之）：只校验「方法+路径」成对出现的形式（标题行/内联字面量/§10 表格行）。
	// 正文中无方法的泛指提及（如 `/stats/*` 这类模块名）不参与本断言，
	// 否则会把叙述性引用误判成虚构端点，噪声大于收益。
	registered := map[string]bool{}
	for _, line := range routes {
		method, path := splitRoute(line)
		registered[method+" "+normalizePath(path)] = true
	}
	var phantom []string
	for key := range strong {
		if !registered[key] {
			phantom = append(phantom, key)
		}
	}
	sort.Strings(phantom)
	if len(phantom) > 0 {
		t.Errorf("docs/API.md 记录了 %d 个实际并未注册的端点（文档与实现不符）：\n\t%s\n"+
			"请删除文档中的虚构接口，或确认实现缺失。",
			len(phantom), strings.Join(phantom, "\n\t"))
	}
}
