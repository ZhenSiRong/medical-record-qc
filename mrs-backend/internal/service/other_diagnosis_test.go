package service

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// 本文件覆盖 ADR-0002 §5「其他诊断的主次标记」的解析与对齐层（批次 B，2026-10-09）。
//
// 被固化的是「隐式契约」——等长对齐在各种脏输入下的行为。全部为纯内存测试：
// 不连数据库、不起服务（对齐 internal/service/qc_condition_d3_test.go 的无依赖风格）。
// 本层刻意不依赖 model.MedFrontPage 字段，故不受其他批次加列的影响。
//
// ⚠️ 改动 other_diagnosis.go 的边界行为前，请先读这里的用例名与注释：
// 每一条都用例名说明了"为什么是这个行为"，其中第 5、6、7 条（空项 / 非法 flag /
// flags 全空）是设计取舍最集中的地方。

// ---------- 用例 1-8：解析与对齐的边界行为 ----------

func TestParseOtherDiagnoses(t *testing.T) {
	cases := []struct {
		name      string
		codes     string
		flags     string
		want      []OtherDiagnosisItem
		wantErr   error // nil 表示期望无错误；非 nil 用 errors.Is 判定
		errWants  []string
		designNot string
	}{
		// ---- 用例 1：正常输入 ----
		{
			name:  "正常：两项，标记分别为无与CC",
			codes: "I10.x00,E11.901",
			flags: "0,C",
			want: []OtherDiagnosisItem{
				{Code: "I10.X00", Flag: "0", Index: 0},
				{Code: "E11.901", Flag: "C", Index: 1},
			},
			designNot: "编码转大写与仓库既有编码比较口径一致（drg_rules.go / qc_condition.go）",
		},
		{
			name:  "正常：MCC 一项",
			codes: "N18.500",
			flags: "M",
			want:  []OtherDiagnosisItem{{Code: "N18.500", Flag: "M", Index: 0}},
		},

		// ---- 用例 2：长度不等（错位风险的核心防线）----
		{
			name:     "长度不等：codes 3 项 / flags 2 项必须报错",
			codes:    "I10.x00,E11.901,N18.500",
			flags:    "0,C",
			wantErr:  ErrOtherDiagnosisLengthMismatch,
			errWants: []string{"3", "2"},
		},
		{
			name:     "长度不等：codes 1 项 / flags 2 项必须报错",
			codes:    "I10.x00",
			flags:    "0,C",
			wantErr:  ErrOtherDiagnosisLengthMismatch,
			errWants: []string{"1", "2"},
		},
		{
			name:  "缺失标记：codes 有编码而 flags 整串为空 ⇒ 按缺失处理，不报长度不等",
			codes: "I10.x00",
			flags: "",
			want:  []OtherDiagnosisItem{{Code: "I10.X00", Flag: "0", Index: 0}},
			designNot: "空串无法谈论「槽位数」（它表达的是「这一列整个没填」而非「0 个槽」），" +
				"故 flags 整串为空时按 codeSlots 长度补零；长度校验只在两侧都非空时生效",
		},
		{
			name:     "长度不等：flags 有标记但 codes 为空",
			codes:    "",
			flags:    "C",
			wantErr:  ErrOtherDiagnosisLengthMismatch,
			errWants: []string{"其他诊断编码为空"},
		},

		// ---- 用例 3：重复编码（ADR §5 硬约束）----
		{
			name:     "重复编码：完全相同必须报错",
			codes:    "I10.x00,I10.x00",
			flags:    "0,C",
			wantErr:  ErrOtherDiagnosisDuplicate,
			errWants: []string{"I10.X00", "第 1 项与第 2 项"},
		},
		{
			name:     "重复编码：大小写与空格差异规范化后仍算重复",
			codes:    "I10.x00, i10.X00 ",
			flags:    "0,C",
			wantErr:  ErrOtherDiagnosisDuplicate,
			errWants: []string{"I10.X00"},
		},
		{
			name:     "重复编码：三处重复逐条列出",
			codes:    "A00.0,B00.0,A00.0,A00.0",
			flags:    "0,0,0,1",
			wantErr:  ErrOtherDiagnosisDuplicate,
			errWants: []string{"A00.0", "第 3 项", "第 4 项"},
		},

		// ---- 用例 4：空输入 ----
		{
			name:  "空输入：两侧都为空返回空切片且无错误",
			codes: "",
			flags: "",
			want:  nil,
		},
		{
			name:  "空输入：两侧都是空白串等同空输入",
			codes: "   ",
			flags: "\t",
			want:  nil,
		},

		// ---- 用例 5：空白与多余分隔符 ----
		{
			name:  "空白：编码与标记两侧空格被裁剪",
			codes: " I10.x00 , E11.901 ",
			flags: " 0 , C ",
			want: []OtherDiagnosisItem{
				{Code: "I10.X00", Flag: "0", Index: 0},
				{Code: "E11.901", Flag: "C", Index: 1},
			},
			designNot: "TrimSpace 后按逗号切分，故空格不影响槽位数",
		},
		{
			name:  "多个连续逗号：空槽与其 flag 成对丢弃，Index 保留原槽位",
			codes: "I10.x00,,E11.901",
			flags: "0,C,M",
			want: []OtherDiagnosisItem{
				{Code: "I10.X00", Flag: "0", Index: 0},
				{Code: "E11.901", Flag: "M", Index: 2},
			},
			designNot: "空槽的 flag 一并丢弃（C 随第 1 槽消失），剩下两项按原位置对齐；Index=2 说明未重排",
		},
		{
			name:  "尾随逗号：两侧槽位数一致时丢弃尾部空槽",
			codes: "I10.x00,",
			flags: "0,",
			want:  []OtherDiagnosisItem{{Code: "I10.X00", Flag: "0", Index: 0}},
		},
		{
			name:  "前导逗号：空槽丢弃后索引不重排（校验必须按槽位而非项数）",
			codes: ",I10.x00",
			flags: ",C",
			want:  []OtherDiagnosisItem{{Code: "I10.X00", Flag: "C", Index: 1}},
		},
		{
			name:      "空槽错位风险：codes 3 槽（2 空）而 flags 只 1 槽必须报错",
			codes:     ",,A00.0",
			flags:     "C",
			wantErr:   ErrOtherDiagnosisLengthMismatch,
			errWants:  []string{"3", "1"},
			designNot: "若先滤空项再对齐，C 会被静默配给 A00.0——本用例专门守住这条错位路径",
		},

		// ---- 用例 6：非法 flag ----
		{
			name:      "非法标记：X 必须报错而非按'无'处理",
			codes:     "I10.x00",
			flags:     "X",
			wantErr:   ErrOtherDiagnosisInvalidFlag,
			errWants:  []string{`"X"`},
			designNot: "静默降级为 0 会把 CC/MCC 抹掉，风险方向是漏报；宁可拒绝也不猜",
		},
		{
			name:      "非法标记：小写 c 不宽容",
			codes:     "I10.x00",
			flags:     "c",
			wantErr:   ErrOtherDiagnosisInvalidFlag,
			errWants:  []string{`"c"`},
			designNot: "大小写是数据错误而非等价写法；只对编码大小写做规范化，不对标记做",
		},
		{
			name:     "非法标记：双字符 CC 报错",
			codes:    "I10.x00",
			flags:    "CC",
			wantErr:  ErrOtherDiagnosisInvalidFlag,
			errWants: []string{`"CC"`},
		},
		// ---- 分隔符口径单独覆盖见 TestOtherDiagnosisSeparatorSemantics ----

		{
			name:  "单槽空白：编码非空而 flag 是空白串 ⇒ trim 后为空 ⇒ 按缺失处理为 0",
			codes: "I10.x00,E11.901",
			flags: "0, ",
			want: []OtherDiagnosisItem{
				{Code: "I10.X00", Flag: "0", Index: 0},
				{Code: "E11.901", Flag: "0", Index: 1},
			},
			designNot: "空白串与空串同义（都 TrimSpace 成 \"\"）：缺失不是断言，取默认值而非报错；" +
				"报错只留给明确写错的值（X / c / CC）",
		},

		// ---- 用例 7：缺失标记（整体为空 / 个别空槽）统一按"无严重度" ----
		{
			name:  "历史数据：codes 有 2 项 + flags 全空视为全部无严重度（向后兼容）",
			codes: "I10.x00,E11.901",
			flags: "",
			want: []OtherDiagnosisItem{
				{Code: "I10.X00", Flag: "0", Index: 0},
				{Code: "E11.901", Flag: "0", Index: 1},
			},
			designNot: "flags 整串为空 ⇒ 长度按 codeSlots 补零，不因长度不符报错",
		},
		{
			name:  "部分填写：中间槽为空串按'无'处理（显式空 ≠ 非法字符）",
			codes: "I10.x00,E11.901,N18.500",
			flags: "C,,M",
			want: []OtherDiagnosisItem{
				{Code: "I10.X00", Flag: "C", Index: 0},
				{Code: "E11.901", Flag: "0", Index: 1},
				{Code: "N18.500", Flag: "M", Index: 2},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseOtherDiagnoses(c.codes, c.flags)

			if c.wantErr != nil {
				if err == nil {
					t.Fatalf("ParseOtherDiagnoses(%q, %q) 未报错，期望 %v（%s）",
						c.codes, c.flags, c.wantErr, c.designNot)
				}
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("ParseOtherDiagnoses(%q, %q) 错误类型 = %v, 期望 %v",
						c.codes, c.flags, err, c.wantErr)
				}
				for _, sub := range c.errWants {
					if !strings.Contains(err.Error(), sub) {
						t.Errorf("错误信息 %q 未包含 %q（应能定位到出错位置）", err.Error(), sub)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseOtherDiagnoses(%q, %q) 意外报错：%v", c.codes, c.flags, err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ParseOtherDiagnoses(%q, %q)\n  实际 = %s\n  期望 = %s",
					c.codes, c.flags, dumpItems(got), dumpItems(c.want))
			}
		})
	}
}

