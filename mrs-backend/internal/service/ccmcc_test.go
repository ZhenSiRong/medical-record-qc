package service

import (
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm/schema"

	"mrs/internal/model"
)

// 本文件覆盖 ADR-0002 §4/§5 批次 B 的 CC/MCC 判读表（service/ccmcc.go）：
//   - 命中 MCC / 命中 CC / 都不命中，以及总体分层 MCC > CC > 0；
//   - 版本过滤：非目标版本的规则不生效（含"空版本 ⇒ fail-closed"）；
//   - 排除规则：主诊断命中 ExcludeWhenMainPrefix 时该条不适用；
//   - 免责声明：Source=demo 时必须存在（且任何输入下都存在）；
//   - 空输入（nil 规则 / nil 编码 / 空主诊断 / 空版本）不 panic；
//   - 与解析层（other_diagnosis.go）的按位置对齐联动；
//   - 演示规则集的表自检，以及新增字段/列名/JSON 名与 Save 白名单的一致性。
//
// 全部为纯内存测试：不连数据库、不起服务（对齐 internal/service/qc_condition_d3_test.go
// 与 internal/router/router_test.go 的无依赖风格）。

// ccFindItem 按编码取判读结果中的一项（保持"按位置对齐"之外的可读断言）。
func ccFindItem(t *testing.T, j CcMccJudgement, code string) CcMccItem {
	t.Helper()
	for _, it := range j.Items {
		if it.Code == code {
			return it
		}
	}
	t.Fatalf("判读结果中没有编码 %s（items=%+v）", code, j.Items)
	return CcMccItem{}
}

// ccSevs 取出判读结果的严重度序列，用于整体比较。
func ccSevs(j CcMccJudgement) []string {
	out := make([]string, 0, len(j.Items))
	for _, it := range j.Items {
		out = append(out, it.Severity)
	}
	return out
}

// ---------- 1. 主诊断 + 其他诊断的判读 ----------

func TestCcMccJudgeSeverity_HitMccCcNone(t *testing.T) {
	cases := []struct {
		name    string
		main    string
		others  []string
		want    []string // 逐项严重度，与 others 等长
		overall string
		mccCnt  int
		ccCnt   int
	}{
		{
			name: "MCC/CC/未命中 混合：总体取 MCC", main: "K35.801",
			others:  []string{"I50.900", "I48.000", "K80.201"},
			want:    []string{SeverityMCC, SeverityCC, SeverityNone},
			overall: SeverityMCC, mccCnt: 1, ccCnt: 1,
		},
		{
			name: "只命中 CC：总体 CC", main: "K35.801",
			others:  []string{"I48.000", "K80.201"},
			want:    []string{SeverityCC, SeverityNone},
			overall: SeverityCC, mccCnt: 0, ccCnt: 1,
		},
		{
			name: "都不命中：总体 0", main: "K35.801",
			others:  []string{"K80.201", "Z98.800"},
			want:    []string{SeverityNone, SeverityNone},
			overall: SeverityNone, mccCnt: 0, ccCnt: 0,
		},
		{
			name: "无其他诊断：总体 0", main: "K35.801",
			others:  nil,
			want:    []string{},
			overall: SeverityNone,
		},
		{
			name: "小写 + 首尾空格同样命中（规范化口径与 drg_rules.go 一致）", main: " k35.801 ",
			others:  []string{" i50.900 ", "i48.000"},
			want:    []string{SeverityMCC, SeverityCC},
			overall: SeverityMCC, mccCnt: 1, ccCnt: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := JudgeSeverity(CcMccDemoVersion, c.main, c.others, CcMccDemoRules)
			if got := ccSevs(j); !reflect.DeepEqual(got, c.want) {
				t.Errorf("逐项严重度 = %v, 期望 %v", got, c.want)
			}
			if j.OverallSeverity != c.overall {
				t.Errorf("总体分层 = %q, 期望 %q", j.OverallSeverity, c.overall)
			}
			if j.MccCount != c.mccCnt || j.CcCount != c.ccCnt {
				t.Errorf("计数 MCC=%d CC=%d, 期望 MCC=%d CC=%d", j.MccCount, j.CcCount, c.mccCnt, c.ccCnt)
			}
			if len(j.Items) != len(c.others) {
				t.Errorf("items 长度 = %d, 期望与入参等长 %d", len(j.Items), len(c.others))
			}
			if j.MainDiagnosisCode != strings.ToUpper(strings.TrimSpace(c.main)) {
				t.Errorf("回显主诊断 = %q, 期望规范化值", j.MainDiagnosisCode)
			}
			for i, it := range j.Items {
				if it.Code != strings.ToUpper(strings.TrimSpace(c.others[i])) {
					t.Errorf("第 %d 项编码 = %q, 期望与入参同序同值", i, it.Code)
				}
			}
		})
	}
}

