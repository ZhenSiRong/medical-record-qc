package service

import (
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm/schema"

	"mrs/internal/model"
)

// =====================================================================
// 本文件覆盖 cc_mcc_rule 的落库侧（model/ccmcc.go + service/ccmcc_store.go）：
//   - ccMccRuleFrom 的逐字段映射（含空 ExcludeWhenMainPrefix 与中文 Remark）；
//   - LoadCcMccRules 的 fail-closed：空版本不查库、不 panic、返回空 + error；
//   - 合规红线：表名固定 cc_mcc_rule，且表里不得出现分组码 / 权重 / 金额类列
//     （ADR-0002 §3/§6：那不是实现工作量问题，而是授权件问题）。
//
// 全部为纯内存测试：不连数据库、不起服务（对齐 ccmcc_test.go 与
// qc_condition_d3_test.go 的无依赖风格），因为映射函数抽成了纯函数才能无库可测。
// =====================================================================

// ---------- 1. 库表行 → 判读规则的映射 ----------

func TestCcMccRuleFrom_MapsEveryField(t *testing.T) {
	cases := []struct {
		name string
		row  model.CcMccRule
		want CcMccRule
	}{
		{
			name: "MCC 常规行：无排除前缀",
			row: model.CcMccRule{
				ID:         1,
				DiagPrefix: "A41",
				Severity:   SeverityMCC,
				Version:    CcMccDemoVersion,
				Source:     CcMccSourceDemo,
				Remark:     "脓毒症（败血症）：演示条目，非官方 MCC 判定",
			},
			want: CcMccRule{
				DiagPrefix:            "A41",
				Severity:              SeverityMCC,
				Version:               CcMccDemoVersion,
				Source:                CcMccSourceDemo,
				ExcludeWhenMainPrefix: "",
				Remark:                "脓毒症（败血症）：演示条目，非官方 MCC 判定",
			},
		},
		{
			name: "CC 带排除前缀的行（D63 / 主诊断 C* 排除）",
			row: model.CcMccRule{
				ID:                    2,
				DiagPrefix:            "D63",
				Severity:              SeverityCC,
				Version:               CcMccDemoVersion,
				Source:                CcMccSourceDemo,
				ExcludeWhenMainPrefix: "C",
				Remark:                "慢性病所致贫血：演示条目。演示排除形态——主诊断已为恶性肿瘤(C*)时该贫血多属疾病本身表现，不计为 CC",
			},
			want: CcMccRule{
				DiagPrefix:            "D63",
				Severity:              SeverityCC,
				Version:               CcMccDemoVersion,
				Source:                CcMccSourceDemo,
				ExcludeWhenMainPrefix: "C",
				Remark:                "慢性病所致贫血：演示条目。演示排除形态——主诊断已为恶性肿瘤(C*)时该贫血多属疾病本身表现，不计为 CC",
			},
		},
		{
			name: "带小数点的长前缀 + 英文备注（来源可切换为 official）",
			row: model.CcMccRule{
				ID:                    3,
				DiagPrefix:            "N18.5",
				Severity:              SeverityMCC,
				Version:               "chs-drg-2.0",
				Source:                CcMccSourceOfficial,
				ExcludeWhenMainPrefix: "N18.5",
				Remark:                "chronic kidney disease stage 5 (official sample)",
			},
			want: CcMccRule{
				DiagPrefix:            "N18.5",
				Severity:              SeverityMCC,
				Version:               "chs-drg-2.0",
				Source:                CcMccSourceOfficial,
				ExcludeWhenMainPrefix: "N18.5",
				Remark:                "chronic kidney disease stage 5 (official sample)",
			},
		},
		{
			name: "全空行（零值）：不 panic，逐字段为空串",
			row:  model.CcMccRule{},
			want: CcMccRule{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ccMccRuleFrom(c.row)
			// 逐字段断言（而非整体 DeepEqual）：某字段映射写错时，报错直接指出字段名，
			// 这正是本文件的负控制能"指位"的前提（见交付报告的负控制过程）。
			if got.DiagPrefix != c.want.DiagPrefix {
				t.Errorf("DiagPrefix = %q, 期望 %q", got.DiagPrefix, c.want.DiagPrefix)
			}
			if got.Severity != c.want.Severity {
				t.Errorf("Severity = %q, 期望 %q", got.Severity, c.want.Severity)
			}
			if got.Version != c.want.Version {
				t.Errorf("Version = %q, 期望 %q", got.Version, c.want.Version)
			}
			if got.Source != c.want.Source {
				t.Errorf("Source = %q, 期望 %q", got.Source, c.want.Source)
			}
			if got.ExcludeWhenMainPrefix != c.want.ExcludeWhenMainPrefix {
				t.Errorf("ExcludeWhenMainPrefix = %q, 期望 %q", got.ExcludeWhenMainPrefix, c.want.ExcludeWhenMainPrefix)
			}
			if got.Remark != c.want.Remark {
				t.Errorf("Remark = %q, 期望 %q", got.Remark, c.want.Remark)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("整体不等：got=%+v want=%+v", got, c.want)
			}
		})
	}
}

