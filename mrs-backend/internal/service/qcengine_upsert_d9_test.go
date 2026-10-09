package service

import (
	"reflect"
	"testing"
	"time"

	"mrs/internal/model"
)

// 本文件覆盖缺陷 D9（重跑质控丢处理状态，2026-10-09）：
// RunQc 旧实现是「先按 record_id 全删、再整批重建」，重建行的 handle_status 一律 OPEN，
// 于是已退回/已整改/已闭环的人工处理痕迹（退回意见/退回人/时间、整改人/时间、复核意见、
// 复核人/时间、fixed 标记）在重跑质控后全部丢失。
//
// 修复办法：把「历史行 + 本次命中的自动判定结果 → 最终应写入的行」这段决策抽成纯函数
// mergeQcResults（见 qcengine.go），本文件对它做表驱动覆盖。
// 这样做是因为 RunQc 依赖 *gorm.DB，而本仓库用 PostgreSQL 且未引入 sqlite 内存驱动，
// 无法在单测里真连库；决策逻辑抽纯后可以完整覆盖，无需数据库。
//
// 识别键：qcKey{RecordID, RuleID, FieldName}（依据见 qcengine.go 中 mergeQcResults 注释）。
//
// 语义约定（本文件同时充当行为规格）：
//  1. 同键命中 → 保留全部人工处理痕迹，只刷新自动判定字段 error_level / error_msg；
//  2. 同键且历史为 CLOSED（含 fixed 标记）→ 保持 CLOSED，不静默重开，仅更新 error_msg；
//  3. 本次新命中（无同键历史）→ handle_status = OPEN，其余处理字段为零值；
//  4. 历史有、本次不再命中 → 该行被「消失」处理（返回的 deleteIDs 列出），由 RunQc 删除前留痕。

// 固定测试时刻：指针字段比较用，避免 time.Now() 造成不可重复断言。
var (
	d9T1 = time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)
	d9T2 = time.Date(2026, 10, 3, 14, 5, 0, 0, time.UTC)
	d9T3 = time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)
)

// d9Auto 构造「本次质控命中的自动判定结果」行（RunQc 循环里构造出来的形态：
// 无 ID、无处理痕迹、handle_status 先按 OPEN 落值，最终由 merge 决定）。
func d9Auto(recordID, ruleID int64, field, level, msg string) model.QcResult {
	return model.QcResult{
		RecordID: recordID, RuleID: ruleID, FieldName: field,
		ErrorLevel: level, ErrorMsg: msg, HandleStatus: model.HandleOpen,
	}
}

// d9Existing 构造「库中历史行」：带 ID 与完整人工处理痕迹。
func d9Existing(id, recordID, ruleID int64, field, level, msg, status string) model.QcResult {
	r := model.QcResult{
		ID: id, RecordID: recordID, RuleID: ruleID, FieldName: field,
		ErrorLevel: level, ErrorMsg: msg, HandleStatus: status,
	}
	switch status {
	case model.HandleReturned:
		r.ReturnOpinion = "主诊断编码与手术不匹配，请核对后重新填写"
		r.ReturnedBy = "archivist"
		r.ReturnedAt = &d9T1
	case model.HandleRectified:
		r.ReturnOpinion = "请补填外部原因编码"
		r.ReturnedBy = "archivist"
		r.ReturnedAt = &d9T1
		r.RectifiedBy = "doctor01"
		r.RectifiedAt = &d9T2
	case model.HandleRejectedByQc:
		r.ReturnOpinion = "请补填外部原因编码"
		r.ReturnedBy = "archivist"
		r.ReturnedAt = &d9T1
		r.RectifiedBy = "doctor01"
		r.RectifiedAt = &d9T2
		r.ReviewOpinion = "仍为空，退回重填"
		r.ReviewedBy = "archivist"
		r.ReviewedAt = &d9T3
	case model.HandleClosed:
		r.ReturnOpinion = "请补填外部原因编码"
		r.ReturnedBy = "archivist"
		r.ReturnedAt = &d9T1
		r.RectifiedBy = "doctor01"
		r.RectifiedAt = &d9T2
		r.ReviewOpinion = "已核对，通过"
		r.ReviewedBy = "archivist"
		r.ReviewedAt = &d9T3
		r.Fixed = true
		r.FixedAt = &d9T3
		r.FixedBy = "archivist"
	}
	return r
}