// TestOtherDiagnosisSeparatorSemantics 固化分隔符口径：**只认半角逗号**。
//
// 这是刻意的"不猜测"：全角逗号 "，"、中文顿号 "、"、分号 ";" 都可能是输入法
// 手滑的产物，但把它们当分隔符意味着"一个字符判错就错位整串"——比"不切分、
// 交给编码字典校验拦截"危险得多。故本层只按半角逗号切，其余字符一律视为编码正文，
// 由后续的编码字典存在性校验（qcengine.go codeExists / drg 预检）负责拦截。
func TestOtherDiagnosisSeparatorSemantics(t *testing.T) {
	t.Run("全角逗号不切分：整串成为一个编码", func(t *testing.T) {
		items, err := ParseOtherDiagnoses("I10.x00，E11.901", "0")
		if err != nil {
			t.Fatalf("意外报错：%v", err)
		}
		want := []OtherDiagnosisItem{{Code: "I10.X00，E11.901", Flag: "0", Index: 0}}
		if !reflect.DeepEqual(items, want) {
			t.Fatalf("实际 = %s, 期望 %s（全角逗号不得被当作分隔符）", dumpItems(items), dumpItems(want))
		}
	})

	t.Run("全角逗号槽位数与 flags 不一致时报长度不等", func(t *testing.T) {
		// 真实场景：录入者用全角逗号分隔了两个编码，flags 串却按两个槽位写。
		// 本层看出 codes 只有 1 个槽 ⇒ 长度不等 ⇒ 报错，把问题暴露在录入阶段，
		// 而不是静默按 1 项处理（那会让第二个编码的严重度丢失）。
		_, err := ParseOtherDiagnoses("I10.x00，E11.901", "0,C")
		if !errors.Is(err, ErrOtherDiagnosisLengthMismatch) {
			t.Fatalf("错误 = %v, 期望 ErrOtherDiagnosisLengthMismatch", err)
		}
	})

	t.Run("分号同样不是分隔符", func(t *testing.T) {
		items, err := ParseOtherDiagnoses("I10.x00;E11.901", "0")
		if err != nil {
			t.Fatalf("意外报错：%v", err)
		}
		if len(items) != 1 || items[0].Code != "I10.X00;E11.901" {
			t.Fatalf("实际 = %s, 期望单个编码 I10.X00;E11.901", dumpItems(items))
		}
	})
}

