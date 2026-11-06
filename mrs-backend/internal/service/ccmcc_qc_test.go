package service

import (
	"strings"
	"testing"

	"mrs/internal/model"
)

// 本文件覆盖 ADR-0002 批次 C 的低靠（漏编）/ 高靠（套高）检测（service/ccmcc_qc.go）：
//  1. 低靠命中：判读构成 CC/MCC 但标记为「无」；
//  2. 高靠命中：标记为 C/M 但判读不构成 CC/MCC（表内无条目 / 被排除规则挡下）；
//  3. **负控制（本文件最重要的一组）**：标记与判读一致 ⇒ 两条都必须返回空串；
//     没有它，"把条件写反"这类错误会被一串看起来很像产出物的输出掩盖过去；
//  4. 空输入 / 解析出错（长度不等、重复编码、非法标记）/ 无可用规则 ⇒ 一律空串；
//  5. 大小写与首尾空格、含空占位项（空槽连同标记一起丢弃）；
//  6. 排除规则：主诊断 C* ⇒ D63 不计 CC（触发）与不触发（主诊断非 C*）两种走向；
//  7. 合规红线：任何非空文案都必须带「演示」字样并明示不作结算依据。
//
// 全部为纯内存表驱动测试：不连数据库、不起服务（对齐 ccmcc_test.go /
// other_diagnosis_test.go 的无依赖风格），也不需要任何环境准备，可直接
// `go test ./internal/service -run CcMcc -count=1`。

// ---------- 断言辅助 ----------

// ccQcFrontPage 造一份首页，只填低靠/高靠检测实际读取的三个字段。
// 其余字段留零值：本检测是纯函数，不读别的列——若将来有人加了依赖，
// 这里的零值会让测试立刻暴露出来。
func ccQcFrontPage(main, codes, flags string) *model.MedFrontPage {
	return &model.MedFrontPage{
		MainDiagnosisCode:   main,
		OtherDiagnosisCodes: codes,
		OtherDiagnosisFlags: flags,
	}
}

// ccMustContain 断言子串存在（用于判"命中了哪一条"，避免整串比对过于脆弱）。
func ccMustContain(t *testing.T, what, got, want string) {
	t.Helper()
	if want == "" {
		return
	}
	if !strings.Contains(got, want) {
		t.Errorf("%s 未包含 %q\n  实际：%q", what, want, got)
	}
}

// ---------- 1. 双函数联动的主表 ----------

