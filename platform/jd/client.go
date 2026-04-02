package jd

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
	"strconv"
	"strings"
	"time"
)

const apiURL = "https://router.jd.com/api"

type PromotionResult struct {
	ShortURL      string `json:"shortURL"`
	ClickURL      string `json:"clickURL"`
	JCommand      string `json:"jCommand"`
	JShortCommand string `json:"jShortCommand"`
}

type GoodsInfo struct {
	SkuID        int64         `json:"skuId"`
	SkuName      string        `json:"skuName"`
	Owner        string        `json:"owner"`
	SkuStatus    int           `json:"skuStatus"`
	MainSkuId    int64         `json:"mainSkuId"`
	ProductId    int64         `json:"productId"`
	ItemId       string        `json:"itemId"`
	ImageInfo    *ImageInfo    `json:"imageInfo,omitempty"`
	CategoryInfo *CategoryInfo `json:"categoryInfo,omitempty"`
}

type ImageInfo struct {
	ImageList  []ImageUrlInfo `json:"imageList"`
	WhiteImage string         `json:"whiteImage"`
}

type ImageUrlInfo struct {
	URL string `json:"url"`
}

type CategoryInfo struct {
	Cid1     int64  `json:"cid1"`
	Cid1Name string `json:"cid1Name"`
	Cid2     int64  `json:"cid2"`
	Cid2Name string `json:"cid2Name"`
	Cid3     int64  `json:"cid3"`
	Cid3Name string `json:"cid3Name"`
}

type OrderRow struct {
	ID               string          `json:"id"`
	OrderID          int64           `json:"orderId"`
	ParentID         int64           `json:"parentId"`
	OrderTime        string          `json:"orderTime"`
	FinishTime       string          `json:"finishTime"`
	ModifyTime       string          `json:"modifyTime"`
	SkuID            int64           `json:"skuId"`
	SkuName          string          `json:"skuName"`
	SkuNum           int             `json:"skuNum"`
	SkuReturnNum     int             `json:"skuReturnNum"`
	Price            float64         `json:"price"`
	CommissionRate   float64         `json:"commissionRate"`
	SubSideRate      float64         `json:"subSideRate"`
	SubsidyRate      float64         `json:"subsidyRate"`
	FinalRate        float64         `json:"finalRate"`
	EstimateCosPrice float64         `json:"estimateCosPrice"`
	EstimateFee      float64         `json:"estimateFee"`
	ActualCosPrice   float64         `json:"actualCosPrice"`
	ActualFee        float64         `json:"actualFee"`
	ValidCode        int             `json:"validCode"`
	PayMonth         string          `json:"payMonth"`
	SubUnionId       string          `json:"subUnionId"`
	OrderEmt         int             `json:"orderEmt"`
	Plus             int             `json:"plus"`
	GoodsInfo        *OrderGoodsInfo `json:"goodsInfo,omitempty"`
}

type OrderGoodsInfo struct {
	ImageUrl string `json:"imageUrl"`
	Owner    string `json:"owner"`
	ShopName string `json:"shopName"`
	ShopId   int64  `json:"shopId"`
}

type Client struct {
	AppKey     string
	SecretKey  string
	UnionID    int64
	HTTPClient *http.Client
}

