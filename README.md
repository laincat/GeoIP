# GeoIP

一份开箱可用的 **`Country.mmdb`**，供 Surge / Clash / sing-box 等客户端做 `GEOIP` 规则匹配。

每天自动构建一次，产物固定发布在 `release` 分支，订阅地址永不变化。

## 订阅地址

首选 GitHub Raw（不限文件体积）：

```
https://github.com/laincat/GeoIP/raw/release/Country.mmdb
```

备用 jsDelivr。它对单文件有 20MB 上限，产物接近该阈值时可能返回 403，此时请以上面的地址为准：

```
https://cdn.jsdelivr.net/gh/laincat/GeoIP@release/Country.mmdb
```

校验产物完整性：

```
https://github.com/laincat/GeoIP/raw/release/Country.mmdb.sha256sum
```

查看本次构建时间：

```
https://github.com/laincat/GeoIP/raw/release/version
```

## 产物里有什么

`Country.mmdb` 里的每个条目就是一个可供 `GEOIP` 规则引用的标签。它同时包含两个维度：

**国家与地区**——ISO 3166-1 alpha-2 全量国家码，另有一个 `PRIVATE` 覆盖私有与保留地址段。

**服务类别**——这些标签优先于国家码，也就是说某段地址若既属于 `US` 又被归入 `CLOUDFLARE`，最终落库的是 `CLOUDFLARE`：

| 条目 | 含义 | 数据来源 |
|---|---|---|
| `CN` | 中国大陆（国家码与运营商 IP 列表合并） | IPInfo + china-operator-ip |
| `CLOUDFLARE` | Cloudflare | 官方 v4/v6 列表 + ASN |
| `CLOUDFRONT` | Amazon CloudFront | AWS `ip-ranges.json` |
| `FACEBOOK` | Meta / Facebook | ASN |
| `FASTLY` | Fastly | 官方列表 + ASN |
| `GOOGLE` | Google | gstatic 官方列表 + ASN |
| `NETFLIX` | Netflix | ASN |
| `TELEGRAM` | Telegram | 官方 CIDR + ASN |
| `TWITTER` | X / Twitter | ASN |
| `TOR` | Tor 出口节点 | Tor Project 官方列表 |
| `PRIVATE` | 私有、保留、回环、组播地址段 | 内置常量 |

## 数据来源

| 用途 | 来源 | 是否需要凭据 |
|---|---|---|
| 国家码 | [IPInfo](https://ipinfo.io/data) 免费 `country.csv` | 需要一个免费 token（`IPINFO_TOKEN`） |
| 服务类别 | [iptoasn.com](https://iptoasn.com/) IP-to-ASN 转储 | 否 |
| `CN` 补充 | [china-operator-ip](https://github.com/gaoyifan/china-operator-ip) | 否 |
| 各服务精确列表 | Cloudflare / Google / Fastly / AWS / Telegram / Tor 官方端点 | 否 |

选择 IPInfo 而不是 MaxMind GeoLite2 作为国家维度的原因，是它的中国区覆盖更完整：以 chnroutes2 为基准逐段比对，IPInfo 多认了约 6300 万个在中国却未被 chnroutes2 收录的地址（中国电信 / 联通 / 移动的大块网段），反向缺口仅约 41.5 万个地址。

## 构建流程

```
IPInfo country.csv.gz ─┐
iptoasn ip2asn-v4/v6 ──┤
各服务官方 IP 列表 ────┼─→ 合并去重 → Country.mmdb
内置 private 段 ───────┘
```

同一个地址段只会落一份数据，因此条目的写入顺序即优先级：`overwriteList` 中的类别最后写入，覆盖先前的国家码。

## 本地构建

```bash
go build -o ./geoip .
./geoip convert -c ./config.json
```

数据准备（与 CI 保持一致）：

```bash
curl -fsSL "https://ipinfo.io/data/free/country.csv.gz?token=${IPINFO_TOKEN}" -o ./ipinfo/country.csv.gz
curl -fsSL "https://iptoasn.com/data/ip2asn-v4.tsv.gz" -o ./iptoasn/ip2asn-v4.tsv.gz
curl -fsSL "https://iptoasn.com/data/ip2asn-v6.tsv.gz" -o ./iptoasn/ip2asn-v6.tsv.gz
```

产物内容校验（不只看结构，还看类别是否真的在、覆盖量是否合理）：

```bash
python3 -m pip install "maxminddb==2.6.1"
python3 ./tools/verify_mmdb.py ./output/Country.mmdb
```

与源文件逐点比对（抽样三万个源区间，要求每个答案都能由源文件或 `overwriteList` 解释）：

```bash
python3 ./tools/verify_mmdb_vs_source.py ./output/Country.mmdb
```

## 可复现构建

同一份输入产出**字节级一致**的文件，可以直接用校验和判断产物有没有变化。

`mmdb` 格式里唯一无法由数据决定的是元数据字段 `build_epoch`（构建时间戳）——`mmdbwriter` 默认填 `time.Now()`。本项目读了 `SOURCE_DATE_EPOCH` 这个约定俗成的环境变量来固定它：

```
export SOURCE_DATE_EPOCH=$(git log -1 --pretty=%ct)
go build -o ./geoip .
./geoip convert -c ./config.json
```

CI 取当前修订的提交时间作为该值，因此**重跑同一个提交不会改变产物校验和**。不设这个变量时行为与之前一致（用当前时间）。

CI 里另有两道校验把这个性质钉死：`mmdbverify`（二进制结构）+ `verify_mmdb_vs_source.py`（内容与源一致）。

## 与上游的关系

代码底座来自 [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip)（GPL-3.0），本项目在其基础上做了四处收敛：

- **国家维度换用 IPInfo**，不再依赖需要 license key、且分发受限的 MaxMind GeoLite2；ASN 维度换用免凭据的 iptoasn.com
- **产物只保留 `Country.mmdb`**，移除了 v2ray dat / sing-box srs / mihomo mrs / text / clash / surge 等全部输出形态及其插件
- **新增区间型输入**：数据源按「起止 IP」成对给出，直接以区间写入底层 IP 集合，不再逐行拆成 CIDR
- **可复现构建**：固定 `build_epoch`，同输入产出同字节流

需要说明的是，本项目**不是** [JohnnySun/geoip](https://github.com/JohnnySun/geoip) 的复刻。两者共用 IPInfo 作为国家维度数据源这个思路，但代码库各自独立演进。

## 许可

- 代码：[GPL-3.0](LICENSE-GPL)
- 数据：[CC BY-SA 4.0](LICENSE)
