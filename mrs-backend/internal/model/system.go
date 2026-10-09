package model

import "time"

// ---------- 系统域 ----------

type SysUser struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"size:32"`
	Password  string    `json:"-" gorm:"size:128"` // bcrypt，永不出网
	RealName  string    `json:"realName"`
	DeptID    int64     `json:"deptId"`
	DeptName  string    `json:"deptName"`
	Phone     string    `json:"phone"`
	RoleCode  string    `json:"roleCode" gorm:"default:DEPT_USER"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (SysUser) TableName() string { return "sys_user" }

type SysRole struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	RoleCode  string    `json:"roleCode"`
	RoleName  string    `json:"roleName"`
	DataScope string    `json:"dataScope" gorm:"default:DEPT"` // ALL全院 / DEPT本科室
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (SysRole) TableName() string { return "sys_role" }

type SysDept struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	DeptCode  string    `json:"deptCode"`
	DeptName  string    `json:"deptName"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (SysDept) TableName() string { return "sys_dept" }

type SysDoctor struct {
	ID         int64     `json:"id" gorm:"primaryKey"`
	DoctorCode string    `json:"doctorCode"`
	DoctorName string    `json:"doctorName"`
	DeptID     int64     `json:"deptId"`
	DeptName   string    `json:"deptName"`
	Title      string    `json:"title"`
	Enabled    bool      `json:"enabled" gorm:"default:true"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (SysDoctor) TableName() string { return "sys_doctor" }

type SysDict struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	DictType  string    `json:"dictType"`
	DictKey   string    `json:"dictKey"`
	DictValue string    `json:"dictValue"`
	SortOrder int       `json:"sortOrder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (SysDict) TableName() string { return "sys_dict" }

// OperLog 操作日志审计（调研节 10 合规要求），由中间件自动记录写操作
type OperLog struct {
	ID           int64     `json:"id" gorm:"primaryKey"`
	Module       string    `json:"module"`
	Operation    string    `json:"operation"`
	OperatorName string    `json:"operatorName"`
	IP           string    `json:"ip"`
	StatusCode   int       `json:"statusCode"`
	CostMs       int64     `json:"costMs"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (OperLog) TableName() string { return "oper_log" }
