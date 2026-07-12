# cloud-ip-crawler

One command to pull the IP ranges of the major clouds, big hosts, and two-thousand-plus hosting providers into a single SQLite database, so you can tell whether an IP belongs to a datacenter.

[![CI](https://github.com/harrisonwang/cloud-ip-crawler/actions/workflows/ci.yml/badge.svg)](https://github.com/harrisonwang/cloud-ip-crawler/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

中文：[README.md](README.md)

## Why

Telling a datacenter IP from a home broadband one is the first step in a lot of things.

Data comes in three tiers, highest confidence first:

1. **Official files** (AWS, GCP, Azure, Cloudflare, Fastly, … 11 of them). The lists the providers publish themselves — most precise, but no two publish them the same way: AWS ships JSON, DigitalOcean ships CSV, Linode and Vultr ship [Geofeeds](https://www.rfc-editor.org/rfc/rfc8805.html), Bunny CDN just ships a list of individual IPs, and Azure hides its file behind a download page whose URL changes every week.
2. **Named ASNs** (Hetzner, OVH, Contabo, Leaseweb, Alibaba Cloud, Tencent Cloud, … 12 of them). They publish no official list, so their currently-announced prefixes are taken from their ASNs.
3. **The `hosting` catch-all**. You can never finish enumerating hosts one by one, so this tier flips the approach: keyword-match **every AS name in the global BGP table** (HOSTING / VPS / CLOUD / SERVER / DATACENTER / IDC…) and pull in the two-thousand-plus ASNs that literally call themselves hosts. The big clouds' backbone ASes (AS16509 AMAZON-02, AS13335 CLOUDFLARENET, …) are explicitly included here too — official lists only cover "ranges we serve traffic from", and this tier catches the famous misses like `1.1.1.1` and `8.8.8.8`.

Tiers 2 and 3 share one download: the full BGP table from [iptoasn.com](https://iptoasn.com/) (a 6.8 MB file, updated daily, sourced from RouteViews) — no per-ASN API calls.

Pure Go, no CGO, one binary, and you end up with a 16 MB SQLite file.

## Quick start

Download the dataset that gets rebuilt daily (SQLite / CSV / MMDB):

```bash
curl -LO https://github.com/harrisonwang/cloud-ip-crawler/releases/download/dataset-latest/cloud-ip.db.gz
gunzip cloud-ip.db.gz

# nginx / HAProxy users: take the MMDB
curl -LO https://github.com/harrisonwang/cloud-ip-crawler/releases/download/dataset-latest/cloud-ip.mmdb.gz
gunzip cloud-ip.mmdb.gz
```

With nginx's [geoip2 module](https://github.com/leev/ngx_http_geoip2_module), three lines get you the verdict:

```nginx
geoip2 /etc/nginx/cloud-ip.mmdb {
    $cloud_provider provider;   # non-empty = datacenter IP; values like aws / hetzner / hosting
    $cloud_tier     tier;       # official / asn / hosting, highest confidence first
}
```

Or run it yourself:

```bash
go install github.com/harrisonwang/cloud-ip-crawler/cmd/cloud-ip-crawler@latest
cloud-ip-crawler --db cloud-ip.db
```

The database is created if it doesn't exist. A full crawl takes about 30 seconds, most of it Azure.

Look up an IP without writing SQL:

```bash
$ cloud-ip-crawler lookup 1.1.1.1
hosting  1.1.1.0/24  region=US  service=AS13335 CLOUDFLARENET

$ cloud-ip-crawler lookup 52.94.76.10
aws  52.94.76.0/22  region=us-west-2  service=AMAZON
hosting  52.94.72.0/21  region=US  service=AS16509 AMAZON-02
```

Exit code 0 on a hit, 1 on a miss — usable directly in scripts. One IP may list several owners (an official-list row plus a `hosting` catch-all row); trust the former.

## Usage

```
cloud-ip-crawler [flags]           crawl
cloud-ip-crawler lookup <ip>       look up one IP
cloud-ip-crawler export [flags]    export MMDB (--out cloud-ip.mmdb)

  --db string          SQLite path (default cloud-ip.db, created on crawl if missing)
  --providers string   Comma-separated providers, or all (default all)
  --dry-run            Fetch and print samples, don't write
  --version            Print version
```

The MMDB keeps only the highest-confidence attribution per IP (official > asn > hosting; the `tier` field says which layer answered); for all attributions use SQLite + `lookup`.

Each provider is replaced wholesale inside one transaction — delete, then insert — so ranges a provider has withdrawn don't linger.

If any source fails, the process exits non-zero. The daily build uses that to decide whether to publish: better to skip a day than to ship a database missing Azure.

## Data sources

All are public sources. No API key needed.

**1. Official lists the providers publish themselves**

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
| Fastly | `api.fastly.com/public-ip-list` | JSON |
| Bunny CDN | `bunnycdn.com/api/system/edgeserverlist` | JSON (list of single IPs) |
| Zscaler | `config.zscaler.com/api/<cloud>/cenr/json` | JSON |

**2. Big hosts / VPS providers, via named ASNs**

These publish no official file, so their announced prefixes are taken from their ASNs. The ASN list lives in [`internal/crawler/asn.go`](internal/crawler/asn.go); every number was verified against RIPEstat's as-overview holder.

| Provider | ASN |
|----------|-----|
| Hetzner | 24940, 213230, 212317 |
| OVH | 16276 |
| Contabo | 51167 |
| Leaseweb | 60781, 30633, 7203, 19148, 396190 |
| G-Core | 199524 |
| Scaleway | 12876 |
| netcup | 197540 |
| IBM SoftLayer | 36351 |
| Frantech / BuyVM | 53667 |
| HostHatch | 63473 |
| Alibaba Cloud | 45102, 37963 |
| Tencent Cloud | 132203, 45090 |

Mind the semantic difference: an ASN gives you *every prefix that AS announces* — coarser than an official service list, and it can include a provider's own or bring-your-own (BYOIP) ranges. For "is this a datacenter IP", erring on the broad side is fine.

**3. The `hosting` catch-all (heuristic)**

Every AS name in the iptoasn table is keyword-matched (HOSTING / VPS / VDS / CLOUD / SERVER / DATACENTER / DEDICATED / COLOCATION / IDC), pulling in 2,400+ ASNs that call themselves hosts — including the datacenter arms of the Chinese telcos (`CHINANET-IDC-BJ` and friends). Rows get `provider = hosting` and `service = "AS<number> <name>"` for traceability.

The big clouds' backbone ASes carry no keyword (there's nothing in "AMAZON-02"), so they're explicitly listed in [`internal/crawler/iptoasn.go`](internal/crawler/iptoasn.go): AS16509 / AS14618 (Amazon), AS8075 (Microsoft), AS15169 / AS396982 (Google), AS13335 (Cloudflare), AS31898 (Oracle), AS14061 (DigitalOcean), AS63949 (Linode/Akamai), AS20473 (Vultr), AS54113 (Fastly) — each holder-verified.

This tier is heuristic and lower-confidence than the other two: a few false positives (small ISPs with VPS/CLOUD in their names — 3 suspicious out of 2,452 ASNs measured), and false negatives (hosts named after their founder). Skip it by leaving `hosting` out of `--providers`.

One full crawl, measured July 2026: **114,575 ranges, 16 MB, ~30 seconds.** Azure alone is 43,310 of them, and its source lists about 35k duplicate CIDRs — the same network tagged under several services and regions — deduplicated on `(provider, cidr)`. Zscaler is the same story: the same node range recurs across dozens of cities and several clouds, so its 3,282 fetched rows collapse to 572. ASN-route counts look small (Hetzner is just 126 rows) because iptoasn aggregates adjacent same-ASN ranges — the address coverage is unchanged.

Upstream data can't be trusted blindly: Vultr's geofeed ships three RFC 5737 test networks (`192.0.2.0/24` and friends). Special-purpose ranges (private, TEST-NET, multicast, …) are dropped before they reach the database.

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

**A hit** — read the provider for confidence:

- **Official-file providers** (`aws`, `gcp`, …): the provider itself says this range is theirs. Most precise, comes with region/service labels — use directly.
- **Named-ASN providers** (`hetzner`, `ovh`, …): the BGP table says they announce it. Very reliable, though it may include their own or BYOIP ranges.
- **`hosting` only**: an AS whose name looks like a host announces it. A strong "datacenter IP" signal, but the classification is heuristic — check the AS name in `service` before treating it as a provider attribution.

The famous misses of official lists are now caught: `1.1.1.1` (Cloudflare's public DNS, absent from `ips-v4`) and `8.8.8.8` (Google's public DNS) both resolve via the `hosting` tier.

**A miss still doesn't prove "not a datacenter"**, but the odds are now low. What remains uncovered: small hosts with no industry word in their AS name (the founder's-name kind), corporate on-prem datacenters, and traffic behind residential proxies — the last one was never solvable with range lists anyway.

**IPv4 only.** The IPv6 sources aren't consistent — AWS keeps v6 under a separate `ipv6_prefixes` key, and most of the parsers here never read a v6 field at all. Shipping no v6 beats shipping v6 with uneven coverage.

## Roadmap

PRs welcome, especially the first two:

- [ ] **IPv6** — audit each official source's v6 fields and lift the skip in `createRange` (several parsers already read v6; it's centrally dropped). For the ASN route, iptoasn ships `ip2asn-v6.tsv.gz` ready to go — just lift the v4 restriction in `rangeToCIDRs`.
- [ ] **Tune the hosting keywords** — false positive/negative reports welcome as issues; the keywords and explicit include list live in `iptoasn.go`, one-line changes.
- [ ] **More named hosts** — append a line in `asn.go` (Huawei Cloud, Kimsufi, RackNerd, …), verify the holder, send a PR.
- [x] ~~MMDB export~~ — the `export` subcommand; the daily Release ships `cloud-ip.mmdb.gz`.
- [ ] JSON export.

## Where it came from

Extracted from [DetectRadar](https://detectradar.com), where it feeds the datacenter-IP classifier. DetectRadar is a web page that shows you whether your browser is leaking your real IP. Free, no signup.

## License

[MIT](LICENSE)
