package router

// =============================================================================
// 前端调用 → 后端路由：可执行的接口契约
//
// 背景：本仓库的「前端 ↔ 后端」是字符串拼 URL 的硬连接（没有生成式 client、
// 没有类型检查）。所以这类缺陷会静默存在：前端调用了一个后端不存在（或路径写错、
// 方法写错）的接口，代码审查看不出来，`go test` 也发现不了，只有用户点到那个按钮
// 才会 404。本测试把这类「漂移」变成 `go test` 时的确定性失败。
//
// 事实源与复用说明：
//   - 后端事实 = gin 路由树（Register 装配后 r.Routes()），无数据库、无 Redis。
//     这里直接复用 contract_test.go 的 actualRoutes()：同一份事实源只能有一处装配
//     代码，否则两份装配各自漂移又会变成新的不一致。若该辅助函数被改名/搬走，
//     本文件只需同步一行调用（contract_test.go 归另一位 agent 所有，本文件不改它）。
//   - 前端事实 = frontend/js/**/*.js 与 frontend/index.html 里的请求字面量。
//
// 两个方向刻意不对称：
//   - 前端 → 后端（本测试）：必须 100% 命中，未命中就是缺陷。
//   - 后端 → 前端：不断言（后端大量接口暂无前端调用方，这是正常的，不是缺陷）。
//
// 提取与规范化规则（对应本仓库真实写法，逐条见各函数注释）：
//  1. 调用点识别：req(METHOD, path[, body])、fetchAll(path)、fetch(URL[, opt])，
//     以及 URL 构造点 `API + '/api/...'`。
//  2. 路径表达式支持：字符串字面量、`'a/' + x + '/b'` 拼接、模板串 `${x}`、
//     三元表达式（两个分支都作为候选）、查询串（`?` 起截断）。
//  3. `/api` 前缀双向统一：req 内部拼的是 `API + '/api' + path`，所以前端字面量
//     大多**不含** `/api`（如 `/records/page`）；而直接 fetch 的导出接口写的是
//     `API + '/api/report-batches/1/export'`，**含** `/api`。两种都支持：
//     含 `/api/` 的从 `/api/` 起取，不含的统一补 `/api` 前缀。
//  4. 动态段归一为 `:param`，比较时**参数名不参与**：真实路由里有 `:id`、`:recordId`、
//     `:recordNo`…… docs/API.md 曾把 `/archives/seal/:id` 写成 `:recordId`，
//     那次教训说明「参数名写错」是描述问题而不是路由不存在；逐字比较会把这类
//     不一致误判成前端缺陷。故只比较路径形状（段数 + 静态段名 + 动态段位置）。
//  5. HTTP 方法能取到就一起比较（方法写错同样是缺陷，且同样只会在运行时暴露）；
//     `fetch` / URL 构造点取不到方法时，只在报告中标注「方法未识别，仅校验路径」。
//
// 已知局限（刻意的取舍，宁可漏报不可误报）：
//   - 经变量/函数中转的路径不可静态判定，如 `req('GET', path + sep + 'page=1')`
//     （fetchAll 内部实现）与 `fetch(self.reportExportURL(id))`；这类一律跳过并记日志。
//   - 形如 `API + '/api' + path` 的通用包装无法判定，跳过（见 normalizeCallPath 规则 4/5）。
//   - 形如 `/api/:param` 的纯动态路径跳过（本仓库不存在这种前端调用）。
//   - 注释里的示例调用同样会被提取（本文件不做 JS 词法级的注释剥离；
//     正则字面量如 `/"/g` 会骗过简单扫描器，强行剥离反而可能吞掉真实代码）。
//     当前前端源码中不存在仅在注释里出现的 req/fetch 调用，故无实际影响。
//   - 只扫 frontend/js 与 frontend/index.html：prototype/、serve.py 等不在契约范围内。
//
// ⚠ 必须用 `-count=1` 跑：frontend/ 位于 Go module（mrs-backend/）之外，
// 而 cmd/go 的测试缓存只跟踪 module 根内的 open 文件——cmd/go/internal/test 的
// computeTestInputsID 对 module 外路径直接跳过（`search.InDir(name, a.Package.Root) == ""`
// → break）。于是「只改前端、不改 Go 代码」时 `go test` 会命中缓存的 PASS：
// 实测把 api-store.js 的 /records/page 改成 /records/pages 后，不加 -count=1 得到的是
// `ok mrs/internal/router (cached)`，加了才 FAIL。对漂移检测器来说「缓存的假绿灯」
// 是最危险的失效模式，故此处显式提醒，并见测试开头的 t.Logf。
// （contract_test.go 读的 mrs-backend/docs/API.md 在 module 内，不受此影响。）
// =============================================================================

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 一、扫描范围与数据结构
// ---------------------------------------------------------------------------

