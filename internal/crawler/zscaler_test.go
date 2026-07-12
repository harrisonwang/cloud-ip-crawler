package crawler

import "testing"

// Zscaler 的层级不固定（cloud→continent→city→[]，外加扁平的 svpnIPs），递归收集器要能：
// 钻到任意深度、从每个 {"range":...} 叶子取 CIDR、并让 IPv6 在 createRange 处被统一跳过。
func TestCollectZscalerRanges(t *testing.T) {
	doc := map[string]any{
		"zscaler.net": map[string]any{
			"continent : EMEA": map[string]any{
				"city : Amsterdam": []any{
					map[string]any{"range": "159.254.250.0/23", "hostname": ""},
					map[string]any{"range": "2a03:eec0:3900::/40"}, // IPv6，应被跳过
				},
			},
		},
		"svpnIPs": []any{ // 顶层的扁平列表，结构与上面不同，也要能收进来
			map[string]any{"range": "165.225.0.0/17"},
		},
	}

	var out []Range
	collectZscalerRanges(doc, "zscaler.net", &out)

	got := map[string]bool{}
	for _, r := range out {
		got[r.CIDR] = true
		if r.Provider != "zscaler" || r.Service != "zscaler.net" {
			t.Errorf("字段未正确填充: %+v", r)
		}
	}

	if len(out) != 2 {
		t.Fatalf("应收集到 2 条 IPv4（v6 被跳过），实际 %d 条: %+v", len(out), out)
	}
	for _, want := range []string{"159.254.250.0/23", "165.225.0.0/17"} {
		if !got[want] {
			t.Errorf("缺少期望的 CIDR %s", want)
		}
	}
}
