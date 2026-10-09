package handler

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"mrs/internal/config"
	"mrs/internal/middleware"
	"mrs/internal/model"
	"mrs/pkg/resp"
)

type AuthHandler struct {
	DB  *gorm.DB
	CFG *config.Config
}

// Login 登录，签发 JWT。骨架阶段账号由种子数据初始化：admin/admin123（病案管理员）、
// deptuser/123456（科室用户）。TODO 上线前接入统一身份或强制改密策略。
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, "请输入用户名和密码")
		return
	}
	var u model.SysUser
	if err := h.DB.Where("username = ? AND enabled = ?", req.Username, true).First(&u).Error; err != nil {
		resp.Unauthorized(c, "用户名或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
		resp.Unauthorized(c, "用户名或密码错误")
		return
	}
	token, err := middleware.SignToken(h.CFG.JWT.Secret, h.CFG.JWT.Hours, u.ID, u.Username, u.RoleCode)
	if err != nil {
		resp.ServerErr(c, err)
		return
	}
	resp.OK(c, gin.H{"token": token, "user": u})
}

func (h *AuthHandler) Me(c *gin.Context) {
	uid, _, _ := middleware.CurrentUser(c)
	var u model.SysUser
	if err := h.DB.First(&u, uid).Error; err != nil {
		resp.NotFound(c, "用户不存在")
		return
	}
	resp.OK(c, u)
}
