package crawler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const bunnyURL = "https://bunnycdn.com/api/system/edgeserverlist"

// BunnyCrawler Bunny CDN 边缘节点 IP 爬虫
type BunnyCrawler struct{}

func (c *BunnyCrawler) Name() string {
	return "bunny"
}

func (c *BunnyCrawler) Crawl() ([]Range, error) {
	body, err := fetchBytes(bunnyURL, 30*time.Second)
	if err != nil {
		return nil, err
	}

	// Bunny 给的是一串单个边缘节点 IP（不是 CIDR），逐个当主机路由处理
	var ips []string
	if err := json.Unmarshal(body, &ips); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	var ranges []Range
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		// 单个地址补成主机路由：v4 → /32，v6 → /128（v6 随后在 createRange 被跳过）
		cidr := ip
		if !strings.Contains(cidr, "/") {
			if strings.Contains(ip, ":") {
				cidr = ip + "/128"
			} else {
				cidr = ip + "/32"
			}
		}

		r, err := createRange("bunny", cidr, "", "cdn")
		if err != nil {
			continue
		}
		ranges = append(ranges, *r)
	}

	return ranges, nil
}
