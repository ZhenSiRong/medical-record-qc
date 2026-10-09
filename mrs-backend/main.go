package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"mrs/internal/cache"
	"mrs/internal/config"
	"mrs/internal/handler"
	"mrs/internal/model"
	"mrs/internal/router"
	"mrs/internal/seed"
	"mrs/internal/service"
)

// mrs 病案管理系统后端（骨架）
// 启动流程：加载配置 → 连接 PG → AutoMigrate → 空库灌种子 → Redis 缓存 → 路由 → 逾期扫描器
func main() {
	cfgPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg := config.Load(*cfgPath)

	gin.SetMode(gin.ReleaseMode)
	db, err := gorm.Open(postgres.Open(cfg.DB.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	// 注：MedFrontPage 已在下表注册，GORM AutoMigrate 会自动为 med_record_front_page
	// 补加模型上新增的列，无需手写 SQL。D3（2026-10-09）新增列即由此覆盖：
	// external_cause_code、external_cause_name、newborn_birth_weight、newborn_gestational_weeks。
	if err := db.AutoMigrate(
		&model.SysUser{}, &model.SysRole{}, &model.SysDept{}, &model.SysDoctor{},
		&model.SysDict{}, &model.OperLog{},
		&model.MedRecord{}, &model.MedFrontPage{}, &model.QcRule{}, &model.QcResult{},
		&model.ArchiveRecord{}, &model.Shelf{}, &model.CatalogEntry{},
		&model.IcdDict{}, &model.IcdDictVersion{},
		&model.BorrowOrder{}, &model.BorrowApproval{}, &model.BorrowReminder{},
		&model.ReportBatch{}, &model.ReportItem{},
		// 扩展功能新增表（2026-10-09）：质量评分、变更留痕、预编码规则、DRG 分组规则
		&model.FrontPageQuality{}, &model.FrontPageRevision{},
		&model.CodeSuggestionRule{}, &model.DrgGroupRule{},
	); err != nil {
		log.Fatalf("AutoMigrate 失败: %v", err)
	}
	if err := seed.RunIfEmpty(db); err != nil {
		log.Fatalf("种子数据灌入失败: %v", err)
	}
	// 扩展功能种子（幂等：已存在则跳过）
	if err := seed.SeedIcdExt(db); err != nil {
		log.Printf("[seed] ICD 编码库补充失败(不影响启动): %v", err)
	}
	if err := handler.SeedQcExt(db); err != nil {
		log.Printf("[seed] 质控扩展种子失败(不影响启动): %v", err)
	}
	if err := handler.SeedBranchExt(db); err != nil {
		log.Printf("[seed] 扩展功能种子失败(不影响启动): %v", err)
	}
	// 科室医师（deptuser）演示数据：让该账号在演示库里有可测的病案（幂等）
	if err := seed.SeedDeptUserDemoData(db); err != nil {
		log.Printf("[seed] 科室医师演示数据失败(不影响启动): %v", err)
	}

	ca := cache.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err := os.MkdirAll(cfg.Report.OutDir, 0o755); err != nil {
		log.Printf("创建上报目录失败(不影响启动): %v", err)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	router.Register(r, db, ca, cfg)

	service.StartOverdueScanner(db, cfg.Borrow.ScanMinutes)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("mrs backend 已启动 %s （账号 admin/admin123 或 deptuser/123456）", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
