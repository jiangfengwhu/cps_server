package handler

import (
	"cps-go/platform/taobao"
	"fmt"
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

func (h *TaobaoHandler) QueryOrders(c *gin.Context) {
	var req struct {
		OrderIDs []string `json:"orderIds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供订单号"})
		return
	}
	if len(req.OrderIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订单号不能为空"})
		return
	}
	if len(req.OrderIDs) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "最多支持同时查询20个订单"})
		return
	}

	orders, err := h.client.QueryOrders(req.OrderIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}

	var allOrders []gin.H
	var totalEstimateFee float64
	var totalActualFee float64

	if len(orders) == 0 {
		for _, id := range req.OrderIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			allOrders = append(allOrders, gin.H{
				"orderId": id,
				"error":   "未找到该订单的推广记录，请确认是否通过推广链接下单",
			})
		}
	} else {
		for _, row := range orders {
			estimateFee := parseFloat(row.PubSharePreFee) * 0.5
			actualFee := parseFloat(row.PubShareFee) * 0.5
			totalEstimateFee += estimateFee
			totalActualFee += actualFee

			allOrders = append(allOrders, gin.H{
				"orderId":         row.TradeParentId,
				"skuName":         row.ItemTitle,
				"price":           row.AlipayTotalPrice,
				"skuNum":          row.ItemNum,
				"orderTime":       row.TkCreateTime,
				"statusText":      taobao.TkStatusText(row.TkStatus),
				"userEstimateFee": estimateFee,
				"userActualFee":   actualFee,
				"platform":        "taobao",
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"orders":           allOrders,
		"totalEstimateFee": totalEstimateFee,
		"totalActualFee":   totalActualFee,
	})
}

func parseFloat(s string) float64 {
	f := 0.0
	fmt.Sscanf(s, "%f", &f)
	return f
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
