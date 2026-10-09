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

	if err := db.AutoMigrate(
		&model.SysUser{}, &model.SysRole{}, &model.SysDept{}, &model.SysDoctor{},
		&model.SysDict{}, &model.OperLog{},
		&model.MedRecord{}, &model.MedFrontPage{}, &model.QcRule{}, &model.QcResult{},
		&model.ArchiveRecord{}, &model.Shelf{}, &model.CatalogEntry{},
		&model.IcdDict{}, &model.IcdDictVersion{},
		&model.BorrowOrder{}, &model.BorrowApproval{}, &model.BorrowReminder{},
		&model.ReportBatch{}, &model.ReportItem{},
	); err != nil {
		log.Fatalf("AutoMigrate 失败: %v", err)
	}
	if err := seed.RunIfEmpty(db); err != nil {
		log.Fatalf("种子数据灌入失败: %v", err)
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
