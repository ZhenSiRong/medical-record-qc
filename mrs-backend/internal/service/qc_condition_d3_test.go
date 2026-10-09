package service

import (
	"reflect"
	"testing"

	"gorm.io/gorm/schema"

	"mrs/internal/model"
)

// 本文件覆盖缺口 D3（条件必填真字段，2026-10-09）：
//   - 条件表达式 evalCondition 的真/假两条路径（含条件不成立 → 规则被跳过）；
//   - isBlank 对新增字段类型（*int 指针 / string）的判定行为；
//   - 新增字段的 GORM 列名与 frontpage.Save 白名单里的 snake_case 列名一致；
//   - 规则 target_field（MedFrontPage 的 Go 字段名）确实能通过反射取到。
//
// 全部为纯内存测试：不连数据库、不起服务（对齐 internal/router/router_test.go 的无依赖风格）。
//
// 注意：本文件里的条件表达式字面量镜像 handler/qc_ext.go 的 SeedQcExt 种子（service 包
// 不能 import handler，否则与 handler → service 形成循环依赖），改动种子时请同步本文件。

// d3Rule 模拟 qc_rule 表中的一条条件必填规则（只取引擎用到的三列）。
type d3Rule struct {
	name      string
	target    string // MedFrontPage 的 Go 字段名（reflect FieldByName 取值）
	condition string // ConditionExpr
}

// d3Rules 镜像 qc_ext.go SeedQcExt 中 D3 相关的条件必填规则。
var d3Rules = []d3Rule{
	{"损伤中毒类主诊断需填外部原因（条件必填）", "ExternalCauseCode", "main_diagnosis_code LIKE 'S%'"},
	{"新生儿需填出生体重/孕周（条件必填）", "NewbornBirthWeight", "main_diagnosis_code LIKE 'P%'"},
	{"新生儿需填孕周（条件必填）", "NewbornGestationalWeeks", "main_diagnosis_code LIKE 'P%'"},
	{"有住院天数则出院诊断必填（条件必填）", "DischargeDiagnosis", "actual_days>0"},
}

func intPtr(v int) *int { return &v }

// ---------- 1. 条件表达式求值：真 / 假两条路径 ----------

func TestEvalConditionD3_ConditionalRequired(t *testing.T) {
	cases := []struct {
		name string
		expr string
		fp   model.MedFrontPage
		want bool
	}{
		// ---- 条件成立（真路径）----
		{"S 编码命中损伤中毒条件", "main_diagnosis_code LIKE 'S%'",
			model.MedFrontPage{MainDiagnosisCode: "S72.001"}, true},
		{"小写编码同样命中（大小写不敏感）", "main_diagnosis_code LIKE 'S%'",
			model.MedFrontPage{MainDiagnosisCode: "s72.001"}, true},
		{"编码前缀前后有空格仍命中", "main_diagnosis_code LIKE 'S%'",
			model.MedFrontPage{MainDiagnosisCode: "  S06.900  "}, true},
		{"P 编码命中新生儿条件", "main_diagnosis_code LIKE 'P%'",
			model.MedFrontPage{MainDiagnosisCode: "P07.100"}, true},
		{"住院天数>0 命中", "actual_days>0", model.MedFrontPage{ActualDays: 7}, true},
		{"有主要手术编码则 has_operation 成立", "has_operation",
			model.MedFrontPage{MainOperationCode: "79.35"}, true},
		{"无条件表达式视为条件成立（保守：宁可多校验）", "",
			model.MedFrontPage{MainDiagnosisCode: "I10"}, true},

		// ---- 条件不成立（假路径 → RunQc 会跳过该规则）----
		{"空主诊断不命中 S 前缀", "main_diagnosis_code LIKE 'S%'", model.MedFrontPage{}, false},
		{"非损伤中毒编码不命中 S 前缀", "main_diagnosis_code LIKE 'S%'",
			model.MedFrontPage{MainDiagnosisCode: "I10"}, false},
		{"T 编码当前不命中（已知限制：见下方 TestEvalConditionD3_TCodeLimitation）",
			"main_diagnosis_code LIKE 'S%'",
			model.MedFrontPage{MainDiagnosisCode: "T14.900"}, false},
		{"成人病案（S 编码）不命中新生儿 P 前缀", "main_diagnosis_code LIKE 'P%'",
			model.MedFrontPage{MainDiagnosisCode: "S72.001"}, false},
		{"住院天数为 0 不命中", "actual_days>0", model.MedFrontPage{ActualDays: 0}, false},
		{"无主要手术则不命中 has_operation", "has_operation", model.MedFrontPage{}, false},
		{"有主要手术则 no_operation 不成立", "no_operation",
			model.MedFrontPage{MainOperationCode: "79.35"}, false},
		{"无主要手术则 no_operation 成立（反向用例）", "no_operation", model.MedFrontPage{}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fp := c.fp
			if got := evalCondition(c.expr, &fp); got != c.want {
				t.Fatalf("evalCondition(%q, mainDx=%q actualDays=%d) = %v, 期望 %v",
					c.expr, fp.MainDiagnosisCode, fp.ActualDays, got, c.want)
			}
		})
	}
}