// TestCcMccJudgeSeverity_MostSpecificPrefixWins 固化"多前缀命中取最长"的口径
// （同 handler/drg_preview.go 的 bestScore：更具体的规则优先）。
func TestCcMccJudgeSeverity_MostSpecificPrefixWins(t *testing.T) {
	rules := []CcMccRule{
		{DiagPrefix: "I50", Severity: SeverityMCC, Version: "v-test", Source: CcMccSourceDemo},
		{DiagPrefix: "I50.1", Severity: SeverityCC, Version: "v-test", Source: CcMccSourceDemo},
		{DiagPrefix: "", Severity: SeverityMCC, Version: "v-test", Source: CcMccSourceDemo}, // 空前缀必须被忽略
	}
	j := JudgeSeverity("v-test", "K35.801", []string{"I50.100", "I50.900"}, rules)
	if got, want := ccSevs(j), []string{SeverityCC, SeverityMCC}; !reflect.DeepEqual(got, want) {
		t.Errorf("逐项严重度 = %v, 期望 %v（长前缀 I50.1 优先；空前缀不参与匹配）", got, want)
	}
	if got := ccFindItem(t, j, "I50.100").MatchedPrefix; got != "I50.1" {
		t.Errorf("命中前缀 = %q, 期望 I50.1", got)
	}
}

// ---------- 2. 版本过滤 ----------

func TestCcMccJudgeSeverity_VersionFilter(t *testing.T) {
	rules := []CcMccRule{
		{DiagPrefix: "I50", Severity: SeverityMCC, Version: CcMccDemoVersion, Source: CcMccSourceDemo},
		{DiagPrefix: "I48", Severity: SeverityCC, Version: "official-2.0", Source: CcMccSourceOfficial},
	}
	codes := []string{"I50.900", "I48.000"}

	t.Run("只应用指定版本：official-2.0 的规则在 demo 版本下不生效", func(t *testing.T) {
		j := JudgeSeverity(CcMccDemoVersion, "K35.801", codes, rules)
		if got, want := ccSevs(j), []string{SeverityMCC, SeverityNone}; !reflect.DeepEqual(got, want) {
			t.Errorf("逐项严重度 = %v, 期望 %v", got, want)
		}
		if j.Version != CcMccDemoVersion {
			t.Errorf("回显版本 = %q, 期望 %q", j.Version, CcMccDemoVersion)
		}
		if got := j.Sources; !reflect.DeepEqual(got, []string{CcMccSourceDemo}) {
			t.Errorf("来源集合 = %v, 期望 [demo]", got)
		}
		if !j.DemoOnly {
			t.Error("只用了演示规则，DemoOnly 应为 true（界面须显著标注非官方）")
		}
	})

	t.Run("切到 official-2.0 版本：demo 规则不生效", func(t *testing.T) {
		j := JudgeSeverity("official-2.0", "K35.801", codes, rules)
		if got, want := ccSevs(j), []string{SeverityNone, SeverityCC}; !reflect.DeepEqual(got, want) {
			t.Errorf("逐项严重度 = %v, 期望 %v", got, want)
		}
		if j.DemoOnly {
			t.Error("命中的是 official 规则，DemoOnly 应为 false")
		}
	})

	t.Run("空版本 fail-closed：任何规则都不生效", func(t *testing.T) {
		for _, v := range []string{"", "   "} {
			j := JudgeSeverity(v, "K35.801", codes, rules)
			if got, want := ccSevs(j), []string{SeverityNone, SeverityNone}; !reflect.DeepEqual(got, want) {
				t.Errorf("版本 %q 下逐项严重度 = %v, 期望 %v（不得静默套用其他版本的表）", v, got, want)
			}
			if j.Sources == nil || len(j.Sources) != 0 {
				t.Errorf("版本 %q 下来源集合应为空, 实际 %v", v, j.Sources)
			}
		}
	})

	t.Run("未知版本不影响其他版本规则的判定结果", func(t *testing.T) {
		j := JudgeSeverity(CcMccDemoVersion, "K35.801", codes, rules)
		if j.OverallSeverity != SeverityMCC {
			t.Errorf("总体分层 = %q, 期望 MCC", j.OverallSeverity)
		}
	})
}

