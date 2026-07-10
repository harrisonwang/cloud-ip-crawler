package crawler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const oracleURL = "https://docs.oracle.com/en-us/iaas/tools/public_ip_ranges.json"

// OracleCrawler Oracle Cloud IP 范围爬虫
type OracleCrawler struct{}

// OracleResponse Oracle API 响应结构
type OracleResponse struct {
	LastUpdatedTimestamp string `json:"last_updated_timestamp"`
	Regions              []struct {
		Region string `json:"region"`
		CIDRs  []struct {
			CIDR string   `json:"cidr"`
			Tags []string `json:"tags"`
		} `json:"cidrs"`
	} `json:"regions"`
}

func (c *OracleCrawler) Name() string {
	return "oracle"
}

func (c *OracleCrawler) Crawl() ([]Range, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(oracleURL)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	var data OracleResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	var ranges []Range

	for _, region := range data.Regions {
		for _, cidrInfo := range region.CIDRs {
			service := ""
			if len(cidrInfo.Tags) > 0 {
				service = cidrInfo.Tags[0]
			}

			r, err := createRange("oracle", cidrInfo.CIDR, region.Region, service)
			if err != nil {
				continue
			}
			ranges = append(ranges, *r)
		}
	}

	return ranges, nil
}