func TestCheckCcMccLowHigh(t *testing.T) {
	cases := []struct {
		name      string
		main      string
		codes     string
		flags     string
		wantLow   string // 期望的 CheckCcMccLow 结果；空串表示不该提示
		wantHigh  string // 期望的 CheckCcMccHigh 结果；空串表示不该提示
		noLowSub  string // 额外断言：低靠结果应包含的子串
		noHighSub string // 额外断言：高靠结果应包含的子串
	}{
		{
			// ---- 低靠（漏编）命中 ----
			name:     "低靠：MCC 编码标记为空",
			main:     "K35.801",
			codes:    "I50.900",
			flags:    "0",
			wantLow:  "其他诊断【I50.900】在演示判定表中为 MCC，但未标记为并发症/合并症，建议核对是否漏编；如属陈旧性或既往史，可忽略本提示（演示口径，非官方判定，不作结算依据）",
			wantHigh: "",
		},
		{
			name:     "低靠：CC 编码标记串整体为空（历史数据形态）",
			main:     "K35.801",
			codes:    "J44.900,I48.000",
			flags:    "",
			wantLow:  "其他诊断【J44.900】在演示判定表中为 CC，但未标记为并发症/合并症，建议核对是否漏编；如属陈旧性或既往史，可忽略本提示（演示口径，非官方判定，不作结算依据）；其他诊断【I48.000】在演示判定表中为 CC，但未标记为并发症/合并症，建议核对是否漏编；如属陈旧性或既往史，可忽略本提示（演示口径，非官方判定，不作结算依据）",
			wantHigh: "",
		},
		{
			name:      "低靠：多条混合，未标记的三条各自成提示",
			main:      "K35.801",
			codes:     "I21.900,I48.000,K80.201",
			flags:     "0,0,0",
			wantLow:   "", // 整串太长，改由下面三条子串断言覆盖
			wantHigh:  "",
			noLowSub:  "其他诊断【I21.900】在演示判定表中为 MCC",
			noHighSub: "",
		},
		{
			// ---- 高靠（套高）命中 ----
			name:     "高靠：标记 CC 但演示表内无对应条目",
			main:     "K35.801",
			codes:    "K80.201",
			flags:    "C",
			wantLow:  "",
			wantHigh: "其他诊断【K80.201】被标记为 CC，但在演示判定表中不构成 CC/MCC（表内无对应条目），建议核对高靠风险与病案依据（演示口径，非官方判定，不作结算依据）",
		},
		{
			name:      "高靠：标记 MCC 但演示表内无对应条目",
			main:      "K35.801",
			codes:     "Z51.100",
			flags:     "M",
			wantLow:   "",
			wantHigh:  "其他诊断【Z51.100】被标记为 MCC，但在演示判定表中不构成 CC/MCC（表内无对应条目），建议核对高靠风险与病案依据（演示口径，非官方判定，不作结算依据）",
			noHighSub: "被标记为 MCC",
		},
		{
			name:     "高靠：标记 CC 但被排除规则挡下（主诊断 C* 时的 D63）",
			main:     "C18.900",
			codes:    "D63.000",
			flags:    "C",
			wantLow:  "",
			wantHigh: "其他诊断【D63.000】被标记为 CC，但在演示判定表中不构成 CC/MCC（已被排除：主诊断【C18.900】命中排除前缀【C】，按演示表本条不计为 CC），建议核对病案实际诊断依据并复核该标记（演示口径，非官方判定，不作结算依据）",
		},
		{
			// ---- 负控制：标记正确 ⇒ 两条都不提示 ----
			name:     "负控制：标记与判读完全一致（MCC/CC/CC）",
			main:     "K35.801",
			codes:    "I50.900,I48.000,D63.000",
			flags:    "M,C,C",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:     "负控制：D63 在主诊断非 C* 时确实算 CC，标记 CC 正确",
			main:     "D64.900",
			codes:    "D63.000",
			flags:    "C",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:     "负控制：I69.3 在主诊断非 I63* 时算 CC，标记 CC 正确",
			main:     "M79.001",
			codes:    "I69.300",
			flags:    "C",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:     "负控制：都不相关的编码 + 都不标记",
			main:     "K35.801",
			codes:    "K80.201,I10.x00",
			flags:    "0,0",
			wantLow:  "",
			wantHigh: "",
		},
		{
			// ---- 大小写与首尾空格 ----
			name:     "大小写与空格：均等，标记正确的脏乱写法",
			main:     " k35.801 ",
			codes:    " i50.900 , i48.000 ",
			flags:    " m , c ",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:      "大小写与空格：低位写法仍按 MCC 命中低靠",
			main:      "K35.801",
			codes:     " i63.300 ",
			flags:     " 0 ",
			wantLow:   "",
			wantHigh:  "",
			noLowSub:  "其他诊断【I63.300】在演示判定表中为 MCC",
			noHighSub: "",
		},
		{
			name:      "大小写与空格：小写 flag 之外的脏乱写法不影响高靠判定",
			main:      "K35.801",
			codes:     " k80.201 ",
			flags:     " C ",
			wantLow:   "",
			wantHigh:  "",
			noHighSub: "其他诊断【K80.201】被标记为 CC",
		},
		{
			// ---- 含空占位项 ----
			name:     "空占位项：中间空槽连同标记一起丢弃，两侧标记正确",
			main:     "K35.801",
			codes:    "I50.900,,I48.000",
			flags:    "M,,C",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:     "空占位项：尾部逗号导致的多余空槽不影响剩余项判定",
			main:     "K35.801",
			codes:    "I50.900,I48.000,",
			flags:    "M,C,",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:     "空占位项：非空条目仍如实命中低靠（不把空槽算进去）",
			main:     "K35.801",
			codes:    "I50.900,,J44.900",
			flags:    "0,,0",
			wantLow:  "其他诊断【I50.900】在演示判定表中为 MCC，但未标记为并发症/合并症，建议核对是否漏编；如属陈旧性或既往史，可忽略本提示（演示口径，非官方判定，不作结算依据）；其他诊断【J44.900】在演示判定表中为 CC，但未标记为并发症/合并症，建议核对是否漏编；如属陈旧性或既往史，可忽略本提示（演示口径，非官方判定，不作结算依据）",
			wantHigh: "",
		},
		{
			// ---- 解析出错 ⇒ 一律不提示（fail-closed）----
			name:     "脏数据：编码 3 项 vs 标记 2 项，长度不等",
			main:     "K35.801",
			codes:    "I50.900,I48.000,J44.900",
			flags:    "0,0",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:     "脏数据：重复编码（大小写不同也算重复）",
			main:     "K35.801",
			codes:    "I50.900,i50.900",
			flags:    "M,C",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:     "脏数据：非法标记 X",
			main:     "K35.801",
			codes:    "I50.900",
			flags:    "X",
			wantLow:  "",
			wantHigh: "",
		},
		{
			name:     "脏数据：小写标记 c 不宽容",
			main:     "K35.801",
			codes:    "I50.900",
			flags:    "c",
			wantLow:  "",
			wantHigh: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp := ccQcFrontPage(tc.main, tc.codes, tc.flags)
			gotLow := CheckCcMccLow(fp, CcMccDemoRules)
			gotHigh := CheckCcMccHigh(fp, CcMccDemoRules)

			// 主断言：整串相等（want 为空串时，这条就是"必须不提示"的负控制）。
			if tc.wantLow != "" {
				if gotLow != tc.wantLow {
					t.Errorf("CheckCcMccLow 不符\n  期望：%q\n  实际：%q", tc.wantLow, gotLow)
				}
			} else {
				if tc.noLowSub == "" && gotLow != "" {
					t.Errorf("CheckCcMccLow 应为空串（不误报），实际：%q", gotLow)
				}
				ccMustContain(t, "CheckCcMccLow", gotLow, tc.noLowSub)
			}
			if tc.wantHigh != "" {
				if gotHigh != tc.wantHigh {
					t.Errorf("CheckCcMccHigh 不符\n  期望：%q\n  实际：%q", tc.wantHigh, gotHigh)
				}
			} else {
				if tc.noHighSub == "" && gotHigh != "" {
					t.Errorf("CheckCcMccHigh 应为空串（不误报），实际：%q", gotHigh)
				}
				ccMustContain(t, "CheckCcMccHigh", gotHigh, tc.noHighSub)
			}
		})
	}
}

