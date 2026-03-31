package handler

import (
	"cps-go/platform/jd"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type JDHandler struct {
	client *jd.Client
}

func NewJDHandler(client *jd.Client) *JDHandler {
	return &JDHandler{client: client}
}

func (h *JDHandler) TestGoodsInfo(c *gin.Context) {
	skuId := c.Query("skuId")
	if skuId == "" {
		skuId = "10198292194581"
	}

	skuIdNum, err := strconv.ParseInt(skuId, 10, 64)
	if err != nil {
		respondBadRequest(c, ErrBadParam, err)
		return
	}

	params := map[string]interface{}{
		"goodsReq": map[string]interface{}{
			"skuIds":  []int64{skuIdNum},
			"fields":  []string{"categoryInfo", "imageInfo"},
			"sceneId": 2,
		},
	}

	raw, err := h.client.CallAPIRaw("jd.union.open.goods.bigfield.query", params)
	if err != nil {
		respondServerError(c, ErrInternal, err)
		return
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *JDHandler) ConvertLink(c *gin.Context) {
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

	// Try to resolve short links and extract skuId (non-fatal)
	resolvedURL, resolved := jd.ResolveURL(inputURL)
	urlForParsing := inputURL
	if resolved {
		urlForParsing = resolvedURL
	}
	skuId, _ := jd.ParseSkuID(urlForParsing)

	type promoResult struct {
		data *jd.PromotionResult
		err  error
	}
	type goodsResult struct {
		data []jd.GoodsInfo
		err  error
	}

	promoCh := make(chan promoResult, 1)
	goodsCh := make(chan goodsResult, 1)

	// Always try the promotion link API with the original URL
	go func() {
		data, err := h.client.GetPromotionLink(inputURL)
		promoCh <- promoResult{data, err}
	}()

	// Only query goods info if we managed to extract a skuId
	go func() {
		if skuId == "" {
			goodsCh <- goodsResult{nil, nil}
			return
		}
		data, err := h.client.GetGoodsInfo(skuId)
		goodsCh <- goodsResult{data, err}
	}()

	pr := <-promoCh
	gr := <-goodsCh

	if pr.err != nil {
		respondServerError(c, ErrConvert, pr.err)
		return
	}

	jCommand := pr.data.JCommand
	if jCommand == "" {
		jCommand = pr.data.JShortCommand
	}

	response := gin.H{
		"shortUrl": pr.data.ShortURL,
		"clickUrl": pr.data.ClickURL,
		"jCommand": jCommand,
	}

	if gr.err == nil && gr.data != nil && len(gr.data) > 0 {
		goods := gr.data[0]

		imgUrl := ""
		if goods.ImageInfo != nil && len(goods.ImageInfo.ImageList) > 0 {
			imgUrl = goods.ImageInfo.ImageList[0].URL
		}

		product := gin.H{
			"skuId":    goods.SkuID,
			"name":     goods.SkuName,
			"imgUrl":   jd.NormalizeImageURL(imgUrl),
			"isJdSale": goods.Owner == "g",
		}

		if goods.CategoryInfo != nil {
			product["category"] = goods.CategoryInfo.Cid1Name
		}

		response["product"] = product
	}

	c.JSON(http.StatusOK, response)
}

func (h *JDHandler) QueryOrders(c *gin.Context) {
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

	for _, idStr := range req.OrderIDs {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}
		orderId, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			allOrders = append(allOrders, gin.H{
				"orderId": idStr,
				"error":   errMsg(ErrBadParam),
			})
			continue
		}

		rows, err := h.client.QueryOrderRows(orderId)
		if err != nil {
			log.Printf("[ERROR] %s: order=%d %v", ErrOrderQuery, orderId, err)
			allOrders = append(allOrders, gin.H{
				"orderId": idStr,
				"error":   errMsg(ErrOrderQuery),
			})
			continue
		}

		if len(rows) == 0 {
			allOrders = append(allOrders, gin.H{
				"orderId": idStr,
				"error":   errMsg(ErrOrderNotFound),
			})
			continue
		}

		for _, row := range rows {
			statusText := getOrderStatusText(row.ValidCode)

			userEstimateFee := roundFloat(row.EstimateFee*0.5, 2)
			userActualFee := roundFloat(row.ActualFee*0.5, 2)

			totalEstimateFee += userEstimateFee
			totalActualFee += userActualFee

			withdrawable := false
			withdrawableTime := ""
			if row.ValidCode == 17 && row.FinishTime != "" {
				finishTime, err := time.Parse("2006-01-02 15:04:05", row.FinishTime)
				if err == nil {
					withdrawDate := finishTime.AddDate(0, 0, 7)
					withdrawableTime = withdrawDate.Format("2006-01-02")
					withdrawable = time.Now().After(withdrawDate)
				}
			}

			allOrders = append(allOrders, gin.H{
				"orderId":          row.OrderID,
				"skuId":            row.SkuID,
				"skuName":          row.SkuName,
				"price":            row.Price,
				"skuNum":           row.SkuNum,
				"orderTime":        row.OrderTime,
				"finishTime":       row.FinishTime,
				"validCode":        row.ValidCode,
				"statusText":       statusText,
				"estimateFee":      row.EstimateFee,
				"actualFee":        row.ActualFee,
				"userEstimateFee":  userEstimateFee,
				"userActualFee":    userActualFee,
				"commissionRate":   row.CommissionRate,
				"withdrawable":     withdrawable,
				"withdrawableTime": withdrawableTime,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"orders":           allOrders,
		"totalEstimateFee": roundFloat(totalEstimateFee, 2),
		"totalActualFee":   roundFloat(totalActualFee, 2),
	})
}

func getOrderStatusText(validCode int) string {
	switch validCode {
	case -1:
		return "未知"
	case 2:
		return "无效-拆单"
	case 3:
		return "无效-取消"
	case 4:
		return "无效"
	case 5:
		return "无效-账号异常"
	case 6:
		return "无效-赠品不返佣"
	case 7, 8, 9:
		return "无效"
	case 11:
		return "无效"
	case 13:
		return "违规订单"
	case 14:
		return "无效-来源不符"
	case 15:
		return "待付款"
	case 16:
		return "已付款"
	case 17:
		return "已完成"
	case 19:
		return "无效-佣金比例为0"
	case 20:
		return "无效-首购订单无效"
	case 25, 26, 27, 28:
		return "违规订单"
	default:
		return "其他"
	}
}

func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