// ---------- 3. 排除规则 ----------

func TestCcMccJudgeSeverity_ExcludeWhenMainPrefix(t *testing.T) {
	t.Run("主诊断命中排除前缀：不计 CC，且标记 excluded 与原因", func(t *testing.T) {
		j := JudgeSeverity(CcMccDemoVersion, "C34.900", []string{"D63.800"}, CcMccDemoRules)
		it := ccFindItem(t, j, "D63.800")
		if it.Severity != SeverityNone {
			t.Errorf("严重度 = %q, 期望 %q（主诊断 C* 已排除）", it.Severity, SeverityNone)
		}
		if !it.Excluded {
			t.Error("Excluded 应为 true")
		}
		if it.ExcludeReason == "" || !strings.Contains(it.ExcludeReason, "C") {
			t.Errorf("排除原因应写明命中的排除前缀, 实际 %q", it.ExcludeReason)
		}
		if it.MatchedPrefix != "D63" {
			t.Errorf("命中前缀 = %q, 期望 D63（仍应记录命中的条目）", it.MatchedPrefix)
		}
		if j.CcCount != 0 || j.OverallSeverity != SeverityNone {
			t.Errorf("计数/分层 = CC%d %q, 期望 0 / %q", j.CcCount, j.OverallSeverity, SeverityNone)
		}
	})

	t.Run("主诊断不命中排除前缀：正常计为 CC", func(t *testing.T) {
		j := JudgeSeverity(CcMccDemoVersion, "K35.801", []string{"D63.800"}, CcMccDemoRules)
		it := ccFindItem(t, j, "D63.800")
		if it.Severity != SeverityCC || it.Excluded {
			t.Errorf("严重度/排除 = %q/%v, 期望 CC/false", it.Severity, it.Excluded)
		}
		if it.ExcludeReason != "" {
			t.Errorf("未排除时不应有排除原因, 实际 %q", it.ExcludeReason)
		}
	})

	t.Run("排除前缀为具体编码：I63* 主诊断排除 I69.3 后遗症", func(t *testing.T) {
		excluded := JudgeSeverity(CcMccDemoVersion, "I63.900", []string{"I69.300"}, CcMccDemoRules)
		if it := ccFindItem(t, excluded, "I69.300"); it.Severity != SeverityNone || !it.Excluded {
			t.Errorf("主诊断 I63.900 下 I69.300 = %q/excluded=%v, 期望 0/true", it.Severity, it.Excluded)
		}
		// 同一编码换主诊断即恢复为 CC：说明排除是针对"主诊断×该诊断"的组合，不是全局降级。
		normal := JudgeSeverity(CcMccDemoVersion, "I10.900", []string{"I69.300"}, CcMccDemoRules)
		if it := ccFindItem(t, normal, "I69.300"); it.Severity != SeverityCC || it.Excluded {
			t.Errorf("主诊断 I10.900 下 I69.300 = %q/excluded=%v, 期望 CC/false", it.Severity, it.Excluded)
		}
	})

	t.Run("被排除的条目不再回退到更宽泛的规则", func(t *testing.T) {
		rules := []CcMccRule{
			{DiagPrefix: "D63", Severity: SeverityCC, Version: "v-test", Source: CcMccSourceDemo,
				ExcludeWhenMainPrefix: "C"},
			// 更宽泛的"兜底"条目（前缀更短，恰好也命中 D63.800）——不得被用来绕开排除。
			{DiagPrefix: "D6", Severity: SeverityMCC, Version: "v-test", Source: CcMccSourceDemo},
		}
		j := JudgeSeverity("v-test", "C34.900", []string{"D63.800"}, rules)
		if it := ccFindItem(t, j, "D63.800"); it.Severity != SeverityNone || it.MatchedPrefix != "D63" {
			t.Errorf("严重度/命中前缀 = %q/%q, 期望 0/D63（最具体条目说了算，不回退）",
				it.Severity, it.MatchedPrefix)
		}
	})

	t.Run("主诊断为空不触发排除", func(t *testing.T) {
		j := JudgeSeverity(CcMccDemoVersion, "", []string{"D63.800"}, CcMccDemoRules)
		if it := ccFindItem(t, j, "D63.800"); it.Severity != SeverityCC || it.Excluded {
			t.Errorf("空主诊断下 = %q/excluded=%v, 期望 CC/false", it.Severity, it.Excluded)
		}
	})
}

