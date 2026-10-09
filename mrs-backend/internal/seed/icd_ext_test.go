package seed

import (
	"strings"
	"testing"

	"mrs/internal/model"
)

// 本文件锁定 icd_ext.go 的两件事，防止后续无意的改动破坏数据前置成果：
//  1. 批次内 (dictType, code) 唯一——判重键就是它，重复会让「新增条数」
//     与「实际入库条数」不一致，也会掩盖归类错误；
//  2. MCC / CC / 非 CC 三档的覆盖与条数——这三档是严重度分层能否演示的前提，
//     条数变化必须是显式修改而不是手滑。
//
// 注意：这里**不校验**编码本身是否符合国标（那需要官方码表，本仓库没有），
// 只校验不重复、非空、格式合理。编码的准确性靠人工逐条核对 + 只收录
// 「编码与中文名都能确认」的条目这一约定（见 icd_ext.go 文件头注释）。

func TestIcd10SeverityDefsUniqueAndWellFormed(t *testing.T) {
	items := icd10SeverityDefs()
	if len(items) == 0 {
		t.Fatal("icd10SeverityDefs 为空")
	}
	seen := map[string]string{}
	for _, it := range items {
		if it.code == "" || it.name == "" {
			t.Fatalf("存在空编码或空名称: %+v", it)
		}
		if !strings.HasPrefix(it.code, string(it.code[0])) || len(it.code) < 3 {
			t.Fatalf("编码过短，疑似笔误: %q (%s)", it.code, it.name)
		}
		// ICD-10 基础编码形如 X00 / X00.0 / X00.00
		if it.code[0] < 'A' || it.code[0] > 'Z' {
			t.Fatalf("编码首字符非字母: %q (%s)", it.code, it.name)
		}
		if prev, dup := seen[it.code]; dup {
			t.Fatalf("编码重复: %q 同时用于 %q 与 %q", it.code, prev, it.name)
		}
		seen[it.code] = it.name
	}
}

func TestIcd9ExtDefsUniqueAndWellFormed(t *testing.T) {
	items := icd9ExtDefs()
	if len(items) == 0 {
		t.Fatal("icd9ExtDefs 为空")
	}
	seen := map[string]string{}
	for _, it := range items {
		if it.code == "" || it.name == "" {
			t.Fatalf("存在空编码或空名称: %+v", it)
		}
		if it.code[0] < '0' || it.code[0] > '9' {
			t.Fatalf("ICD-9-CM3 编码应以数字开头: %q (%s)", it.code, it.name)
		}
		if prev, dup := seen[it.code]; dup {
			t.Fatalf("编码重复: %q 同时用于 %q 与 %q", it.code, prev, it.name)
		}
		seen[it.code] = it.name
	}
}

// TestIcdExtSeverityCoverage 锁定三档规模。
// 断言的是「下限」而非精确值：新增编码是允许的（加码是数据前置的正常演进），
// 但任何一档被削到下界以下都会让严重度分层演示失去素材，因此必须失败。
func TestIcdExtSeverityCoverage(t *testing.T) {
	type tier struct {
		sev      string
		desc     string
		minCount int
	}
	tiers := []tier{
		{"M", "MCC 级（急危重）", 20},
		{"C", "CC 级（有意义合并症）", 30},
		{"", "非 CC（基础病/轻症）", 10},
	}
	counts := map[string]int{}
	for _, it := range icd10SeverityDefs() {
		counts[it.sev]++
	}
	total := 0
	for _, tr := range tiers {
		total += counts[tr.sev]
		if counts[tr.sev] < tr.minCount {
			t.Errorf("%s 仅 %d 条，少于下界 %d 条：严重度分层演示素材不足",
				tr.desc, counts[tr.sev], tr.minCount)
		}
	}
	// ADR-0002 §7：24 条 ICD-10 撑不起严重度分层，编码库规模是该功能的前置。
	// 这里要求补充条目总量不低于 60 条（实际远高于此下界）。
	if total < 60 {
		t.Errorf("ICD-10 补充条目仅 %d 条，少于下界 60 条", total)
	}
	t.Logf("ICD-10 补充条目 %d 条：MCC %d、CC %d、非 CC %d",
		total, counts["M"], counts["C"], counts[""])
}

// TestIcdExtDoesNotCollideWithBaseSeed 保证补充条目不与原种子 seed.go 的
// 24 条 ICD-10 编码冲突。
//
// 为什么重要：SeedIcdExt 的判重键是 (dictType, code)，若补充条目与原种子
// 撞码，该条目会被静默跳过，于是「归类统计」与「实际入库」出现偏差。
// 原种子的编码在此显式列出（而不是去调 RunIfEmpty，那需要数据库），
// 一旦原种子的这 24 条被改动，本测试会失败并提醒同步核对。
func TestIcdExtDoesNotCollideWithBaseSeed(t *testing.T) {
	base := []string{
		"I10.x00", "I25.101", "I50.901", "I63.901", "I61.901", "G40.901",
		"J18.901", "J44.101", "J06.900", "K35.801", "K80.201", "K25.901",
		"S72.001", "S06.001", "S82.601", "C34.901", "C50.901", "E11.901",
		"E05.901", "N20.001", "N40.001", "O80.001", "O82.001", "A41.901",
	}
	inBase := map[string]struct{}{}
	for _, c := range base {
		inBase[c] = struct{}{}
	}
	for _, it := range icd10SeverityDefs() {
		if _, ok := inBase[it.code]; ok {
			t.Errorf("补充编码 %q (%s) 与原种子 ICD-10 编码冲突，会被判重跳过", it.code, it.name)
		}
	}
	// ICD-9-CM3 原种子
	baseOp := []string{
		"00.0000", "47.0900", "51.2300", "36.0600", "81.5100", "74.1000",
		"55.5101", "32.4901", "45.1300", "39.9500", "60.2901",
	}
	inBaseOp := map[string]struct{}{}
	for _, c := range baseOp {
		inBaseOp[c] = struct{}{}
	}
	for _, it := range icd9ExtDefs() {
		if _, ok := inBaseOp[it.code]; ok {
			t.Errorf("补充操作编码 %q (%s) 与原种子 ICD-9-CM3 编码冲突，会被判重跳过", it.code, it.name)
		}
	}
}

// TestIcdExtDictTypesAreValid 防止把编码塞进未注册的字典类型。
func TestIcdExtDictTypesAreValid(t *testing.T) {
	valid := map[string]struct{}{model.Icd10: {}, model.Icd9Cm3: {}}
	for _, dt := range []string{model.Icd10, model.Icd9Cm3} {
		if _, ok := valid[dt]; !ok {
			t.Fatalf("字典类型 %q 不在 icd_dict 支持范围内", dt)
		}
	}
	if extVersion == "" {
		t.Fatal("extVersion 不能为空（版本行必须有版本号）")
	}
}
