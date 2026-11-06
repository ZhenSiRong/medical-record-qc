package service

import (
	"fmt"
	"slices"
	"strings"

	"gorm.io/gorm"

	"mrs/internal/model"
)

// =====================================================================
// cc_mcc_rule 的读取层（ADR-0002 §4：把「规则」从代码搬进库表）
//
// 分工：ccmcc.go 只管「编码 → 严重度」的判读算法（纯函数，规则以入参传入）；
// 本文件只管「从库里取出某一版本的规则」。两侧不互相改动，规则成为数据后
// 判读逻辑保持不动——这正是 ADR §4「换数据不改代码」的落地方式。
//
// 合规：本层不产出分组码 / 权重 / 金额（ADR §3/§6），也不做任何兜底猜测：
// 取不到规则就返回空，让上层判"无严重度"，绝不用演示数据静默顶替。
// =====================================================================

// LoadCcMccRules 读取指定版本的 CC/MCC 判读规则，转换成判读层可直接使用的
// []CcMccRule（字段一一映射，见 ccMccRuleFrom）。
//
// 调用方拿到结果后直接喂给 JudgeSeverity(version, mainDiag, otherCodes, rules)：
// 版本一致是本函数过滤保证的，JudgeSeverity 内部还会再按 Version 过滤一次，
// 两道过滤都指向同一件事——不让别的版本的规则混进本次判读。
//
// fail-closed：version 为空（或全是空白）⇒ 返回空切片 + error，不查库。
// 与 JudgeSeverity「未声明版本不让规则参与判断」同口径：静默使用某份未声明版本
// 的演示数据，是 ADR §6 明写的合规红线，宁可报错也不能悄悄放行。
//
// 查询出错时原样返回 error（不吞、不降级为空）：规则缺失与"库挂了"是两回事，
// 上层需要能区分，否则会把故障误读成"本例没有 CC/MCC"。
func LoadCcMccRules(db *gorm.DB, version string) ([]CcMccRule, error) {
	ver := strings.TrimSpace(version)
	if ver == "" {
		return []CcMccRule{}, fmt.Errorf("cc_mcc_rule: version 为空，拒绝加载规则（fail-closed，不查库）")
	}

	var rows []model.CcMccRule
	if err := db.Where("version = ?", ver).Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]CcMccRule, 0, len(rows))
	for _, r := range rows {
		out = append(out, ccMccRuleFrom(r))
	}
	// 按 DiagPrefix 稳定排序：判读本身取"最长前缀命中"，与顺序无关，
	// 排序只是让同一份数据每次读出同一序列（日志/快照/测试可比对）。
	slices.SortFunc(out, func(a, b CcMccRule) int {
		return strings.Compare(a.DiagPrefix, b.DiagPrefix)
	})
	return out, nil
}

// ccMccRuleFrom 把库表行映射成判读层规则（纯函数：不碰 DB，便于无库单测）。
//
// 这里刻意做**逐字段显式赋值**而不是按位置转换：将来表里加列（例如官方排除表的
// 多前缀形态）时，编译器不会替你检查位置，显式赋值至少让"新字段没被映射"这件事
// 在 review 里可见——映射漏字段的表现是"规则悄悄少了一个维度"，很难从结果反查。
func ccMccRuleFrom(m model.CcMccRule) CcMccRule {
	return CcMccRule{
		DiagPrefix:            m.DiagPrefix,
		Severity:              m.Severity,
		Version:               m.Version,
		Source:                m.Source,
		ExcludeWhenMainPrefix: m.ExcludeWhenMainPrefix,
		Remark:                m.Remark,
	}
}
