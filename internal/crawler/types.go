package crawler

import (
	"fmt"
	"net"
)

// Version 由 -ldflags "-X .../internal/crawler.Version=..." 注入
var Version = "dev"

func userAgent() string {
	return "cloud-ip-crawler/" + Version + " (+https://github.com/harrisonwang/cloud-ip-crawler)"
}

// Range 云厂商 IP 范围
type Range struct {
	Provider  string
	CIDR      string
	IPStart   net.IP
	IPEnd     net.IP
	Region    string
	Service   string
	IPVersion int
}

// Crawler 爬虫接口
type Crawler interface {
	Name() string
	Crawl() ([]Range, error)
}

// errSkipIPv6 是预期的跳过，不是抓取错误。调用方一律 continue。
var errSkipIPv6 = fmt.Errorf("skip IPv6")

// createRange 是全部八个爬虫写入前的唯一收口，IPv6 的取舍集中在这里
func createRange(provider, cidr, region, service string) (*Range, error) {
	start, end, version, err := parseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	if version == 6 {
		return nil, errSkipIPv6
	}
	return &Range{
		Provider:  provider,
		CIDR:      cidr,
		IPStart:   start,
		IPEnd:     end,
		Region:    region,
		Service:   service,
		IPVersion: version,
	}, nil
}

// parseCIDR 解析 CIDR 并返回该网段的首尾地址
func parseCIDR(cidr string) (start, end net.IP, version int, err error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, 0, err
	}

	start = ipNet.IP
	end = make(net.IP, len(start))
	copy(end, start)
	for i := range end {
		end[i] |= ^ipNet.Mask[i]
	}

	version = 4
	if start.To4() == nil {
		version = 6
	}
	return start, end, version, nil
}

// ipToBytes 定长存储，使 ip_start <= x <= ip_end 的 BLOB 比较成立
func ipToBytes(ip net.IP) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip.To16()
}