// ---------- 用例 9：严重度分层 ----------

func TestSeverityLevel(t *testing.T) {
	cases := []struct {
		name  string
		items []OtherDiagnosisItem
		want  string
	}{
		{"空列表 → NONE", nil, "NONE"},
		{"全为 0 → NONE", []OtherDiagnosisItem{{Code: "I10.X00", Flag: "0"}}, "NONE"},
		{"一项 CC → CC", []OtherDiagnosisItem{
			{Code: "I10.X00", Flag: "0"}, {Code: "E11.901", Flag: "C"}}, "CC"},
		{"一项 MCC → MCC", []OtherDiagnosisItem{{Code: "N18.500", Flag: "M"}}, "MCC"},
		{"CC 与 MCC 并存 → 取最高级 MCC", []OtherDiagnosisItem{
			{Code: "E11.901", Flag: "C"}, {Code: "N18.500", Flag: "M"}}, "MCC"},
		{"多项 CC 仍为 CC", []OtherDiagnosisItem{
			{Code: "E11.901", Flag: "C"}, {Code: "I50.900", Flag: "C"}}, "CC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SeverityLevel(c.items); got != c.want {
				t.Fatalf("SeverityLevel(%s) = %q, 期望 %q", dumpItems(c.items), got, c.want)
			}
		})
	}
}