// TestEvalConditionD3_TCodeLimitation 固化"现行条件引擎不支持 OR/多前缀"这一事实，
// 它是 SeedQcExt 只写 `LIKE 'S%'`（而不是 S|T）的直接依据。
// 若后续为条件表达式引入 OR 支持，本测试应改为断言 T 编码命中。
func TestEvalConditionD3_TCodeLimitation(t *testing.T) {
	// ① 标准要求覆盖 S 或 T；把 OR 写进表达式时 likeCondRe 不匹配，
	//    evalCondition 保守回退为"条件成立"——这会让规则在所有病案上生效（误报面扩大），
	//    因此本次不写 OR 表达式，改为在规则 remark 中注明待后续批次。
	orExpr := "main_diagnosis_code LIKE 'S%' OR main_diagnosis_code LIKE 'T%'"
	fpI10 := model.MedFrontPage{MainDiagnosisCode: "I10"}
	if !evalCondition(orExpr, &fpI10) {
		t.Fatalf("未识别的条件表达式应保守判定为条件成立；当前实现已变更，请重新评估 S|T 写法")
	}

	// ② 只写 S%：T 编码病案当前不会触发外部原因必填（漏报，已知限制）。
	fpT := model.MedFrontPage{MainDiagnosisCode: "T14.900"}
	if evalCondition("main_diagnosis_code LIKE 'S%'", &fpT) {
		t.Fatalf("T 编码不应命中 S 前缀条件（若已支持，请更新 qc_ext.go 种子与本节注释）")
	}
}

// TestEvalConditionD3_UnmappedFieldIsSkipped 固化另一处已知限制：
// frontPageFieldString 未映射 D3 新字段，故以其为条件的表达式取值为空串 → 条件不成立 → 规则被跳过。
// 结论：本次不允许把新字段写进 ConditionExpr（只用 main_diagnosis_code）。
// 本次未修改 service/qc_condition.go（文件所有权限定），该限制已在交付报告中列明。
func TestEvalConditionD3_UnmappedFieldIsSkipped(t *testing.T) {
	fp := model.MedFrontPage{ExternalCauseCode: "V01.900"}
	if evalCondition("external_cause_code LIKE 'V%'", &fp) {
		t.Fatalf("external_cause_code 未纳入条件取值映射，条件应不成立（不成立即跳过规则）")
	}
}

// ---------- 2. isBlank 对新增字段类型的判定 ----------

