package handler

import (
	"cps-go/platform/pdd"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type PDDHandler struct {
	client *pdd.Client
}

func NewPDDHandler(client *pdd.Client) *PDDHandler {
	return &PDDHandler{client: client}
}

func (h *PDDHandler) CheckAuthority(c *gin.Context) {
	bizParams := map[string]string{
		"pid": h.client.Pid,
	}
	raw, err := h.client.CallAPIRaw("pdd.ddk.member.authority.query", bizParams)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *PDDHandler) GenerateAuthorityURL(c *gin.Context) {
	bizParams := map[string]string{
		"p_id":                   h.client.Pid,
		"goods_sign_list":        `["c9r2omogKFFAc7WBwvbZU1ikIb16_J3CTa8HNN"]`,
		"generate_authority_url": "true",
		"generate_short_url":     "true",
	}
	raw, err := h.client.CallAPIRaw("pdd.ddk.goods.promotion.url.generate", bizParams)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *PDDHandler) TestPromotionURL(c *gin.Context) {
	goodsId := c.Query("goods_id")
	if goodsId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供goods_id参数"})
		return
	}

	bizParams := map[string]string{
		"p_id":               h.client.Pid,
		"goods_sign_list":    `["` + goodsId + `"]`,
		"generate_short_url": "true",
	}

	raw, err := h.client.CallAPIRaw("pdd.ddk.goods.promotion.url.generate", bizParams)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *PDDHandler) ConvertLink(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供商品链接"})
		return
	}

	inputURL := strings.TrimSpace(req.URL)
	if inputURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "商品链接不能为空"})
		return
	}

	promoURL, err := h.client.ConvertURL(inputURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成推广链接失败: " + err.Error()})
		return
	}

	shortURL := promoURL.MobileShortURL
	if shortURL == "" {
		shortURL = promoURL.ShortURL
	}
	clickURL := promoURL.MobileURL
	if clickURL == "" {
		clickURL = promoURL.URL
	}

	c.JSON(http.StatusOK, gin.H{
		"shortUrl": shortURL,
		"clickUrl": clickURL,
	})
}