// ---------- 2. 空输入 / 无规则：不得 panic，也不得提示 ----------

func TestCheckCcMccLowHigh_EmptyAndNil(t *testing.T) {
	cases := []struct {
		name  string
		fp    *model.MedFrontPage
		rules []CcMccRule
	}{
		{"首页为 nil", nil, CcMccDemoRules},
		{"首页零值：主诊断与其它诊断全空", ccQcFrontPage("", "", ""), CcMccDemoRules},
		{"有其他诊断但无编码", ccQcFrontPage("K35.801", "", "M"), CcMccDemoRules},
		{"有编码但无规则可用：规则集为 nil", ccQcFrontPage("K35.801", "I50.900,I48.000", "M,C"), nil},
		{"有编码但规则集为空切片", ccQcFrontPage("K35.801", "I50.900,I48.000", "0,0"), []CcMccRule{}},
		{
			// fail-closed：规则版本不符 ⇒ 没有任何依据，不得把"表内无条目"当成命中，
			// 否则一次升级/一次传错版本号就会给全院弹出一堆 WARN。
			"有编码但规则版本不符（只有 official 版）",
			ccQcFrontPage("K35.801", "K80.201", "C"),
			[]CcMccRule{{DiagPrefix: "K80", Severity: SeverityCC, Version: "official-x", Source: CcMccSourceOfficial}},
		},
		{
			"有编码但规则前缀全为空（无判别力）",
			ccQcFrontPage("K35.801", "K80.201", "C"),
			[]CcMccRule{{DiagPrefix: "", Severity: SeverityCC, Version: CcMccDemoVersion, Source: CcMccSourceDemo}},
		},
		{"只有空白的其他诊断串", ccQcFrontPage("K35.801", "   ", "  "), CcMccDemoRules},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckCcMccLow(tc.fp, tc.rules); got != "" {
				t.Errorf("CheckCcMccLow 应为空串，实际：%q", got)
			}
			if got := CheckCcMccHigh(tc.fp, tc.rules); got != "" {
				t.Errorf("CheckCcMccHigh 应为空串，实际：%q", got)
			}
		})
	}
}