// ---------- 4. 免责声明（ADR §4 强约束）----------

func TestCcMccJudgement_DisclaimerAlwaysPresent(t *testing.T) {
	cases := []struct {
		name   string
		ver    string
		main   string
		others []string
		rules  []CcMccRule
	}{
		{"命中 demo 规则", CcMccDemoVersion, "K35.801", []string{"I50.900"}, CcMccDemoRules},
		{"命中 official 规则", "official-2.0", "K35.801", []string{"I50.900"},
			[]CcMccRule{{DiagPrefix: "I50", Severity: SeverityMCC, Version: "official-2.0", Source: CcMccSourceOfficial}}},
		{"空规则集", CcMccDemoVersion, "K35.801", []string{"I50.900"}, nil},
		{"空输入", "", "", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := JudgeSeverity(c.ver, c.main, c.others, c.rules)
			if j.Disclaimer != CcMccDisclaimer || j.Disclaimer == "" {
				t.Errorf("免责声明 = %q, 期望内置常量 %q", j.Disclaimer, CcMccDisclaimer)
			}
			// 文案层面确认它确实在否认结算效力（不是一句占位空话）。
			for _, kw := range []string{"非国家医保局", "不作为医保结算"} {
				if !strings.Contains(j.Disclaimer, kw) {
					t.Errorf("免责声明缺少关键表述 %q：%s", kw, j.Disclaimer)
				}
			}
			if j.Items == nil {
				t.Error("Items 不应为 nil（JSON 需输出 [] 而非 null）")
			}
			if j.Sources == nil {
				t.Error("Sources 不应为 nil")
			}
		})
	}

	t.Run("Source=demo 时明确标出仅演示数据", func(t *testing.T) {
		j := JudgeSeverity(CcMccDemoVersion, "K35.801", []string{"I50.900", "I48.000"}, CcMccDemoRules)
		if !j.DemoOnly || !reflect.DeepEqual(j.Sources, []string{CcMccSourceDemo}) {
			t.Errorf("DemoOnly=%v Sources=%v, 期望 true/[demo]", j.DemoOnly, j.Sources)
		}
		if !strings.Contains(j.Items[0].Remark, "演示条目") {
			t.Errorf("命中条目的备注应带上演示标注, 实际 %q", j.Items[0].Remark)
		}
	})

	t.Run("未命中任何规则时不冒充演示结论", func(t *testing.T) {
		j := JudgeSeverity(CcMccDemoVersion, "K35.801", []string{"K80.201"}, CcMccDemoRules)
		if j.DemoOnly {
			t.Error("一条规则都没命中时 DemoOnly 应为 false")
		}
		if len(j.Sources) != 0 {
			t.Errorf("来源集合应为空, 实际 %v", j.Sources)
		}
		if j.Disclaimer != CcMccDisclaimer {
			t.Error("未命中同样必须带免责声明")
		}
	})
}

