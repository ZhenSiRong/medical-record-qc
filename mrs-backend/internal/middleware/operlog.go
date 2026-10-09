package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"mrs/internal/model"
)

// OperLog 写操作审计（调研节 10）：POST/PUT/DELETE 请求异步落 oper_log
func OperLog(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		switch c.Request.Method {
		case "POST", "PUT", "DELETE":
		default:
			return
		}

		entry := model.OperLog{
			Module:     pathModule(c.FullPath()),
			Operation:  c.Request.Method + " " + c.FullPath(),
			IP:         c.ClientIP(),
			StatusCode: c.Writer.Status(),
			CostMs:     time.Since(start).Milliseconds(),
		}
		if _, username, _ := CurrentUser(c); username != "" {
			entry.OperatorName = username
		}

		go func() {
			defer func() { _ = recover() }()
			if err := db.Create(&entry).Error; err != nil {
				log.Printf("[operlog] 写入失败: %v", err)
			}
		}()
	}
}

// pathModule 取 /api/xxx 的 xxx 作为模块名
func pathModule(p string) string {
	if len(p) > 5 && p[:5] == "/api/" {
		rest := p[5:]
		for i := 0; i < len(rest); i++ {
			if rest[i] == '/' {
				return rest[:i]
			}
		}
		return rest
	}
	return p
}