// d9Ident 只取识别键与处理痕迹字段做比较，忽略 CreatedAt/UpdatedAt 等库维护字段
// （纯函数不负责这些列，避免测试断言与 GORM 行为耦合）。
func d9Ident(rs []model.QcResult) []model.QcResult {
	out := make([]model.QcResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, model.QcResult{
			ID: r.ID, RecordID: r.RecordID, RuleID: r.RuleID, FieldName: r.FieldName,
			ErrorLevel: r.ErrorLevel, ErrorMsg: r.ErrorMsg,
			Fixed: r.Fixed, FixedAt: r.FixedAt, FixedBy: r.FixedBy,
			HandleStatus:  r.HandleStatus,
			ReturnOpinion: r.ReturnOpinion, ReturnedBy: r.ReturnedBy, ReturnedAt: r.ReturnedAt,
			RectifiedBy: r.RectifiedBy, RectifiedAt: r.RectifiedAt,
			ReviewOpinion: r.ReviewOpinion, ReviewedBy: r.ReviewedBy, ReviewedAt: r.ReviewedAt,
		})
	}
	return out
}

// d9TraceFields 人工处理痕迹字段清单（逐一断言，失败信息比整体 DeepEqual 更可读）。
var d9TraceFields = []string{
	"HandleStatus", "ReturnOpinion", "ReturnedBy", "ReturnedAt",
	"RectifiedBy", "RectifiedAt", "ReviewOpinion", "ReviewedBy", "ReviewedAt",
	"Fixed", "FixedAt", "FixedBy",
}

func d9Field(r model.QcResult, name string) any {
	v := reflect.ValueOf(r).FieldByName(name)
	if !v.IsValid() {
		return nil
	}
	return v.Interface()
}

// assertTracePreserved 断言 got 相对 want 保留了全部人工处理痕迹字段。
func assertTracePreserved(t *testing.T, got, want model.QcResult) {
	t.Helper()
	for _, f := range d9TraceFields {
		if g, w := d9Field(got, f), d9Field(want, f); !reflect.DeepEqual(g, w) {
			t.Errorf("人工处理痕迹字段 %s 未保留: got=%v want=%v", f, g, w)
		}
	}
}

// ---------- 1. 表驱动主场景 ----------