// frontendRepoRoot 返回仓库根（yrzk/）的绝对路径。
// 本文件位于 mrs-backend/internal/router/，故由编译期源文件路径向上三级，
// 与 `go test` 的工作目录无关（contract_test.go 用同样的方式反推 mrs-backend/）。
func frontendRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 frontend_calls_test.go 源文件路径")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

// frontendSourceFiles 列出需要扫描的前端文件：frontend/js 下所有 .js（递归）
// 以及 frontend/index.html（确认是否存在内联脚本直接调接口；当前没有，但它是契约的一部分）。
func frontendSourceFiles(t *testing.T, root string) []string {
	t.Helper()

	jsDir := filepath.Join(root, "frontend", "js")
	if _, err := os.Stat(jsDir); err != nil {
		t.Fatalf("前端源码目录不存在（%s）: %v", jsDir, err)
	}

	var out []string
	err := filepath.WalkDir(jsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".js") {
			return nil
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatalf("遍历前端源码目录失败: %v", err)
	}

	html := filepath.Join(root, "frontend", "index.html")
	if _, err := os.Stat(html); err != nil {
		t.Fatalf("frontend/index.html 不存在: %v", err)
	}
	out = append(out, html)

	sort.Strings(out)
	return out
}

// frontendRequest 是「前端发起的一次后端请求」候选。
type frontendRequest struct {
	File   string // 相对仓库根，如 frontend/js/api-store.js
	Line   int    // 1-based 行号
	Raw    string // 原始写法（截断后），用于报告里人工核对
	Method string // 识别出的 HTTP 方法；空串表示未识别
	Path   string // 规范化后的 /api/... 路径
}

// registeredRoute 是已注册路由的形状投影。
type registeredRoute struct {
	method string
	segs   []string // 按 / 切开的段序列，动态段形如 ":id"
	line   string   // "METHOD /path" 原样，用于报告
}

// extractionResult 是一次源码扫描的结果：可判定的请求 + 不可判定的片段（附原因）。
type extractionResult struct {
	calls []frontendRequest
	skips []string
}

// ---------------------------------------------------------------------------
// 二、主测试：前端调用的每个后端路径都必须命中一条已注册路由
// ---------------------------------------------------------------------------