// TestCcMccRuleFrom_DoesNotLeakModelOnlyFields 防漏/防串的兜底：映射结果里
// 不应出现库表主键 ID，也不应把 Source 与 Remark 等字段串位（逐字段交叉比对）。
func TestCcMccRuleFrom_DoesNotLeakModelOnlyFields(t *testing.T) {
	row := model.CcMccRule{
		ID:                    7,
		DiagPrefix:            "I69.3",
		Severity:              SeverityCC,
		Version:               CcMccDemoVersion,
		Source:                CcMccSourceDemo,
		ExcludeWhenMainPrefix: "I63",
		Remark:                "脑梗死后遗症：演示条目",
	}
	got := ccMccRuleFrom(row)

	if strings.Contains(got.Remark, CcMccSourceDemo) {
		t.Errorf("Remark 混进了 Source 的值：%q", got.Remark)
	}
	if got.Source != row.Source || got.Remark != row.Remark {
		t.Errorf("Source/Remark 串位：got.Source=%q got.Remark=%q", got.Source, got.Remark)
	}
	// 判读层结构体没有 ID 字段，若哪天加了 ID 说明模型字段漏进判读层，需重新评估。
	if _, ok := reflect.TypeOf(got).FieldByName("ID"); ok {
		t.Errorf("CcMccRule 不应包含库表主键 ID 字段")
	}
}

// ---------- 2. LoadCcMccRules 的 fail-closed ----------

// TestLoadCcMccRules_EmptyVersionFailClosed 空版本 ⇒ 不查库、返回空 + error。
// 这里刻意传 nil db：如果实现是先查库再判空版本，nil db 会 panic，
// 测试就会以 panic（而非断言失败）的形式暴露顺序错误——这正是要防的回归。
func TestLoadCcMccRules_EmptyVersionFailClosed(t *testing.T) {
	for _, v := range []string{"", "   ", "\t\n"} {
		got, err := LoadCcMccRules(nil, v)
		if err == nil {
			t.Errorf("version=%q 期望 error（fail-closed），实际返回 nil", v)
		}
		if len(got) != 0 {
			t.Errorf("version=%q 期望空切片，实际 len=%d（%+v）", v, len(got), got)
		}
		if got == nil {
			t.Errorf("version=%q 期望非 nil 空切片（JSON 出 [] 而非 null），实际 nil", v)
		}
	}
}

// ---------- 3. 落表形态与合规红线 ----------

func TestCcMccRuleTableName(t *testing.T) {
	if got := (model.CcMccRule{}).TableName(); got != "cc_mcc_rule" {
		t.Errorf("TableName() = %q, 期望 cc_mcc_rule", got)
	}
}

