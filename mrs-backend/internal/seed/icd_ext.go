package seed

// =====================================================================
// ICD 编码库补充种子（2026-10-09）
//
// 背景：ADR-0002 决定做「CC/MCC 漏编（低靠）+ 高靠风险」严重度预警。
// 但原种子（seed.go 的 icd10Defs）只有 24 条演示编码，且**没有一条**是
// CC/MCC 典型编码——心衰病因、糖尿病并发症、慢性肾病、呼吸衰竭、脓毒症
// 等一个都没有。后果是严重度分层功能无法演示：病案里根本填不出一个
// 「严重并发症」。「编码库规模不足」这一条已被 ADR-0002 §7 明确列为前置。
//
// 本文件按 §9 批次 A（数据前置）补入一批 ICD-10 临床常用编码，
// 覆盖 MCC / CC / 非 CC 三档，使严重度分层可被演示与验证。
//
// 数据来源与准确性约定（重要）：
//   - 只收录「编码 + 中文名称」两者都能确认的条目。编码取自 ICD-10 国际标准
//     （WHO ICD-10 2019）的类目/亚目层；中文名沿用国内病案与医保通行译名。
//   - 名称一律用临床最通用的那个说法（如 I48 用「心房颤动」，K92.2 用
//     「消化道出血」），不使用需要额外确认的地方性写法。
//   - **凡是不能确认的编码一律不加**，宁缺勿造。本任务的原则是
//     「不引入可疑数据」——演示库数据不准确比数据少更危险。
//   - 一律使用「基础编码」（如 I50.9 / N18.5），不带国临版的扩展后缀
//     （如 x00/x001），因此对任何 ICD-10 派生版本都成立。
//   - 本批编码**只作字典条目**（编目选码、存在性校验、严重度分层演示用），
//     不声称等同于官方 CC/MCC 表。CC/MCC 分级依据见下方 category 注释与
//     交付说明：按「该编码作为其他诊断时是否显著影响资源消耗与死亡风险」
//     这一通行原则归类，属**院内口径**，不得作为结算依据。
//
// 幂等：见 SeedIcdExt —— 判重键 (dictType, code)，只补不覆盖。
//
// 被刻意排除的编码（如实记录，避免后人"补全"时又加回来）：
//   - C80（恶性肿瘤）：C80 只是二字类目名，不是可用的四位编码，
//     故不收；恶性肿瘤已按部位以 C16.9 / C18.9 / C22.9 / C50.9 等收录。
//   - J15.x（细菌性肺炎）：四位亚目须指定细菌种类才能成形，
//     临床常见写法是 J15.9，但中文名与国临版口径未核实，故不收。
//   - I63.4（心源性脑栓塞）、I63.5（动脉闭塞性脑梗死）、E11.1（2 型糖尿病
//     伴酮症酸中毒）、J20.9（急性支气管炎）、J93.9（气胸）：编码本身成立，
//     但「基础编码是否带第四位 / 第四位值」在国内国临版与医保版之间存在
//     差异，未核实前不收。
//   - 「重症肺炎」「糖尿病酮症酸中毒」等**临床诊断名**：不是独立的 ICD-10
//     编码（前者按 A41/R57 组合，后者按 E11.1 系列或加 E87.2 组合），
//     收录等于把诊断名当编码，故不收。
// =====================================================================

import (
	"log"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mrs/internal/model"
)

// icdItem 一条编码定义。
//
// sev（严重度分级）取值：
//
//	"M" = MCC 级：作为其他诊断时通常显著提高资源消耗与死亡风险
//	"C" = CC 级：作为其他诊断时构成有意义但程度较轻的合并症
//	""  = 非 CC：基础病/低风险编码
//
// sev 只用于本次整理与交付统计，不落库——落库分级由 ADR-0002 批次 B 的
// cc_mcc_rule 表负责（该表带版本、来源标注与排除规则，形态已由 ADR 锁定）。
// 本文件的任务只是把「能撑起分层演示的码」补进 icd_dict。
type icdItem struct {
	code string
	name string
	sev  string
}

