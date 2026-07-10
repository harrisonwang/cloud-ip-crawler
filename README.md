# cloud-ip-crawler

一条命令，把八家云厂商公开的 IP 段抓下来，汇总成一个 SQLite 数据库，用来判断某个 IP 是不是机房 IP。

[![CI](https://github.com/harrisonwang/cloud-ip-crawler/actions/workflows/ci.yml/badge.svg)](https://github.com/harrisonwang/cloud-ip-crawler/actions/workflows/ci.yml)
[![Daily dataset](https://github.com/harrisonwang/cloud-ip-crawler/actions/workflows/daily.yml/badge.svg)](https://github.com/harrisonwang/cloud-ip-crawler/actions/workflows/daily.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

English: [README.en.md](README.en.md)

## 为什么做这个

判断一个 IP 是机房的还是家庭宽带的，是很多事情的第一步。

八家云厂商其实都公开了自己的 IP 段，麻烦的是格式各不相同：AWS 给 JSON，DigitalOcean 给 CSV，Linode 和 Vultr 用 [Geofeed](https://www.rfc-editor.org/rfc/rfc8805.html)，Azure 则藏在一个每周换网址的下载页后面。

这个工具把它们统一抓下来，写进一张表。纯 Go，不需要 CGO，一个二进制文件跑完就得到一个 8.7 MB 的 SQLite 数据库。

## 快速开始

不想自己跑，直接下载每天自动构建的数据库：

```bash
curl -LO https://github.com/harrisonwang/cloud-ip-crawler/releases/download/dataset-latest/cloud-ip.db.gz
gunzip cloud-ip.db.gz
```

自己跑：

```bash
go install github.com/harrisonwang/cloud-ip-crawler/cmd/cloud-ip-crawler@latest
cloud-ip-crawler --db cloud-ip.db
```

数据库文件不存在会自动建好。全量抓一次大约 30 秒，Azure 占了大半时间。

## 用法

```
cloud-ip-crawler [flags]

  --db string          SQLite 文件路径（默认 cloud-ip.db，不存在则自动创建）
  --providers string   逗号分隔的厂商，或 all（默认 all）
  --dry-run            只抓取并打印样例，不写数据库
  --version            打印版本
```

```bash
# 只更新 AWS 和 GCP，其它厂商的数据原样保留
cloud-ip-crawler --providers=aws,gcp

# 先看看能抓到什么，不落库
cloud-ip-crawler --providers=cloudflare --dry-run
```

每家厂商的数据在一个事务里先删后插、整体替换，所以厂商下架的网段不会留在库里。

只要有一个数据源抓失败，程序就以非零状态退出。每天的自动构建靠这个判断要不要发布——宁可今天不更新，也不发一份缺了 Azure 的库。

## 数据源

都是各家自己公开的地址，不需要任何 API key。

| 厂商 | 数据源 | 格式 |
|------|--------|------|
| AWS | `ip-ranges.amazonaws.com/ip-ranges.json` | JSON |
| GCP | `gstatic.com/ipranges/cloud.json` | JSON |
| Azure | Microsoft 下载中心 | JSON |
| Cloudflare | `cloudflare.com/ips-v4` | 纯文本 |
| Oracle | `docs.oracle.com/.../public_ip_ranges.json` | JSON |
| DigitalOcean | `digitalocean.com/geo/google.csv` | CSV |
| Linode | `geoip.linode.com` | Geofeed |
| Vultr | `geofeed.constant.com` | Geofeed |

抓一次的量（2026 年 7 月实测）：

```
azure  43362 │ aws  7835 │ linode 5312 │ oracle 1089
digitalocean 1078 │ gcp 991 │ vultr 439 │ cloudflare 15
────────────────────────────────────────────────────
去重后 60121 条，8.7 MB
```

Azure 的原始数据里有三万多条重复网段（同一个 CIDR 挂在好几个 service / region 标签下），入库时按 `(provider, cidr)` 去重，只留一条。

## 数据表和查询

```sql
CREATE TABLE cloud_ip_ranges (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    provider   VARCHAR(100) NOT NULL,   -- aws / gcp / azure / ...
    cidr       VARCHAR(50)  NOT NULL,   -- 52.94.76.0/22
    ip_start   BLOB NOT NULL,           -- 定长存储，可以直接比大小
    ip_end     BLOB NOT NULL,
    region     VARCHAR(100),
    service    VARCHAR(100),
    ip_version TINYINT DEFAULT 4,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, cidr)
);
```

`ip_start` 和 `ip_end` 按定长存，所以判断一个 IP 在不在某个网段里，就是一次走索引的范围比较：

```python
import sqlite3, ipaddress

db = sqlite3.connect("cloud-ip.db")
key = ipaddress.ip_address("52.94.76.10").packed

row = db.execute(
    "SELECT provider, cidr FROM cloud_ip_ranges "
    "WHERE ip_version = 4 AND ip_start <= ? AND ip_end >= ? LIMIT 1",
    (key, key),
).fetchone()

print(row)   # ('aws', '52.94.76.0/22')
```

## 它能告诉你什么，不能告诉你什么

这一节比上面所有内容都重要。

**查到了**，说明这个 IP 在某家云厂商公开的网段里。这个结论很准，可以直接用。

**查不到，不等于它不是机房 IP。** 只能说明它不在这八家公开的清单里。可能是：

- 它属于别的托管商，比如 Hetzner、OVH、阿里云、腾讯云，这个工具还没覆盖；
- 它属于这八家之一，但厂商没把它写进公开清单。比如 `1.1.1.1` 是 Cloudflare 的公共 DNS，而 `cloudflare.com/ips-v4` 列的是 CDN 网段，所以查不到。

一句话：**厂商公开的是「对外提供服务的网段」，不是「名下所有的 IP」。**

**只收 IPv4。** 各家的 IPv6 格式不统一——AWS 把 v6 单独放在 `ipv6_prefixes` 里，其余几家的解析代码压根没读 v6 字段。与其给一份各家覆盖不一致的数据，不如先不给。

## 后续计划

欢迎 PR，尤其是前两项：

- [ ] **支持 IPv6**：给 AWS 补上 `ipv6_prefixes` 的解析，再逐个核对其余七家的 v6 字段，然后放开 `internal/crawler` 里 `createRange` 跳过 v6 的那一行。
- [ ] **更多厂商**：Hetzner、OVH、Scaleway、阿里云、腾讯云。
- [ ] 导出成 JSON / MMDB。

## 它的来历

原本是 [DetectRadar](https://detectradar.com) 里的一个组件，用来识别机房 IP。DetectRadar 是个网页工具，打开就能看到自己的浏览器有没有在泄露真实 IP，免费、不用注册。

## 许可证

[MIT](LICENSE)
