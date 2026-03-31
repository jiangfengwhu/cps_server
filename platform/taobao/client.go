package taobao

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const apiURL = "https://eco.taobao.com/router/rest"

type MaterialItem struct {
	ItemId         string `json:"item_id"`
	Title          string `json:"title"`
	PictURL        string `json:"pict_url"`
	SmallImages    any    `json:"small_images"`
	ZkFinalPrice   string `json:"zk_final_price"`
	ReservePrice   string `json:"reserve_price"`
	ClickURL       string `json:"click_url"`
	CouponClickURL string `json:"coupon_click_url"`
	CommissionRate string `json:"commission_rate"`
	CouponAmount   string `json:"coupon_amount"`
	ShopTitle      string `json:"shop_title"`
	Volume         int64  `json:"volume"`
	Nick           string `json:"nick"`
	CategoryId     int64  `json:"category_id"`
	CouponInfo     string `json:"coupon_info"`
	UserType       int    `json:"user_type"` // 0=淘宝, 1=天猫
	Tpwd           string `json:"tpwd"`
}

type Client struct {
	AppKey    string
	AppSecret string
	AdzoneId  string
	HTTP      *http.Client
}

func NewClient(appKey, appSecret, adzoneId string) *Client {
	return &Client{
		AppKey:    appKey,
		AppSecret: appSecret,
		AdzoneId:  adzoneId,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) generateSign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(c.AppSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(c.AppSecret)

	hash := md5.Sum([]byte(sb.String()))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

func (c *Client) callAPI(method string, bizParams map[string]string) ([]byte, error) {
	params := map[string]string{
		"method":      method,
		"app_key":     c.AppKey,
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"format":      "json",
		"v":           "2.0",
		"sign_method": "md5",
	}

	for k, v := range bizParams {
		params[k] = v
	}

	params["sign"] = c.generateSign(params)

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	log.Printf("[Taobao API] %s request: %v", method, bizParams)

	resp, err := c.HTTP.PostForm(apiURL, values)
	if err != nil {
		return nil, fmt.Errorf("请求淘宝API失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	log.Printf("[Taobao API] %s response: %s", method, string(body))
	return body, nil
}

func (c *Client) CallAPIRaw(method string, bizParams map[string]string) (json.RawMessage, error) {
	body, err := c.callAPI(method, bizParams)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func (c *Client) parseErrorResponse(body []byte) error {
	var wrapper struct {
		ErrorResponse *struct {
			Code    int    `json:"code"`
			Msg     string `json:"msg"`
			SubCode string `json:"sub_code"`
			SubMsg  string `json:"sub_msg"`
		} `json:"error_response"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.ErrorResponse != nil {
		msg := wrapper.ErrorResponse.SubMsg
		if msg == "" {
			msg = wrapper.ErrorResponse.Msg
		}
		return fmt.Errorf("淘宝API错误: %s (code: %d, sub_code: %s)", msg, wrapper.ErrorResponse.Code, wrapper.ErrorResponse.SubCode)
	}
	return nil
}

func (c *Client) MaterialRecommend(itemId string) (*MaterialItem, error) {
	bizParams := map[string]string{
		"adzone_id":   c.AdzoneId,
		"material_id": "13256",
		"item_id":     itemId,
		"page_size":   "1",
		"page_no":     "1",
	}

	body, err := c.callAPI("taobao.tbk.dg.material.recommend", bizParams)
	if err != nil {
		return nil, err
	}

	if apiErr := c.parseErrorResponse(body); apiErr != nil {
		return nil, apiErr
	}

	// Parse the nested response - try multiple possible structures
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// Find the response key (e.g., tbk_dg_material_recommend_response)
	for k, v := range raw {
		if !strings.HasSuffix(k, "_response") || k == "error_response" {
			continue
		}

		var items []MaterialItem
		if err := extractItems(v, &items); err != nil {
			return nil, fmt.Errorf("解析商品数据失败: %w (response key: %s)", err, k)
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("未找到该商品的推广信息")
		}
		return &items[0], nil
	}

	return nil, fmt.Errorf("未知的响应格式: %s", string(body))
}

// extractItems recursively searches for the items array in the response.
func extractItems(data json.RawMessage, items *[]MaterialItem) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	// Look for common result list field names
	for _, key := range []string{"result_list", "results", "result"} {
		if inner, ok := obj[key]; ok {
			var innerObj map[string]json.RawMessage
			if err := json.Unmarshal(inner, &innerObj); err == nil {
				for _, k2 := range []string{"map_data", "result", "data"} {
					if arr, ok := innerObj[k2]; ok {
						return json.Unmarshal(arr, items)
					}
				}
			}
			// Maybe it's directly an array
			if err := json.Unmarshal(inner, items); err == nil && len(*items) > 0 {
				return nil
			}
		}
	}

	// Fallback: try every field
	for _, v := range obj {
		var innerObj map[string]json.RawMessage
		if err := json.Unmarshal(v, &innerObj); err == nil {
			for _, arr := range innerObj {
				if err := json.Unmarshal(arr, items); err == nil && len(*items) > 0 {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("未找到商品列表数据")
}

func ParseItemId(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	if matched, _ := regexp.MatchString(`^\d{5,20}$`, rawURL); matched {
		return rawURL, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("无效的URL: %w", err)
	}

	// item.taobao.com/item.htm?id=xxx or detail.tmall.com/item.htm?id=xxx
	if id := u.Query().Get("id"); id != "" {
		if matched, _ := regexp.MatchString(`^\d{5,20}$`, id); matched {
			return id, nil
		}
	}

	// Try to find a long number in the URL path or query
	re := regexp.MustCompile(`(\d{8,20})`)
	if matches := re.FindStringSubmatch(rawURL); len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("无法从链接中提取商品ID")
}

func IsTaobaoLink(rawURL string) bool {
	rawURL = strings.ToLower(rawURL)
	taobaoHosts := []string{
		"taobao.com", "tmall.com", "tb.cn",
		"m.tb.cn", "a.m.taobao.com",
		"detail.tmall.com", "item.taobao.com",
		"s.click.taobao.com",
	}
	for _, h := range taobaoHosts {
		if strings.Contains(rawURL, h) {
			return true
		}
	}
	tpwdPattern := regexp.MustCompile(`[€¥₳$£][a-zA-Z0-9]+[€¥₳$£]`)
	return tpwdPattern.MatchString(rawURL)
}

func ResolveURL(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, false
	}

	shortHosts := map[string]bool{
		"m.tb.cn": true,
		"tb.cn":   true,
	}

	if !shortHosts[u.Host] {
		return rawURL, false
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return rawURL, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15")

	resp, err := client.Do(req)
	if err != nil {
		return rawURL, false
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	if strings.Contains(finalURL, "taobao.com") || strings.Contains(finalURL, "tmall.com") {
		log.Printf("[Taobao] Resolved short link: %s -> %s", rawURL, finalURL)
		return finalURL, true
	}

	return rawURL, false
}
