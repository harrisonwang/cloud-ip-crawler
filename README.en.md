# cloud-ip-crawler

One command to pull the published IP ranges of eight cloud providers into a single SQLite database, so you can tell whether an IP belongs to a datacenter.

[![CI](https://github.com/harrisonwang/cloud-ip-crawler/actions/workflows/ci.yml/badge.svg)](https://github.com/harrisonwang/cloud-ip-crawler/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

中文：[README.md](README.md)

## Why

Telling a datacenter IP from a home broadband one is the first step in a lot of things.

All eight providers do publish their ranges. The annoying part is that no two publish them the same way: AWS ships JSON, DigitalOcean ships CSV, Linode and Vultr ship [Geofeeds](https://www.rfc-editor.org/rfc/rfc8805.html), and Azure hides its file behind a download page whose URL changes every week.

This tool fetches all of them into one table. Pure Go, no CGO, one binary, and you end up with an 8.7 MB SQLite file.

## Quick start

Download the dataset that gets rebuilt daily:

```bash
curl -LO https://github.com/harrisonwang/cloud-ip-crawler/releases/download/dataset-latest/cloud-ip.db.gz
gunzip cloud-ip.db.gz
```

Or run it yourself:

```bash
go install github.com/harrisonwang/cloud-ip-crawler/cmd/cloud-ip-crawler@latest
cloud-ip-crawler --db cloud-ip.db
```

The database is created if it doesn't exist. A full crawl takes about 30 seconds, most of it Azure.

## Usage

```
cloud-ip-crawler [flags]

  --db string          SQLite path (default cloud-ip.db, created if missing)
  --providers string   Comma-separated providers, or all (default all)
  --dry-run            Fetch and print samples, don't write
  --version            Print version
```

Each provider is replaced wholesale inside one transaction — delete, then insert — so ranges a provider has withdrawn don't linger.

If any source fails, the process exits non-zero. The daily build uses that to decide whether to publish: better to skip a day than to ship a database missing Azure.

## Data sources

All are addresses the providers publish themselves. No API key needed.

| Provider | Source | Format |
|----------|--------|--------|
| AWS | `ip-ranges.amazonaws.com/ip-ranges.json` | JSON |
| GCP | `gstatic.com/ipranges/cloud.json` | JSON |
| Azure | Microsoft Download Center | JSON |
| Cloudflare | `cloudflare.com/ips-v4` | Text |
| Oracle | `docs.oracle.com/.../public_ip_ranges.json` | JSON |
| DigitalOcean | `digitalocean.com/geo/google.csv` | CSV |
| Linode | `geoip.linode.com` | Geofeed |
| Vultr | `geofeed.constant.com` | Geofeed |

One full crawl, measured July 2026: **60,121 ranges after dedup, 8.7 MB.** Azure alone is 43,362 of them, and its source lists about 35k duplicate CIDRs — the same network tagged under several services and regions — deduplicated on `(provider, cidr)`.

## Schema and lookups

`ip_start` and `ip_end` are fixed-width blobs, so checking whether an IP falls inside a range is an indexed range comparison:

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

## What it can and can't tell you

This section matters more than everything above.

**A hit** means the IP sits inside a range that provider publishes. That's reliable — use it directly.

**A miss does not mean "not a datacenter".** It only means the IP isn't in these eight published lists. It might be:

- another host entirely — Hetzner, OVH, Alibaba Cloud, Tencent Cloud — none of which this tool covers yet;
- one of the eight, but an address the provider never published. `1.1.1.1` is Cloudflare's public DNS, while `cloudflare.com/ips-v4` lists CDN ranges, so it's a miss.

In short: **providers publish the ranges they serve traffic from, not every IP they own.**

**IPv4 only.** The IPv6 sources aren't consistent — AWS keeps v6 under a separate `ipv6_prefixes` key, and most of the parsers here never read a v6 field at all. Shipping no v6 beats shipping v6 with uneven coverage.

## Roadmap

PRs welcome, especially the first two:

- [ ] **IPv6** — parse `ipv6_prefixes` for AWS, audit the v6 fields of the other seven, then lift the skip in `createRange` under `internal/crawler`.
- [ ] **More providers** — Hetzner, OVH, Scaleway, Alibaba Cloud, Tencent Cloud.
- [ ] JSON / MMDB export.

## Where it came from

Extracted from [DetectRadar](https://detectradar.com), where it feeds the datacenter-IP classifier. DetectRadar is a web page that shows you whether your browser is leaking your real IP. Free, no signup.

## License

[MIT](LICENSE)