// ---------- 5. 空/异常输入不 panic ----------

func TestCcMccJudgeSeverity_EmptyInputNoPanic(t *testing.T) {
	cases := []struct {
		name   string
		ver    string
		main   string
		others []string
		rules  []CcMccRule
	}{
		{"全部为空", "", "", nil, nil},
		{"空编码切片（非 nil）", CcMccDemoVersion, "K35.801", []string{}, CcMccDemoRules},
		{"含空编码与空白的槽位", CcMccDemoVersion, "K35.801", []string{"", "   ", "I50.900", ""}, CcMccDemoRules},
		{"只有空编码", CcMccDemoVersion, "K35.801", []string{"", ""}, CcMccDemoRules},
		{"规则里字段残缺", CcMccDemoVersion, "K35.801", []string{"I50.900"},
			[]CcMccRule{{}, {DiagPrefix: "I50"}, {DiagPrefix: "I50", Severity: SeverityNone, Version: CcMccDemoVersion}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := JudgeSeverity(c.ver, c.main, c.others, c.rules) // 不 panic 即为通过
			if len(j.Items) != len(c.others) {
				t.Errorf("items 长度 = %d, 期望与入参等长 %d（空槽也占位，保持索引对齐）",
					len(j.Items), len(c.others))
			}
			if j.Disclaimer == "" {
				t.Error("免责声明缺失")
			}
			if j.OverallSeverity != SeverityMCC && j.OverallSeverity != SeverityCC && j.OverallSeverity != SeverityNone {
				t.Errorf("总体分层取值非法：%q", j.OverallSeverity)
			}
		})
	}
}

// ---------- 6. 演示规则集自检 ----------

func TestCcMccDemoRules_TableSelfCheck(t *testing.T) {
	if n := len(CcMccDemoRules); n < 10 || n > 20 {
		t.Errorf("演示规则条数 = %d, 期望 10–20 条（ADR §9 批次 B）", n)
	}
	seen := map[string]bool{}
	mcc, cc, withExclude := 0, 0, 0
	for i, r := range CcMccDemoRules {
		if r.Source != CcMccSourceDemo {
			t.Errorf("第 %d 条 Source = %q, 内置表只允许 demo（ADR §6 合规红线）", i, r.Source)
		}
		if r.Version != CcMccDemoVersion {
			t.Errorf("第 %d 条 Version = %q, 期望 %q", i, r.Version, CcMccDemoVersion)
		}
		if r.DiagPrefix == "" || r.DiagPrefix != strings.ToUpper(strings.TrimSpace(r.DiagPrefix)) {
			t.Errorf("第 %d 条前缀 %q 不规范（应为非空大写、无空格）", i, r.DiagPrefix)
		}
		switch r.Severity {
		case SeverityMCC:
			mcc++
		case SeverityCC:
			cc++
		default:
			t.Errorf("第 %d 条严重度 %q 非法（只允许 MCC / CC）", i, r.Severity)
		}
		if r.ExcludeWhenMainPrefix != "" {
			withExclude++
		}
		if r.Remark == "" {
			t.Errorf("第 %d 条缺少备注（须写明是演示条目）", i)
		}
		key := r.Version + "|" + r.DiagPrefix
		if seen[key] {
			t.Errorf("第 %d 条与前文重复（版本+前缀 %s）", i, key)
		}
		seen[key] = true
	}
	if mcc == 0 || cc == 0 {
		t.Errorf("演示表应同时覆盖 MCC 与 CC, 实际 MCC=%d CC=%d", mcc, cc)
	}
	if withExclude == 0 {
		t.Error("演示表应至少含 1 条带排除前缀的规则（排除规则留位的可用样例）")
	}
}

