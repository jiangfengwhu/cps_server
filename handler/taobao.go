package handler

import (
	"cps-go/platform/taobao"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type TaobaoHandler struct {
	client *taobao.Client
}

func NewTaobaoHandler(client *taobao.Client) *TaobaoHandler {
	return &TaobaoHandler{client: client}
}

func (h *TaobaoHandler) TestRecommend(c *gin.Context) {
	itemId := c.Query("item_id")
	if itemId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供item_id参数"})
		return
	}

	bizParams := map[string]string{
		"adzone_id":   h.client.AdzoneId,
		"material_id": "13256",
		"item_id":     itemId,
		"page_size":   "1",
		"page_no":     "1",
	}

	raw, err := h.client.CallAPIRaw("taobao.tbk.dg.material.recommend", bizParams)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *TaobaoHandler) ConvertLink(c *gin.Context) {
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

	resolvedURL, resolved := taobao.ResolveURL(inputURL)
	urlForParsing := inputURL
	if resolved {
		urlForParsing = resolvedURL
	}

	itemId, err := taobao.ParseItemId(urlForParsing)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法识别商品ID: " + err.Error()})
		return
	}

	item, err := h.client.MaterialRecommend(itemId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取推广信息失败: " + err.Error()})
		return
	}

	clickURL := item.CouponClickURL
	if clickURL == "" {
		clickURL = item.ClickURL
	}

	response := gin.H{
		"shortUrl": clickURL,
		"clickUrl": clickURL,
		"tpwd":     item.Tpwd,
	}

	if item.Title != "" {
		shopType := "淘宝"
		if item.UserType == 1 {
			shopType = "天猫"
		}
		response["product"] = gin.H{
			"name":     item.Title,
			"imgUrl":   item.PictURL,
			"price":    item.ZkFinalPrice,
			"shopName": item.ShopTitle,
			"shopType": shopType,
		}
	}

	c.JSON(http.StatusOK, response)
}
