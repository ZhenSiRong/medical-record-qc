package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 支持三级覆盖：代码默认值 <- config.yaml <- MRS_* 环境变量（容器部署用）
type Config struct {
	Server struct {
		Port int `yaml:"port"`
	}
	DB struct {
		DSN string `yaml:"dsn"`
	}
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	}
	JWT struct {
		Secret string `yaml:"secret"`
		Hours  int    `yaml:"hours"`
	}
	Report struct {
		OutDir string `yaml:"out_dir"`
	}
	Borrow struct {
		ApproveLevels int `yaml:"approve_levels"` // 借阅审批层级数
		BorrowDays    int `yaml:"borrow_days"`    // 默认借阅期限
		ScanMinutes   int `yaml:"scan_minutes"`   // 逾期扫描间隔
	}
}

func Load(path string) *Config {
	c := &Config{}
	c.Server.Port = 8080
	c.JWT.Hours = 72
	c.Report.OutDir = "./report-out"
	c.Borrow.ApproveLevels = 1
	c.Borrow.BorrowDays = 30
	c.Borrow.ScanMinutes = 60

	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			_ = yaml.Unmarshal(b, c)
		} else {
			fmt.Printf("[config] 未找到配置文件 %s，使用默认值+环境变量\n", path)
		}
	}

	if v := os.Getenv("MRS_PORT"); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &c.Server.Port)
	}
	if v := os.Getenv("MRS_DB_DSN"); v != "" {
		c.DB.DSN = v
	}
	if v := os.Getenv("MRS_REDIS_ADDR"); v != "" {
		c.Redis.Addr = v
	}
	if v := os.Getenv("MRS_JWT_SECRET"); v != "" {
		c.JWT.Secret = v
	}
	return c
}
