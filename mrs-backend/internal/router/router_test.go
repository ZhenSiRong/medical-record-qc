package router

import (
	"testing"

	"github.com/gin-gonic/gin"

	"mrs/internal/cache"
	"mrs/internal/config"
)

// TestRoutesRegisterNoPanic 路由树装配冒烟：静态路由与 :param 同级冲突会让 gin
// 在注册期 panic，此测试保证路由表始终可构建（不依赖数据库与 Redis）。
func TestRoutesRegisterNoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, nil, cache.New("127.0.0.1:1", "", 0), &config.Config{})
	if len(r.Routes()) == 0 {
		t.Fatal("路由注册为空")
	}
}