func TestFrontendCallsResolveToRegisteredRoutes(t *testing.T) {
	root := frontendRepoRoot(t)
	t.Logf("提示：本测试读取 module 之外的 frontend/ 源码，Go 测试缓存不跟踪它——" +
		"改动前端后请用 `go test -count=1` 运行，否则可能拿到缓存的假绿灯。")

	// ---- 事实源 1：后端已注册路由 ----
	routes := actualRoutes(t)
	regs := make([]registeredRoute, 0, len(routes))
	for _, line := range routes {
		method, path, _ := strings.Cut(line, " ")
		regs = append(regs, registeredRoute{method: method, segs: pathSegments(path), line: line})
	}

	// ---- 事实源 2：前端源码里的请求 ----
	targets := frontendSourceFiles(t, root)

	var (
		calls    []frontendRequest
		skips    []string
		skipNote int
	)
	for _, file := range targets {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("读取前端源码失败（%s）: %v", file, err)
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			rel = file
		}
		rel = filepath.ToSlash(rel)

		res := extractFrontendRequests(string(body))
		for i := range res.calls {
			res.calls[i].File = rel
		}
		calls = append(calls, res.calls...)
		for _, s := range res.skips {
			skips = append(skips, rel+":"+s)
		}
	}

	calls = dedupCalls(calls)
	t.Logf("扫描前端文件 %d 个（含 frontend/index.html）；提取到后端请求 %d 条；已注册路由 %d 条",
		len(targets), len(calls), len(regs))
	if len(calls) == 0 {
		// 空跑守卫：提取器失效（前端写法变更/正则失配）时必须响亮地失败，
		// 而不是「0 条调用 → 全部命中 → 绿灯」这种假通过。
		t.Fatal("未从前端源码提取到任何后端请求：提取器可能已失效，本测试拒绝空跑")
	}
	// 把提取结果全部打出来（`go test -v` 可见）：一旦匹配逻辑被改坏，
	// 从这份清单能立刻看出是「没有提取到」还是「提取错了」。
	listed := make([]string, 0, len(calls))
	for _, c := range calls {
		listed = append(listed, c.File+":"+strconv.Itoa(c.Line)+" "+methodLabel(c.Method)+" "+c.Path)
	}
	t.Logf("前端调用清单（%d 条）:\n\t%s", len(listed), strings.Join(listed, "\n\t"))

	// ---- 断言：逐条匹配 ----
	unknownMethod := 0
	for _, c := range calls {
		got := pathSegments(c.Path)

		var (
			pathMatched bool
			methodOK    bool
			methods     []string
		)
		for _, rt := range regs {
			if !shapesMatch(got, rt.segs) {
				continue
			}
			pathMatched = true
			methods = append(methods, rt.method)
			if c.Method != "" && rt.method == c.Method {
				methodOK = true
			}
		}

		switch {
		case !pathMatched:
			t.Errorf("前端调用了一个后端不存在的路径：\n"+
				"  位置:     %s:%d\n"+
				"  前端写法: %s\n"+
				"  规范化后: %s（方法 %s）\n"+
				"  判定:     路径不存在——后端没有任何已注册路由与之形状一致\n"+
				"  相近的已注册路由:\n\t%s",
				c.File, c.Line, c.Raw, c.Path, methodLabel(c.Method),
				strings.Join(nearRoutes(regs, got), "\n\t"))
		case c.Method == "":
			// 方法未识别（直接 fetch 的导出 URL 等）：只校验路径，但要在报告里说明。
			unknownMethod++
		case !methodOK:
			t.Errorf("前端调用的后端路径存在，但 HTTP 方法不匹配：\n"+
				"  位置:     %s:%d\n"+
				"  前端写法: %s\n"+
				"  规范化后: %s\n"+
				"  判定:     路径存在但方法不匹配——后端该路径已注册方法 [%s]，前端用的是 %s",
				c.File, c.Line, c.Raw, c.Path,
				strings.Join(dedupStrings(sortCopy(methods)), "/"), c.Method)
		}
	}

	if unknownMethod > 0 {
		t.Logf("%d 条请求未能识别 HTTP 方法（直接 fetch 的 URL / URL 构造点），仅校验路径", unknownMethod)
	}

	if len(skips) > 0 {
		skipNote = len(skips)
		sort.Strings(skips)
		const show = 30
		if len(skips) > show {
			skips = append(skips[:show], "…（其余省略）")
		}
		t.Logf("%d 处路径片段无法静态判定，已跳过（局限见文件头注释）：\n\t%s",
			skipNote, strings.Join(skips, "\n\t"))
	}
}

// ---------------------------------------------------------------------------
// 三、前端源码 → 请求清单
// ---------------------------------------------------------------------------

var (
	// 调用点。\b 保证 req( 不会匹配到 subreq(；fetch( 不会匹配到 fetchAll(（后面是 A 不是 (）。
	reReqCallSite      = regexp.MustCompile(`\breq\s*\(`)
	reFetchAllCallSite = regexp.MustCompile(`\bfetchAll\s*\(`)
	reFetchCallSite    = regexp.MustCompile(`\bfetch\s*\(`)
	// URL 构造点：本仓库唯一的 URL 拼接习惯 `API + '/api/...'`（如 ext.reportExportURL）。
	reAPIBaseConcat = regexp.MustCompile(`\bAPI\s*\+`)
)