func TestIsBlankD3_NewFieldTypes(t *testing.T) {
	cases := []struct {
		name  string
		field string
		fp    model.MedFrontPage
		want  bool
		note  string
	}{
		// string：外部原因编码 / 名称
		{"外部原因编码为空", "ExternalCauseCode", model.MedFrontPage{}, true, ""},
		{"外部原因编码为空白串", "ExternalCauseCode", model.MedFrontPage{ExternalCauseCode: "   "}, true, "TrimSpace 后视为空"},
		{"外部原因编码已填", "ExternalCauseCode", model.MedFrontPage{ExternalCauseCode: "W01.001"}, false, ""},
		{"外部原因名称为空", "ExternalCauseName", model.MedFrontPage{}, true, ""},
		{"外部原因名称已填", "ExternalCauseName", model.MedFrontPage{ExternalCauseName: "在住宅内跌倒"}, false, ""},

		// *int：新生儿出生体重 / 孕周（三态语义）
		{"出生体重 nil（未采集/非新生儿）", "NewbornBirthWeight", model.MedFrontPage{}, true, "指针 nil → 空"},
		{"出生体重点向 0", "NewbornBirthWeight",
			model.MedFrontPage{NewbornBirthWeight: intPtr(0)}, true,
			"设计取舍：0 克非合法临床值，判为'空'同样应命中必填校验"},
		{"出生体重点向正常值", "NewbornBirthWeight",
			model.MedFrontPage{NewbornBirthWeight: intPtr(3250)}, false, ""},
		{"出生体重点向负值", "NewbornBirthWeight",
			model.MedFrontPage{NewbornBirthWeight: intPtr(-1)}, false,
			"已知限制：引擎只判非空，不做取值范围校验（越界值由后续数值域规则覆盖）"},
		{"孕周 nil", "NewbornGestationalWeeks", model.MedFrontPage{}, true, ""},
		{"孕周指向 0", "NewbornGestationalWeeks",
			model.MedFrontPage{NewbornGestationalWeeks: intPtr(0)}, true, "同出生体重取舍"},
		{"孕周指向 39", "NewbornGestationalWeeks",
			model.MedFrontPage{NewbornGestationalWeeks: intPtr(39)}, false, ""},

		// 既有字段回归护栏：判空口径未被本次改动影响
		{"既有 ActualDays=0 仍判空（回归护栏）", "ActualDays", model.MedFrontPage{}, true, ""},
		{"既有 MainDiagnosisCode 空串仍判空（回归护栏）", "MainDiagnosisCode", model.MedFrontPage{}, true, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fp := c.fp
			fv := reflect.ValueOf(fp).FieldByName(c.field)
			if !fv.IsValid() {
				t.Fatalf("模型 MedFrontPage 不存在字段 %s", c.field)
			}
			if got := isBlank(fv); got != c.want {
				t.Fatalf("isBlank(%s=%v) = %v, 期望 %v (%s)", c.field, fv.Interface(), got, c.want, c.note)
			}
		})
	}
}

// ---------- 3. 「条件不成立 → 规则被跳过」的判定（复刻 RunQc 的门控组合） ----------

// requiredViolated 复刻 service.RunQc 中 REQUIRED 规则的门控：
//
//	if conditionExpr != "" && !evalCondition(...) { 跳过 }
//	if isBlank(targetField) { 记为“必填字段为空” }
func requiredViolated(t *testing.T, r d3Rule, fp *model.MedFrontPage) bool {
	t.Helper()
	if r.condition != "" && !evalCondition(r.condition, fp) {
		return false // 条件不成立：规则被跳过
	}
	fv := reflect.ValueOf(*fp).FieldByName(r.target)
	if !fv.IsValid() {
		t.Fatalf("规则 %q 的 target_field=%q 在 MedFrontPage 上不存在", r.name, r.target)
	}
	return isBlank(fv)
}

