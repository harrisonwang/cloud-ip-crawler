package crawler

import (
	"fmt"
	"net"
	"net/netip"
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

// errSkipReserved 同上：特殊用途网段的预期跳过
var errSkipReserved = fmt.Errorf("skip reserved network")

// reservedV4 IPv4 特殊用途网段（RFC 6890 等）：私网、TEST-NET、组播、保留……
// 上游数据偶尔混进这类垃圾——Vultr 的 geofeed 就把三个 TEST-NET 当数据发出来。
// 公共数据集不该声称它们属于任何厂商，入库前统一丢弃；MMDB 导出也会拒绝它们。
var reservedV4 = func() []netip.Prefix {
	cidrs := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
		"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
		"192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	}
	ps := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		ps[i] = netip.MustParsePrefix(c)
	}
	return ps
}()

// isReservedV4CIDR 判断一个 CIDR 是否整段落在特殊用途网段里
func isReservedV4CIDR(cidr string) bool {
	p, err := netip.ParsePrefix(cidr)
	if err != nil || !p.Addr().Is4() {
		return false
	}
	for _, res := range reservedV4 {
		if res.Overlaps(p) && res.Bits() <= p.Bits() {
			return true
		}
	}
	return false
}

// createRange 是全部八个爬虫写入前的唯一收口，IPv6 的取舍集中在这里
func createRange(provider, cidr, region, service string) (*Range, error) {
	start, end, version, err := parseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	if version == 6 {
		return nil, errSkipIPv6
	}
	if isReservedV4CIDR(cidr) {
		return nil, errSkipReserved
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
