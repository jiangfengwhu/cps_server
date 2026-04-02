package handler

import (
	"cps-go/platform/pdd"
	"fmt"
	"log"
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
		respondServerError(c, ErrInternal, err)
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
		respondServerError(c, ErrInternal, err)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *PDDHandler) TestPromotionURL(c *gin.Context) {
	goodsId := c.Query("goods_id")
	if goodsId == "" {
		respondBadRequest(c, ErrBadParam, nil)
		return
	}

	bizParams := map[string]string{
		"p_id":               h.client.Pid,
		"goods_sign_list":    `["` + goodsId + `"]`,
		"generate_short_url": "true",
	}

	raw, err := h.client.CallAPIRaw("pdd.ddk.goods.promotion.url.generate", bizParams)
	if err != nil {
		respondServerError(c, ErrInternal, err)
		return
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *PDDHandler) QueryOrders(c *gin.Context) {
	var req struct {
		OrderIDs []string `json:"orderIds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, ErrBadParam, err)
		return
	}
	if len(req.OrderIDs) == 0 {
		respondBadRequest(c, ErrBadParam, nil)
		return
	}
	if len(req.OrderIDs) > 20 {
		respondBadRequest(c, ErrBadParam, nil)
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

		row, err := h.client.QueryOrder(orderSn)
		if err != nil {
			log.Printf("[ERROR] %s: order=%s %v", ErrOrderQuery, orderSn, err)
			allOrders = append(allOrders, gin.H{
				"orderId": orderSn,
				"error":   errMsg(ErrOrderQuery),
			})
			continue
		}
		if row == nil {
			allOrders = append(allOrders, gin.H{
				"orderId": orderSn,
				"error":   errMsg(ErrOrderNotFound),
			})
			continue
		}

		serviceFee := row.GetDuoIdServiceFee()
		netAmount := float64(row.PromotionAmount-serviceFee) / 100
		userFee := netAmount * 0.5
		totalEstimateFee += userFee
		if row.OrderStatus == 5 {
			totalActualFee += userFee
		}

		orderTime := ""
		if row.OrderCreateTime > 0 {
			orderTime = time.Unix(row.OrderCreateTime, 0).Format("2006-01-02 15:04:05")
		}

		orderItem := gin.H{
			"orderId":         row.OrderSn,
			"skuName":         row.GoodsName,
			"imgUrl":          row.GoodsThumbnailURL,
			"price":           fmt.Sprintf("%.2f", float64(row.GoodsPrice)/100),
			"orderAmount":     fmt.Sprintf("%.2f", float64(row.OrderAmount)/100),
			"skuNum":          row.GoodsQuantity,
			"orderTime":       orderTime,
			"statusText":      row.OrderStatusDesc,
			"userEstimateFee": userFee,
			"userActualFee": func() float64 {
				if row.OrderStatus == 5 {
					return userFee
				}
				return 0
			}(),
			"platform":     "pdd",
			"mallName":     row.MallName,
			"categoryName": row.GoodsCategoryName,
		}
		if row.FailReason != "" {
			orderItem["failReason"] = row.FailReason
		}
		allOrders = append(allOrders, orderItem)
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
		respondBadRequest(c, ErrBadParam, err)
		return
	}

	inputURL := strings.TrimSpace(req.URL)
	if inputURL == "" {
		respondBadRequest(c, ErrBadParam, nil)
		return
	}

	result, err := h.client.ConvertURL(inputURL)
	if err != nil {
		respondServerError(c, ErrConvert, err)
		return
	}

	response := gin.H{
		"hasCommission": result.HasCommission,
	}

	if detail := result.GoodsDetail; detail != nil && detail.GoodsName != "" {
		price := float64(detail.MinGroupPrice) / 100
		commissionRate := float64(detail.PromotionRate) / 10
		product := gin.H{
			"name":           detail.GoodsName,
			"imgUrl":         detail.GoodsImageURL,
			"price":          fmt.Sprintf("%.2f", price),
			"shopName":       detail.MallName,
			"shopType":       "拼多多",
			"commissionRate": commissionRate,
		}
		if detail.HasCoupon && detail.CouponDiscount > 0 {
			product["coupon"] = fmt.Sprintf("%.2f", float64(detail.CouponDiscount)/100)
		}
		response["product"] = product
	}

	if result.HasCommission && result.Promotion != nil {
		response["clickUrl"] = result.Promotion.URL
		response["schemaUrl"] = result.Promotion.SchemaURL
	}

	if len(result.Recommendations) > 0 {
		var recs []gin.H
		for _, r := range result.Recommendations {
			rec := gin.H{
				"goodsSign":      r.GoodsSign,
				"name":           r.GoodsName,
				"imgUrl":         r.GoodsImageURL,
				"price":          fmt.Sprintf("%.2f", float64(r.MinGroupPrice)/100),
				"commissionRate": float64(r.PromotionRate) / 10,
				"shopName":       r.MallName,
				"salesTip":       r.SalesTip,
			}
			if r.GoodsThumbnail != "" {
				rec["imgUrl"] = r.GoodsThumbnail
			}
			if r.HasCoupon && r.CouponDiscount > 0 {
				rec["coupon"] = fmt.Sprintf("%.2f", float64(r.CouponDiscount)/100)
			}
			recs = append(recs, rec)
		}
		response["recommendations"] = recs
	}

	c.JSON(http.StatusOK, response)
}

func (h *PDDHandler) Promote(c *gin.Context) {
	var req struct {
		GoodsSign string `json:"goodsSign" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, ErrBadParam, err)
		return
	}

	schemaURL, err := h.client.GenerateSchemaURL(req.GoodsSign)
	if err != nil {
		respondServerError(c, ErrConvert, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"schemaUrl": schemaURL,
	})
}
