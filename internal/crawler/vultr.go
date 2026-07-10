package crawler

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const vultrURL = "https://geofeed.constant.com/?list-format=csv"

// VultrCrawler Vultr IP 范围爬虫
type VultrCrawler struct{}

func (c *VultrCrawler) Name() string {
	return "vultr"
}

func (c *VultrCrawler) Crawl() ([]Range, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", vultrURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	var ranges []Range
	scanner := bufio.NewScanner(resp.Body)

	// Geofeed CSV 格式: ip_prefix,country_code,region_code,city,postal_code
	// 例如: 108.61.0.0/18,US,US-NJ,Piscataway,08854
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 1 {
			continue
		}

		cidr := strings.TrimSpace(parts[0])
		if cidr == "" || !strings.Contains(cidr, "/") {
			continue
		}

		region := ""
		if len(parts) >= 4 {
			country := strings.TrimSpace(parts[1])
			regionCode := strings.TrimSpace(parts[2])
			city := strings.TrimSpace(parts[3])
			if city != "" {
				region = fmt.Sprintf("%s-%s", regionCode, city)
			} else if regionCode != "" {
				region = regionCode
			} else {
				region = country
			}
		}

		r, err := createRange("vultr", cidr, region, "")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, scanner.Err()
}