func TestMergeQcResultsD9(t *testing.T) {
	cases := []struct {
		name       string
		existing   []model.QcResult
		fresh      []model.QcResult
		wantOut    []model.QcResult
		wantDelete []int64
	}{
		{
			name: "历史 OPEN + 本次同键命中：保留 OPEN，error_msg 更新",
			existing: []model.QcResult{
				d9Existing(11, 1, 5, "MainOperationCode", "WARN", "旧描述", model.HandleOpen),
			},
			fresh: []model.QcResult{
				d9Auto(1, 5, "MainOperationCode", "WARN", "新描述：主诊断与主手术不匹配"),
			},
			wantOut: []model.QcResult{
				{
					ID: 11, RecordID: 1, RuleID: 5, FieldName: "MainOperationCode",
					ErrorLevel: "WARN", ErrorMsg: "新描述：主诊断与主手术不匹配",
					HandleStatus: model.HandleOpen,
				},
			},
			wantDelete: nil,
		},
		{
			name: "历史 RETURNED + 本次同键命中：退回意见/退回人/时间全部保留，error_msg 更新",
			existing: []model.QcResult{
				d9Existing(12, 1, 5, "MainOperationCode", "WARN", "旧描述", model.HandleReturned),
			},
			fresh: []model.QcResult{
				d9Auto(1, 5, "MainOperationCode", "ERROR", "新描述"),
			},
			wantOut: []model.QcResult{
				func() model.QcResult {
					r := d9Existing(12, 1, 5, "MainOperationCode", "ERROR", "新描述", model.HandleReturned)
					return r
				}(),
			},
			wantDelete: nil,
		},
		{
			name: "历史 RECTIFIED + 本次同键命中：整改痕迹保留，error_msg 更新",
			existing: []model.QcResult{
				d9Existing(13, 1, 7, "ExternalCauseCode", "WARN", "旧描述", model.HandleRectified),
			},
			fresh: []model.QcResult{
				d9Auto(1, 7, "ExternalCauseCode", "WARN", "新描述：字段仍为空"),
			},
			wantOut: []model.QcResult{
				d9Existing(13, 1, 7, "ExternalCauseCode", "WARN", "新描述：字段仍为空", model.HandleRectified),
			},
			wantDelete: nil,
		},
		{
			name: "历史 REJECTED_BY_QC + 本次同键命中：复核不通过痕迹保留，error_msg 更新",
			existing: []model.QcResult{
				d9Existing(14, 1, 7, "ExternalCauseCode", "WARN", "旧描述", model.HandleRejectedByQc),
			},
			fresh: []model.QcResult{
				d9Auto(1, 7, "ExternalCauseCode", "WARN", "新描述"),
			},
			wantOut: []model.QcResult{
				d9Existing(14, 1, 7, "ExternalCauseCode", "WARN", "新描述", model.HandleRejectedByQc),
			},
			wantDelete: nil,
		},
		{
			name: "历史 CLOSED 再次命中（修正未生效/数据变回去）：保持 CLOSED 不重开，仅更新 error_msg",
			existing: []model.QcResult{
				d9Existing(15, 1, 7, "ExternalCauseCode", "WARN", "旧描述", model.HandleClosed),
			},
			fresh: []model.QcResult{
				d9Auto(1, 7, "ExternalCauseCode", "WARN", "新描述：外部原因编码仍为空"),
			},
			wantOut: []model.QcResult{
				d9Existing(15, 1, 7, "ExternalCauseCode", "WARN", "新描述：外部原因编码仍为空", model.HandleClosed),
			},
			wantDelete: nil,
		},
		{
			name: "历史有、本次不再命中（已修正）：行被删除，deleteIDs 列出待留痕行",
			existing: []model.QcResult{
				d9Existing(16, 1, 9, "TotalCost", "WARN", "费用为 0", model.HandleClosed),
			},
			fresh:      nil,
			wantOut:    []model.QcResult{},
			wantDelete: []int64{16},
		},
		{
			name:     "本次全新命中（无同键历史）：handle_status=OPEN，无任何处理痕迹",
			existing: nil,
			fresh: []model.QcResult{
				d9Auto(1, 21, "DoctorName", "ERROR", "必填字段为空"),
			},
			wantOut: []model.QcResult{
				{
					RecordID: 1, RuleID: 21, FieldName: "DoctorName",
					ErrorLevel: "ERROR", ErrorMsg: "必填字段为空", HandleStatus: model.HandleOpen,
				},
			},
			wantDelete: nil,
		},
		{
			name:     "历史空 + 本次空：无写入无删除",
			existing: nil, fresh: nil,
			wantOut: []model.QcResult{}, wantDelete: nil,
		},
		{
			name: "混合场景：一条 RETURNED 保留、一条 CLOSED 保留、一条已修正消失、两条新命中",
			existing: []model.QcResult{
				d9Existing(31, 1, 5, "MainOperationCode", "WARN", "旧描述A", model.HandleReturned),
				d9Existing(32, 1, 7, "ExternalCauseCode", "WARN", "旧描述B", model.HandleClosed),
				d9Existing(33, 1, 9, "TotalCost", "WARN", "费用异常", model.HandleOpen),
			},
			fresh: []model.QcResult{
				d9Auto(1, 5, "MainOperationCode", "WARN", "新描述A"),
				d9Auto(1, 7, "ExternalCauseCode", "ERROR", "新描述B"),
				d9Auto(1, 21, "DoctorName", "ERROR", "必填字段为空"),
				d9Auto(1, 22, "DeptName", "WARN", "科室为空"),
			},
			wantOut: []model.QcResult{
				d9Existing(31, 1, 5, "MainOperationCode", "WARN", "新描述A", model.HandleReturned),
				d9Existing(32, 1, 7, "ExternalCauseCode", "ERROR", "新描述B", model.HandleClosed),
				{
					RecordID: 1, RuleID: 21, FieldName: "DoctorName",
					ErrorLevel: "ERROR", ErrorMsg: "必填字段为空", HandleStatus: model.HandleOpen,
				},
				{
					RecordID: 1, RuleID: 22, FieldName: "DeptName",
					ErrorLevel: "WARN", ErrorMsg: "科室为空", HandleStatus: model.HandleOpen,
				},
			},
			wantDelete: []int64{33},
		},
		{
			name: "同字段不同规则不算同键：各自独立合并（field_name 单独不构成识别键）",
			existing: []model.QcResult{
				d9Existing(41, 1, 5, "MainDiagnosisCode", "WARN", "规则5旧描述", model.HandleReturned),
				d9Existing(42, 1, 6, "MainDiagnosisCode", "WARN", "规则6旧描述", model.HandleRectified),
			},
			fresh: []model.QcResult{
				d9Auto(1, 5, "MainDiagnosisCode", "WARN", "规则5新描述"),
				d9Auto(1, 6, "MainDiagnosisCode", "WARN", "规则6新描述"),
			},
			wantOut: []model.QcResult{
				d9Existing(41, 1, 5, "MainDiagnosisCode", "WARN", "规则5新描述", model.HandleReturned),
				d9Existing(42, 1, 6, "MainDiagnosisCode", "WARN", "规则6新描述", model.HandleRectified),
			},
			wantDelete: nil,
		},
		{
			name: "同规则不同字段不算同键：各自独立合并",
			existing: []model.QcResult{
				d9Existing(51, 1, 8, "NewbornBirthWeight", "WARN", "体重旧描述", model.HandleRectified),
				d9Existing(52, 1, 8, "NewbornGestationalWeeks", "WARN", "孕周旧描述", model.HandleOpen),
			},
			fresh: []model.QcResult{
				d9Auto(1, 8, "NewbornBirthWeight", "WARN", "体重新描述"),
				d9Auto(1, 8, "NewbornGestationalWeeks", "WARN", "孕周新描述"),
			},
			wantOut: []model.QcResult{
				d9Existing(51, 1, 8, "NewbornBirthWeight", "WARN", "体重新描述", model.HandleRectified),
				func() model.QcResult {
					return model.QcResult{
						ID: 52, RecordID: 1, RuleID: 8, FieldName: "NewbornGestationalWeeks",
						ErrorLevel: "WARN", ErrorMsg: "孕周新描述", HandleStatus: model.HandleOpen,
					}
				}(),
			},
			wantDelete: nil,
		},
		{
			name: "历史行 RuleID=0（规则已删除的遗留行）：无法与本次命中同键，按消失处理",
			existing: []model.QcResult{
				{ID: 61, RecordID: 1, RuleID: 0, FieldName: "MainDiagnosisCode", ErrorLevel: "ERROR", ErrorMsg: "遗留", HandleStatus: model.HandleOpen},
			},
			fresh: []model.QcResult{
				d9Auto(1, 0, "MainDiagnosisCode", "ERROR", "本次命中"),
			},
			wantOut: []model.QcResult{
				{
					RecordID: 1, RuleID: 0, FieldName: "MainDiagnosisCode",
					ErrorLevel: "ERROR", ErrorMsg: "本次命中", HandleStatus: model.HandleOpen,
				},
			},
			wantDelete: []int64{61},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, deleteIDs, err := mergeQcResults(tc.existing, tc.fresh)
			if err != nil {
				t.Fatalf("mergeQcResults 返回错误: %v", err)
			}
			if got := d9Ident(out); !reflect.DeepEqual(got, d9Ident(tc.wantOut)) {
				t.Errorf("最终写入行不符\n got = %+v\nwant = %+v", got, d9Ident(tc.wantOut))
			}
			if !reflect.DeepEqual(deleteIDs, tc.wantDelete) {
				t.Errorf("deleteIDs 不符: got=%v want=%v", deleteIDs, tc.wantDelete)
			}

			// 同键行必须逐一断言「人工处理痕迹保留、自动判定字段更新」。
			// 依据识别键在 wantOut 中定位对应行，避免顺序假设。
			for _, w := range tc.wantOut {
				if w.ID == 0 { // 新增行无历史痕迹可言
					continue
				}
				var got model.QcResult
				found := false
				for _, o := range out {
					if o.ID == w.ID {
						got, found = o, true
						break
					}
				}
				if !found {
					t.Fatalf("同键历史行 ID=%d 未出现在最终写入结果中（处理痕迹会丢失）", w.ID)
				}
				assertTracePreserved(t, got, w)
				if got.ErrorMsg != w.ErrorMsg || got.ErrorLevel != w.ErrorLevel {
					t.Errorf("自动判定字段未更新 ID=%d: got=(%s,%s) want=(%s,%s)",
						w.ID, got.ErrorLevel, got.ErrorMsg, w.ErrorLevel, w.ErrorMsg)
				}
			}
		})
	}
}