// callSite 是一处调用点：start 为函数名起始下标，open 为左括号下标。
type callSite struct {
	start, open int
	kind        string
}

// extractFrontendRequests 扫描一份前端源码，提取其中所有后端请求。
func extractFrontendRequests(src string) extractionResult {
	var res extractionResult
	lines := lineIndex(src)
	seen := map[string]bool{}

	// record 把一处「路径表达式」记成 0..n 条候选请求（三元会展开成多条）。
	record := func(at int, method, raw, expr string) {
		line := lineOf(lines, at)
		cands := pathCandidates(expr)
		if len(cands) == 0 {
			res.skips = append(res.skips, strconv.Itoa(line)+" 行：路径表达式为空（可能是纯变量/函数调用）")
			return
		}
		for _, cand := range cands {
			p, ok := normalizeCallPath(cand)
			if !ok {
				// ph 是 NUL，直接打印看不见；这里换成可读记号，便于人工核对跳过了什么。
				res.skips = append(res.skips,
					strconv.Itoa(line)+" 行："+trimRaw(strings.ReplaceAll(cand, ph, "${动态}"))+
						" 不可静态判定（通用包装/纯动态路径）")
				continue
			}
			key := strconv.Itoa(line) + "|" + method + "|" + p
			if seen[key] {
				continue
			}
			seen[key] = true
			res.calls = append(res.calls, frontendRequest{
				Line:   line,
				Raw:    trimRaw(raw),
				Method: method,
				Path:   p,
			})
		}
	}

	// A) 调用点：req(METHOD, path[, body]) / fetchAll(path) / fetch(URL[, opt])
	for _, site := range findCallSites(src) {
		args, end := splitArgs(src, site.open)
		if end > len(src) {
			end = len(src)
		}
		raw := src[site.start : end+1]
		switch site.kind {
		case "req":
			record(site.start, literalMethod(argAt(args, 0)), raw, argAt(args, 1))
		case "fetchAll":
			// fetchAll(path) 内部固定 req('GET', path + '?page=..&size=100')，方法可确定为 GET。
			record(site.start, "GET", raw, argAt(args, 0))
		case "fetch":
			// 直接 fetch：方法取不到（浏览器默认 GET），按「方法未识别，仅校验路径」处理。
			record(site.start, "", raw, argAt(args, 0))
		}
	}

	// B) URL 构造点：`API + '/api/...'`（如 ext.reportExportURL 返回给 fetch 用的 URL）。
	//    刻意只认 `API +`：若放宽成「任意含 /api 的字符串字面量」，api-store.js 里
	//    OP_LABELS 的日志匹配标签（'/api/archives/seal' 等，形状与 /api/archives/seal/:id
	//    不同）会被当成请求，凭空造出缺陷。
	for _, m := range reAPIBaseConcat.FindAllStringIndex(src, -1) {
		end := exprEnd(src, m[1])
		if end > len(src) {
			end = len(src)
		}
		record(m[0], "", src[m[0]:end], src[m[1]:end])
	}

	return res
}

// findCallSites 找出所有 req(/fetchAll(/fetch( 调用点，按左括号下标排序。
func findCallSites(src string) []callSite {
	var sites []callSite
	add := func(re *regexp.Regexp, kind string) {
		for _, m := range re.FindAllStringIndex(src, -1) {
			open := strings.LastIndexByte(src[m[0]:m[1]], '(')
			if open < 0 {
				continue
			}
			sites = append(sites, callSite{start: m[0], open: m[0] + open, kind: kind})
		}
	}
	add(reReqCallSite, "req")
	add(reFetchAllCallSite, "fetchAll")
	add(reFetchCallSite, "fetch")
	sort.Slice(sites, func(i, j int) bool { return sites[i].open < sites[j].open })
	return sites
}

