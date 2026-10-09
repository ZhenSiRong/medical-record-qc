package seed

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"mrs/internal/model"
)

// RunIfEmpty 空库首次启动自动灌种子（幂等：有用户则跳过）。
// 数据规模对齐演示需求：10 科室、12 医师、ICD-10×24 + ICD-9-CM3×11（演示同款码表）、
// 近 6 个月约 90 条病案（状态按月份加权，确定性伪随机）、借阅单与历史上报批次。
func RunIfEmpty(db *gorm.DB) error {
	var cnt int64
	if err := db.Model(&model.SysUser{}).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	log.Println("[seed] 检测到空库，正在灌入种子数据...")

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&[]model.SysRole{
			{RoleCode: "ADMIN", RoleName: "病案管理员", DataScope: "ALL", Remark: "全院数据"},
			{RoleCode: "DEPT_USER", RoleName: "科室用户", DataScope: "DEPT", Remark: "本科室数据（数据隔离待调研口径确认后生效）"},
		}).Error; err != nil {
			return err
		}

		deptNames := []string{"心血管内科", "神经内科", "呼吸内科", "消化内科", "普通外科",
			"骨科", "妇产科", "儿科", "肿瘤科", "泌尿外科"}
		depts := make([]model.SysDept, 0, len(deptNames))
		for i, n := range deptNames {
			depts = append(depts, model.SysDept{DeptCode: fmt.Sprintf("D%02d", i+1), DeptName: n})
		}
		if err := tx.Create(&depts).Error; err != nil {
			return err
		}
		deptID := map[string]int64{}
		for _, d := range depts {
			deptID[d.DeptName] = d.ID
		}

		hash := func(p string) string {
			b, _ := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
			return string(b)
		}
		if err := tx.Create(&[]model.SysUser{
			{Username: "admin", Password: hash("admin123"), RealName: "王主任", RoleCode: "ADMIN", DeptName: "病案统计科", Enabled: true},
			{Username: "deptuser", Password: hash("123456"), RealName: "陈志远", RoleCode: "DEPT_USER", DeptName: "心血管内科", Enabled: true},
		}).Error; err != nil {
			return err
		}

		doctorNames := []string{"张建国", "李慧敏", "王振华", "赵雅琴", "陈立新", "刘文博",
			"周晓东", "孙丽萍", "吴海燕", "郑秋月", "徐明远", "何静怡"}
		doctors := make([]model.SysDoctor, 0, len(doctorNames))
		titles := []string{"主任医师", "副主任医师", "主治医师"}
		for i, n := range doctorNames {
			dn := deptNames[i%len(deptNames)]
			doctors = append(doctors, model.SysDoctor{
				DoctorCode: fmt.Sprintf("D1%02d", i+1), DoctorName: n,
				DeptID: deptID[dn], DeptName: dn,
				Title: titles[i%3], Enabled: true,
			})
		}
		if err := tx.Create(&doctors).Error; err != nil {
			return err
		}

		if err := tx.Create(&[]model.SysDict{
			{DictType: "gender", DictKey: "1", DictValue: "男", SortOrder: 1},
			{DictType: "gender", DictKey: "2", DictValue: "女", SortOrder: 2},
			{DictType: "borrow_reason", DictKey: "01", DictValue: "临床病例讨论", SortOrder: 1},
			{DictType: "borrow_reason", DictKey: "02", DictValue: "医疗质量检查", SortOrder: 2},
			{DictType: "borrow_reason", DictKey: "03", DictValue: "科研数据采集", SortOrder: 3},
			{DictType: "borrow_reason", DictKey: "04", DictValue: "医患纠纷取证", SortOrder: 4},
			{DictType: "borrow_reason", DictKey: "05", DictValue: "教学查房示教", SortOrder: 5},
			{DictType: "borrow_reason", DictKey: "06", DictValue: "医保稽核", SortOrder: 6},
			{DictType: "borrow_reason", DictKey: "07", DictValue: "死亡病例讨论", SortOrder: 7},
		}).Error; err != nil {
			return err
		}

		// ---------- ICD 编码库（演示同款码表，保证编目选码可过存在性校验） ----------
		type icdDef struct{ code, name string }
		icd10Defs := []icdDef{
			{"I10.x00", "原发性高血压"}, {"I25.101", "冠状动脉粥样硬化性心脏病"}, {"I50.901", "心力衰竭"},
			{"I63.901", "脑梗死"}, {"I61.901", "脑出血"}, {"G40.901", "癫痫"},
			{"J18.901", "肺炎"}, {"J44.101", "慢性阻塞性肺疾病伴急性加重"}, {"J06.900", "急性上呼吸道感染"},
			{"K35.801", "急性阑尾炎"}, {"K80.201", "胆囊结石"}, {"K25.901", "胃溃疡"},
			{"S72.001", "股骨颈骨折"}, {"S06.001", "脑震荡"}, {"S82.601", "踝关节骨折"},
			{"C34.901", "支气管肺癌"}, {"C50.901", "乳腺恶性肿瘤"}, {"E11.901", "2型糖尿病"},
			{"E05.901", "甲状腺功能亢进症"}, {"N20.001", "肾结石"}, {"N40.001", "前列腺增生"},
			{"O80.001", "顺产"}, {"O82.001", "剖宫产"}, {"A41.901", "脓毒症"},
		}
		icd9Defs := []icdDef{
			{"00.0000", "无手术操作"}, {"47.0900", "阑尾切除术"}, {"51.2300", "腹腔镜胆囊切除术"},
			{"36.0600", "冠状动脉药物洗脱支架植入术"}, {"81.5100", "全髋关节置换术"}, {"74.1000", "剖宫产术"},
			{"55.5101", "肾盂切开取石术"}, {"32.4901", "肺叶切除术"}, {"45.1300", "结肠镜检查"},
			{"39.9500", "血液透析"}, {"60.2901", "经尿道前列腺电切术"},
		}
		if err := tx.Create(&[]model.IcdDictVersion{
			{Version: "ICD-10-DEMO-2024", DictType: model.Icd10, ImporterName: "seed", TotalCount: len(icd10Defs), Status: "ACTIVE"},
			{Version: "ICD-9CM3-DEMO-2024", DictType: model.Icd9Cm3, ImporterName: "seed", TotalCount: len(icd9Defs), Status: "ACTIVE"},
		}).Error; err != nil {
			return err
		}
		var v10, v9 model.IcdDictVersion
		tx.Where("dict_type = ?", model.Icd10).First(&v10)
		tx.Where("dict_type = ?", model.Icd9Cm3).First(&v9)
		icd10Rows := make([]model.IcdDict, 0, len(icd10Defs))
		for _, d := range icd10Defs {
			icd10Rows = append(icd10Rows, model.IcdDict{DictType: model.Icd10, Code: d.code, Name: d.name, VersionID: v10.ID, Enabled: true})
		}
		icd9Rows := make([]model.IcdDict, 0, len(icd9Defs))
		for _, d := range icd9Defs {
			icd9Rows = append(icd9Rows, model.IcdDict{DictType: model.Icd9Cm3, Code: d.code, Name: d.name, VersionID: v9.ID, Enabled: true})
		}
		if err := tx.Create(&icd10Rows).Error; err != nil {
			return err
		}
		if err := tx.Create(&icd9Rows).Error; err != nil {
			return err
		}

		if err := tx.Create(&[]model.QcRule{
			{RuleName: "患者姓名必填", RuleType: model.QcRequired, TargetField: "PatientName", ErrorLevel: "ERROR", Enabled: true},
			{RuleName: "入院时间必填", RuleType: model.QcRequired, TargetField: "AdmitTime", ErrorLevel: "ERROR", Enabled: true},
			{RuleName: "出院时间必填", RuleType: model.QcRequired, TargetField: "DischargeTime", ErrorLevel: "ERROR", Enabled: true},
			{RuleName: "主要诊断编码必填", RuleType: model.QcRequired, TargetField: "MainDiagnosisCode", ErrorLevel: "ERROR", Enabled: true},
			{RuleName: "出院晚于入院", RuleType: model.QcLogic, TargetField: "DischargeTime", Expression: "discharge_time>admit_time", ErrorLevel: "ERROR", Enabled: true},
			{RuleName: "住院天数合法性", RuleType: model.QcLogic, TargetField: "ActualDays", Expression: "actual_days>0", ErrorLevel: "WARN", Enabled: true},
			{RuleName: "主要诊断编码存在性", RuleType: model.QcCodeExist, TargetField: "MainDiagnosisCode", Expression: model.Icd10, ErrorLevel: "ERROR", Enabled: true},
			{RuleName: "主要手术编码存在性", RuleType: model.QcCodeExist, TargetField: "MainOperationCode", Expression: model.Icd9Cm3, ErrorLevel: "ERROR", Enabled: true},
		}).Error; err != nil {
			return err
		}

		if err := tx.Create(&[]model.Shelf{
			{ShelfCode: "A-01", Room: "病案库房一", LocationDesc: "第1列-1~3层", Capacity: 60},
			{ShelfCode: "A-02", Room: "病案库房一", LocationDesc: "第2列-1~3层", Capacity: 60},
			{ShelfCode: "B-01", Room: "病案库房二", LocationDesc: "第1列-1~4层", Capacity: 80},
		}).Error; err != nil {
			return err
		}

		// ---------- 病案生成器（确定性伪随机：固定种子，重灌结果一致） ----------
		rng := rand.New(rand.NewSource(20260921))
		ri := func(lo, hi int) int { return lo + rng.Intn(hi-lo+1) }
		chance := func(p float64) bool { return rng.Float64() < p }
		surnames := []string{"赵", "钱", "孙", "李", "周", "吴", "郑", "王", "冯", "陈", "褚", "卫", "蒋", "沈", "韩", "杨"}
		givens := []string{"国", "建", "文", "志", "明", "海", "秀", "淑", "玉", "桂", "德", "春", "华", "强", "军", "芳"}
		genName := func() string {
			n := surnames[rng.Intn(len(surnames))] + givens[rng.Intn(len(givens))]
			if chance(0.72) {
				n += givens[rng.Intn(len(givens))]
			}
			return n
		}

		now := time.Now()
		var records []model.MedRecord
		type meta struct {
			def    icdDef
			opCode string
			opName string
			stay   int
			fee    float64
		}
		var metas []meta
		seq := 0
		for off := 5; off >= 0; off-- {
			monthStart := time.Date(now.Year(), time.Month(int(now.Month())-off), 1, 0, 0, 0, 0, time.Local)
			count := ri(14, 18)
			if off == 0 {
				count = ri(8, 12)
			}
			for i := 0; i < count; i++ {
				seq++
				day := ri(1, 27)
				discharge := monthStart.AddDate(0, 0, day).Add(time.Duration(ri(8, 17)) * time.Hour)
				if discharge.After(now) {
					discharge = now.Add(-time.Duration(ri(2, 72)) * time.Hour)
				}
				stay := ri(3, 21)
				admit := discharge.AddDate(0, 0, -stay)

				def := icd10Defs[rng.Intn(len(icd10Defs))]
				dept := deptNames[rng.Intn(len(deptNames))]
				obstetric := def.code == "O80.001" || def.code == "O82.001"
				fracture := def.code == "S72.001" || def.code == "S06.001" || def.code == "S82.601"
				tumor := def.code == "C34.901" || def.code == "C50.901"
				switch {
				case obstetric:
					dept = "妇产科"
				case fracture && chance(0.75):
					dept = "骨科"
				case tumor && chance(0.75):
					dept = "肿瘤科"
				}
				age := ri(2, 88)
				if obstetric {
					age = ri(21, 39)
				}
				if dept == "儿科" {
					age = ri(1, 12)
				}
				gender := "男"
				if chance(0.48) || obstetric {
					gender = "女"
				}
				birth := time.Date(now.Year()-age, now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

				var status string
				r := rng.Intn(100)
				switch {
				case off == 0:
					switch {
					case r < 45:
						status = model.RecNotRecycled
					case r < 80:
						status = model.RecRecycled
					case r < 92:
						status = model.RecCataloged
					default:
						status = model.RecArchived
					}
				case off == 1:
					switch {
					case r < 15:
						status = model.RecNotRecycled
					case r < 45:
						status = model.RecRecycled
					case r < 65:
						status = model.RecCataloged
					case r < 95:
						status = model.RecArchived
					default:
						status = model.RecBorrowed
					}
				default:
					switch {
					case r < 5:
						status = model.RecNotRecycled
					case r < 17:
						status = model.RecRecycled
					case r < 32:
						status = model.RecCataloged
					case r < 92:
						status = model.RecArchived
					default:
						status = model.RecBorrowed
					}
				}

				rec := model.MedRecord{
					RecordNo:      fmt.Sprintf("BA%d%05d", now.Year(), seq),
					PatientName:   genName(),
					Gender:        gender,
					BirthDate:     &birth,
					HospitalNo:    fmt.Sprintf("ZY%d%04d", now.Year(), 1000+seq),
					DeptID:        deptID[dept],
					DeptName:      dept,
					DoctorName:    doctorNames[rng.Intn(len(doctorNames))],
					AdmitTime:     &admit,
					DischargeTime: &discharge,
					Status:        status,
				}
				m := meta{def: def, stay: stay, fee: float64(ri(3200, 128000)) + rng.Float64()}
				if status != model.RecNotRecycled {
					catAt := discharge.AddDate(0, 0, ri(6, 15))
					if catAt.After(now) {
						catAt = now
					}
					rec.CatalogedAt = &catAt
					if chance(0.7) {
						d := icd9Defs[ri(1, len(icd9Defs)-1)] // 0 号是"无手术操作"占位
						m.opCode, m.opName = d.code, d.name
					}
					if status == model.RecArchived || status == model.RecBorrowed {
						archAt := catAt.AddDate(0, 0, ri(1, 3))
						if archAt.After(now) {
							archAt = now
						}
						rec.ArchivedAt = &archAt
					}
				}
				records = append(records, rec)
				metas = append(metas, m)
			}
		}
		if err := tx.Create(&records).Error; err != nil {
			return err
		}

		// ---------- 编目 + 首页（已回收及之后的病案） ----------
		var frontPages []model.MedFrontPage
		var catalogs []model.CatalogEntry
		fpPeriod := map[string][]int{} // 周期 -> frontPages 下标（用于历史上报批次）
		for i := range records {
			if records[i].Status == model.RecNotRecycled {
				continue
			}
			m := metas[i]
			catalogs = append(catalogs, model.CatalogEntry{
				RecordID: records[i].ID, CatalogType: model.CatalogDiagnosis,
				IcdCode: m.def.code, IcdName: m.def.name, IsMain: true, CoderName: "王主任",
			})
			if m.opCode != "" {
				catalogs = append(catalogs, model.CatalogEntry{
					RecordID: records[i].ID, CatalogType: model.CatalogOperation,
					IcdCode: m.opCode, IcdName: m.opName, IsMain: true, CoderName: "王主任",
				})
			}
			if records[i].Status == model.RecRecycled {
				continue // 已回收待编目：还没有首页数据
			}
			fpStatus := model.PageQcPassed
			if records[i].Status == model.RecCataloged && chance(0.6) {
				fpStatus = model.PageWaitQc
			}
			fp := model.MedFrontPage{
				RecordID:    records[i].ID,
				PatientName: records[i].PatientName, Gender: records[i].Gender,
				BirthDate: records[i].BirthDate, HospitalNo: records[i].HospitalNo,
				DeptName: records[i].DeptName, DoctorName: records[i].DoctorName,
				AdmitTime: records[i].AdmitTime, DischargeTime: records[i].DischargeTime,
				ActualDays:     m.stay,
				AdmitDiagnosis: m.def.name + "待查", DischargeDiagnosis: m.def.name,
				MainDiagnosisCode: m.def.code, MainOperationCode: m.opCode,
				TotalCost: m.fee, Status: fpStatus,
			}
			frontPages = append(frontPages, fp)
			if fpStatus == model.PageQcPassed {
				period := records[i].DischargeTime.Format("2006-01")
				fpPeriod[period] = append(fpPeriod[period], len(frontPages)-1)
			}
		}
		if len(catalogs) > 0 {
			if err := tx.Create(&catalogs).Error; err != nil {
				return err
			}
		}
		if len(frontPages) > 0 {
			if err := tx.Create(&frontPages).Error; err != nil {
				return err
			}
		}

		// ---------- 借阅单（12 条，覆盖各状态；状态与病案实际状态严格一致） ----------
		var borrows []model.BorrowOrder
		var approvals []model.BorrowApproval
		var reminders []model.BorrowReminder
		reasons := []string{"临床病例讨论", "医疗质量检查", "科研数据采集", "医患纠纷取证", "教学查房示教", "医保稽核", "死亡病例讨论"}
		statusCycle := []string{model.BorReturned, model.BorReturned, model.BorReturned, model.BorBorrowed,
			model.BorBorrowed, model.BorApplying, model.BorOverdue, model.BorRejected}
		// 分桶：在架病案承接 申请/驳回/已归还；借出中病案承接 借出/逾期
		archivedIdx, borrowedIdx := []int{}, []int{}
		for i := range records {
			if records[i].Status == model.RecArchived {
				archivedIdx = append(archivedIdx, i)
			}
			if records[i].Status == model.RecBorrowed {
				borrowedIdx = append(borrowedIdx, i)
			}
		}
		rng.Shuffle(len(archivedIdx), func(a, b int) { archivedIdx[a], archivedIdx[b] = archivedIdx[b], archivedIdx[a] })
		rng.Shuffle(len(borrowedIdx), func(a, b int) { borrowedIdx[a], borrowedIdx[b] = borrowedIdx[b], borrowedIdx[a] })
		ai := 0
		// addOrder 为一条病案生成指定状态的借阅单
		addOrder := func(rec *model.MedRecord, st string, seq int) {
			applyAt := now.AddDate(0, 0, -ri(5, 60))
			o := model.BorrowOrder{
				OrderNo:      fmt.Sprintf("JY%d%03d", now.Year(), 601+seq),
				RecordID:     rec.ID,
				RecordNo:     rec.RecordNo,
				BorrowerName: doctorNames[rng.Intn(len(doctorNames))],
				BorrowerDept: deptNames[rng.Intn(len(deptNames))],
				Reason:       reasons[rng.Intn(len(reasons))],
				ApplyTime:    &applyAt,
				HandlerName:  "王主任",
			}
			switch st {
			case model.BorApplying:
				o.Status = model.BorApplying
				borrows = append(borrows, o)
			case model.BorRejected:
				o.Status = model.BorRejected
				borrows = append(borrows, o)
				approvals = append(approvals, model.BorrowApproval{OrderID: o.ID, Level: 1, ApproverName: "王主任", Result: "REJECTED", Opinion: "借阅事由不够具体，请补充说明后重新提交"})
			default:
				lendAt := applyAt.AddDate(0, 0, ri(0, 2))
				due := lendAt.AddDate(0, 0, 14)
				o.Status = st
				o.LendTime = &lendAt
				o.DueTime = &due
				borrows = append(borrows, o)
				approvals = append(approvals, model.BorrowApproval{OrderID: o.ID, Level: 1, ApproverName: "王主任", Result: "APPROVED", Opinion: "同意"})
				if st == model.BorReturned {
					ret := lendAt.AddDate(0, 0, ri(2, 13))
					o.ReturnTime = &ret
					borrows[len(borrows)-1] = o
				}
				if st == model.BorOverdue {
					reminders = append(reminders, model.BorrowReminder{OrderID: o.ID, Channel: "SYSTEM", Result: "超过应还时间，系统自动标记逾期"})
				}
			}
		}
		// 借出中的病案必须各有在借单（借出/逾期交替）
		for j := 0; j < len(borrowedIdx); j++ {
			st := model.BorBorrowed
			if j%3 == 2 {
				st = model.BorOverdue
			}
			addOrder(&records[borrowedIdx[j]], st, j)
		}
		// 其余槽位用在架病案补齐历史单（归还/申请/驳回）
		oseq := len(borrowedIdx)
		for orderCount := 0; orderCount < 12 && ai < len(archivedIdx); orderCount++ {
			st := statusCycle[orderCount%len(statusCycle)]
			if st == model.BorBorrowed || st == model.BorOverdue {
				st = model.BorReturned
			}
			addOrder(&records[archivedIdx[ai]], st, oseq)
			oseq++
			ai++
		}
		if len(borrows) > 0 {
			if err := tx.Create(&borrows).Error; err != nil {
				return err
			}
		}
		if len(approvals) > 0 {
			if err := tx.Create(&approvals).Error; err != nil {
				return err
			}
		}
		if len(reminders) > 0 {
			if err := tx.Create(&reminders).Error; err != nil {
				return err
			}
		}

		// ---------- 历史上报批次（前两个完整月份，回执全成功） ----------
		periods := make([]string, 0, 2)
		for off := 3; off >= 2; off-- {
			periods = append(periods, now.AddDate(0, -off, 0).Format("2006-01"))
		}
		for bi, period := range periods {
			idxs := fpPeriod[period]
			if len(idxs) == 0 {
				continue
			}
			submitAt := time.Date(now.Year(), time.Month(int(now.Month())-(3-bi)), 5, 9, ri(10, 55), 0, 0, time.Local)
			batch := model.ReportBatch{
				BatchNo: "SB" + strings.ReplaceAll(period, "-", "") + "01", Period: period,
				Status: model.BatSuccess, TotalCount: len(idxs), SuccessCount: len(idxs),
				SubmitTime: &submitAt, OperatorName: "王主任",
				Remark: "数据包已生成并提交，平台已确认接收",
			}
			if err := tx.Create(&batch).Error; err != nil {
				return err
			}
			var items []model.ReportItem
			for _, idx := range idxs {
				fp := frontPages[idx]
				items = append(items, model.ReportItem{
					BatchID: batch.ID, RecordID: fp.RecordID,
					ReceiptStatus: model.RcSuccess, ReceiptMsg: "本地桩回执：XML 已生成（骨架阶段）",
					ReceiptTime: &submitAt,
				})
				tx.Model(&model.MedFrontPage{}).Where("id = ?", fp.ID).Update("status", model.PageReported)
				frontPages[idx].Status = model.PageReported
			}
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}

		log.Printf("[seed] 完成：科室 %d、医师 %d、病案 %d、首页 %d、编目 %d、借阅 %d、批次 %d",
			len(depts), len(doctors), len(records), len(frontPages), len(catalogs), len(borrows), 2)
		return nil
	})
}