// TestSeverityLevelIntegration 从原始双串一路走到分层结果，覆盖 ADR §2 要的
// 「严重度分层预览」端到端路径（解析 → 汇总）。
func TestSeverityLevelIntegration(t *testing.T) {
	cases := []struct {
		codes, flags, want string
	}{
		{"", "", "NONE"},
		{"I10.x00,E11.901", "", "NONE"},             // 历史数据：无标记 ⇒ 无严重度
		{"I10.x00,E11.901", "0,C", "CC"},            // 有 CC
		{"I10.x00,E11.901,N18.500", "0,C,M", "MCC"}, // 有 MCC
	}
	for _, c := range cases {
		items, err := ParseOtherDiagnoses(c.codes, c.flags)
		if err != nil {
			t.Fatalf("ParseOtherDiagnoses(%q, %q) 意外报错：%v", c.codes, c.flags, err)
		}
		if got := SeverityLevel(items); got != c.want {
			t.Errorf("codes=%q flags=%q → 分层 %q, 期望 %q", c.codes, c.flags, got, c.want)
		}
	}
}

// ---------- 用例 10：反向函数与往返回归 ----------

// TestOtherDiagnosisFormatParseRoundTrip 固化 Parse 与 Format 的往返一致性。
//
// 精确不变式：**Parse(Format(items)) 逐项等于规范化后的 items**（大小写已是大写的输入）；
// 以及 Format(Parse(x)) 对规范化输入是恒等的。
// 不对"任意脏输入 x 都满足 Parse(Format(x)) == x"作断言——那要求保留尾部空槽等
// 非规范形态，与「规范化」的目的相悖（脏输入见 TestOtherDiagnosisDirtyInputIsNotCanonical）。
func TestOtherDiagnosisFormatParseRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		items []OtherDiagnosisItem
	}{
		{"空列表", nil},
		{"单项", []OtherDiagnosisItem{{Code: "I10.X00", Flag: "0"}}},
		{"两项", []OtherDiagnosisItem{
			{Code: "I10.X00", Flag: "0"}, {Code: "E11.901", Flag: "C"}}},
		{"含 MCC", []OtherDiagnosisItem{
			{Code: "I10.X00", Flag: "0"}, {Code: "E11.901", Flag: "C"}, {Code: "N18.500", Flag: "M"}}},
		{"输入编码未规范化（小写空格）", []OtherDiagnosisItem{
			{Code: " i10.x00 ", Flag: "0"}, {Code: "e11.901", Flag: "C"}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			codes, flags, err := FormatOtherDiagnoses(c.items)
			if err != nil {
				t.Fatalf("FormatOtherDiagnoses(%s) 意外报错：%v", dumpItems(c.items), err)
			}

			// ① Format 的输出必须是双串规范形态：项数相等，空列表则为空串。
			if len(c.items) > 0 && strings.Count(codes, ",") != strings.Count(flags, ",") {
				t.Fatalf("Format 输出的分隔符数不等：codes=%q flags=%q", codes, flags)
			}

			back, err := ParseOtherDiagnoses(codes, flags)
			if err != nil {
				t.Fatalf("Parse(Format(...)) 报错：codes=%q flags=%q err=%v", codes, flags, err)
			}

			// ② 与规范化后的期望逐项对齐（Index 不参与：Format 不保留原始槽位号）。
			want := make([]OtherDiagnosisItem, 0, len(c.items))
			for i, it := range c.items {
				want = append(want, OtherDiagnosisItem{
					Code:  strings.ToUpper(strings.TrimSpace(it.Code)),
					Flag:  it.Flag,
					Index: i,
				})
			}
			if len(want) == 0 {
				want = nil
			}
			if !reflect.DeepEqual(back, want) {
				t.Fatalf("往返不一致：Format → codes=%q flags=%q → Parse = %s, 期望 %s",
					codes, flags, dumpItems(back), dumpItems(want))
			}

			// ③ Format ∘ Parse ∘ Format 幂等。
			codes2, flags2, err := FormatOtherDiagnoses(back)
			if err != nil {
				t.Fatalf("二次 Format 意外报错：%v", err)
			}
			if codes2 != codes || flags2 != flags {
				t.Fatalf("Format 不幂等：第一次 (%q,%q)，第二次 (%q,%q)", codes, flags, codes2, flags2)
			}
		})
	}
}