// TestCcMccRuleNoPaymentColumns 合规红线：表里不得出现分组码 / 权重 / 金额类列
// （ADR-0002 §3：那是授权件问题，不是实现工作量问题）。
func TestCcMccRuleNoPaymentColumns(t *testing.T) {
	ns := schema.NamingStrategy{}
	typ := reflect.TypeOf(model.CcMccRule{})
	forbidden := []string{"weight", "group_code", "drg_code", "dip_code", "amount", "cost", "fee", "payment"}

	for i := 0; i < typ.NumField(); i++ {
		col := ns.ColumnName("", typ.Field(i).Name)
		for _, bad := range forbidden {
			if col == bad || strings.HasSuffix(col, "_"+bad) {
				t.Errorf("cc_mcc_rule 出现禁止列 %q（字段 %s）：本表不得承载分组码/权重/金额", col, typ.Field(i).Name)
			}
		}
	}
}

// TestCcMccRuleRequiredColumnsPresent 必要的列必须在：版本、来源、排除前缀是
// 「可替换 + 版本化 + 来源标注」三件事的落点，缺任何一个合规与可追溯性就断了。
func TestCcMccRuleRequiredColumnsPresent(t *testing.T) {
	ns := schema.NamingStrategy{}
	typ := reflect.TypeOf(model.CcMccRule{})
	want := map[string]string{
		"DiagPrefix":            "diag_prefix",
		"Severity":              "severity",
		"Version":               "version",
		"Source":                "source",
		"ExcludeWhenMainPrefix": "exclude_when_main_prefix",
		"Remark":                "remark",
	}
	for field, col := range want {
		f, ok := typ.FieldByName(field)
		if !ok {
			t.Errorf("缺少字段 %s", field)
			continue
		}
		if got := ns.ColumnName("", f.Name); got != col {
			t.Errorf("字段 %s 列名 = %q, 期望 %q", field, got, col)
		}
	}
}

// TestCcMccRuleFromRoundTripIntoJudgeSeverity 端到端兜底：从库表行映射出来的规则
// 能被判读层直接用（版本一致时命中），证明"落表 → 判读"链路没断。
func TestCcMccRuleFromRoundTripIntoJudgeSeverity(t *testing.T) {
	rows := []model.CcMccRule{
		{ID: 1, DiagPrefix: "I50", Severity: SeverityMCC, Version: CcMccDemoVersion, Source: CcMccSourceDemo, Remark: "心力衰竭：演示条目"},
		{ID: 2, DiagPrefix: "I48", Severity: SeverityCC, Version: CcMccDemoVersion, Source: CcMccSourceDemo, Remark: "心房颤动：演示条目"},
		{ID: 3, DiagPrefix: "D63", Severity: SeverityCC, Version: CcMccDemoVersion, Source: CcMccSourceDemo, ExcludeWhenMainPrefix: "C", Remark: "慢性病贫血：演示条目"},
	}
	rules := make([]CcMccRule, 0, len(rows))
	for _, r := range rows {
		rules = append(rules, ccMccRuleFrom(r))
	}

	// 主诊断取 C50.9（乳腺恶性肿瘤）：D63 的 ExcludeWhenMainPrefix="C" 命中 ⇒ 不计 CC，
	// 以此证明"排除前缀"这个字段确实随映射进了判读层（漏映射该字段时本用例会变红）。
	j := JudgeSeverity(CcMccDemoVersion, "C50.900", []string{"I50.900", "I48.000", "D63.000"}, rules)
	want := []string{SeverityMCC, SeverityCC, SeverityNone} // 末条：主诊断 C* ⇒ D63 被排除
	got := make([]string, 0, len(j.Items))
	for _, it := range j.Items {
		got = append(got, it.Severity)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("判读严重度 = %v, 期望 %v", got, want)
	}
	if !j.Items[2].Excluded || j.Items[2].ExcludeReason == "" {
		t.Errorf("末条应被排除且有排除原因：%+v", j.Items[2])
	}
	if !j.DemoOnly {
		t.Errorf("DemoOnly = false, 期望 true（命中规则全为 demo，界面须显著标注非官方）")
	}
}
