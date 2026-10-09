package seed

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	"mrs/internal/model"
)

// SeedDeptUserDemoData 让「科室医师」账号（deptuser / 陈志远）在演示数据里有真实可测的病案。
//
// 背景（实测发现的问题）：
//   seed.go 的病案医师姓名是从一个 12 人的硬编码 doctorNames 池里随机取的
//   （seed.go:272 `doctorNames[rng.Intn(len(doctorNames))]`），而 sys_user 里的
//   deptuser（陈志远，心血管内科）**不在那个池子里**，且 seed.go 也从不给病案指派
//   「医师 = 陈志远」。因此 deptuser 登录后，任何"按医师归属"的视图都是空的，
//   无法用该账号做任何演示或测试。
//
// 本函数把若干条演示病案改派给陈志远，覆盖演示所需的各类状态：
//   NOT_RECYCLED 未归档（可测归档回收）· RECYCLED 已回收待编目 · CATALOGED 已编目 · ARCHIVED 已归档在架
//
// 设计原则：
//  1. **幂等**：已有「医师 = 陈志远」的病案则整体跳过；每条状态的补派也各自判重。
//  2. **不影响既有演示数据**：只改 `med_record.doctor_name` 一个字段，不新建/不删除病案，
//     也不动编目、首页、归档流水、借阅单——因此既有的 87 份演示病案、统计口径、截图素材都不变。
//  3. **不动权限**：编目仍是病案室（ADMIN）的作业，本函数只解决"数据归属"问题。
//
// 依据：项目负责人 2026-10-09 确认——「先测编目相关流程，其他场景也需要陈志远能测」，
// 且明确选择「不动权限：编目继续只给 admin」。
func SeedDeptUserDemoData(db *gorm.DB) error {
	const doctorName = "陈志远"
	const deptName = "心血管内科"

	// 0) 医师表补登记：陈志远本就不在 sys_doctor 里（种子池 12 人没有他）。
	//    补登记是为了让"按医师统计/下拉选择"等场景也能看到他，与病案改派互为补充。
	var docCnt int64
	if err := db.Model(&model.SysDoctor{}).Where("doctor_name = ?", doctorName).Count(&docCnt).Error; err != nil {
		return err
	}
	if docCnt == 0 {
		var dept model.SysDept
		deptID := int64(0)
		if err := db.Where("dept_name = ?", deptName).First(&dept).Error; err == nil {
			deptID = dept.ID
		}
		doc := model.SysDoctor{
			DoctorCode: "D199", DoctorName: doctorName,
			DeptID: deptID, DeptName: deptName,
			Title: "主治医师", Enabled: true,
		}
		if err := db.Create(&doc).Error; err != nil {
			return fmt.Errorf("补登记医师 %s 失败: %w", doctorName, err)
		}
		log.Printf("[seed] 医师表补登记: %s（%s）", doctorName, deptName)
	}

	// 1) 已有陈志远的病案则跳过（幂等闸门）。
	var mine int64
	if err := db.Model(&model.MedRecord{}).Where("doctor_name = ?", doctorName).Count(&mine).Error; err != nil {
		return err
	}
	if mine > 0 {
		return nil
	}

	// 2) 按状态各补派 1 份，取 id 最小的未改派病案，确定性且可复现。
	//
	//    ⚠️ 科室优先（重要）：deptuser 的用户档案科室是「心血管内科」，而
	//    `GET /api/qc-results/my-pending` 按**科室**限定数据范围（ADR/缺口 D4 的设计）。
	//    若只按状态挑病案、不管科室，该账号的「我的待整改」会永远是空的——
	//    因为被改派的病案可能属于别的科室。故这里**优先在同科室里挑**，
	//    同科室确实没有符合状态的病案时，才退化为跨科室挑选（并在日志里说明）。
	wanted := []struct {
		status string
		note   string
	}{
		{model.RecNotRecycled, "未归档（可测归档回收）"},
		{model.RecRecycled, "已回收待编目（待编目队列）"},
		{model.RecCataloged, "已编目"},
		{model.RecArchived, "已归档在架（可测借阅）"},
	}

	assigned := 0
	for _, w := range wanted {
		rec, sameDept, err := pickDemoRecord(db, w.status, doctorName, deptName)
		if err != nil {
			log.Printf("[seed] 陈志远演示数据：状态 %s（%s）无可用病案，跳过", w.status, w.note)
			continue
		}
		if err := db.Model(&model.MedRecord{}).Where("id = ?", rec.ID).
			Update("doctor_name", doctorName).Error; err != nil {
			return fmt.Errorf("改派病案 %s 给 %s 失败: %w", rec.RecordNo, doctorName, err)
		}
		scope := "本科室"
		if !sameDept {
			scope = "跨科室（本科室无该状态病案）"
		}
		log.Printf("[seed] 陈志远演示数据：病案 %s（%s）已改派，状态 %s，%s",
			rec.RecordNo, w.note, w.status, scope)
		assigned++
	}

	if assigned == 0 {
		log.Printf("[seed] 陈志远演示数据：未找到可改派病案（可能演示库为空或状态分布异常）")
	}
	return nil
}

// pickDemoRecord 为演示改派挑一条病案：优先同科室，其次跨科室。
// 只挑「仍归他人」且非借出/封存的病案——借出与封存状态和借阅单强关联，改派会造成数据不自洽。
func pickDemoRecord(db *gorm.DB, status, doctorName, deptName string) (model.MedRecord, bool, error) {
	base := func() *gorm.DB {
		return db.Where("status = ? AND doctor_name <> ? AND doctor_name <> ''", status, doctorName)
	}
	var rec model.MedRecord
	// 优先：同科室
	if err := base().Where("dept_name = ?", deptName).Order("id ASC").First(&rec).Error; err == nil {
		return rec, true, nil
	}
	// 退化：跨科室
	if err := base().Order("id ASC").First(&rec).Error; err != nil {
		return rec, false, err
	}
	return rec, false, nil
}