// TestOtherDiagnosisDirtyInputIsNotCanonical 固化"脏输入会被规范化掉"这一事实，
// 用具体样例说明往返断言为何只对规范输入成立。
func TestOtherDiagnosisDirtyInputIsNotCanonical(t *testing.T) {
	dirty := []struct{ name, codes, flags string }{
		{"大小写不一致", "i10.x00,e11.901", "0,C"},
		{"含多余空格", " I10.x00 , E11.901 ", "0,C"},
	}
	for _, c := range dirty {
		t.Run(c.name, func(t *testing.T) {
			items, err := ParseOtherDiagnoses(c.codes, c.flags)
			if err != nil {
				t.Fatalf("ParseOtherDiagnoses(%q, %q) 意外报错：%v", c.codes, c.flags, err)
			}
			codes, flags, err := FormatOtherDiagnoses(items)
			if err != nil {
				t.Fatalf("FormatOtherDiagnoses 意外报错：%v", err)
			}
			if codes == c.codes && flags == c.flags {
				t.Fatalf("脏输入 %q/%q 未被规范化，规范化能力失效", c.codes, c.flags)
			}
			if codes != "I10.X00,E11.901" || flags != "0,C" {
				t.Fatalf("规范化结果 = (%q,%q), 期望 (\"I10.X00,E11.901\",\"0,C\")", codes, flags)
			}
		})
	}
}

// TestFormatOtherDiagnosesRejectsInvalid 反向函数不得成为绕过契约的后门。
func TestFormatOtherDiagnosesRejectsInvalid(t *testing.T) {
	cases := []struct {
		name    string
		items   []OtherDiagnosisItem
		wantErr error
	}{
		{"重复编码", []OtherDiagnosisItem{
			{Code: "I10.X00", Flag: "0"}, {Code: "i10.x00", Flag: "C"}}, ErrOtherDiagnosisDuplicate},
		{"非法标记", []OtherDiagnosisItem{{Code: "I10.X00", Flag: "X"}}, ErrOtherDiagnosisInvalidFlag},
		{"空编码", []OtherDiagnosisItem{{Code: "  ", Flag: "0"}}, ErrOtherDiagnosisInvalidFlag},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := FormatOtherDiagnoses(c.items); !errors.Is(err, c.wantErr) {
				t.Fatalf("FormatOtherDiagnoses(%s) 错误 = %v, 期望 %v", dumpItems(c.items), err, c.wantErr)
			}
		})
	}
}

func TestValidSeverityFlag(t *testing.T) {
	for _, flag := range []string{"0", "C", "M"} {
		if !ValidSeverityFlag(flag) {
			t.Errorf("ValidSeverityFlag(%q) = false, 期望 true", flag)
		}
	}
	for _, flag := range []string{"", "c", "m", "CC", "MCC", "X", "1", "无"} {
		if ValidSeverityFlag(flag) {
			t.Errorf("ValidSeverityFlag(%q) = true, 期望 false", flag)
		}
	}
}

func TestOtherDiagnosisCodesKeepsOrder(t *testing.T) {
	items, err := ParseOtherDiagnoses(" I10.x00 ,, E11.901 ", "0,C,M")
	if err != nil {
		t.Fatalf("意外报错：%v", err)
	}
	got := OtherDiagnosisCodes(items)
	want := []string{"I10.X00", "E11.901"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OtherDiagnosisCodes = %v, 期望 %v", got, want)
	}
}

