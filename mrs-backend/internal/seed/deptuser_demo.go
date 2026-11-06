package seed

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"mrs/internal/model"
)

// 科室医师（deptuser）演示账号的医师姓名与科室，与 seed.go 的用户种子保持一致。
const (
	deptUserDoctor  = "陈志远"
	deptUserDept    = "心血管内科"
	deptUserDocCode = "D199"
)

// deptUserTargets 陈志远名下各状态的**目标份数下限**（补足式，不是一次性改派）。
//
// 为什么需要这么多：该账号要能覆盖演示/测试的所有病案相关场景，
// 而不同场景对状态的要求不同（见每行的用途注释）。
//
// ⚠️ 这些数字是**下限**，不是上限：实际份数可能超出。原因很自然——
// 通过界面把「待编目」的病案编目后，动作链在质控通过时会自动上架，
// 该病案就变成 ARCHIVED，于是 ARCHIVED 的份数会增加（RECYCLED 相应减少）。
// 这是预期的真实流转，**不是**种子重复灌入；判据始终是"当前份数 vs 目标下限"。
var deptUserTargets = []struct {
	status string
	count  int
	use    string
}{
	{model.RecRecycled, 6, "待编目队列：练习编目、验证编目动作链"},
	{model.RecCataloged, 10, "「最近完成的编目」列表、编目完成数 KPI、上报候选集"},
	{model.RecArchived, 2, "已归档在架：借阅申请、归档流转履历"},
	{model.RecNotRecycled, 2, "未归档：归档回收操作"},
}

// SeedDeptUserDemoData 让「科室医师」账号（deptuser / 陈志远）在演示数据里有足够可测的病案。
//
// 背景（实测发现的问题）：
//
//	seed.go 的病案医师姓名是从一个 12 人的硬编码 doctorNames 池里随机取的
//	（seed.go:272 `doctorNames[rng.Intn(len(doctorNames))]`），而 sys_user 里的
//	deptuser（陈志远，心血管内科）**不在那个池子里**，且 seed.go 也从不给病案指派
//	「医师 = 陈志远」。因此 deptuser 登录后，任何"按医师归属"的视图都是空的，
//	无法用该账号做任何演示或测试。
//
// 做法：
//  1. 医师表补登记陈志远（种子池没有他）
//  2. 按 deptUserTargets 逐状态**补足**份数——**优先同科室**（deptuser 的
//     「我的待整改」按科室限定数据范围，跨科室病案会让它永远是空的）
//  3. 把改造过的病案状态**直接设为目标状态**（而非只挑现成的），使份数可控
//  4. 分散 cataloged_at，让「最近完成的编目」按时间排序时呈现真实分布
//
// 幂等：判据是"当前份数 vs 目标份数"，只补差额；已达标则不做任何写操作。
// 影响面：只改 med_record 的 doctor_name / status / cataloged_at / archived_at。
// 不新建、不删除病案，不动编目记录、首页、归档流水、借阅单——
// 既有演示数据规模（87 份）与统计口径不变，仅归属与状态在演示库上被重新分配。
func SeedDeptUserDemoData(db *gorm.DB) error {
	if err := ensureDeptUserDoctor(db); err != nil {
		return err
	}

	for _, t := range deptUserTargets {
		var have int64
		if err := db.Model(&model.MedRecord{}).
			Where("doctor_name = ? AND status = ?", deptUserDoctor, t.status).
			Count(&have).Error; err != nil {
			return err
		}
		need := t.count - int(have)
		if need <= 0 {
			continue
		}
		recs, sameDept, err := pickDemoRecords(db, t.status, need, deptUserDept)
		if err != nil {
			log.Printf("[seed] 陈志远演示数据：状态 %s 需要 %d 份，无可用病案，跳过（%v）", t.status, need, err)
			continue
		}
		scope := "本科室优先"
		if !sameDept {
			scope = "本科室不足，已跨科室补充"
		}
		if err := assignDemoRecords(db, recs, t.status); err != nil {
			return err
		}
		log.Printf("[seed] 陈志远演示数据：状态 %s 补足 %d 份（%s，用途：%s）", t.status, len(recs), scope, t.use)
	}

	// 分散编目时间：仅在"编目时间完全相同"时调整，保证幂等（第二次运行无改动）。
	if err := spreadCatalogTimes(db); err != nil {
		return err
	}
	return nil
}

