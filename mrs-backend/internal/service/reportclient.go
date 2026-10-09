package service

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"

	"mrs/internal/model"
)

// ReportClient 上报防腐层（调研节 9 + 质量控制要求：上报接口以官方正式发布的
// 标准文档为准，禁止口头需求作为设计依据）。官方接口文档到位后实现官方版客户端
// 并替换 LocalStubReportClient，路由与批次/回执逻辑不变。
type ReportClient interface {
	// Submit 提交一条首页数据，返回（回执状态, 回执信息, 错误）
	Submit(batch *model.ReportBatch, item *model.ReportItem, fp *model.MedFrontPage) (string, string, error)
}

// reportXML 住院病案首页上报 XML 的骨架字段集（WS/T 系列规范完整字段待官方文档对齐）
type reportXML struct {
	XMLName           xml.Name `xml:"MedicalRecordFrontPage"`
	BatchNo           string   `xml:"batchNo"`
	Period            string   `xml:"period"`
	RecordNo          string   `xml:"recordNo"`
	PatientName       string   `xml:"patientName"`
	Gender            string   `xml:"gender"`
	HospitalNo        string   `xml:"hospitalNo"`
	DeptName          string   `xml:"deptName"`
	AdmitTime         string   `xml:"admitTime"`
	DischargeTime     string   `xml:"dischargeTime"`
	MainDiagnosisCode string   `xml:"mainDiagnosisCode"`
	MainOperationCode string   `xml:"mainOperationCode"`
	TotalCost         float64  `xml:"totalCost"`
}

// LocalStubReportClient 本地桩：把首页数据组包为 XML 落盘到 outDir/批次号/病案号.xml，
// 直接返回成功回执。用于骨架联调与课件演示。
type LocalStubReportClient struct {
	OutDir string
}

func (s *LocalStubReportClient) Submit(batch *model.ReportBatch, item *model.ReportItem, fp *model.MedFrontPage) (string, string, error) {
	payload := reportXML{
		BatchNo: batch.BatchNo, Period: batch.Period,
		RecordNo: fp.HospitalNo, PatientName: fp.PatientName, Gender: fp.Gender,
		HospitalNo: fp.HospitalNo, DeptName: fp.DeptName,
		MainDiagnosisCode: fp.MainDiagnosisCode, MainOperationCode: fp.MainOperationCode,
		TotalCost: fp.TotalCost,
	}
	if fp.AdmitTime != nil {
		payload.AdmitTime = fp.AdmitTime.Format("2006-01-02 15:04:05")
	}
	if fp.DischargeTime != nil {
		payload.DischargeTime = fp.DischargeTime.Format("2006-01-02 15:04:05")
	}

	dir := filepath.Join(s.OutDir, batch.BatchNo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return model.RcFail, "", fmt.Errorf("创建上报目录失败: %w", err)
	}
	out, err := xml.MarshalIndent(payload, "", "  ")
	if err != nil {
		return model.RcFail, "", err
	}
	file := filepath.Join(dir, fmt.Sprintf("%s.xml", fp.HospitalNo))
	if err := os.WriteFile(file, append([]byte(xml.Header), out...), 0o644); err != nil {
		return model.RcFail, "", err
	}
	return model.RcSuccess, "本地桩回执：XML 已生成（骨架阶段，官方通道待对接）", nil
}
