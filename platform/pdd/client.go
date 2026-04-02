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
	OrderSn               string      `json:"order_sn"`
	GoodsName             string      `json:"goods_name"`
	GoodsPrice            int64       `json:"goods_price"`
	GoodsQuantity         int64       `json:"goods_quantity"`
	OrderAmount           int64       `json:"order_amount"`
	PromotionRate         int64       `json:"promotion_rate"`
	PromotionAmount       int64       `json:"promotion_amount"`
	DuoIdServiceFee       json.Number `json:"duo_id_service_fee"`
	OrderStatus           int         `json:"order_status"`
	OrderStatusDesc       string      `json:"order_status_desc"`
	OrderCreateTime       int64       `json:"order_create_time"`
	OrderGroupSuccessTime int64       `json:"order_group_success_time"`
	OrderPayTime          int64       `json:"order_pay_time"`
	OrderSettleTime       int64       `json:"order_settle_time"`
	GoodsThumbnailURL     string      `json:"goods_thumbnail_url"`
	GoodsCategoryName     string      `json:"goods_category_name"`
	MallName              string      `json:"mall_name"`
	GoodsSign             string      `json:"goods_sign"`
	Pid                   string      `json:"pid"`
	PriceCompareStatus    int         `json:"price_compare_status"`
	FailReason            string      `json:"fail_reason"`
	NoSubsidyReason       string      `json:"no_subsidy_reason"`
}

func (o *OrderRow) GetDuoIdServiceFee() int64 {
	n, err := o.DuoIdServiceFee.Int64()
	if err != nil {
		return 0
	}
	return n
}

func (c *Client) QueryOrder(orderSn string) (*OrderRow, error) {
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
		Order *OrderRow `json:"order_detail_response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析订单数据失败: %w", err)
	}
	if resp.Order == nil || resp.Order.OrderSn == "" {
		return nil, nil
	}

	return resp.Order, nil
}

func (c *Client) ConvertURL(sourceURL string) (*PromotionURL, error) {
	goods, err := c.searchGoods(sourceURL)
	if err != nil {
		return nil, err
	}

	result, err := c.generatePromotionURL(goods.GoodsSign)
	if err != nil {
		return nil, err
	}

	result.GoodsDetail = goods
	return result, nil
}

func (c *Client) searchGoods(sourceURL string) (*GoodsBasic, error) {
	bizParams := map[string]interface{}{
		"keyword":   sourceURL,
		"page":      1,
		"page_size": 10,
		"pid":       c.Pid,
	}

	body, err := c.callAPI("pdd.ddk.goods.search", bizParams)
	if err != nil {
		return nil, fmt.Errorf("搜索商品失败: %w", err)
	}
	if apiErr := c.parseError(body); apiErr != nil {
		return nil, apiErr
	}

	var resp struct {
		Response *struct {
			GoodsList []GoodsBasic `json:"goods_list"`
		} `json:"goods_search_response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}
	if resp.Response == nil || len(resp.Response.GoodsList) == 0 {
		return nil, fmt.Errorf("该商品暂无返利")
	}

	g := &resp.Response.GoodsList[0]
	log.Printf("[PDD] Got goods: name=%s price=%d rate=%d sign=%s", g.GoodsName, g.MinGroupPrice, g.PromotionRate, g.GoodsSign)
	return g, nil
}

func (c *Client) generatePromotionURL(goodsSign string) (*PromotionURL, error) {
	bizParams := map[string]interface{}{
		"p_id":                c.Pid,
		"goods_sign_list":     []string{goodsSign},
		"generate_short_url":  true,
		"generate_schema_url": true,
	}

	body, err := c.callAPI("pdd.ddk.goods.promotion.url.generate", bizParams)
	if err != nil {
		return nil, fmt.Errorf("生成推广链接失败: %w", err)
	}
	if apiErr := c.parseError(body); apiErr != nil {
		return nil, apiErr
	}

	var resp struct {
		Response *struct {
			UrlList []struct {
				URL            string `json:"url"`
				ShortURL       string `json:"short_url"`
				MobileURL      string `json:"mobile_url"`
				MobileShortURL string `json:"mobile_short_url"`
				SchemaURL      string `json:"schema_url"`
			} `json:"goods_promotion_url_list"`
		} `json:"goods_promotion_url_generate_response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析推广链接失败: %w", err)
	}
	if resp.Response == nil || len(resp.Response.UrlList) == 0 {
		return nil, fmt.Errorf("生成推广链接返回为空")
	}

	u := resp.Response.UrlList[0]
	log.Printf("[PDD] Got promotion URL: short=%s schema=%s", u.ShortURL, u.SchemaURL)
	return &PromotionURL{
		URL:            u.URL,
		ShortURL:       u.ShortURL,
		MobileURL:      u.MobileURL,
		MobileShortURL: u.MobileShortURL,
		SchemaURL:      u.SchemaURL,
	}, nil
}