// ensureDeptUserDoctor 医师表补登记陈志远（幂等）。
func ensureDeptUserDoctor(db *gorm.DB) error {
	var cnt int64
	if err := db.Model(&model.SysDoctor{}).Where("doctor_name = ?", deptUserDoctor).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	deptID := int64(0)
	var dept model.SysDept
	if err := db.Where("dept_name = ?", deptUserDept).First(&dept).Error; err == nil {
		deptID = dept.ID
	}
	doc := model.SysDoctor{
		DoctorCode: deptUserDocCode, DoctorName: deptUserDoctor,
		DeptID: deptID, DeptName: deptUserDept,
		Title: "主治医师", Enabled: true,
	}
	if err := db.Create(&doc).Error; err != nil {
		return fmt.Errorf("补登记医师 %s 失败: %w", deptUserDoctor, err)
	}
	log.Printf("[seed] 医师表补登记: %s（%s）", deptUserDoctor, deptUserDept)
	return nil
}

// pickDemoRecords 挑 need 份可改派的病案：先同科室，不够再跨科室。
// 排除：已归陈志远的、无医师名的、目标状态已是最终态的（BORROWED/SEALED 与借阅单强关联，一律不动）。
func pickDemoRecords(db *gorm.DB, status string, need int, deptName string) ([]model.MedRecord, bool, error) {
	base := func() *gorm.DB {
		return db.Where("doctor_name <> ? AND doctor_name <> '' AND status NOT IN ?",
			deptUserDoctor, []string{model.RecBorrowed, model.RecSealed})
	}
	var same []model.MedRecord
	if err := base().Where("dept_name = ?", deptName).Order("id ASC").Limit(need).Find(&same).Error; err != nil {
		return nil, false, err
	}
	if len(same) >= need {
		return same, true, nil
	}
	var rest []model.MedRecord
	got := map[int64]bool{}
	for _, r := range same {
		got[r.ID] = true
	}
	q := base().Order("id ASC")
	if len(got) > 0 {
		ids := make([]int64, 0, len(got))
		for id := range got {
			ids = append(ids, id)
		}
		q = q.Where("id NOT IN ?", ids)
	}
	if err := q.Limit(need - len(same)).Find(&rest).Error; err != nil {
		return nil, false, err
	}
	out := append(same, rest...)
	if len(out) == 0 {
		return nil, false, fmt.Errorf("无可用候选")
	}
	return out, false, nil
}

// assignDemoRecords 把选中病案的医师改为陈志远，并把状态设为目标状态，
// 同时补齐该状态对应的时间戳（使状态自洽：已编目必须有 catalogedAt、已归档必须有 archivedAt）。
func assignDemoRecords(db *gorm.DB, recs []model.MedRecord, status string) error {
	now := time.Now()
	for i, r := range recs {
		updates := map[string]any{
			"doctor_name": deptUserDoctor,
			"dept_name":   deptUserDept, // 与账号科室一致，保证「我的待整改」按科室能命中
			"status":      status,
		}
		switch status {
		case model.RecCataloged:
			// 分散到最近 30 天内，让「最近完成的编目」排序接近真实
			t := now.AddDate(0, 0, -(i*3 + 1))
			updates["cataloged_at"] = t
			updates["archived_at"] = nil
		case model.RecArchived:
			updates["archived_at"] = now.AddDate(0, 0, -(i + 1))
		case model.RecRecycled, model.RecNotRecycled:
			updates["cataloged_at"] = nil
			updates["archived_at"] = nil
		}
		if err := db.Model(&model.MedRecord{}).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("改派病案 %s 失败: %w", r.RecordNo, err)
		}
	}
	return nil
}

// spreadCatalogTimes 为陈志远名下"已有编目时间但彼此完全相同"的病案做一次错开，
// 避免「最近完成的编目」里一堆并列时间、排序看不出先后。
// 幂等：时间已互不相同则不做任何写入。
func spreadCatalogTimes(db *gorm.DB) error {
	var recs []model.MedRecord
	if err := db.Where("doctor_name = ? AND cataloged_at IS NOT NULL", deptUserDoctor).
		Order("cataloged_at ASC, id ASC").Find(&recs).Error; err != nil {
		return err
	}
	seen := map[string]int{}
	for _, r := range recs {
		if r.CatalogedAt == nil {
			continue
		}
		key := r.CatalogedAt.Format(time.RFC3339)
		seen[key]++
	}
	// 只有存在重复时间时才错开
	dup := false
	for _, n := range seen {
		if n > 1 {
			dup = true
			break
		}
	}
	if !dup {
		return nil
	}
	base := time.Now().AddDate(0, 0, -1)
	for i, r := range recs {
		t := base.AddDate(0, 0, -(i*2 + 1))
		if err := db.Model(&model.MedRecord{}).Where("id = ?", r.ID).
			Update("cataloged_at", t).Error; err != nil {
			return err
		}
	}
	log.Printf("[seed] 陈志远演示数据：已错开 %d 份病案的编目时间", len(recs))
	return nil
}
