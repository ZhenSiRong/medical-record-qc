package middleware

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"username"`
	RoleCode string `json:"role"`
	jwt.RegisteredClaims
}

func SignToken(secret string, hours int, userID int64, username, roleCode string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(hours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "mrs",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func Auth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"code": 401, "msg": "未登录"})
			return
		}
		var claims Claims
		_, err := jwt.ParseWithClaims(strings.TrimPrefix(h, "Bearer "), &claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("非法签名算法")
			}
			return []byte(secret), nil
		})
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"code": 401, "msg": "登录已失效"})
			return
		}
		c.Set("uid", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.RoleCode)
		c.Next()
	}
}

// RequireRole 功能角色校验：业务组挂 ADMIN（归档/编目/审批/上报等病案室操作），
// 借阅申请、统计查询等对所有登录用户开放。
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		rs, _ := role.(string)
		for _, want := range roles {
			if rs == want {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "无权限执行该操作"})
	}
}

// CurrentUser 从上下文取登录人信息（JWT 中间件已注入）
func CurrentUser(c *gin.Context) (int64, string, string) {
	uid, _ := c.Get("uid")
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	id, _ := uid.(int64)
	un, _ := username.(string)
	rc, _ := role.(string)
	return id, un, rc
}