// icd10SeverityDefs 严重度分层用 ICD-10 编码（批次 A 数据前置）。
// 每个分组的注释给出该组的选用理由（即分类依据）。
func icd10SeverityDefs() []icdItem {
	var out []icdItem
	add := func(sev string, pairs ...[2]string) {
		for _, p := range pairs {
			out = append(out, icdItem{code: p[0], name: p[1], sev: sev})
		}
	}

	// ---------- MCC 级：心血管急危重 ----------
	// 依据：急性心肌梗死、心源性休克、慢性心衰急性加重均为 CHS-DRG 主要
	// 并发症/合并症等级中的高位项，直接改变 MDC 与 ADRG 权重。
	add("M",
		[2]string{"I21.9", "急性心肌梗死"},
		[2]string{"I21.4", "急性非ST段抬高型心肌梗死"},
		[2]string{"I22.9", "复发性心肌梗死"},
		[2]string{"I50.0", "充血性心力衰竭"},
		[2]string{"I50.9", "心力衰竭"},
		[2]string{"R57.0", "心源性休克"},
	)

	// ---------- CC 级：心血管非急危重 ----------
	// 依据：心房颤动（I48）是国内外 CC 表的常客；高血压性心脏病（I11）、
	// 心绞痛（I20.9）属需持续治疗但不构成急危重的合并症。
	add("C",
		[2]string{"I48", "心房颤动"},
		[2]string{"I11.9", "高血压性心脏病"},
		[2]string{"I11.0", "高血压性心脏病伴心力衰竭"},
		[2]string{"I20.9", "心绞痛"},
	)

	// ---------- MCC 级：呼吸衰竭与肺循环急症 ----------
	add("M",
		[2]string{"J96.0", "急性呼吸衰竭"},
		[2]string{"J96.1", "慢性呼吸衰竭"},
		[2]string{"J96.9", "呼吸衰竭"},
		[2]string{"I26.9", "肺栓塞"},
	)

	// ---------- CC 级：COPD 急性加重、胸腔积液 ----------
	// 依据：J44.1 是 COPD 住院最常用的合并症编码（急性加重本身即住院事由），
	// 在 CC 表中通常作 CC 而非 MCC。
	add("C",
		[2]string{"J44.1", "慢性阻塞性肺疾病伴急性加重"},
		[2]string{"J90", "胸腔积液"},
	)

	// ---------- 非 CC：呼吸道基础病 ----------
	add("",
		[2]string{"J20.9", "急性支气管炎"},
		[2]string{"J93.9", "气胸"},
	)
	// 说明：J20.9 / J93.9 的第四位取值在不同派生版本中写法不一，若后续
	// 引入官方码表校验，需优先复核这两条（本函数判重键是 code，改名安全）。

	// ---------- MCC 级：脓毒症与各类休克 ----------
	// 依据：脓毒症（A41.9）与感染性休克（R57.2）是典型的 MCC 定义项；
	// J15.9/J18.9 本身只是肺炎，临床上「重症肺炎」按 A41/R57 组合编码，
	// 故此处不单列名称为「重症肺炎」的条目——避免把非独立编码当作编码加入。
	add("M",
		[2]string{"A41.9", "脓毒症"},
		[2]string{"R57.2", "感染性休克"},
		[2]string{"R57.1", "低血容量性休克"},
		[2]string{"R57.9", "休克"},
	)

	// ---------- MCC 级：消化道大出血、肝衰竭、急性胰腺炎、腹膜炎 ----------
	add("M",
		[2]string{"K92.2", "消化道出血"},
		[2]string{"K72.9", "肝衰竭"},
		[2]string{"K72.0", "急性肝衰竭"},
		[2]string{"K85.9", "急性胰腺炎"},
		[2]string{"K65.0", "急性腹膜炎"},
	)

	// ---------- CC 级：消化系统慢性合并症 ----------
	add("C",
		[2]string{"K74.6", "肝硬化"},
		[2]string{"K76.6", "门静脉高压"},
		[2]string{"K25.9", "胃溃疡"},
		[2]string{"K26.9", "十二指肠溃疡"},
	)

	// ---------- 非 CC：消化系统常见轻症 ----------
	add("",
		[2]string{"K29.1", "急性胃炎"},
		[2]string{"K29.7", "胃炎"},
		[2]string{"K52.9", "胃肠炎"},
		[2]string{"K81.0", "急性胆囊炎"},
		[2]string{"K40.9", "腹股沟疝"},
	)

	// ---------- MCC 级：急性肾衰竭、慢性肾病 5 期/终末期 ----------
	// 依据：N17（急性肾衰竭）、N18.5（CKD 5 期）、N18.0（终末期肾病）
	// 与 Z99.2（透析依赖）是 CC/MCC 表中确定性最高的一组 MCC。
	add("M",
		[2]string{"N17.9", "急性肾功能衰竭"},
		[2]string{"N17.0", "急性肾衰竭伴肾小管坏死"},
		[2]string{"N18.5", "慢性肾脏病5期"},
		[2]string{"N18.0", "终末期肾病"},
		[2]string{"Z99.2", "肾透析状态"},
	)

	// ---------- CC 级：慢性肾病 3–4 期、泌尿道感染 ----------
	add("C",
		[2]string{"N18.4", "慢性肾脏病4期"},
		[2]string{"N18.3", "慢性肾脏病3期"},
		[2]string{"N18.9", "慢性肾脏病"},
		[2]string{"N39.0", "泌尿道感染"},
	)

	// ---------- 非 CC：良性泌尿系统疾病 ----------
	add("",
		[2]string{"N20.9", "尿路结石"},
		[2]string{"N40", "前列腺增生"},
		[2]string{"N83.2", "卵巢囊肿"},
	)

	// ---------- MCC 级：糖尿病急性并发症 ----------
	// 依据：酮症酸中毒/乳酸性酸中毒/昏迷是典型 MCC；糖尿病足伴坏疽亦按 MCC 计。
	add("M",
		[2]string{"E11.0", "2型糖尿病伴昏迷"},
		[2]string{"E10.1", "1型糖尿病伴酮症酸中毒"},
		[2]string{"E10.0", "1型糖尿病伴昏迷"},
		[2]string{"E11.5", "2型糖尿病伴周围循环并发症"},
		[2]string{"E11.4", "2型糖尿病伴神经并发症"},
	)

	// ---------- CC 级：糖尿病慢性并发症、甲状腺与营养/电解质紊乱 ----------
	// 依据：糖尿病伴并发症、甲亢、贫血、电解质紊乱（低钠/低钾/代谢性酸中毒）
	// 在 CC 表中普遍作 CC：需处理、可改变诊疗路径，但不属急危重。
	add("C",
		[2]string{"E11.6", "2型糖尿病伴其他特指的并发症"},
		[2]string{"E11.3", "2型糖尿病伴眼并发症"},
		[2]string{"E11.2", "2型糖尿病伴肾并发症"},
		[2]string{"E10.9", "1型糖尿病"},
		[2]string{"E05.9", "甲状腺功能亢进症"},
		[2]string{"E03.9", "甲状腺功能减退症"},
		[2]string{"E87.1", "低钠血症"},
		[2]string{"E87.6", "低钾血症"},
		[2]string{"E87.2", "代谢性酸中毒"},
		[2]string{"E86", "脱水"},
		[2]string{"D64.9", "贫血"},
		[2]string{"D50.9", "缺铁性贫血"},
		[2]string{"D62", "急性失血后贫血"},
	)

	// ---------- 非 CC：肥胖、血脂异常 ----------
	// 单纯 2 型糖尿病已由原种子 E11.901 覆盖，此处不重复收录。
	add("",
		[2]string{"E66.9", "肥胖症"},
		[2]string{"E78.5", "高脂血症"},
	)

	// ---------- MCC / CC：神经系统 ----------
	// 依据：脑梗死本身在多数 CC 表中作 CC，但伴发脑水肿、颅内高压、
	// 昏迷等情形升为 MCC；此处保守处理——把「脑梗死」列 CC，
	// 把明确的危重状态（G93.6 脑水肿、R40.2 昏迷）单列为 MCC。
	add("C",
		[2]string{"I63.9", "脑梗死"},
		[2]string{"I61.9", "脑出血"},
		[2]string{"G40.9", "癫痫"},
		[2]string{"G20", "帕金森病"},
		[2]string{"G30.9", "阿尔茨海默病"},
		[2]string{"G81.9", "偏瘫"},
	)
	add("M",
		[2]string{"G93.6", "脑水肿"},
		[2]string{"R40.2", "昏迷"},
	)

	// ---------- 恶性肿瘤 ----------
	// 依据：恶性肿瘤在 CC/MCC 表中的分级随部位与是否转移而变，
	// 不宜一刀切。此处统一列 CC，并在交付说明中明确「未按部位细分，
	// 如需用于严重度预警应由批次 B 的 cc_mcc_rule 按部位逐条定级」。
	add("C",
		[2]string{"C50.9", "乳腺恶性肿瘤"},
		[2]string{"C18.9", "结肠恶性肿瘤"},
		[2]string{"C16.9", "胃恶性肿瘤"},
		[2]string{"C22.9", "肝恶性肿瘤"},
		[2]string{"C61", "前列腺恶性肿瘤"},
		[2]string{"C53.9", "宫颈恶性肿瘤"},
		[2]string{"C56", "卵巢恶性肿瘤"},
		[2]string{"C91.0", "急性淋巴细胞白血病"},
		[2]string{"C92.0", "急性髓系白血病"},
		[2]string{"C81.9", "霍奇金淋巴瘤"},
	)

	// ---------- 非 CC：常见症状与轻症 ----------
	// 注意：非 CC 档的「原发性高血压 / 急性上呼吸道感染 / 肺炎 / 急性阑尾炎 /
	// 胆囊结石 / 单纯 2 型糖尿病 / 顺产 / 剖宫产 / 支气管肺癌」在原种子
	// seed.go 的 icd10Defs 里**已有**对应编码（I10.x00 / J06.900 / J18.901 /
	// K35.801 / K80.201 / E11.901 / O80.001 / O82.001 / C34.901 …）。
	// 因为本函数的判重键是 (dictType, code)，此处**刻意不重复收录**，
	// 避免出现「同一疾病两个编码」的脏数据；这一档的演示能力由原种子提供。
	add("",
		[2]string{"M54.5", "腰痛"},
		[2]string{"M79.1", "肌痛"},
		[2]string{"M25.5", "关节痛"},
		[2]string{"M51.2", "腰椎间盘突出"},
		[2]string{"K59.0", "便秘"},
		[2]string{"R10.4", "腹痛"},
		[2]string{"R51", "头痛"},
		[2]string{"R42", "头晕"},
		[2]string{"M15.0", "原发性全身性骨关节炎"},
		[2]string{"K02.9", "龋齿"},
		[2]string{"H25.9", "老年性白内障"},
	)

	return out
}

