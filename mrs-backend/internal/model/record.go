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

	// ---------- 条件必填真字段（缺口 D3，2026-10-09）----------
	// 依据《住院病案首页数据采集质量与接口标准（2020版）》：损伤中毒（主诊断 S/T 编码）时"外部原因"
	// 为条件必填；新生儿（P 编码）时"出生体重/孕周"为条件必填。此前这三项无独立列，
	// SeedQcExt 的条件必填规则用 other_diagnosis_codes / ext_json 顶替，规则语义与字段不一致。
	// 说明：规范中"外部原因编码"与"外部原因名称"成对出现，故按编码+名称两个字段落库；
	// 条件必填规则的校验目标是 ExternalCauseCode（编码是实质校验项，名称为随附描述）。
	ExternalCauseCode string `json:"externalCauseCode"` // 损伤中毒外部原因编码（ICD-10 第二十章 V01–Y98）
	ExternalCauseName string `json:"externalCauseName"` // 损伤中毒外部原因名称（与编码配对）
	// NewbornBirthWeight 新生儿出生体重（克）、NewbornGestationalWeeks 新生儿孕周（周）。
	// 设计取舍：用 *int 而非裸 int —— 这两项对绝大多数非新生儿首页根本不存在，
	// 裸 int 的零值 0 与"未采集"在 JSON（0 vs null）、变更留痕快照与前端表单里无法区分；
	// 用指针可表达三态：nil=未采集/不适用，指向 0=填了 0（可疑值），指向正值=已采集。
	//
	// 与质控引擎 isBlank 的配合（service/qcengine.go，已 read 确认）：isBlank 对 reflect.Ptr
	// 先判 IsNil（nil→空），非 nil 则递归判断指向值，故 *int 指向 0 时也被判为"空"。
	// 本字段语义下这是可接受的取舍：出生体重 0 克 / 孕周 0 周均非合法临床取值，
	// 判为"未填/无效"同样应当命中"必填字段为空"，宁可多报不漏报；代价是报错文案统一为
	// "必填字段为空"而非"取值无效"。不改 isBlank 是因为它是全部 REQUIRED 规则的公共判定，
	// 为个别字段调整会波及既有规则（回归风险大于收益）。
	NewbornBirthWeight      *int `json:"newbornBirthWeight"`      // 新生儿出生体重（克）
	NewbornGestationalWeeks *int `json:"newbornGestationalWeeks"` // 新生儿孕周（周）

	Status    string    `json:"status" gorm:"default:DRAFT"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (MedFrontPage) TableName() string { return "med_record_front_page" }

// 质控规则类型
const (
	QcRequired  = "REQUIRED"
	QcLogic     = "LOGIC"
	QcCodeExist = "CODE_EXIST"
	// 以下两类为扩展规则（任务6，DRG/DIP 付费改革下的病案首页质控）：
	QcDiagOpMatch  = "DIAG_OP_MATCH"  // 主诊断-主要手术匹配校验，防落 DRG 歧义组(QY)
	QcMainDiagRule = "MAIN_DIAG_RULE" // 主要诊断选择合理性提示（WARN 级，不拦截）
)

type QcRule struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	RuleName    string `json:"ruleName"`
	RuleType    string `json:"ruleType"`
	TargetField string `json:"targetField"` // MedFrontPage 结构体字段名
	Expression  string `json:"expression"`  // LOGIC: discharge_time>admit_time；CODE_EXIST: ICD10/ICD9CM3
	// ConditionExpr 条件表达式（任务4）：仅当条件成立时才执行本规则（用于"条件必填"）。
	// 支持两种轻量写法：① 编码前缀匹配 `main_diagnosis_code LIKE 'S%'`；
	// ② 复用 checkLogic 的表达式，如 `actual_days>0`、`discharge_time>admit_time`。
	// 依据《住院病案首页数据采集质量与接口标准（2020版）》条件必填项要求。
	ConditionExpr string    `json:"conditionExpr"`
	ErrorLevel    string    `json:"errorLevel" gorm:"default:ERROR"`
	Enabled       bool      `json:"enabled" gorm:"default:true"`
	Remark        string    `json:"remark"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (QcRule) TableName() string { return "qc_rule" }

// 质控结果处理状态机（任务5：质控退回返修闭环）
// OPEN 待处理 → RETURNED 已退回临床 → RECTIFIED 已整改待复核
// → CLOSED 复核通过（闭环完成）/ REJECTED_BY_QC 复核不通过退回临床
const (
	HandleOpen         = "OPEN"
	HandleReturned     = "RETURNED"
	HandleRectified    = "RECTIFIED"
	HandleClosed       = "CLOSED"
	HandleRejectedByQc = "REJECTED_BY_QC"
)

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
	// 退回返修闭环字段（任务5）：操作痕迹（操作人/时间/意见）直接落在本表，避免额外关联查询。
	HandleStatus  string     `json:"handleStatus" gorm:"default:OPEN"`
	ReturnOpinion string     `json:"returnOpinion"` // 退回意见（病案室→临床）
	ReturnedBy    string     `json:"returnedBy"`
	ReturnedAt    *time.Time `json:"returnedAt"`
	RectifiedBy   string     `json:"rectifiedBy"` // 临床整改人
	RectifiedAt   *time.Time `json:"rectifiedAt"`
	ReviewOpinion string     `json:"reviewOpinion"` // 复核意见（病案室）
	ReviewedBy    string     `json:"reviewedBy"`
	ReviewedAt    *time.Time `json:"reviewedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
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
