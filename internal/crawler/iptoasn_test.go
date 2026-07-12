package crawler

import (
	"net/netip"
	"strings"
	"testing"
)

func TestRangeToCIDRs(t *testing.T) {
	cases := []struct {
		start, end string
		want       []string
	}{
		// 对齐的整段
		{"10.0.0.0", "10.0.0.255", []string{"10.0.0.0/24"}},
		// 单个地址
		{"10.0.0.1", "10.0.0.1", []string{"10.0.0.1/32"}},
		// 不对齐：跨对齐边界要拆多段
		{"10.0.0.1", "10.0.0.2", []string{"10.0.0.1/32", "10.0.0.2/32"}},
		{"1.0.0.0", "1.0.1.255", []string{"1.0.0.0/23"}},
		{"1.0.0.0", "1.0.2.255", []string{"1.0.0.0/23", "1.0.2.0/24"}},
		// 全空间（s==0 的低位技巧边界）
		{"0.0.0.0", "255.255.255.255", []string{"0.0.0.0/0"}},
		// start > end 是非法区间，应得到空
		{"10.0.0.2", "10.0.0.1", nil},
	}
	for _, c := range cases {
		got := rangeToCIDRs(netip.MustParseAddr(c.start), netip.MustParseAddr(c.end))
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("rangeToCIDRs(%s, %s) = %v, want %v", c.start, c.end, got, c.want)
		}
	}
}

func TestIsHostingDesc(t *testing.T) {
	positive := []string{
		"ABC-HOSTING Ltd",
		"SUPERVPS NETWORK", // 关键词嵌在合成词里，子串匹配才抓得到
		"MYCLOUD-AS Something",
		"CLOUDFLARENET",
		"XYZ DEDICATED SERVERS",
		"FOO COLOCATION GMBH",
		"CHINANET-IDC-BJ", // IDC 作为独立 token
		"ACME DATA CENTER SOLUTIONS",
	}
	for _, d := range positive {
		if !isHostingDesc(d) {
			t.Errorf("应判为托管商: %q", d)
		}
	}

	// 误伤防线：SERVICES 不含 SERVER，COLOMBIA 不含 COLOC，
	// MIDCONTINENT 含 IDC 但不是独立 token（家宽 ISP，绝不能进来）
	negative := []string{
		"COMCAST-7922 - Comcast Cable",
		"COLOMBIA-TELECOM",
		"TELECOM SERVICES SA",
		"CHINANET-BACKBONE",
		"VODAFONE-BROADBAND",
		"MIDCONTINENT-COMMUNICATIONS",
	}
	for _, d := range negative {
		if isHostingDesc(d) {
			t.Errorf("不该判为托管商: %q", d)
		}
	}
}

func TestParseIPToASN(t *testing.T) {
	tsv := strings.Join([]string{
		// 命名厂商（hetzner AS24940）
		"5.9.0.0\t5.9.255.255\t24940\tDE\tHETZNER-AS Hetzner Online GmbH",
		// 关键词命中 → hosting
		"192.0.2.0\t192.0.2.255\t65001\tUS\tEXAMPLE-HOSTING Ltd",
		// 显式清单命中（Cloudflare 骨干 AS）→ hosting
		"1.1.1.0\t1.1.1.255\t13335\tUS\tCLOUDFLARENET",
		// 无关 ASN，跳过
		"198.51.100.0\t198.51.100.255\t65002\tUS\tSOME-TELECOM",
		// AS0（未宣告空洞），跳过
		"203.0.113.0\t203.0.113.255\t0\tNone\tNot routed",
	}, "\n")

	ds, err := parseIPToASN(strings.NewReader(tsv))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	hz := ds.byProvider["hetzner"]
	if len(hz) != 1 || hz[0].CIDR != "5.9.0.0/16" || hz[0].Service != "AS24940" || hz[0].Region != "DE" {
		t.Errorf("hetzner 解析不对: %+v", hz)
	}

	if len(ds.hosting) != 2 {
		t.Fatalf("hosting 应有 2 条（关键词 1 + 显式清单 1），实际 %d: %+v", len(ds.hosting), ds.hosting)
	}
	for _, r := range ds.hosting {
		if r.Provider != "hosting" || !strings.HasPrefix(r.Service, "AS") {
			t.Errorf("hosting 字段不对: %+v", r)
		}
	}
}