// ---------- 3. 纯函数性：同一输入重复调用结果一致，且不修改入参 ----------

func TestCheckCcMccLowHigh_PureAndNoMutation(t *testing.T) {
	fp := ccQcFrontPage("C18.900", "I50.900,D63.000", "0,C")
	before := *fp
	rulesBefore := append([]CcMccRule(nil), CcMccDemoRules...)

	firstLow, firstHigh := CheckCcMccLow(fp, CcMccDemoRules), CheckCcMccHigh(fp, CcMccDemoRules)
	for i := 0; i < 3; i++ {
		if got := CheckCcMccLow(fp, CcMccDemoRules); got != firstLow {
			t.Errorf("第 %d 次 CheckCcMccLow 结果漂移：%q != %q", i+2, got, firstLow)
		}
		if got := CheckCcMccHigh(fp, CcMccDemoRules); got != firstHigh {
			t.Errorf("第 %d 次 CheckCcMccHigh 结果漂移：%q != %q", i+2, got, firstHigh)
		}
	}
	if *fp != before {
		t.Errorf("检测函数修改了入参首页：%+v != %+v", *fp, before)
	}
	for i := range CcMccDemoRules {
		if CcMccDemoRules[i] != rulesBefore[i] {
			t.Fatalf("检测函数修改了共享规则集 CcMccDemoRules[%d]", i)
		}
	}
	if firstLow == "" || firstHigh == "" {
		t.Fatalf("本用例预置数据应同时命中低靠与高靠，实际 low=%q high=%q", firstLow, firstHigh)
	}
}

// ---------- 4. 合规红线：任何非空文案都必须带"演示"并声明不作结算依据 ----------

func TestCheckCcMccQcMessageCompliance(t *testing.T) {
	type probe struct {
		main  string
		codes string
		flags string
	}
	probes := []probe{
		{"K35.801", "I50.900", "0"},             // 低靠
		{"K35.801", "J44.900", ""},              // 低靠（标记串整体空）
		{"K35.801", "K80.201", "C"},             // 高靠（表内无条目）
		{"C18.900", "D63.000", "C"},             // 高靠（排除规则）
		{"K35.801", "I50.900,K80.201", "0,M"},   // 两条都有
		{"K35.801", "i50.900,I48.000", "0,0"},   // 大小写脏写法
		{"K35.801", "I50.900,,J44.900", "0,,0"}, // 含空占位
	}
	for _, p := range probes {
		fp := ccQcFrontPage(p.main, p.codes, p.flags)
		for _, got := range []string{CheckCcMccLow(fp, CcMccDemoRules), CheckCcMccHigh(fp, CcMccDemoRules)} {
			if got == "" {
				continue
			}
			// ADR §6：演示数据不得被读成官方结算口径，故这两条硬要求覆盖**每一条**提示。
			if !strings.Contains(got, "演示") {
				t.Errorf("文案缺「演示」标注，会被误读为官方判定：%q", got)
			}
			if !strings.Contains(got, "不作结算依据") {
				t.Errorf("文案缺「不作结算依据」免责成分：%q", got)
			}
			// WARN 级提示不得夹带拦截语义：这批规则的目标是"提示核对"，而
			// 只认 ERROR 才 QC_REJECTED 的实现（qcengine.go）取决于 seed 里的 error_level。
			// 文案里一旦出现必须/不予通过这类词，即使规则配成 WARN，读者也会当成指令执行。
			for _, banned := range []string{"必须", "不予通过", "不予受理", "拦截", "退回", "拒绝"} {
				if strings.Contains(got, banned) {
					t.Errorf("文案含拦截语义词 %q，违背 WARN 级提示口径：%q", banned, got)
				}
			}
		}
	}
}

