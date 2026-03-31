package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Database struct {
		URL string `json:"url"`
	} `json:"database"`
	Port        string `json:"port"`
	JDAppKey    string `json:"jd_appKey"`
	JDSecretKey string `json:"jd_secretKey"`
	JDUnionID   string `json:"jd_id"`

	TaobaoAppKey    string `json:"taobao_appKey"`
	TaobaoAppSecret string `json:"taobao_appSecret"`
	TaobaoAdzoneId  string `json:"taobao_adzoneId"`

	PDDClientId     string `json:"pdd_clientId"`
	PDDClientSecret string `json:"pdd_clientSecret"`
	PDDPid          string `json:"pdd_pid"`
}

func Load(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("无法打开配置文件 %s: %v", filename, err)
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	return &config, nil
}

func (c *Config) Validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("请在配置文件中设置数据库连接 URL")
	}
	if c.JDAppKey == "" {
		return fmt.Errorf("请在配置文件中设置京东联盟 AppKey (jd_appKey)")
	}
	if c.JDSecretKey == "" {
		return fmt.Errorf("请在配置文件中设置京东联盟 SecretKey (jd_secretKey)")
	}
	if c.JDUnionID == "" {
		return fmt.Errorf("请在配置文件中设置京东联盟 ID (jd_id)")
	}
	return nil
}
