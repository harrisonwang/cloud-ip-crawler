package crawler

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const digitaloceanURL = "https://digitalocean.com/geo/google.csv"

// DigitalOceanCrawler DigitalOcean IP 范围爬虫
type DigitalOceanCrawler struct{}

func (c *DigitalOceanCrawler) Name() string {
	return "digitalocean"
}

func (c *DigitalOceanCrawler) Crawl() ([]Range, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(digitaloceanURL)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	var ranges []Range
	scanner := bufio.NewScanner(resp.Body)

	// CSV 格式: ip_range,country_code,region,city,postal_code
	// 例如: 104.131.0.0/18,US,NJ,North Bergen,07047
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
		if cidr == "" {
			continue
		}

		region := ""
		if len(parts) >= 4 {
			// 组合区域信息: 国家-州-城市
			country := strings.TrimSpace(parts[1])
			state := strings.TrimSpace(parts[2])
			city := strings.TrimSpace(parts[3])
			if city != "" {
				region = fmt.Sprintf("%s-%s-%s", country, state, city)
			} else if state != "" {
				region = fmt.Sprintf("%s-%s", country, state)
			} else {
				region = country
			}
		}

		r, err := createRange("digitalocean", cidr, region, "")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, scanner.Err()
}
