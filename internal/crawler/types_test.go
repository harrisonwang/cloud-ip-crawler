package crawler

import (
	"errors"
	"net"
	"testing"
)

func TestParseCIDR(t *testing.T) {
	cases := []struct {
		cidr       string
		start, end string
		version    int
	}{
		{"52.94.76.0/22", "52.94.76.0", "52.94.79.255", 4},
		{"1.2.3.4/32", "1.2.3.4", "1.2.3.4", 4},
		{"10.0.0.0/8", "10.0.0.0", "10.255.255.255", 4},
		{"2606:4700::/32", "2606:4700::", "2606:4700:ffff:ffff:ffff:ffff:ffff:ffff", 6},
	}
	for _, c := range cases {
		start, end, version, err := parseCIDR(c.cidr)
		if err != nil {
			t.Fatalf("parseCIDR(%q): %v", c.cidr, err)
		}
		if !start.Equal(net.ParseIP(c.start)) || !end.Equal(net.ParseIP(c.end)) || version != c.version {
			t.Errorf("parseCIDR(%q) = %v..%v v%d, want %s..%s v%d", c.cidr, start, end, version, c.start, c.end, c.version)
		}
	}
	if _, _, _, err := parseCIDR("not-a-cidr"); err == nil {
		t.Error("parseCIDR 对非法输入应返回错误")
	}
}

// 八个爬虫都经 createRange 收口，IPv6 必须在此被一致地跳过：
// 各家数据源的 v6 覆盖不齐（AWS 的 v6 在独立的 ipv6_prefixes 键里，多数解析器根本没读），
// 放行会得到各厂商覆盖不一致的库。
func TestCreateRangeSkipsIPv6(t *testing.T) {
	if _, err := createRange("cloudflare", "2606:4700::/32", "", "cdn"); !errors.Is(err, errSkipIPv6) {
		t.Errorf("IPv6 CIDR 应返回 errSkipIPv6，实际: %v", err)
	}
	// Vultr 的 geofeed 真把 TEST-NET 当数据发过，特殊用途网段必须在入库前被丢弃
	for _, cidr := range []string{"192.0.2.0/24", "198.51.100.0/24", "10.0.0.0/8", "192.0.2.128/25"} {
		if _, err := createRange("vultr", cidr, "", ""); !errors.Is(err, errSkipReserved) {
			t.Errorf("保留网段 %s 应返回 errSkipReserved，实际: %v", cidr, err)
		}
	}
	r, err := createRange("aws", "52.94.76.0/22", "us-east-1", "EC2")
	if err != nil {
		t.Fatalf("IPv4 CIDR 不应报错: %v", err)
	}
	if r.IPVersion != 4 || r.Provider != "aws" || r.Region != "us-east-1" {
		t.Errorf("字段未正确填充: %+v", r)
	}
}

// ip_start / ip_end 必须定长，否则 SQLite 里 ip_start <= x <= ip_end 的 BLOB 比较会错
func TestIPToBytesFixedWidth(t *testing.T) {
	if got := len(ipToBytes(net.ParseIP("1.2.3.4"))); got != 4 {
		t.Errorf("IPv4 应编码为 4 字节，实际 %d", got)
	}
	if got := len(ipToBytes(net.ParseIP("2606:4700::"))); got != 16 {
		t.Errorf("IPv6 应编码为 16 字节，实际 %d", got)
	}
}