// ---------- 2. 幂等：同一输入连续跑两次结果相同 ----------

// TestMergeQcResultsD9_Idempotent 模拟「同一请求重复执行」：
// 第一次 merge 的写入行落库后（INSERT 行获得自增 ID、被删行从库中消失），
// 用「落库后的状态」当历史行、同样的本次命中再跑一次，结果（存活行与处理痕迹）必须不变，
// 且不再产生删除——否则重复执行会不断删改数据。
func TestMergeQcResultsD9_Idempotent(t *testing.T) {
	existing := []model.QcResult{
		d9Existing(71, 1, 5, "MainOperationCode", "WARN", "旧A", model.HandleReturned),
		d9Existing(72, 1, 7, "ExternalCauseCode", "WARN", "旧B", model.HandleClosed),
		d9Existing(73, 1, 9, "TotalCost", "WARN", "已修正项", model.HandleOpen),
	}
	fresh := []model.QcResult{
		d9Auto(1, 5, "MainOperationCode", "WARN", "新A"),
		d9Auto(1, 7, "ExternalCauseCode", "ERROR", "新B"),
		d9Auto(1, 21, "DoctorName", "ERROR", "新命中"),
	}

	out1, del1, err := mergeQcResults(existing, fresh)
	if err != nil {
		t.Fatalf("第一次 mergeQcResults 错误: %v", err)
	}
	if !reflect.DeepEqual(del1, []int64{73}) {
		t.Fatalf("第一次应只删除消失项 73，got=%v", del1)
	}

	// 把第一次结果当作「落库后的历史行」：INSERT 行（ID=0）在库中会获得自增主键。
	persisted := make([]model.QcResult, len(out1))
	copy(persisted, out1)
	var nextID int64 = 100
	for i := range persisted {
		if persisted[i].ID == 0 {
			persisted[i].ID = nextID
			nextID++
		}
	}

	out2, del2, err := mergeQcResults(persisted, fresh)
	if err != nil {
		t.Fatalf("第二次 mergeQcResults 错误: %v", err)
	}
	// 落库后新命中行获得自增主键（本测试模拟为 100），除主键外必须与第一次逐字段一致。
	wantOut1 := make([]model.QcResult, len(out1))
	copy(wantOut1, out1)
	var assignID int64 = 100
	for i := range wantOut1 {
		if wantOut1[i].ID == 0 {
			wantOut1[i].ID = assignID
			assignID++
		}
	}
	if !reflect.DeepEqual(d9Ident(out2), d9Ident(wantOut1)) {
		t.Errorf("非幂等：两次存活行不同\n第一次 = %+v\n第二次 = %+v", d9Ident(wantOut1), d9Ident(out2))
	}
	if len(del1) != 1 || del1[0] != 73 {
		t.Errorf("deleteIDs 应为 [73]（仅消失项），got=%v", del1)
	}
	if len(del2) != 0 {
		t.Errorf("第二次不应再产生删除（消失行已落库删除），got=%v", del2)
	}
	// 状态也必须逐字段一致：处理痕迹不得在重复执行中发生漂移。
	for i := range d9Ident(wantOut1) {
		for _, f := range d9TraceFields {
			if g, w := d9Field(wantOut1[i], f), d9Field(out2[i], f); !reflect.DeepEqual(g, w) {
				t.Errorf("第 %d 行的处理痕迹字段 %s 在重复执行后漂移: 第一次=%v 第二次=%v", i, f, g, w)
			}
		}
	}
}

