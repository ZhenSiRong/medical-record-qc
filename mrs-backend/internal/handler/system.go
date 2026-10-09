package handler

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"mrs/internal/model"
	"mrs/pkg/resp"
)

// RegisterSystem 系统管理域（调研节 10/11）：科室、医师、字典、角色、用户、架位的
// 通用 CRUD + 操作日志查询。写操作限 ADMIN，查询对所有登录用户开放（下拉框要用）。
func RegisterSystem(auth, admin *gin.RouterGroup, db *gorm.DB) {
	registerCrud[model.SysDept](auth, admin, db, "/depts", func(c *gin.Context, q *gorm.DB) *gorm.DB {
		if kw := c.Query("keyword"); kw != "" {
			q = q.Where("dept_name ILIKE ? OR dept_code ILIKE ?", like(kw), like(kw))
		}
		return q
	})
	registerCrud[model.SysDoctor](auth, admin, db, "/doctors", func(c *gin.Context, q *gorm.DB) *gorm.DB {
		if kw := c.Query("keyword"); kw != "" {
			q = q.Where("doctor_name ILIKE ? OR doctor_code ILIKE ?", like(kw), like(kw))
		}
		return q
	})
	registerCrud[model.SysDict](auth, admin, db, "/dicts", func(c *gin.Context, q *gorm.DB) *gorm.DB {
		if t := c.Query("type"); t != "" {
			q = q.Where("dict_type = ?", t)
		}
		return q
	})
	registerCrud[model.SysRole](auth, admin, db, "/roles", nil)
	registerCrud[model.Shelf](auth, admin, db, "/shelves", func(c *gin.Context, q *gorm.DB) *gorm.DB {
		if kw := c.Query("keyword"); kw != "" {
			q = q.Where("shelf_code ILIKE ? OR room ILIKE ?", like(kw), like(kw))
		}
		return q
	})
	registerCrud[model.SysUser](auth, admin, db, "/users", func(c *gin.Context, q *gorm.DB) *gorm.DB {
		if kw := c.Query("keyword"); kw != "" {
			q = q.Where("username ILIKE ? OR real_name ILIKE ?", like(kw), like(kw))
		}
		return q
	})
	admin.PUT("/users/:id/password", resetPassword(db))

	auth.GET("/oper-logs/page", func(c *gin.Context) {
		q := db.Model(&model.OperLog{})
		if m := c.Query("module"); m != "" {
			q = q.Where("module = ?", m)
		}
		resp.OK(c, resp.Page[model.OperLog](c, q))
	})
}

// registerCrud 泛型 CRUD：GET /page、POST ""、PUT /:id、DELETE /:id。
// search 为该表的查询过滤钩子（可空）；写操作走 admin 组（RequireRole ADMIN）。
// PUT 为部分更新语义（GORM Updates 结构体忽略零值字段）。
func registerCrud[T any](auth, admin *gin.RouterGroup, db *gorm.DB, path string, search func(c *gin.Context, q *gorm.DB) *gorm.DB) {
	g := auth.Group(path)
	g.GET("/page", func(c *gin.Context) {
		q := db.Model(new(T))
		if search != nil {
			q = search(c, q)
		}
		resp.OK(c, resp.Page[T](c, q))
	})
	g.GET(":id", func(c *gin.Context) {
		obj := new(T)
		if err := db.First(obj, c.Param("id")).Error; err != nil {
			resp.NotFound(c, "记录不存在")
			return
		}
		resp.OK(c, obj)
	})

	w := admin.Group(path)
	w.POST("", func(c *gin.Context) {
		obj := new(T)
		if err := c.ShouldBindJSON(obj); err != nil {
			resp.BadRequest(c, "参数错误: "+err.Error())
			return
		}
		if err := db.Create(obj).Error; err != nil {
			resp.ServerErr(c, err)
			return
		}
		resp.OK(c, obj)
	})
	w.PUT(":id", func(c *gin.Context) {
		obj := new(T)
		if err := c.ShouldBindJSON(obj); err != nil {
			resp.BadRequest(c, "参数错误: "+err.Error())
			return
		}
		res := db.Model(new(T)).Where("id = ?", c.Param("id")).Updates(obj)
		if res.Error != nil {
			resp.ServerErr(c, res.Error)
			return
		}
		if res.RowsAffected == 0 {
			resp.NotFound(c, "记录不存在")
			return
		}
		resp.OK(c, nil)
	})
	w.DELETE(":id", func(c *gin.Context) {
		if err := db.Delete(new(T), c.Param("id")).Error; err != nil {
			resp.ServerErr(c, err)
			return
		}
		resp.OK(c, nil)
	})
}

func resetPassword(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Password string `json:"password" binding:"required,min=6"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			resp.BadRequest(c, "密码至少 6 位")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			resp.ServerErr(c, err)
			return
		}
		res := db.Model(&model.SysUser{}).Where("id = ?", c.Param("id")).Update("password", string(hash))
		if res.Error != nil {
			resp.ServerErr(c, res.Error)
			return
		}
		if res.RowsAffected == 0 {
			resp.NotFound(c, "用户不存在")
			return
		}
		resp.OK(c, nil)
	}
}

func like(kw string) string { return "%" + kw + "%" }
