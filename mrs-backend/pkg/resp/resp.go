package resp

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// R 全部接口的统一响应外壳：{code, msg, data}，code=0 成功，非 0 时与 HTTP 状态码一致
type R struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, R{Code: 0, Msg: "ok", Data: data})
}

func Fail(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, R{Code: httpCode, Msg: msg})
}

func BadRequest(c *gin.Context, msg string)   { Fail(c, http.StatusBadRequest, msg) }
func Unauthorized(c *gin.Context, msg string) { Fail(c, http.StatusUnauthorized, msg) }
func Forbidden(c *gin.Context, msg string)    { Fail(c, http.StatusForbidden, msg) }
func NotFound(c *gin.Context, msg string)     { Fail(c, http.StatusNotFound, msg) }

// Conflict 状态机冲突等业务不可执行场景
func Conflict(c *gin.Context, msg string) { Fail(c, http.StatusConflict, msg) }

func ServerErr(c *gin.Context, err error) {
	log.Printf("[err] %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
	Fail(c, http.StatusInternalServerError, "服务器内部错误")
}

// PageQuery 分页参数
type PageQuery struct {
	Page int
	Size int
}

func ParsePage(c *gin.Context) PageQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	return PageQuery{Page: page, Size: size}
}

type PageResult[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}

// Page 对调用方已挂好 Model/Where 的查询做 count + 分页 find，按 id 倒序
func Page[T any](c *gin.Context, q *gorm.DB) PageResult[T] {
	var total int64
	_ = q.Count(&total).Error
	pg := ParsePage(c)
	var list []T
	_ = q.Order("id DESC").Offset((pg.Page - 1) * pg.Size).Limit(pg.Size).Find(&list).Error
	return PageResult[T]{List: list, Total: total}
}