func TestConditionalRequiredDecisionD3(t *testing.T) {
	cases := []struct {
		name       string
		fp         model.MedFrontPage
		wantFired  map[string]bool // 规则名 → 是否应命中
		firedNote  string
		skipReason string
	}{
		{
			name: "损伤中毒病案未填外部原因：外部原因规则命中，新生儿规则跳过",
			fp: model.MedFrontPage{MainDiagnosisCode: "S72.001", ActualDays: 10,
				DischargeDiagnosis: "股骨骨折"},
			wantFired: map[string]bool{
				"损伤中毒类主诊断需填外部原因（条件必填）": true,
				"新生儿需填出生体重/孕周（条件必填）":   false,
				"新生儿需填孕周（条件必填）":        false,
				"有住院天数则出院诊断必填（条件必填）":   false,
			},
			skipReason: "非 P 编码：新生儿两条规则条件不成立被跳过",
		},
		{
			name: "损伤中毒病案已填外部原因编码：规则不命中",
			fp: model.MedFrontPage{MainDiagnosisCode: "S72.001", ActualDays: 10,
				DischargeDiagnosis: "股骨骨折", ExternalCauseCode: "W01.001",
				ExternalCauseName: "在住宅内跌倒"},
			wantFired: map[string]bool{
				"损伤中毒类主诊断需填外部原因（条件必填）": false,
				"新生儿需填出生体重/孕周（条件必填）":   false,
				"新生儿需填孕周（条件必填）":        false,
				"有住院天数则出院诊断必填（条件必填）":   false,
			},
		},
		{
			name: "新生儿病案未填体重/孕周：两条新生儿规则均命中",
			fp: model.MedFrontPage{MainDiagnosisCode: "P07.100", ActualDays: 20,
				DischargeDiagnosis: "早产儿"},
			wantFired: map[string]bool{
				"损伤中毒类主诊断需填外部原因（条件必填）": false,
				"新生儿需填出生体重/孕周（条件必填）":   true,
				"新生儿需填孕周（条件必填）":        true,
				"有住院天数则出院诊断必填（条件必填）":   false,
			},
			skipReason: "非 S 编码：损伤中毒规则条件不成立被跳过",
		},
		{
			name: "新生儿病案只填了体重：孕周规则仍命中（一规则一字段，不能互相顶替）",
			fp: model.MedFrontPage{MainDiagnosisCode: "P07.100", ActualDays: 20,
				DischargeDiagnosis: "早产儿", NewbornBirthWeight: intPtr(1800)},
			wantFired: map[string]bool{
				"损伤中毒类主诊断需填外部原因（条件必填）": false,
				"新生儿需填出生体重/孕周（条件必填）":   false,
				"新生儿需填孕周（条件必填）":        true,
				"有住院天数则出院诊断必填（条件必填）":   false,
			},
		},
		{
			name: "普通内科病案（I10）未填任何 D3 字段：条件必填全部跳过，不误报",
			fp: model.MedFrontPage{MainDiagnosisCode: "I10", ActualDays: 5,
				DischargeDiagnosis: "高血压"},
			wantFired: map[string]bool{
				"损伤中毒类主诊断需填外部原因（条件必填）": false,
				"新生儿需填出生体重/孕周（条件必填）":   false,
				"新生儿需填孕周（条件必填）":        false,
				"有住院天数则出院诊断必填（条件必填）":   false,
			},
			skipReason: "非 S/P 编码：指针字段 nil 不会因“零值”而误报（这正是选用 *int 的目的）",
		},
		{
			name: "住院天数为 0：出院诊断规则条件不成立被跳过",
			fp:   model.MedFrontPage{MainDiagnosisCode: "I10", ActualDays: 0},
			wantFired: map[string]bool{
				"损伤中毒类主诊断需填外部原因（条件必填）": false,
				"新生儿需填出生体重/孕周（条件必填）":   false,
				"新生儿需填孕周（条件必填）":        false,
				"有住院天数则出院诊断必填（条件必填）":   false,
			},
		},
		{
			name: "住院天数>0 且出院诊断为空：既有规则照常命中（回归护栏）",
			fp:   model.MedFrontPage{MainDiagnosisCode: "I10", ActualDays: 3},
			wantFired: map[string]bool{
				"损伤中毒类主诊断需填外部原因（条件必填）": false,
				"新生儿需填出生体重/孕周（条件必填）":   false,
				"新生儿需填孕周（条件必填）":        false,
				"有住院天数则出院诊断必填（条件必填）":   true,
			},
		},
		{
			name: "T 编码病案：外部原因规则当前不触发（已知漏报，待条件引擎支持 OR）",
			fp: model.MedFrontPage{MainDiagnosisCode: "T14.900", ActualDays: 2,
				DischargeDiagnosis: "损伤"},
			wantFired: map[string]bool{
				"损伤中毒类主诊断需填外部原因（条件必填）": false,
				"新生儿需填出生体重/孕周（条件必填）":   false,
				"新生儿需填孕周（条件必填）":        false,
				"有住院天数则出院诊断必填（条件必填）":   false,
			},
			skipReason: "标准要求 S 或 T，现行 likeCondRe 仅支持单前缀，故 T 编码漏报",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fp := c.fp
			for _, r := range d3Rules {
				want, ok := c.wantFired[r.name]
				if !ok {
					t.Fatalf("用例缺少规则 %q 的期望值", r.name)
				}
				if got := requiredViolated(t, r, &fp); got != want {
					t.Errorf("规则 %q：命中=%v, 期望 %v（%s）", r.name, got, want, c.skipReason)
				}
			}
		})
	}
}

