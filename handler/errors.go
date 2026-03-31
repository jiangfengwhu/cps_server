package handler

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	ErrBadParam     = "E1001" // 参数错误
	ErrConvert      = "E2001" // 转链失败
	ErrOrderQuery   = "E2002" // 订单查询失败
	ErrOrderNotFound = "E2003" // 订单未找到
	ErrInternal     = "E5000" // 内部错误
)

var errMessages = map[string]string{
	ErrBadParam:     "参数错误",
	ErrConvert:      "生成推广链接失败，请稍后重试",
	ErrOrderQuery:   "查询失败，请稍后重试",
	ErrOrderNotFound: "未找到推广记录",
	ErrInternal:     "服务异常，请稍后重试",
}

func errMsg(code string) string {
	if m, ok := errMessages[code]; ok {
		return m
	}
	return "未知错误"
}

func respondError(c *gin.Context, httpStatus int, code string, internalErr error) {
	if internalErr != nil {
		log.Printf("[ERROR] %s: %v", code, internalErr)
	}
	c.JSON(httpStatus, gin.H{"error": errMsg(code), "code": code})
}

func respondBadRequest(c *gin.Context, code string, internalErr error) {
	respondError(c, http.StatusBadRequest, code, internalErr)
}

func respondServerError(c *gin.Context, code string, internalErr error) {
	respondError(c, http.StatusInternalServerError, code, internalErr)
}
