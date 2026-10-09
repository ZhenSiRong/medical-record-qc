// createdb —— 便携 PG 环境辅助：目标库不存在时创建（zonky 精简版无 createdb 客户端）
package main

import (
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "drop" {
		drop()
		return
	}
	create()
}

func drop() {
	db := open()
	if err := db.Exec(`DROP DATABASE IF EXISTS mrs WITH (FORCE)`).Error; err != nil {
		fmt.Println("删库失败:", err)
		os.Exit(1)
	}
	fmt.Println("数据库 mrs 已删除")
}

func create() {
	db := open()
	var cnt int64
	db.Raw("SELECT count(*) FROM pg_database WHERE datname = 'mrs'").Scan(&cnt)
	if cnt > 0 {
		fmt.Println("数据库 mrs 已存在")
		return
	}
	if err := db.Exec(`CREATE DATABASE mrs OWNER mrs`).Error; err != nil {
		fmt.Println("建库失败:", err)
		os.Exit(1)
	}
	fmt.Println("数据库 mrs 已创建")
}

func open() *gorm.DB {
	dsn := os.Getenv("MRS_DB_DSN")
	if dsn == "" {
		dsn = "host=127.0.0.1 user=mrs password=mrs123456 dbname=postgres port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Println("连接失败:", err)
		os.Exit(1)
	}
	return db
}
