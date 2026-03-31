package handler

import (
	"cps-go/platform/pdd"
	"fmt"
	"net/http"
	"strings"
	"time"

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

func (h *PDDHandler) QueryOrders(c *gin.Context) {
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

	var allOrders []gin.H
	var totalEstimateFee float64
	var totalActualFee float64

	for _, orderSn := range req.OrderIDs {
		orderSn = strings.TrimSpace(orderSn)
		if orderSn == "" {
			continue
		}

		rows, err := h.client.QueryOrder(orderSn)
		if err != nil {
			allOrders = append(allOrders, gin.H{
				"orderId": orderSn,
				"error":   "查询失败: " + err.Error(),
			})
			continue
		}
		if len(rows) == 0 {
			allOrders = append(allOrders, gin.H{
				"orderId": orderSn,
				"error":   "未找到该订单的推广记录，请确认是否通过推广链接下单",
			})
			continue
		}

		for _, row := range rows {
			netAmount := float64(row.PromotionAmount-row.DuoIdServiceFee) / 100
			userFee := netAmount * 0.5
			totalEstimateFee += userFee
			if row.OrderStatus == 5 {
				totalActualFee += userFee
			}

			statusText := pddOrderStatusText(row.OrderStatus)
			orderTime := ""
			if row.OrderCreateTime > 0 {
				orderTime = time.Unix(row.OrderCreateTime, 0).Format("2006-01-02 15:04:05")
			}

			allOrders = append(allOrders, gin.H{
				"orderId":         row.OrderSn,
				"skuName":         row.GoodsName,
				"price":           fmt.Sprintf("%.2f", float64(row.GoodsPrice)/100),
				"skuNum":          row.GoodsQuantity,
				"orderTime":       orderTime,
				"statusText":      statusText,
				"userEstimateFee": userFee,
				"userActualFee":   func() float64 { if row.OrderStatus == 5 { return userFee }; return 0 }(),
				"platform":        "pdd",
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"orders":           allOrders,
		"totalEstimateFee": totalEstimateFee,
		"totalActualFee":   totalActualFee,
	})
}

func pddOrderStatusText(status int) string {
	switch status {
	case 0:
		return "已付款"
	case 1:
		return "已成团"
	case 2:
		return "确认收货"
	case 3:
		return "审核成功"
	case 4:
		return "审核失败(不可提现)"
	case 5:
		return "已结算"
	case 8:
		return "非多多进宝订单"
	case 10:
		return "已处罚"
	default:
		return "其他"
	}
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
