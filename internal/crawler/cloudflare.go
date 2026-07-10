package crawler

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	cloudflareIPv4URL = "https://www.cloudflare.com/ips-v4"
	cloudflareIPv6URL = "https://www.cloudflare.com/ips-v6"
)

// CloudflareCrawler Cloudflare IP 范围爬虫
type CloudflareCrawler struct{}

func (c *CloudflareCrawler) Name() string {
	return "cloudflare"
}

func (c *CloudflareCrawler) Crawl() ([]Range, error) {
	var ranges []Range

	// IPv4
	ipv4Ranges, err := c.fetchTextList(cloudflareIPv4URL)
	if err != nil {
		return nil, fmt.Errorf("获取 IPv4 失败: %w", err)
	}
	ranges = append(ranges, ipv4Ranges...)

	// IPv6
	ipv6Ranges, err := c.fetchTextList(cloudflareIPv6URL)
	if err != nil {
		return nil, fmt.Errorf("获取 IPv6 失败: %w", err)
	}
	ranges = append(ranges, ipv6Ranges...)

	return ranges, nil
}

func (c *CloudflareCrawler) fetchTextList(url string) ([]Range, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	var ranges []Range
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		cidr := strings.TrimSpace(scanner.Text())
		if cidr == "" || strings.HasPrefix(cidr, "#") {
			continue
		}

		r, err := createRange("cloudflare", cidr, "", "cdn")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, scanner.Err()
}