// ---------- 7. 与解析层的按位置对齐联动 ----------

func TestCcMccJudgeSeverity_AlignsWithParsedOtherDiagnoses(t *testing.T) {
	// 解析层（other_diagnosis.go，他人负责）产出编码列表；判读层按同一顺序出结果。
	items, err := ParseOtherDiagnoses("I50.900,I48.000,K80.201", "0,C,0")
	if err != nil {
		t.Fatalf("解析其他诊断失败：%v", err)
	}
	codes := OtherDiagnosisCodes(items)
	j := JudgeSeverity(CcMccDemoVersion, "K35.801", codes, CcMccDemoRules)

	if got, want := ccSevs(j), []string{SeverityMCC, SeverityCC, SeverityNone}; !reflect.DeepEqual(got, want) {
		t.Errorf("逐项严重度 = %v, 期望 %v", got, want)
	}
	// 关键对齐断言：第 2 项已录标记 'C' 与判读 CC 一致；若不一致即为批次 C 的低靠/高靠线索。
	for i, it := range items {
		if j.Items[i].Code != it.Code {
			t.Errorf("第 %d 项错位：判读 %q vs 解析 %q", i, j.Items[i].Code, it.Code)
		}
	}
}

// ---------- 8. 新增字段 / 列名 / JSON 名与 Save 白名单的一致性 ----------

// TestCcMccFlagFieldContract 固化 ADR §5 新增字段的对外契约：
// Go 名 OtherDiagnosisFlags、JSON 名 otherDiagnosisFlags、GORM 列名 other_diagnosis_flags
// （= handler/frontpage.go Save 白名单里手写的 snake_case），类型 string（可空列）。
// 白名单写错列名或漏项时，前端传值会被静默丢弃——D3 踩过同一个坑，这里给可验证对照。
func TestCcMccFlagFieldContract(t *testing.T) {
	typ := reflect.TypeOf(model.MedFrontPage{})
	f, ok := typ.FieldByName("OtherDiagnosisFlags")
	if !ok {
		t.Fatal("MedFrontPage 缺少字段 OtherDiagnosisFlags")
	}
	if f.Type.Kind() != reflect.String {
		t.Errorf("字段类型 = %s, 期望 string（与 otherDiagnosisCodes 同为等长字符串）", f.Type)
	}
	if got := f.Tag.Get("json"); got != "otherDiagnosisFlags" {
		t.Errorf("json tag = %q, 期望 otherDiagnosisFlags", got)
	}
	ns := schema.NamingStrategy{}
	if got := ns.ColumnName("", "OtherDiagnosisFlags"); got != "other_diagnosis_flags" {
		t.Errorf("GORM 列名 = %q, 期望 other_diagnosis_flags（frontpage.go Save 白名单须用此值）", got)
	}
}

// TestCcMccSeverityNoneMatchesFlagVocabulary 固化"判读层的 None 与标记串的 '0' 同口径"
// （ADR §5）。other_diagnosis.go 的 SeverityLevelNone="NONE" 是展示口径，两者不可混用，
// 故此处显式断言，避免后续有人把判读层也改成 "NONE"。
func TestCcMccSeverityNoneMatchesFlagVocabulary(t *testing.T) {
	if SeverityNone != SeverityFlagNone {
		t.Errorf("SeverityNone = %q, 期望与 SeverityFlagNone = %q 一致", SeverityNone, SeverityFlagNone)
	}
	if SeverityCC == SeverityFlagCC || SeverityMCC == SeverityFlagMCC {
		t.Error("判读层等级（CC/MCC）与单字符标记（C/M）是两套取值，不应相等")
	}
}
