// cloud-ip-crawler 抓取主流云厂商公开发布的 IP 段，汇总成一个 SQLite 数据库，
// 用来回答「这个 IP 是不是机房 IP」。
//
// 只收录 IPv4：各家的 IPv6 数据源格式不统一（AWS 的 v6 在独立的 ipv6_prefixes 键里，
// 多数厂商的解析器根本没读 v6 字段），放开会得到各家覆盖不一致的结果。见 README 的「后续计划」。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/harrisonwang/cloud-ip-crawler/internal/crawler"

	_ "modernc.org/sqlite" // 纯 Go 实现，无需 CGO，可交叉编译到所有平台
)

func main() {
	dbPath := flag.String("db", "cloud-ip.db", "SQLite 数据库路径（不存在则自动创建）")
	providers := flag.String("providers", "all", "要爬取的云厂商，逗号分隔 (aws,gcp,azure,cloudflare,oracle,digitalocean,linode,vultr) 或 all")
	dryRun := flag.Bool("dry-run", false, "仅抓取并打印样例，不写数据库")
	showVersion := flag.Bool("version", false, "打印版本后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println("cloud-ip-crawler", crawler.Version)
		return
	}

	crawlers := map[string]crawler.Crawler{
		"aws":          &crawler.AWSCrawler{},
		"gcp":          &crawler.GCPCrawler{},
		"azure":        &crawler.AzureCrawler{},
		"cloudflare":   &crawler.CloudflareCrawler{},
		"oracle":       &crawler.OracleCrawler{},
		"digitalocean": &crawler.DigitalOceanCrawler{},
		"linode":       &crawler.LinodeCrawler{},
		"vultr":        &crawler.VultrCrawler{},
	}

	// map 迭代序随机，固定顺序让日志可比对
	order := []string{"aws", "gcp", "azure", "cloudflare", "oracle", "digitalocean", "linode", "vultr"}

	var selected []crawler.Crawler
	if *providers == "all" {
		for _, name := range order {
			selected = append(selected, crawlers[name])
		}
	} else {
		for _, name := range strings.Split(*providers, ",") {
			name = strings.TrimSpace(strings.ToLower(name))
			c, ok := crawlers[name]
			if !ok {
				log.Fatalf("未知的云厂商 %q，可选：%s", name, strings.Join(order, ", "))
			}
			selected = append(selected, c)
		}
	}

	var db *sql.DB
	if !*dryRun {
		var err error
		db, err = sql.Open("sqlite", *dbPath)
		if err != nil {
			log.Fatalf("无法打开数据库 %s: %v", *dbPath, err)
		}
		defer db.Close()
		if err := crawler.EnsureSchema(db); err != nil {
			log.Fatalf("建表失败: %v", err)
		}
	}

	total, failed := 0, 0
	for _, c := range selected {
		log.Printf("开始爬取 %s...", c.Name())
		start := time.Now()

		ranges, err := c.Crawl()
		if err != nil {
			log.Printf("爬取 %s 失败: %v", c.Name(), err)
			failed++
			continue
		}
		log.Printf("%s 获取到 %d 条 IPv4 范围, 耗时 %v", c.Name(), len(ranges), time.Since(start))

		if *dryRun {
			for i, r := range ranges {
				if i >= 5 {
					fmt.Printf("  ... 还有 %d 条\n", len(ranges)-5)
					break
				}
				fmt.Printf("  %s: %s (region=%s, service=%s)\n", r.Provider, r.CIDR, r.Region, r.Service)
			}
		} else {
			inserted, err := crawler.InsertRanges(db, c.Name(), ranges)
			if err != nil {
				log.Printf("入库 %s 失败: %v", c.Name(), err)
				failed++
				continue
			}
			log.Printf("%s 入库 %d 条", c.Name(), inserted)
		}
		total += len(ranges)
	}

	log.Printf("完成! 共处理 %d 条 IP 范围，%d 个数据源失败", total, failed)

	// 每日自动构建要能据此判断产物是否可信：任一数据源失败就非零退出
	if failed > 0 {
		os.Exit(1)
	}
}