func NewClient(appKey, secretKey, unionID string) (*Client, error) {
	uid, err := strconv.ParseInt(unionID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("无效的京东联盟ID: %v", err)
	}
	return &Client{
		AppKey:     appKey,
		SecretKey:  secretKey,
		UnionID:    uid,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *Client) generateSign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(c.SecretKey)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(c.SecretKey)

	hash := md5.Sum([]byte(sb.String()))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

func (c *Client) callAPI(method string, bizParams interface{}) ([]byte, error) {
	paramBytes, err := json.Marshal(bizParams)
	if err != nil {
		return nil, fmt.Errorf("序列化参数失败: %w", err)
	}

	params := map[string]string{
		"method":      method,
		"app_key":     c.AppKey,
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		"format":      "json",
		"v":           "1.0",
		"sign_method": "md5",
		"param_json":  string(paramBytes),
	}
	params["sign"] = c.generateSign(params)

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	reqURL := apiURL + "?" + values.Encode()
	log.Printf("[JD API] %s request: %s", method, string(paramBytes))

	resp, err := c.HTTPClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("请求京东API失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	log.Printf("[JD API] %s response: %s", method, string(body))
	return body, nil
}

// parseAPIResponse extracts the inner result string from the JD API response.
// JD API response format varies: code can be string or int, result field can be "result", "getResult", or "queryResult".
func parseAPIResponse(body []byte) (string, error) {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(body, &outer); err != nil {
		return "", fmt.Errorf("解析响应JSON失败: %w", err)
	}

	// Check for error_response first
	if errRaw, ok := outer["error_response"]; ok {
		var errResp struct {
			Code   json.RawMessage `json:"code"`
			Msg    string          `json:"zh_desc"`
			EnDesc string          `json:"en_desc"`
		}
		if err := json.Unmarshal(errRaw, &errResp); err == nil {
			msg := errResp.Msg
			if msg == "" {
				msg = errResp.EnDesc
			}
			return "", fmt.Errorf("京东API错误: %s (code: %s)", msg, string(errResp.Code))
		}
	}

	for k, v := range outer {
		if !strings.HasSuffix(k, "_response") {
			continue
		}
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(v, &inner); err != nil {
			return "", fmt.Errorf("解析内部响应失败: %w", err)
		}

		// Look for the result field: "result", "getResult", or "queryResult"
		for _, field := range []string{"result", "getResult", "queryResult"} {
			if raw, ok := inner[field]; ok {
				var resultStr string
				if err := json.Unmarshal(raw, &resultStr); err == nil && resultStr != "" {
					return resultStr, nil
				}
			}
		}

		return "", fmt.Errorf("响应中未找到结果数据")
	}

	return "", fmt.Errorf("未知的响应格式: %s", string(body))
}

func ParseSkuID(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	if matched, _ := regexp.MatchString(`^\d{5,20}$`, rawURL); matched {
		return rawURL, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("无效的URL: %w", err)
	}

	// item.jd.com/{skuId}.html
	re1 := regexp.MustCompile(`/(\d{5,20})\.html`)
	if matches := re1.FindStringSubmatch(u.Path); len(matches) > 1 {
		return matches[1], nil
	}

	// item.m.jd.com/product/{skuId}.html or /product/{skuId}
	re2 := regexp.MustCompile(`/product/(\d{5,20})`)
	if matches := re2.FindStringSubmatch(u.Path); len(matches) > 1 {
		return matches[1], nil
	}

	// Query params: wareId, skuId, sku
	for _, param := range []string{"wareId", "skuId", "sku"} {
		if val := u.Query().Get(param); val != "" {
			if matched, _ := regexp.MatchString(`^\d{5,20}$`, val); matched {
				return val, nil
			}
		}
	}

	// Fallback: any long number in path
	re3 := regexp.MustCompile(`(\d{8,20})`)
	if matches := re3.FindStringSubmatch(u.Path); len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("无法从链接中提取商品ID，请确认是京东商品链接")
}

// ResolveURL follows redirects for JD short links to get the final URL.
// Returns (resolvedURL, true) if resolved to a product URL, (originalURL, false) otherwise.
func ResolveURL(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, false
	}

	shortHosts := map[string]bool{
		"u.jd.com":  true,
		"3.cn":      true,
		"re.jd.com": true,
	}

	if !shortHosts[u.Host] {
		return rawURL, false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return rawURL, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1")

	resp, err := client.Do(req)
	if err != nil {
		return rawURL, false
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()

	// Check if resolved to an actual product page, not homepage
	if strings.Contains(finalURL, "item.jd.com") || strings.Contains(finalURL, "item.m.jd.com") ||
		strings.Contains(finalURL, "/product/") {
		log.Printf("[JD] Resolved short link: %s -> %s", rawURL, finalURL)
		return finalURL, true
	}

	log.Printf("[JD] Short link resolved to non-product URL, using original: %s -> %s", rawURL, finalURL)
	return rawURL, false
}

func NormalizeImageURL(imgUrl string) string {
	if imgUrl == "" {
		return ""
	}
	if strings.HasPrefix(imgUrl, "http") {
		return imgUrl
	}
	return "https://img14.360buyimg.com/n1/" + imgUrl
}

func (c *Client) CallAPIRaw(method string, bizParams interface{}) (json.RawMessage, error) {
	body, err := c.callAPI(method, bizParams)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func (c *Client) GetPromotionLink(materialURL string) (*PromotionResult, error) {
	params := map[string]interface{}{
		"promotionCodeReq": map[string]interface{}{
			"materialId": materialURL,
			"unionId":    c.UnionID,
			"chainType":  3,
			"sceneId":    2,
			"command":    1,
		},
	}

	body, err := c.callAPI("jd.union.open.promotion.byunionid.get", params)
	if err != nil {
		return nil, err
	}

	resultStr, err := parseAPIResponse(body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Code    int             `json:"code"`
		Data    PromotionResult `json:"data"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		return nil, fmt.Errorf("解析推广链接结果失败: %w (raw: %s)", err, resultStr)
	}
	if result.Code != 200 {
		return nil, fmt.Errorf("获取推广链接失败: %s (code: %d)", result.Message, result.Code)
	}

	return &result.Data, nil
}

func (c *Client) GetGoodsInfo(skuIds string) ([]GoodsInfo, error) {
	skuIdStrs := strings.Split(skuIds, ",")
	skuIdNums := make([]int64, 0, len(skuIdStrs))
	for _, s := range skuIdStrs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		skuIdNums = append(skuIdNums, n)
	}
	if len(skuIdNums) == 0 {
		return nil, fmt.Errorf("无有效的skuId")
	}

	params := map[string]interface{}{
		"goodsReq": map[string]interface{}{
			"skuIds":  skuIdNums,
			"fields":  []string{"categoryInfo", "imageInfo"},
			"sceneId": 2,
		},
	}

	body, err := c.callAPI("jd.union.open.goods.bigfield.query", params)
	if err != nil {
		return nil, err
	}

	resultStr, err := parseAPIResponse(body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Code    int         `json:"code"`
		Data    []GoodsInfo `json:"data"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		return nil, fmt.Errorf("解析商品信息失败: %w (raw: %s)", err, resultStr)
	}
	if result.Code != 200 {
		return nil, fmt.Errorf("查询商品信息失败: %s (code: %d)", result.Message, result.Code)
	}

	return result.Data, nil
}

func (c *Client) QueryOrderRows(orderId int64) ([]OrderRow, error) {
	params := map[string]interface{}{
		"orderReq": map[string]interface{}{
			"pageIndex": 1,
			"pageSize":  500,
			"type":      1,
			"orderId":   orderId,
		},
	}

	body, err := c.callAPI("jd.union.open.order.row.query", params)
	if err != nil {
		return nil, err
	}

	resultStr, err := parseAPIResponse(body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Code    int        `json:"code"`
		Data    []OrderRow `json:"data"`
		HasMore bool       `json:"hasMore"`
		Message string     `json:"message"`
	}
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		return nil, fmt.Errorf("解析订单数据失败: %w", err)
	}
	if result.Code != 200 {
		return nil, fmt.Errorf("查询订单失败: %s (code: %d)", result.Message, result.Code)
	}

	return result.Data, nil
}