// splitArgs 从 src[open]（左括号）开始切分实参，返回实参文本与右括号下标。
// 只在顶层（括号/中括号/大括号深度为 0）的逗号处切分；字符串与模板串内容不参与切分。
func splitArgs(src string, open int) ([]string, int) {
	var args []string
	depth, start := 0, open+1
	for i := open + 1; i < len(src); i++ {
		switch c := src[i]; c {
		case '\'', '"', '`':
			i = skipJSString(src, i) - 1
		case '(', '[', '{':
			depth++
		case ')':
			if depth == 0 {
				args = append(args, src[start:i])
				return args, i
			}
			depth--
		case ']', '}':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, src[start:i])
				start = i + 1
			}
		}
	}
	args = append(args, src[start:])
	return args, len(src)
}

// exprEnd 从 src[i] 起找到一个 JS 表达式（语句/实参）的结束下标：
// 顶层的 `,` `;` 换行，或 `)` `]` `}` 收尾。
func exprEnd(src string, i int) int {
	depth := 0
	for ; i < len(src); i++ {
		switch c := src[i]; c {
		case '\'', '"', '`':
			i = skipJSString(src, i) - 1
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return i
			}
			depth--
		case ',', ';', '\n':
			if depth == 0 {
				return i
			}
		}
	}
	return len(src)
}

func argAt(args []string, i int) string {
	if i < 0 || i >= len(args) {
		return ""
	}
	return args[i]
}

// literalMethod 解析 req() 的第一个实参是否为字符串字面量方法名。
func literalMethod(arg string) string {
	s := strings.TrimSpace(arg)
	lit, ok := singleStringLiteral(s)
	if !ok {
		return ""
	}
	switch m := strings.ToUpper(lit); m {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return m
	}
	return ""
}

// ---------------------------------------------------------------------------
// 四、表达式 → 候选路径片段
// ---------------------------------------------------------------------------

// ph 是动态段的内部占位符。用 NUL 而不是 ":param"，是为了让「字面量里真的写了
// :param」与「动态拼接」在中间态可区分，最后统一替换。
const ph = "\x00"

// pathCandidates 把一个 JS 表达式解析成若干候选路径片段（占位符用 ph 表示）。
// 支持：字符串/模板字面量、`+` 拼接、三元（两个分支都算候选）、其余动态值 → ph。
func pathCandidates(expr string) []string {
	expr = strings.TrimSpace(expr)
	// 剥掉整体包裹的括号：`(period ? '?period=' + period : '')`
	for len(expr) >= 2 && expr[0] == '(' {
		end := matchCloseBracket(expr, 0)
		if end != len(expr)-1 {
			break
		}
		expr = strings.TrimSpace(expr[1:end])
	}
	if expr == "" {
		return nil
	}

	if lit, ok := singleStringLiteral(expr); ok {
		return []string{lit}
	}

	// 三元必须先于 `+` 判断：`a ? '?x=' + b : ''` 里的 `+` 在顶层，先切 `+` 会把三元撕坏。
	if q, c := topLevelTernary(expr); q >= 0 {
		out := append([]string{}, pathCandidates(expr[q+1:c])...)
		out = append(out, pathCandidates(expr[c+1:])...)
		return dedupStrings(out)
	}

	if parts := splitTopLevelPlus(expr); len(parts) > 1 {
		acc := []string{""}
		for _, part := range parts {
			cs := pathCandidates(part)
			if len(cs) == 0 {
				cs = []string{ph}
			}
			next := make([]string, 0, len(acc)*len(cs))
			for _, a := range acc {
				for _, c := range cs {
					next = append(next, a+c)
				}
			}
			if len(next) > 32 { // 组合上限，防止人为构造的深嵌套三元把内存打爆
				next = next[:32]
			}
			acc = next
		}
		return dedupStrings(acc)
	}

	// 叶子节点不是字面量：变量名、函数调用、`months || 6` 等 → 动态段。
	return []string{ph}
}

// singleStringLiteral 判断表达式整体是否就是一个字符串/模板字面量；
// 模板串里的 ${...} 变成占位符 ph。
func singleStringLiteral(expr string) (string, bool) {
	if len(expr) < 2 {
		return "", false
	}
	switch expr[0] {
	case '\'', '"':
		if expr[len(expr)-1] != expr[0] || skipJSString(expr, 0) != len(expr) {
			return "", false
		}
		return unquoteJS(expr), true
	case '`':
		if expr[len(expr)-1] != '`' || skipJSString(expr, 0) != len(expr) {
			return "", false
		}
		return templateToPattern(expr), true
	}
	return "", false
}

