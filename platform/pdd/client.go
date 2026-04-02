package pdd

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const apiURL = "https://gw-api.pinduoduo.com/api/router"

type PromotionURL struct {
	URL            string      `json:"url"`
	ShortURL       string      `json:"short_url"`
	MobileURL      string      `json:"mobile_url"`
	MobileShortURL string      `json:"mobile_short_url"`
	SchemaURL      string      `json:"schema_url"`
	GoodsDetail    *GoodsBasic `json:"-"`
}

type GoodsBasic struct {
	GoodsName      string `json:"goods_name"`
	GoodsImageURL  string `json:"goods_thumbnail_url"`
	MinGroupPrice  int64  `json:"min_group_price"`
	PromotionRate  int64  `json:"promotion_rate"`
	CouponDiscount int64  `json:"coupon_discount"`
	HasCoupon      bool   `json:"has_coupon"`
	MallName       string `json:"mall_name"`
	GoodsSign      string `json:"goods_sign"`
}

type Client struct {
	ClientId     string
	ClientSecret string
	Pid          string
	HTTP         *http.Client
}

func NewClient(clientId, clientSecret, pid string) *Client {
	return &Client{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		Pid:          pid,
		HTTP:         &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) callAPI(method string, bizParams map[string]interface{}) ([]byte, error) {
	// Flatten all params to string values for consistent signing
	params := map[string]string{
		"type":      method,
		"client_id": c.ClientId,
		"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
	}

	for k, v := range bizParams {
		switch val := v.(type) {
		case string:
			params[k] = val
		case bool:
			if val {
				params[k] = "true"
			} else {
				params[k] = "false"
			}
		default:
			b, _ := json.Marshal(val)
			params[k] = string(b)
		}
	}

	params["sign"] = c.generateSignStr(params)

	log.Printf("[PDD API] %s request: %v", method, params)

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	resp, err := c.HTTP.PostForm(apiURL, values)
	if err != nil {
		return nil, fmt.Errorf("请求拼多多API失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	log.Printf("[PDD API] %s response: %s", method, string(respBody))
	return respBody, nil
}

func (c *Client) generateSignStr(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(c.ClientSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(c.ClientSecret)

	signStr := sb.String()
	log.Printf("[PDD Sign] pre-hash: %s", signStr)

	hash := md5.Sum([]byte(signStr))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

func (c *Client) CallAPIRaw(method string, bizParams map[string]string) (json.RawMessage, error) {
	// Convert string map to interface map for callAPI
	ifaceParams := make(map[string]interface{}, len(bizParams))
	for k, v := range bizParams {
		ifaceParams[k] = v
	}
	body, err := c.callAPI(method, ifaceParams)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func (c *Client) parseError(body []byte) error {
	var wrapper struct {
		ErrorResponse *struct {
			ErrorCode int    `json:"error_code"`
			ErrorMsg  string `json:"error_msg"`
			SubCode   string `json:"sub_code"`
			SubMsg    string `json:"sub_msg"`
		} `json:"error_response"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.ErrorResponse != nil {
		msg := wrapper.ErrorResponse.SubMsg
		if msg == "" {
			msg = wrapper.ErrorResponse.ErrorMsg
		}
		return fmt.Errorf("拼多多API错误: %s (code: %d)", msg, wrapper.ErrorResponse.ErrorCode)
	}
	return nil
}

type OrderRow struct {
	OrderSn           string `json:"order_sn"`
	GoodsName         string `json:"goods_name"`
	GoodsPrice        int64  `json:"goods_price"`
	GoodsQuantity     int64  `json:"goods_quantity"`
	OrderAmount       int64  `json:"order_amount"`
	PromotionRate     int64  `json:"promotion_rate"`
	PromotionAmount   int64  `json:"promotion_amount"`
	DuoIdServiceFee   int64  `json:"duo_id_service_fee"`
	OrderStatus       int    `json:"order_status"`
	OrderStatusDesc   string `json:"order_status_desc"`
	OrderCreateTime   int64  `json:"order_create_time"`
	OrderSettleTime   int64  `json:"order_settle_time"`
	GoodsThumbnailURL string `json:"goods_thumbnail_url"`
}

func (c *Client) QueryOrder(orderSn string) ([]OrderRow, error) {
	bizParams := map[string]interface{}{
		"order_sn": orderSn,
	}

	body, err := c.callAPI("pdd.ddk.order.detail.get", bizParams)
	if err != nil {
		return nil, err
	}

	if apiErr := c.parseError(body); apiErr != nil {
		return nil, apiErr
	}

	var resp struct {
		Response *struct {
			OrderList []OrderRow `json:"order_detail_response"`
		} `json:"order_detail_get_response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析订单数据失败: %w", err)
	}
	if resp.Response == nil || len(resp.Response.OrderList) == 0 {
		return nil, nil
	}

	return resp.Response.OrderList, nil
}

// ConvertURL uses pdd.ddk.goods.zs.unit.url.gen to convert a source URL,
// then tries to fetch product details via search API.
func (c *Client) ConvertURL(sourceURL string) (*PromotionURL, error) {
	bizParams := map[string]interface{}{
		"pid":        c.Pid,
		"source_url": sourceURL,
	}

	body, err := c.callAPI("pdd.ddk.goods.zs.unit.url.gen", bizParams)
	if err != nil {
		return nil, err
	}

	if apiErr := c.parseError(body); apiErr != nil {
		return nil, apiErr
	}

	var resp struct {
		Response *struct {
			URL            string `json:"url"`
			ShortURL       string `json:"short_url"`
			MobileURL      string `json:"mobile_url"`
			MobileShortURL string `json:"mobile_short_url"`
		} `json:"goods_zs_unit_generate_response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析转链结果失败: %w", err)
	}
	if resp.Response == nil {
		return nil, fmt.Errorf("转链返回结果为空")
	}

	result := &PromotionURL{
		URL:            resp.Response.URL,
		ShortURL:       resp.Response.ShortURL,
		MobileURL:      resp.Response.MobileURL,
		MobileShortURL: resp.Response.MobileShortURL,
	}

	result.GoodsDetail = c.trySearchGoods(sourceURL)

	if result.GoodsDetail != nil && result.GoodsDetail.GoodsSign != "" {
		if schemaURL := c.getSchemaURL(result.GoodsDetail.GoodsSign); schemaURL != "" {
			result.SchemaURL = schemaURL
		}
	}

	return result, nil
}

func (c *Client) getSchemaURL(goodsSign string) string {
	bizParams := map[string]interface{}{
		"p_id":                c.Pid,
		"goods_sign_list":     []string{goodsSign},
		"generate_short_url":  true,
		"generate_schema_url": true,
	}

	body, err := c.callAPI("pdd.ddk.goods.promotion.url.generate", bizParams)
	if err != nil {
		log.Printf("[PDD] get schema_url failed: %v", err)
		return ""
	}
	if apiErr := c.parseError(body); apiErr != nil {
		log.Printf("[PDD] get schema_url API error: %v", apiErr)
		return ""
	}

	var resp struct {
		Response *struct {
			UrlList []struct {
				SchemaURL string `json:"schema_url"`
			} `json:"goods_promotion_url_list"`
		} `json:"goods_promotion_url_generate_response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Response == nil || len(resp.Response.UrlList) == 0 {
		return ""
	}

	schemaURL := resp.Response.UrlList[0].SchemaURL
	log.Printf("[PDD] Got schema_url: %s", schemaURL)
	return schemaURL
}

// trySearchGoods attempts to get product details via search API (best effort).
func (c *Client) trySearchGoods(sourceURL string) *GoodsBasic {
	bizParams := map[string]interface{}{
		"keyword":   sourceURL,
		"page":      1,
		"page_size": 10,
		"pid":       c.Pid,
	}

	body, err := c.callAPI("pdd.ddk.goods.search", bizParams)
	if err != nil {
		log.Printf("[PDD] search for details failed: %v", err)
		return nil
	}
	if apiErr := c.parseError(body); apiErr != nil {
		log.Printf("[PDD] search for details API error: %v", apiErr)
		return nil
	}

	var resp struct {
		Response *struct {
			GoodsList []GoodsBasic `json:"goods_list"`
		} `json:"goods_search_response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Response == nil || len(resp.Response.GoodsList) == 0 {
		return nil
	}

	g := resp.Response.GoodsList[0]
	log.Printf("[PDD] Got goods detail: name=%s price=%d rate=%d", g.GoodsName, g.MinGroupPrice, g.PromotionRate)
	return &g
}