// ---------- 3. 副作用与防御性 ----------

// mergeQcResults 不得就地改动 RunQc 传入的本次命中结果（RunQc 用 fresh 计算评分与返回给前端）。
func TestMergeQcResultsD9_DoesNotMutateFresh(t *testing.T) {
	fresh := []model.QcResult{
		d9Auto(1, 5, "MainOperationCode", "WARN", "新A"),
		d9Auto(1, 7, "ExternalCauseCode", "ERROR", "新B"),
	}
	snapshot := make([]model.QcResult, len(fresh))
	copy(snapshot, fresh)

	existing := []model.QcResult{
		d9Existing(81, 1, 5, "MainOperationCode", "WARN", "旧A", model.HandleReturned),
	}
	if _, _, err := mergeQcResults(existing, fresh); err != nil {
		t.Fatalf("mergeQcResults 错误: %v", err)
	}
	if !reflect.DeepEqual(fresh, snapshot) {
		t.Errorf("本次命中结果被就地改动\n got = %+v\nwant = %+v", fresh, snapshot)
	}
}

// 同键重复的历史脏行（并发/历史脏数据）：保留最后一条的痕迹（ID=92，CLOSED），
// 删除更早的重复行（ID=91），收敛为 1 行，避免 upsert 后前端工作台出现两条同样的问题项。
func TestMergeQcResultsD9_DuplicateExistingKey(t *testing.T) {
	existing := []model.QcResult{
		d9Existing(91, 1, 5, "MainOperationCode", "WARN", "旧1", model.HandleReturned),
		d9Existing(92, 1, 5, "MainOperationCode", "WARN", "旧2", model.HandleClosed),
	}
	fresh := []model.QcResult{d9Auto(1, 5, "MainOperationCode", "WARN", "新描述")}

	out, del, err := mergeQcResults(existing, fresh)
	if err != nil {
		t.Fatalf("mergeQcResults 错误: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("同键重复行必须收敛为 1 行，got=%d 行: %+v", len(out), out)
	}
	if out[0].ID != 92 {
		t.Errorf("重复键应保留最后一条（ID=92），got ID=%d", out[0].ID)
	}
	if out[0].HandleStatus != model.HandleClosed {
		t.Errorf("重复行应保留最新一条（ID=92 的 CLOSED）状态，got=%s", out[0].HandleStatus)
	}
	if !reflect.DeepEqual(del, []int64{91}) {
		t.Errorf("重复键的非保留行应列为删除留痕，got=%v", del)
	}
}

// 历史行缺主键（ID=0，数据异常）：无法原地 UPDATE，退化为按本次命中重新落库（OPEN），
// 但必须留日志提示异常（避免悄悄把处理痕迹换成一个新 ID）。
func TestMergeQcResultsD9_ExistingWithoutPrimaryKey(t *testing.T) {
	existing := []model.QcResult{
		{
			ID: 0, RecordID: 1, RuleID: 5, FieldName: "MainOperationCode",
			ErrorLevel: "WARN", ErrorMsg: "旧描述", HandleStatus: model.HandleReturned,
			ReturnOpinion: "请核对主手术", ReturnedBy: "archivist", ReturnedAt: &d9T1,
		},
	}
	fresh := []model.QcResult{d9Auto(1, 5, "MainOperationCode", "WARN", "新描述")}

	out, del, err := mergeQcResults(existing, fresh)
	if err != nil {
		t.Fatalf("mergeQcResults 错误: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("应恰好写入 1 行（重新落库的新命中），got=%d: %+v", len(out), out)
	}
	if out[0].ID != 0 || out[0].HandleStatus != model.HandleOpen {
		t.Errorf("缺主键历史行应退化为新增行（ID=0, OPEN），got ID=%d status=%s", out[0].ID, out[0].HandleStatus)
	}
	if len(del) != 0 {
		t.Errorf("ID=0 的行无法按主键删除，不应进入 deleteIDs，got=%v", del)
	}
}

// ---------- 4. 回归护栏：识别键的构成 ----------

// 识别键必须是三元组：只比 record_id 或只比 field_name 都会把不同项的痕迹串在一起。
func TestQcKeyD9_Composition(t *testing.T) {
	base := qcKey{RecordID: 1, RuleID: 2, FieldName: "MainDiagnosisCode"}
	same := qcKey{RecordID: 1, RuleID: 2, FieldName: "MainDiagnosisCode"}
	if base != same {
		t.Error("相同三元组必须是同一识别键")
	}
	for _, other := range []qcKey{
		{RecordID: 9, RuleID: 2, FieldName: "MainDiagnosisCode"},
		{RecordID: 1, RuleID: 3, FieldName: "MainDiagnosisCode"},
		{RecordID: 1, RuleID: 2, FieldName: "MainOperationCode"},
	} {
		if base == other {
			t.Errorf("三元组任一不同即为不同识别键，%+v 与 %+v 被判为相同", base, other)
		}
	}
}