// unquoteJS 去掉字符串字面量的引号并还原简单转义。
func unquoteJS(s string) string {
	if len(s) < 2 {
		return s
	}
	body := s[1 : len(s)-1]
	var sb strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			i++
		}
		sb.WriteByte(body[i])
	}
	return sb.String()
}

// templateToPattern 把模板串（反引号包围）转成路径模式，`${...}` 变成占位符 ph。
func templateToPattern(lit string) string {
	if len(lit) < 2 {
		return ph
	}
	body := lit[1 : len(lit)-1]
	var sb strings.Builder
	for i := 0; i < len(body); i++ {
		switch c := body[i]; c {
		case '\\':
			if i+1 < len(body) {
				i++
				sb.WriteByte(body[i])
			}
		case '$':
			if i+1 < len(body) && body[i+1] == '{' {
				end := matchCloseBracket(body, i+1)
				sb.WriteString(ph)
				i = end
				continue
			}
			sb.WriteByte(c)
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// topLevelTernary 在表达式顶层查找三元运算符 `cond ? a : b`，
// 返回 `?` 与配对 `:` 的下标（找不到返回 -1, -1）。
// 跳过字符串/模板串与括号内部，并忽略 `??`、`?.` 这类非三元写法。
func topLevelTernary(expr string) (int, int) {
	firstQ, open, depth := -1, 0, 0
	for i := 0; i < len(expr); i++ {
		switch c := expr[i]; c {
		case '\'', '"', '`':
			i = skipJSString(expr, i) - 1
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '?':
			if depth != 0 {
				continue
			}
			if i+1 < len(expr) && (expr[i+1] == '?' || expr[i+1] == '.') {
				i++
				continue
			}
			if firstQ < 0 {
				firstQ = i
			}
			open++
		case ':':
			if depth != 0 || open == 0 {
				continue
			}
			open--
			if open == 0 {
				return firstQ, i
			}
		}
	}
	return -1, -1
}

// splitTopLevelPlus 在顶层 `+` 处切分表达式（字符串内的 `+` 不切）。
func splitTopLevelPlus(expr string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(expr); i++ {
		switch c := expr[i]; c {
		case '\'', '"', '`':
			i = skipJSString(expr, i) - 1
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '+':
			if depth == 0 {
				parts = append(parts, expr[start:i])
				start = i + 1
			}
		}
	}
	if len(parts) == 0 {
		return []string{expr}
	}
	return append(parts, expr[start:])
}

// skipJSString 从 src[i]（'、" 或 `）跳到该字符串结束后的下标。
// 单/双引号串遇到换行即视为未闭合（JS 里它们本来也不能跨行）：这是刻意的伤害限制——
// 正则字面量（如 api-store.js 里的 /"/g）会骗过简单词法扫描，若不加限制，
// 一次误判可能吞掉文件剩下的大量内容。
func skipJSString(src string, i int) int {
	q := src[i]
	i++
	for i < len(src) {
		c := src[i]
		if c == '\\' {
			i += 2
			continue
		}
		if c == '\n' && q != '`' {
			return i
		}
		if c == q {
			return i + 1
		}
		i++
	}
	return i
}

// matchCloseBracket 返回 src[open] 配对的下标（open 处为 ( [ { 之一）；未闭合返回 len(src)。
func matchCloseBracket(src string, open int) int {
	depth := 0
	for i := open; i < len(src); i++ {
		switch c := src[i]; c {
		case '\'', '"', '`':
			i = skipJSString(src, i) - 1
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(src)
}

// ---------------------------------------------------------------------------
// 五、规范化与形状比较
// ---------------------------------------------------------------------------

// normalizeCallPath 把前端提取到的路径片段规范化为可与注册路由比较的 `/api/...` 形式。
// 返回 false 表示「无法静态判定」，按局限处理（跳过并在日志中列出），不参与断言。
func normalizeCallPath(cand string) (string, bool) {
	p := strings.ReplaceAll(cand, ph, ":param")

	// 规则 1：截断查询串与锚点——注册路由里没有 `?keyword=...`。
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if p == "" {
		return "", false
	}

	// 规则 2：`/api` 前缀双向统一。
	if i := strings.Index(p, "/api/"); i >= 0 {
		// 含 /api 的写法（`API + '/api/report-batches/' + id + '/export'`）：
		// 前缀里的 API 是动态段，取 `/api/` 起的部分即可。
		p = p[i:]
	} else if !strings.HasPrefix(p, "/") {
		// 既不含 /api/ 又不以 / 开头：是 `path + sep + 'page='` 这类中转表达式，跳过。
		return "", false
	}

	// /healthz 不在 /api 前缀下（router.go 直接注册在根上），原样返回。
	if strings.HasPrefix(p, "/healthz") {
		return p, true
	}

	// 规则 4：`API + '/api' + path` 这类通用包装（req/fetchAll 内部实现）——
	// 字面量在 `/api` 处就断了，后面接的是动态段，无法判定，跳过。
	if strings.HasPrefix(p, "/api") && !strings.HasPrefix(p, "/api/") {
		return "", false
	}
	// 规则 3：不含 /api 的字面量（req 会补前缀）统一补成 /api/...
	if !strings.HasPrefix(p, "/api/") {
		p = "/api" + p
	}
	p = strings.TrimRight(p, "/")

	// 规则 5：`/api/:param` 这种「首段就是动态」的路径无法判定（通用包装），跳过。
	segs := pathSegments(p)
	if len(segs) < 2 || segs[1] == ":param" {
		return "", false
	}
	return p, true
}

// pathSegments 把路径切成段：`/api/records/:id` → ["api","records",":id"]。
func pathSegments(p string) []string {
	return strings.Split(strings.Trim(p, "/"), "/")
}

// shapesMatch 只比较路径「形状」：
//   - 段数必须相同；
//   - 动态段与动态段互为同形（参数名不参与比较，见文件头规则 4）；
//   - 静态段与静态段必须逐字相同（后端路由区分大小写）。
func shapesMatch(got, route []string) bool {
	if len(got) != len(route) {
		return false
	}
	for i := range got {
		gp, rp := isPathParam(got[i]), isPathParam(route[i])
		if gp || rp {
			if !(gp && rp) {
				return false
			}
			continue
		}
		if got[i] != route[i] {
			return false
		}
	}
	return true
}

func isPathParam(seg string) bool { return strings.HasPrefix(seg, ":") }

// nearRoutes 找出与 got 前缀最相近的已注册路由，让失败信息能直接指导修复。
func nearRoutes(regs []registeredRoute, got []string) []string {
	best := 0
	var out []string
	for _, rt := range regs {
		n := 0
		for n < len(got) && n < len(rt.segs) && got[n] == rt.segs[n] {
			n++
		}
		if n == 0 {
			continue
		}
		if n > best {
			best = n
			out = out[:0]
		}
		if n == best {
			out = append(out, rt.line)
		}
	}
	sort.Strings(out)
	if len(out) > 6 {
		out = out[:6]
	}
	if len(out) == 0 {
		return []string{"（无）"}
	}
	return out
}

// ---------------------------------------------------------------------------
// 六、小工具
// ---------------------------------------------------------------------------

// lineIndex 返回每行起始下标（0 起）。
func lineIndex(src string) []int {
	idx := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			idx = append(idx, i+1)
		}
	}
	return idx
}

// lineOf 返回下标 i 所在的 1-based 行号。
func lineOf(idx []int, i int) int {
	return sort.SearchInts(idx, i+1)
}

func dedupStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func dedupCalls(in []frontendRequest) []frontendRequest {
	seen := make(map[string]bool, len(in))
	out := make([]frontendRequest, 0, len(in))
	for _, c := range in {
		key := c.File + "|" + strconv.Itoa(c.Line) + "|" + c.Method + "|" + c.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func sortCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func methodLabel(m string) string {
	if m == "" {
		return "未识别，仅校验路径"
	}
	return m
}

// trimRaw 压缩空白并按 rune 截断，用于把原始写法放进报告。
func trimRaw(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 140 {
		return string(r[:140]) + "…"
	}
	return s
}
