package crawler

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math/bits"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 所有 ASN 系数据（命名厂商 + hosting 分类）共用 iptoasn.com 的全量 BGP 表：
// 一个 6.8MB 的文件覆盖全球所有 ASN 的宣告前缀，比逐 ASN 打在线接口（RIPEstat）
// 便宜得多——加一家厂商的边际成本是零次请求。文件每日更新，源头是 RouteViews。
const iptoasnV4URL = "https://iptoasn.com/data/ip2asn-v4.tsv.gz"

// asnDataset 一次下载、单遍扫描后按用途分好的成品，全体 ASN 系爬虫共享
type asnDataset struct {
	byProvider map[string][]Range // 命名厂商（hetzner、ovh …），键与 asnProviderList 一致
	hosting    []Range            // 启发式分类出的托管商，provider 统一记 "hosting"
	err        error
}

var (
	asnDatasetOnce sync.Once
	asnDatasetVal  asnDataset
)

// loadASNDataset 惰性加载 + 缓存：同一次运行里 13 个爬虫只触发一次下载
func loadASNDataset() *asnDataset {
	asnDatasetOnce.Do(func() {
		asnDatasetVal = buildASNDataset()
	})
	return &asnDatasetVal
}

func buildASNDataset() asnDataset {
	body, err := fetchBytes(iptoasnV4URL, 60*time.Second)
	if err != nil {
		return asnDataset{err: fmt.Errorf("下载 iptoasn 全量表失败: %w", err)}
	}

	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return asnDataset{err: fmt.Errorf("解压失败: %w", err)}
	}
	defer gz.Close()

	ds, err := parseIPToASN(gz)
	if err != nil {
		return asnDataset{err: err}
	}
	return ds
}

