package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 开发期全放开，前端同事可在任意端口联调；上线收紧 Origins
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// 数据权限过滤（科室级隔离）待调研口径确认后在查询层实现，
// 参考 sys_role.data_scope（ALL/DEPT），当前骨架仅做功能角色校验。