// TestParseOtherDiagnosesAlignmentMismatchGuards 是本设计**最危险那条路径**的专项护栏，
// 与上面的表驱动用例分开写，因为两个方向的失败后果不同：
//
//	A. 长度不等（flags 比 codes 短）——现行实现报错。若有人"顺手"加一句
//	   "缺的位补 0" 来"修好"报错，就会静默错位，必须由本测试拦下。
//	B. 长度相等但含空槽（codes 与 flags 的空槽位置不一致）——**只能通过成对丢弃空槽
//	   来保证正确**。若实现改成"先滤掉所有空项、再按位置配 flag"（看起来很自然，
//	   而且能让 A 的一部分输入"通过"），那么 ",,A00.0" + "C" 会把 C 配给 A00.0。
//	   本测试把这条错误路径固化为必须报错。
//
// 结论：**长度校验必须在去空之前、按槽位数进行**。这是本文件存在的主要理由。
func TestParseOtherDiagnosesAlignmentMismatchGuards(t *testing.T) {
	t.Run("flags 比 codes 短：报错，不允许补 0 静默对齐", func(t *testing.T) {
		_, err := ParseOtherDiagnoses("I10.x00,E11.901,N18.500", "0,C")
		if !errors.Is(err, ErrOtherDiagnosisLengthMismatch) {
			t.Fatalf("错误 = %v, 期望 ErrOtherDiagnosisLengthMismatch", err)
		}
	})

	t.Run("codes 含空槽而 flags 不含：必须报错，不得把 flag 错配给后面的编码", func(t *testing.T) {
		_, err := ParseOtherDiagnoses(",,A00.0", "C")
		if !errors.Is(err, ErrOtherDiagnosisLengthMismatch) {
			t.Fatalf("错误 = %v, 期望 ErrOtherDiagnosisLengthMismatch（先滤空项再对齐会静默通过，是错位）", err)
		}
	})

	t.Run("两侧空槽位置一致：成对丢弃后仍然对齐正确", func(t *testing.T) {
		items, err := ParseOtherDiagnoses(",,A00.0", ",,C")
		if err != nil {
			t.Fatalf("意外报错：%v", err)
		}
		want := []OtherDiagnosisItem{{Code: "A00.0", Flag: "C", Index: 2}}
		if !reflect.DeepEqual(items, want) {
			t.Fatalf("实际 = %s, 期望 %s", dumpItems(items), dumpItems(want))
		}
	})

	t.Run("已知非对称：编码非空而 flag 为空 ⇒ 按 0，编码为空而 flag 非空 ⇒ 丢弃 flag", func(t *testing.T) {
		// 这是设计上**刻意保留的一处非对称**，也是本设计最隐蔽的残余风险，故单独固化：
		//   - 编码非空 + flag 空 ⇒ 视为 0（宽容）。理由：otherDiagnosisCodes 目前没有结构化
		//     录入界面（rectify.js / api-store.js 都只是整串回传的自由文本），尾随逗号是
		//     真实存在的脏数据形态；据此拒绝整份病案会挡住合法编码，代价高于收益。
		//   - 编码为空 + flag 非空 ⇒ 丢弃 flag（严格）。理由：这只能来自录入被截断或
		//     两个串行列手工改写，保留一个无所依附的 C/M 会让分层虚高。
		// 残余风险：用户若在最后一个编码后手滑多打一个逗号、同时 flag 串恰好是
		// "0,0,M" 这种形态，则 M 会随空槽静默消失（漏报一个 MCC）。这是"整串自由文本 +
		// 索引对齐"这一组合的固有代价，不是本函数可以单方面修掉的——见文件末尾说明。
		items, err := ParseOtherDiagnoses("I10.x00,", ",C")
		if err != nil {
			t.Fatalf("意外报错：%v", err)
		}
		want := []OtherDiagnosisItem{{Code: "I10.X00", Flag: "0", Index: 0}}
		if !reflect.DeepEqual(items, want) {
			t.Fatalf("实际 = %s, 期望 %s（按槽位对齐，不按「有值的项」配对）", dumpItems(items), dumpItems(want))
		}

		// 反向：编码为空 + flag 非空 ⇒ flag 丢弃（不是保留、也不是全局报错）。
		items2, err := ParseOtherDiagnoses(",I10.x00", "C,")
		if err != nil {
			t.Fatalf("意外报错：%v", err)
		}
		want2 := []OtherDiagnosisItem{{Code: "I10.X00", Flag: "0", Index: 1}}
		if !reflect.DeepEqual(items2, want2) {
			t.Fatalf("实际 = %s, 期望 %s（空编码槽的 flag 一并丢弃）", dumpItems(items2), dumpItems(want2))
		}
	})
}

// dumpItems 把解析结果渲染成便于比对失败时阅读的形式（含原槽位号）。
func dumpItems(items []OtherDiagnosisItem) string {
	if len(items) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, fmt.Sprintf("{#%d %s=%s}", it.Index, it.Code, it.Flag))
	}
	return "[" + strings.Join(parts, " ") + "]"
}