// parseIPToASN 单遍扫描 TSV（range_start \t range_end \t AS 号 \t 国家码 \t AS 描述），
// 把命中命名厂商的行分发到 byProvider，命中托管关键词或显式清单的行归入 hosting。
func parseIPToASN(r io.Reader) (asnDataset, error) {
	asnToProvider := map[int]string{}
	for _, p := range asnProviderList {
		for _, a := range p.asns {
			asnToProvider[a] = p.name
		}
	}

	ds := asnDataset{byProvider: map[string][]Range{}}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		cols := strings.Split(scanner.Text(), "\t")
		if len(cols) < 5 {
			continue
		}
		asn, err := strconv.Atoi(cols[2])
		if err != nil || asn == 0 { // AS0 是未宣告的空洞（"Not routed"）
			continue
		}

		provider, named := asnToProvider[asn]
		hosting := !named && (extraHostingASNs[asn] || isHostingDesc(cols[4]))
		if !named && !hosting {
			continue
		}

		start, err1 := netip.ParseAddr(strings.TrimSpace(cols[0]))
		end, err2 := netip.ParseAddr(strings.TrimSpace(cols[1]))
		if err1 != nil || err2 != nil {
			continue
		}

		country := cols[3]
		if country == "None" {
			country = ""
		}

		// iptoasn 给的是首尾地址（不保证对齐 CIDR），拆成等价的 CIDR 集合入库
		for _, cidr := range rangeToCIDRs(start, end) {
			if named {
				if rg, err := createRange(provider, cidr, country, fmt.Sprintf("AS%d", asn)); err == nil {
					ds.byProvider[provider] = append(ds.byProvider[provider], *rg)
				}
			} else {
				service := fmt.Sprintf("AS%d %s", asn, cols[4])
				if len(service) > 100 {
					service = service[:100]
				}
				if rg, err := createRange("hosting", cidr, country, service); err == nil {
					ds.hosting = append(ds.hosting, *rg)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return asnDataset{}, fmt.Errorf("扫描 iptoasn 表失败: %w", err)
	}
	return ds, nil
}

// rangeToCIDRs 把 [start, end] 闭区间拆成最少的一组 CIDR。仅处理 IPv4（v6 表是另一个文件，
// 见 README 后续计划）。全程 uint64 运算避免 2^32 溢出。
func rangeToCIDRs(start, end netip.Addr) []string {
	if !start.Is4() || !end.Is4() {
		return nil
	}
	s4, e4 := start.As4(), end.As4()
	s := uint64(s4[0])<<24 | uint64(s4[1])<<16 | uint64(s4[2])<<8 | uint64(s4[3])
	e := uint64(e4[0])<<24 | uint64(e4[1])<<16 | uint64(e4[2])<<8 | uint64(e4[3])

	var out []string
	for s <= e {
		size := s & (^s + 1) // 从 s 开始能对齐的最大块
		if size == 0 {       // s == 0 时低位技巧失效，最大块是整个 v4 空间
			size = 1 << 32
		}
		for s+size-1 > e { // 收缩到不超过区间尾
			size >>= 1
		}
		prefixLen := 32 - (bits.Len64(size) - 1)
		out = append(out, fmt.Sprintf("%d.%d.%d.%d/%d", byte(s>>24), byte(s>>16), byte(s>>8), byte(s), prefixLen))
		s += size
	}
	return out
}

// ---- hosting 启发式分类 ----
//
// 一家一家列举托管商永远列不完。这里换思路：对全量表里每个 ASN 的名称做关键词匹配，
// 把几千个「名字里就写着自己是托管商」的 ASN 一次性收进来，provider 统一记 "hosting"，
// service 记 "AS号 + 名称" 便于溯源。这层是启发式，置信度低于官方文件和命名 ASN，
// 有误报（叫 Cloud 的 ISP）也有漏报（用创始人名字命名的机房），README 里如实交代。

// hostingKeywordSubstrings 按子串匹配：托管商爱把关键词嵌进合成词里
// （SUPERVPS、MYCLOUD、WEBHOST），前缀匹配会漏。这批词做子串足够安全：
// SERVICES 不含 SERVER，COLOMBIA 不含 COLOC。
var hostingKeywordSubstrings = []string{
	"HOST",     // HOSTING / WEBHOST / UAHOST…
	"VPS",      //
	"VDS",      //
	"CLOUD",    // CLOUDFLARENET、MYCLOUD…
	"SERVER",   // SERVERS / SERVERSTACK / XSERVER…
	"DATACENT", // DATACENTER / DATACENTRE…
	"DEDIC",    // DEDICATED / DEDIPATH…
	"COLOC",    // COLOCATION / COLOCROSSING…
}

// hostingKeywordTokens 只做整词匹配的短词：IDC 当子串会误伤 MIDCONTINENT
// 这类家宽 ISP，但作为独立 token 出现时（CHINANET-IDC-BJ）几乎总是机房。
var hostingKeywordTokens = []string{"IDC"}

// extraHostingASNs 关键词抓不到、但明确该进 hosting 层的 ASN。
// 主要是官方文件厂商的骨干 AS：官方清单只列「对外服务的网段」，这些 AS 宣告的其余
// 空间（比如 1.1.1.1 所在的 1.1.1.0/24）就靠这层兜底。每个号都用 RIPEstat 的
// as-overview 核对过 holder。
var extraHostingASNs = map[int]bool{
	16509:  true, // AMAZON-02 - Amazon.com, Inc.
	14618:  true, // AMAZON-AES - Amazon.com, Inc.
	8075:   true, // MICROSOFT-CORP-MSN-AS-BLOCK - Microsoft Corporation
	15169:  true, // GOOGLE - Google LLC
	396982: true, // GOOGLE-CLOUD-PLATFORM - Google LLC
	13335:  true, // CLOUDFLARENET - Cloudflare, Inc.
	31898:  true, // ORACLE-BMC-31898 - Oracle Corporation
	14061:  true, // DIGITALOCEAN-ASN - DigitalOcean, LLC
	63949:  true, // AKAMAI-LINODE-AP - Akamai Connected Cloud
	20473:  true, // AS-VULTR - The Constant Company, LLC
	54113:  true, // FASTLY - Fastly, Inc.
}

func isHostingDesc(desc string) bool {
	u := strings.ToUpper(desc)
	if strings.Contains(u, "DATA CENTER") || strings.Contains(u, "DATA-CENTER") {
		return true
	}
	for _, kw := range hostingKeywordSubstrings {
		if strings.Contains(u, kw) {
			return true
		}
	}
	tokens := strings.FieldsFunc(u, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	for _, tok := range tokens {
		if slices.Contains(hostingKeywordTokens, tok) {
			return true
		}
	}
	return false
}

// HostingCrawler 启发式托管商分类层，见上方说明
type HostingCrawler struct{}

func (c *HostingCrawler) Name() string {
	return "hosting"
}

func (c *HostingCrawler) Crawl() ([]Range, error) {
	ds := loadASNDataset()
	if ds.err != nil {
		return nil, ds.err
	}
	if len(ds.hosting) == 0 {
		return nil, fmt.Errorf("分类结果为空，疑似上游表结构变化")
	}
	return ds.hosting, nil
}