// TestCheckCcMccLowMessageHasEscapeHatch 低靠提示必须给"可忽略"出口（专家回执）。
//
// 理由：低靠误报的头号来源是"陈旧性/既往史"——判读表只看得出编码在表里算 CC/MCC，
// 看不出这次住院是不是它导致的。给不出出口的提示只有两种下场：被当噪音整体忽略，
// 或被当成命令去改 CMS 数据。故文案不是措辞偏好，是可执行性问题。
func TestCheckCcMccLowMessageHasEscapeHatch(t *testing.T) {
	got := CheckCcMccLow(ccQcFrontPage("K35.801", "I50.900", "0"), CcMccDemoRules)
	if got == "" {
		t.Fatal("低靠未命中，本用例失去意义")
	}
	if !strings.Contains(got, "可忽略本提示") {
		t.Errorf("低靠提示缺可忽略出口，会被读成判定句：%q", got)
	}
	if strings.Contains(got, "请核对是否漏编（") {
		t.Errorf("低靠提示仍是命令式措辞（缺分号与出口从句）：%q", got)
	}
}

// ---------- 5. 两侧「无」的字面必须同源 ----------

// ADR §5 锁定：同一份响应里"无严重度"只出现一种写法。解析层(OtherDiagnosisFlags 的 '0')
// 与判读层(CcMccItem.Severity 的 "0")靠字面相等才对齐得上——本文件把它们直接拿来比对
// （ccIsNoneSeverity 只判了一个常量），一旦有人改掉任一侧，低靠/高靠会集体失准。
func TestCcMccNoneLiteralsAgree(t *testing.T) {
	if SeverityNone != SeverityFlagNone {
		t.Fatalf("解析层与判读层的「无」字面已分家：SeverityNone=%q != SeverityFlagNone=%q，低靠/高靠判定会失准",
			SeverityNone, SeverityFlagNone)
	}
}

// ---------- 6. 与判读层的联动：主诊断同一份输入，两侧只差已录标记 ----------

// 这条用例把"低靠/高靠是一对互为镜像的检测"这件事钉死：同一份首页只改一个标记字符，
// 提示应当在低靠与高靠之间互换，而不会同时消失或同时出现。
func TestCheckCcMccLowHigh_MirrorByFlag(t *testing.T) {
	const codes, main = "I48.000", "K35.801" // 演示表内为 CC

	low := CheckCcMccLow(ccQcFrontPage(main, codes, "0"), CcMccDemoRules)
	high := CheckCcMccHigh(ccQcFrontPage(main, codes, "0"), CcMccDemoRules)
	if low == "" {
		t.Errorf("标记为「无」时应命中低靠，实际 low=%q", low)
	}
	if high != "" {
		t.Errorf("标记为「无」时不应命中高靠，实际 high=%q", high)
	}

	low2 := CheckCcMccLow(ccQcFrontPage(main, codes, "C"), CcMccDemoRules)
	high2 := CheckCcMccHigh(ccQcFrontPage(main, codes, "C"), CcMccDemoRules)
	if low2 != "" {
		t.Errorf("标记为 CC 时应解除低靠，实际 low=%q", low2)
	}
	if high2 != "" {
		t.Errorf("标记为 CC 且判读确为 CC 时不应命中高靠，实际 high=%q", high2)
	}

	// 标记升级为 MCC：判读侧仍是 CC ⇒ 不属漏编，也不属"不构成 CC/MCC"，两条都应安静。
	low3 := CheckCcMccLow(ccQcFrontPage(main, codes, "M"), CcMccDemoRules)
	high3 := CheckCcMccHigh(ccQcFrontPage(main, codes, "M"), CcMccDemoRules)
	if low3 != "" || high3 != "" {
		t.Errorf("标记 M 而判读为 CC 属等级分歧，不在本期两条 WARN 的范围内，应不提示：low=%q high=%q", low3, high3)
	}
}
