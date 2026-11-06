package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/cache"
	"mrs/internal/config"
	"mrs/internal/handler"
	"mrs/internal/middleware"
	"mrs/internal/service"
	"mrs/pkg/resp"
)

// Register 全部路由。权限模型（骨架）：
//   - 公开：/api/auth/login、/healthz
//   - 登录用户：所有查询、借阅申请、统计查看
//   - ADMIN（病案室）：登记/归档/编目/审批/借出归还/首页质控/上报/系统管理写操作
func Register(r *gin.Engine, db *gorm.DB, ca *cache.Cache, cfg *config.Config) {
	r.Use(middleware.CORS())
	r.GET("/healthz", func(c *gin.Context) { resp.OK(c, gin.H{"status": "up"}) })

	api := r.Group("/api")
	api.Use(middleware.OperLog(db))

	authH := &handler.AuthHandler{DB: db, CFG: cfg}
	api.POST("/auth/login", authH.Login)

	auth := api.Group("")
	auth.Use(middleware.Auth(cfg.JWT.Secret))
	auth.GET("/auth/me", authH.Me)

	admin := auth.Group("")
	admin.Use(middleware.RequireRole("ADMIN"))

	// 系统管理域（节 10/11）：depts/doctors/dicts/roles/users/shelves/oper-logs
	handler.RegisterSystem(auth, admin, db)

	// 病案主档
	recH := &handler.RecordHandler{DB: db}
	auth.GET("/records/page", recH.Page)
	auth.GET("/records/:id", recH.Detail)
	admin.POST("/records", recH.Create)
	admin.PUT("/records/:id", recH.Update)

	// 归档（节 3）
	archH := &handler.ArchiveHandler{DB: db}
	admin.POST("/archives/recycle", archH.Recycle)
	admin.POST("/archives/shelve", archH.Shelve)
	admin.POST("/archives/seal/:id", archH.Seal)
	auth.GET("/archives/records/:id/flow", archH.Flow)

	// 编目（节 4）
	catH := &handler.CatalogHandler{DB: db}
	auth.GET("/catalogs", catH.List)
	admin.POST("/catalogs", catH.Create)
	admin.DELETE("/catalogs/:id", catH.Delete)

	icdH := &handler.IcdHandler{DB: db, Ca: ca}
	auth.GET("/icd-dicts/search", icdH.Search)
	auth.GET("/icd-dicts/page", icdH.Page)
	admin.POST("/icd-dicts/import", icdH.Import)

	// 借阅（节 5）
	borH := &handler.BorrowHandler{DB: db, CFG: cfg}
	auth.POST("/borrows", borH.Apply)
	auth.PUT("/borrows/:id/renew", borH.Renew)
	admin.PUT("/borrows/:id/approve", borH.Approve)
	admin.PUT("/borrows/:id/lend", borH.Lend)
	admin.PUT("/borrows/:id/return", borH.Return)

	// 借阅台账（节 6）
	ledH := &handler.LedgerHandler{DB: db}
	auth.GET("/borrow-ledger/page", ledH.Page)
	auth.GET("/borrow-ledger/:id/approvals", ledH.Approvals)
	admin.POST("/borrow-ledger/:id/remind", ledH.Remind)

	// 病案首页与质控（节 8）
	fpH := &handler.FrontPageHandler{DB: db}
	auth.GET("/front-pages/page", fpH.Page)
	auth.GET("/front-pages/:recordId", fpH.Get)
	admin.PUT("/front-pages/:recordId", fpH.Save)
	admin.POST("/front-pages/:recordId/qc", fpH.Qc)
	admin.PUT("/qc-results/:id/fixed", fpH.MarkFixed)

	// 首页上报（节 9）
	repH := &handler.ReportHandler{DB: db, Client: &service.LocalStubReportClient{OutDir: cfg.Report.OutDir}}
	auth.GET("/report-batches/page", repH.Page)
	auth.GET("/report-batches/:id/items", repH.Items)
	admin.POST("/report-batches", repH.Create)
	admin.POST("/report-batches/:id/submit", repH.Submit)

	// 统计（节 7）
	stH := &handler.StatsHandler{DB: db, Ca: ca}
	auth.GET("/stats/overview", stH.Overview)
	auth.GET("/stats/archive-rate", stH.ArchiveRate)
	auth.GET("/stats/disease-top", stH.DiseaseTop)
	auth.GET("/stats/borrow-monthly", stH.BorrowMonthly)
	auth.POST("/stats/export", stH.Export)

	// ===== 扩展功能（2026-10-09 新增，分两路并行实现后在此统一注册）=====
	// 路 1 · 质控增强与闭环：条件必填、主诊断选择规则、主诊断-主手术匹配、
	//        病历质量评分（甲乙丙）、质控退回返修闭环、归档锁定与变更留痕
	handler.RegisterQcExt(auth, admin, db)
	// 路 2 · 智能化与统计扩展：智能预编码推荐、DRG/DIP 预检、上报一致性核对、
	//        归档及时率与超期催缴、上报 CSV 导出（符合国家接口标准骨架字段）
	handler.RegisterBranchExt(auth, admin, db)

	// ADR-0002 批次 D：CC/MCC 严重度判读出口（响应体必带 disclaimer —— ADR §4 强约束，非注释）
	ccH := handler.NewCcMccHandler(db)
	auth.POST("/ccmcc-preview", ccH.Preview)
}