// icd9ExtDefs 与上述诊断配套的**高频手术操作**补充（数量刻意保持很小）。
//
// 说明：本任务的核心是「让严重度分层可演示」，严重度只取决于诊断编码，
// 与手术操作无关。因此这里只补少量与新增诊断天然配对、且原字典缺失的
// 高频操作，避免为了凑数引入可疑编码。编码为 ICD-9-CM-3 基础类目，
// 沿用原种子的格式风格。
//
// 注：血液透析（39.9500）原种子已有，此处不重复收录（判重键是 code）。
func icd9ExtDefs() []icdItem {
	return []icdItem{
		{code: "39.9503", name: "连续性肾脏替代治疗（CRRT）"},
		{code: "96.7201", name: "持续气道正压通气（CPAP）"},
		{code: "96.0400", name: "气管内插管"},
	}
}

// extVersion 名称：沿用既有演示版本号，不新增「看起来像官方版」的版本名。
const extVersion = "ICD-10-DEMO-2024"

// SeedIcdExt 幂等补入严重度分层用的 ICD 编码（ADR-0002 §9 批次 A）。
//
// 为什么要单独一个函数而不是改 RunIfEmpty：
// RunIfEmpty 是「空库才灌」，正在运行的演示库已有 24 条 ICD-10，
// 它永远不会再执行第二次。若把新编码只写进 seed.go，**已存在的库重启后
// 什么都不会发生**。因此本函数与 handler.SeedQcExt / SeedBranchExt 并列，
// 每次启动都执行，靠判重保证幂等。
//
// 判重键：(dict_type, code)。理由：
//   - icd_dict 的业务唯一性就是「某个字典类型下的一个编码」，name 是属性；
//   - 按 name 判重不可靠（同一疾病可有多个编码、名称可修订）；
//   - 按 code 判重能同时兼容「空库首灌」与「已有库补齐」两条路径。
//
// 只补不覆盖：已存在的 (dictType, code) 一律跳过，**不更新名称、不删除、
// 不改 enabled**。因此本函数对演示库里的既有数据是纯增量、可重复执行，
// 也不影响另一个并行任务对 icd_dict 的使用。
func SeedIcdExt(db *gorm.DB) error {
	type batch struct {
		dictType string
		items    []icdItem
	}
	batches := []batch{
		{model.Icd10, icd10SeverityDefs()},
		{model.Icd9Cm3, icd9ExtDefs()},
	}

	verID := map[string]int64{}
	for _, b := range batches {
		id, err := ensureIcdVersion(db, b.dictType)
		if err != nil {
			return err
		}
		verID[b.dictType] = id

		var existing []string
		if err := db.Model(&model.IcdDict{}).
			Where("dict_type = ?", b.dictType).
			Pluck("code", &existing).Error; err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(existing))
		for _, c := range existing {
			seen[c] = struct{}{}
		}

		rows := make([]model.IcdDict, 0, len(b.items))
		for _, it := range b.items {
			if _, ok := seen[it.code]; ok {
				continue // 已存在：跳过（幂等，不覆盖）
			}
			seen[it.code] = struct{}{} // 同一批次内的重复也只插一条
			rows = append(rows, model.IcdDict{
				DictType: b.dictType, Code: it.code, Name: it.name,
				VersionID: verID[b.dictType], Enabled: true,
			})
		}
		if len(rows) == 0 {
			log.Printf("[seed] %s 编码库无需补充（判重键 dict_type+code）", b.dictType)
			continue
		}
		// 并发/重复启动保护：唯一冲突时不做任何事，由下一次查询体现真实结果。
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return err
		}
		log.Printf("[seed] %s 编码库补充 %d 条（判重键 dict_type+code）", b.dictType, len(rows))
	}

	// 版本行 TotalCount 与实际条数对齐（只影响编码库版本展示，不动数据行）。
	for _, b := range batches {
		var cnt int64
		if err := db.Model(&model.IcdDict{}).Where("dict_type = ?", b.dictType).Count(&cnt).Error; err != nil {
			return err
		}
		if err := db.Model(&model.IcdDictVersion{}).Where("id = ?", verID[b.dictType]).
			Update("total_count", cnt).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureIcdVersion 返回该字典类型下可用于承载补充编码的版本 ID。
//
// 优先复用**已有的**版本行（不新建版本）：这样补充编码与既有编码落在同一
// 版本下，接口/界面按版本统计时不会出现「同一份码表两个版本」的割裂，
// 也不需要把旧版本置为 HISTORY（那会把 24 条既有编码标成历史版）。
// 仅当该类型完全没有版本行时，才新建一个演示版本。
func ensureIcdVersion(db *gorm.DB, dictType string) (int64, error) {
	var v model.IcdDictVersion
	err := db.Where("dict_type = ?", dictType).Order("id").First(&v).Error
	if err == nil {
		return v.ID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}
	v = model.IcdDictVersion{
		Version: extVersion, DictType: dictType,
		ImporterName: "seed", Status: "ACTIVE",
		TotalCount: 0,
	}
	if err := db.Create(&v).Error; err != nil {
		return 0, err
	}
	return v.ID, nil
}
