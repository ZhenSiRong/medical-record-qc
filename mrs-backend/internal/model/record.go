package model

import "time"

// ---------- 病案主状态机 ----------
// NOT_RECYCLED 未回收 → RECYCLED 已回收(待编目) → CATALOGED 已编目 → ARCHIVED 已归档(在架)
// ARCHIVED ⇄ BORROWED 借出；ARCHIVED → SEALED 封存
const (
	RecNotRecycled = "NOT_RECYCLED"
	RecRecycled    = "RECYCLED"
	RecCataloged   = "CATALOGED"
	RecArchived    = "ARCHIVED"
	RecBorrowed    = "BORROWED"
	RecSealed      = "SEALED"
)

// ---------- 首页数据子状态机 ----------
// DRAFT → WAIT_QC → QC_PASSED(可上报) / QC_REJECTED(修正后重新 WAIT_QC) → REPORTED
const (
	PageDraft      = "DRAFT"
	PageWaitQc     = "WAIT_QC"
	PageQcPassed   = "QC_PASSED"
	PageQcRejected = "QC_REJECTED"
	PageReported   = "REPORTED"
)

type MedRecord struct {
	ID            int64      `json:"id" gorm:"primaryKey"`
	RecordNo      string     `json:"recordNo" gorm:"size:32"`
	PatientName   string     `json:"patientName"`
	Gender        string     `json:"gender"`
	BirthDate     *time.Time `json:"birthDate"`
	HospitalNo    string     `json:"hospitalNo"`
	DeptID        int64      `json:"deptId"`
	DeptName      string     `json:"deptName"`
	DoctorName    string     `json:"doctorName"`
	AdmitTime     *time.Time `json:"admitTime"`
	DischargeTime *time.Time `json:"dischargeTime"`
	Status        string     `json:"status" gorm:"default:NOT_RECYCLED"`
	ShelfID       int64      `json:"shelfId"`
	ArchivedAt    *time.Time `json:"archivedAt"`  // 归档上架时间
	CatalogedAt   *time.Time `json:"catalogedAt"` // 完成编目时间
	Remark        string     `json:"remark"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (MedRecord) TableName() string { return "med_record" }

type MedFrontPage struct {
	ID                  int64      `json:"id" gorm:"primaryKey"`
	RecordID            int64      `json:"recordId"`
	PatientName         string     `json:"patientName"`
	Gender              string     `json:"gender"`
	BirthDate           *time.Time `json:"birthDate"`
	HospitalNo          string     `json:"hospitalNo"`
	DeptName            string     `json:"deptName"`
	DoctorName          string     `json:"doctorName"`
	AdmitTime           *time.Time `json:"admitTime"`
	DischargeTime       *time.Time `json:"dischargeTime"`
	ActualDays          int        `json:"actualDays"`
	AdmitDiagnosis      string     `json:"admitDiagnosis"`
	DischargeDiagnosis  string     `json:"dischargeDiagnosis"`
	MainDiagnosisCode   string     `json:"mainDiagnosisCode"`
	OtherDiagnosisCodes string     `json:"otherDiagnosisCodes"`
	MainOperationCode   string     `json:"mainOperationCode"`
	OtherOperationCodes string     `json:"otherOperationCodes"`
	TotalCost           float64    `json:"totalCost"`
	ExtJSON             string     `json:"extJson" gorm:"column:ext_json"`
	Status              string     `json:"status" gorm:"default:DRAFT"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

func (MedFrontPage) TableName() string { return "med_record_front_page" }

// 质控规则类型
const (
	QcRequired  = "REQUIRED"
	QcLogic     = "LOGIC"
	QcCodeExist = "CODE_EXIST"
)

type QcRule struct {
	ID          int64     `json:"id" gorm:"primaryKey"`
	RuleName    string    `json:"ruleName"`
	RuleType    string    `json:"ruleType"`
	TargetField string    `json:"targetField"` // MedFrontPage 结构体字段名
	Expression  string    `json:"expression"`  // LOGIC: discharge_time>admit_time；CODE_EXIST: ICD10/ICD9CM3
	ErrorLevel  string    `json:"errorLevel" gorm:"default:ERROR"`
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	Remark      string    `json:"remark"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (QcRule) TableName() string { return "qc_rule" }

type QcResult struct {
	ID         int64      `json:"id" gorm:"primaryKey"`
	RecordID   int64      `json:"recordId"`
	RuleID     int64      `json:"ruleId"`
	FieldName  string     `json:"fieldName"`
	ErrorLevel string     `json:"errorLevel"`
	ErrorMsg   string     `json:"errorMsg"`
	Fixed      bool       `json:"fixed"`
	FixedAt    *time.Time `json:"fixedAt"`
	FixedBy    string     `json:"fixedBy"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

func (QcResult) TableName() string { return "front_page_qc_result" }

// 归档流水动作
const (
	ArRecycle = "RECYCLE"
	ArArchive = "ARCHIVE"
	ArSeal    = "SEAL"
)

type ArchiveRecord struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	RecordID     int64     `json:"recordId"`
	Action       string    `json:"action"`
	BatchNo      string    `json:"batchNo"`
	ShelfID      int64     `json:"shelfId"`
	OperatorName string    `json:"operatorName"`
	Remark       string    `json:"remark"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (ArchiveRecord) TableName() string { return "archive_record" }

type Shelf struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	ShelfCode    string    `json:"shelfCode"`
	Room         string    `json:"room"`
	LocationDesc string    `json:"locationDesc"`
	Capacity     int       `json:"capacity"`
	UsedCount    int       `json:"usedCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (Shelf) TableName() string { return "shelf" }

// 编目类型
const (
	CatalogDiagnosis = "DIAGNOSIS"
	CatalogOperation = "OPERATION"
)

type CatalogEntry struct {
	ID          int64     `json:"id" gorm:"primaryKey"`
	RecordID    int64     `json:"recordId"`
	CatalogType string    `json:"catalogType"`
	IcdCode     string    `json:"icdCode"`
	IcdName     string    `json:"icdName"`
	IsMain      bool      `json:"isMain"`
	SortOrder   int       `json:"sortOrder"`
	CoderName   string    `json:"coderName"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (CatalogEntry) TableName() string { return "catalog_entry" }

// 字典类型
const (
	Icd10   = "ICD10"
	Icd9Cm3 = "ICD9CM3"
)

type IcdDict struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	DictType  string    `json:"dictType"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	VersionID int64     `json:"versionId"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (IcdDict) TableName() string { return "icd_dict" }

type IcdDictVersion struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	Version      string    `json:"version"`
	DictType     string    `json:"dictType"`
	ImporterName string    `json:"importerName"`
	TotalCount   int       `json:"totalCount"`
	Status       string    `json:"status" gorm:"default:ACTIVE"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (IcdDictVersion) TableName() string { return "icd_dict_version" }