// ---------- 4. 种子规则与模型字段 / 数据库列名的一致性 ----------

// TestD3RuleTargetFieldsExistOnModel 保证 qc_rule.target_field 写的是真实存在的
// MedFrontPage 字段名（RunQc 用 reflect FieldByName 取值，写错只会静默变成“规则配置错误”）。
func TestD3RuleTargetFieldsExistOnModel(t *testing.T) {
	v := reflect.ValueOf(model.MedFrontPage{})
	for _, r := range d3Rules {
		if !v.FieldByName(r.target).IsValid() {
			t.Errorf("规则 %q 的 target_field=%q 在 MedFrontPage 上不存在", r.name, r.target)
		}
	}
}

// TestD3ColumnNamesMatchSaveWhitelist 校验 GORM 命名策略推导出的列名，
// 与 handler/frontpage.go Save 白名单里手写的 snake_case 列名一致——白名单写错列名时
// 前端传值会被静默丢弃，本测试给出可验证的对照。
func TestD3ColumnNamesMatchSaveWhitelist(t *testing.T) {
	ns := schema.NamingStrategy{}
	cases := []struct{ field, column string }{
		{"ExternalCauseCode", "external_cause_code"},
		{"ExternalCauseName", "external_cause_name"},
		{"NewbornBirthWeight", "newborn_birth_weight"},
		{"NewbornGestationalWeeks", "newborn_gestational_weeks"},
	}
	for _, c := range cases {
		if got := ns.ColumnName("", c.field); got != c.column {
			t.Errorf("列名不一致：%s → %q, 期望 %q（frontpage.go Save 白名单须用期望值）",
				c.field, got, c.column)
		}
	}
}

// TestD3FieldJSONTags 固化对外契约的 JSON 名（供 docs/API.md 与前端表单对齐）。
func TestD3FieldJSONTags(t *testing.T) {
	want := map[string]string{
		"ExternalCauseCode":       "externalCauseCode",
		"ExternalCauseName":       "externalCauseName",
		"NewbornBirthWeight":      "newbornBirthWeight",
		"NewbornGestationalWeeks": "newbornGestationalWeeks",
	}
	typ := reflect.TypeOf(model.MedFrontPage{})
	for field, jsonName := range want {
		f, ok := typ.FieldByName(field)
		if !ok {
			t.Fatalf("MedFrontPage 缺少字段 %s", field)
		}
		if got := f.Tag.Get("json"); got != jsonName {
			t.Errorf("字段 %s 的 json tag = %q, 期望 %q", field, got, jsonName)
		}
	}
}

// TestD3NewbornFieldsArePointer 固化"出生体重/孕周必须为 *int（可空三态）"这一设计决策，
// 防止后续被改回裸 int（裸 int 的 0 与"未采集"无法区分，且会让非新生儿首页在 JSON 里出现 0）。
func TestD3NewbornFieldsArePointer(t *testing.T) {
	typ := reflect.TypeOf(model.MedFrontPage{})
	for _, field := range []string{"NewbornBirthWeight", "NewbornGestationalWeeks"} {
		f, ok := typ.FieldByName(field)
		if !ok {
			t.Fatalf("MedFrontPage 缺少字段 %s", field)
		}
		if f.Type.Kind() != reflect.Ptr || f.Type.Elem().Kind() != reflect.Int {
			t.Errorf("字段 %s 类型为 %s, 期望 *int（可空语义）", field, f.Type)
		}
	}
}
