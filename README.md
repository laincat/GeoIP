# GeoIP

开箱可用的 IP 与域名地理数据，覆盖 mihomo 的四个字段：`geoip`、`mmdb`、`asn`、`geosite`，同时兼容 Surge / Clash / sing-box。

每天自动构建一次，产物发布到 GitHub Release 附件（`releases/latest` 是 GitHub 的魔法别名，永远指向最新一版），并同步到 CNB 镜像。

> Release 的 tag 固定为 `geoip`，**每次构建覆盖同名附件** —— 所以这里永远只有一条 Release、一个 tag。
> （2026-09-17 之前是每版打一个时间戳 tag，三年攒了 1409 个，已清理并修正。）

## 四个产物

| 文件 | 对应字段 | 数据内容 |
|---|---|---|
| `Country.mmdb` | mihomo `mmdb` / Surge `GEOIP` | 国家码 + 服务类别（CN / CLOUDFLARE / CLOUDFRONT / FACEBOOK / FASTLY / GOOGLE / NETFLIX / TELEGRAM / TWITTER / TOR / PRIVATE） |
| `geoip.dat` | mihomo `geoip`（v2ray 容器） | 同源 IP 数据的 v2ray GeoIP 格式 |
| `GeoLite2-ASN.mmdb` | mihomo `asn` | iptoasn 全量 ASN（ASN 号 + 组织名） |
| `geosite.dat` | mihomo `geosite` | 域名数据（category-ai / ozon / advertising），来自 [laincat/Rules](https://github.com/laincat/Rules) |

`Country.mmdb` 与 `geoip.dat` 是同一份 IP 数据的两种容器；`GeoLite2-ASN.mmdb` 是独立的 ASN 查询库；`geosite.dat` 是域名数据，与 IP 完全无关。

## 订阅地址

所有文件走 `releases/latest/download/`，把文件名换成任意产物即可，地址永不变化：

```
https://github.com/laincat/GeoIP/releases/latest/download/Country.mmdb
https://github.com/laincat/GeoIP/releases/latest/download/geoip.dat
https://github.com/laincat/GeoIP/releases/latest/download/GeoLite2-ASN.mmdb
https://github.com/laincat/GeoIP/releases/latest/download/geosite.dat
```

校验和与版本号：

```
https://github.com/laincat/GeoIP/releases/latest/download/artifacts.sha256sum
https://github.com/laincat/GeoIP/releases/latest/download/version
```

备用链路：`release` 分支（jsDelivr 读的分支）与 jsDelivr。jsDelivr 对单文件有 20MB 上限，`Country.mmdb` 接近该阈值时可能 403，此时以上面的 Release 地址为准：

```
https://cdn.jsdelivr.net/gh/laincat/GeoIP@release/Country.mmdb
```

CNB 镜像（国内直连更友好）：

```
https://cnb.cool/laincat/GeoIP/-/raw/main/Country.mmdb
https://cnb.cool/laincat/GeoIP/-/raw/main/geoip.dat
https://cnb.cool/laincat/GeoIP/-/raw/main/GeoLite2-ASN.mmdb
https://cnb.cool/laincat/GeoIP/-/raw/main/geosite.dat
```

> CNB 与 GitHub 两侧都用 `main` 分支，内容一致。

mihomo 配置里这样引：

```yaml
geodata-mode: true
geo-auto-update: true
geox-url:
  mmdb: "https://github.com/laincat/GeoIP/releases/latest/download/Country.mmdb"
  geoip: "https://github.com/laincat/GeoIP/releases/latest/download/geoip.dat"
  asn: "https://github.com/laincat/GeoIP/releases/latest/download/GeoLite2-ASN.mmdb"
  geosite: "https://github.com/laincat/GeoIP/releases/latest/download/geosite.dat"
```

## 每个产物里的条目

四个产物各自的标签（条目）如下。`geoip.dat` 与 `Country.mmdb` 是同一份 IP 数据的两种容器，标签集相同；`GeoLite2-ASN.mmdb` 与 `geosite.dat` 各自独立。

### Country.mmdb / geoip.dat

每个条目就是一个可供 `GEOIP` 规则引用的标签，含两个维度：

- **国家与地区**：ISO 3166-1 alpha-2 全量国家码（如 `US`、`JP`、`DE` …），另有 `PRIVATE` 覆盖私有与保留地址段。
- **服务类别**：这些标签优先于国家码，某段地址若既属于 `US` 又被归入 `CLOUDFLARE`，最终落库的是 `CLOUDFLARE`。

| 条目 | 含义 | 数据来源 |
|---|---|---|
| 全部 ISO 国家码（`CN` / `US` / `JP` / `DE` …） | 国家与地区 | IPInfo 免费 `country.csv` |
| `CN` | 中国大陆（国家码与运营商 IP 列表合并） | IPInfo + [china-operator-ip](https://github.com/gaoyifan/china-operator-ip) |
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

### GeoLite2-ASN.mmdb

结构兼容 MaxMind GeoLite2-ASN，每个 IP 段返回两个字段（mihomo 的 `asn` 字段直接可读）：

| 字段 | 含义 | 数据来源 |
|---|---|---|
| `autonomous_system_number` | ASN 号（如 `13335`） | [iptoasn.com](https://iptoasn.com/) 全量 IP-to-ASN 转储 |
| `autonomous_system_organization` | ASN 组织名（如 `CLOUDFLARENET`） | 同上 |

### geosite.dat

域名数据（来自 [laincat/Rules](https://github.com/laincat/Rules)），三个分类：

| 分类 | 含义 | 数据来源 |
|---|---|---|
| `category-ai` | AI 服务域名 | Rules 的 `Surge/Ruleset/AI.list` |
| `ozon` | Ozon 电商域名 | Rules 的 `Surge/Ruleset/Ozon.list` |
| `advertising` | 广告 / 追踪域名 | Rules 的 `Surge/Advertising/Advertising.list` |

## 数据来源

| 用途 | 来源 | 凭据 |
|---|---|---|
| 国家码 | [IPInfo](https://ipinfo.io/data) 免费 `country.csv` | 需免费 token（`IPINFO_TOKEN`） |
| 服务类别 + ASN | [iptoasn.com](https://iptoasn.com/) IP-to-ASN 转储 | 否 |
| `CN` 补充 | [china-operator-ip](https://github.com/gaoyifan/china-operator-ip) | 否 |
| 各服务精确列表 | Cloudflare / Google / Fastly / AWS / Telegram / Tor 官方端点 | 否 |
| geosite 域名 | [laincat/Rules](https://github.com/laincat/Rules) 的 AI / Ozon / Advertising 列表 | 否 |

选 IPInfo 而非 MaxMind GeoLite2 做国家维度，是因为它的中国区覆盖更完整：以 chnroutes2 为基准逐段比对，IPInfo 多认约 6300 万个在中国却未被 chnroutes2 收录的地址（三大运营商的大块网段），反向缺口仅约 41.5 万个地址。

## 构建流程

```
IPInfo country.csv.gz ─┐
iptoasn ip2asn-v4/v6 ──┤                          ┌→ Country.mmdb
各服务官方 IP 列表 ────┼─→ 合并去重（同一个 Container）─→ geoip.dat
内置 private 段 ───────┘                          └→ GeoLite2-ASN.mmdb（独立 Container）

Rules 的域名列表 ──→ dlc 数据 ──→ v2fly 官方工具 ──→ geosite.dat
```

同一个地址段在 mmdb 里只落一份数据，写入顺序即优先级：`overwriteList` 中的类别最后写入，覆盖先前的国家码。

全量 ASN（约 10 万个）走独立的 `config-asn.json` 单独构建 —— 与 Country 共用 Container 会把 `ASxxxx` 条目当国家码混进库，必须隔离。geosite 是域名数据，走独立的 `scripts/build-geosite.sh`，复用 v2fly 官方构建工具，不进 `geoip convert` 的 Go 管线。

## 项目结构

```
cmd/geoip/        CLI 入口与子命令：convert / list / lookup / merge
lib/              核心库：配置解析、IP 集合容器、区间写入、取数（超时 + 重试）
plugin/           数据源与格式插件
  ipinfo/         IPInfo 免费 country.csv —— 国家维度
  iptoasn/        iptoasn.com IP-to-ASN —— 类别维度 + 全量 ASN
  plaintext/      text / json / clash / surge 形式的列表
  special/        private、lookup、stdin、stdout
  maxmind/        mmdb 的读与写（Country + ASN）
  v2ray/          geoip.dat 的 v2ray protobuf 读写
config.json       Country.mmdb + geoip.dat 的构建配置
config-asn.json   GeoLite2-ASN.mmdb 的构建配置（独立，避免 AS 条目污染国家维度）
scripts/          fetch-data.sh 取数 + build-geosite.sh 编译 + 两个校验脚本
data/             下载的输入数据（不入库）
output/           构建产物（不入库）
```

## 本地构建

```bash
go build -o ./geoip ./cmd/geoip
```

配置里的输入路径都指向 `./data/`，构建前先取数。下载逻辑在脚本里，本地与 CI 调同一份，判据不会漂移：

```bash
IPINFO_TOKEN=你的token ./scripts/fetch-data.sh
```

三个 IP 产物一次构建：

```bash
./geoip convert -c ./config.json      # Country.mmdb + geoip.dat
./geoip convert -c ./config-asn.json  # GeoLite2-ASN.mmdb
```

geosite 需要 v2fly 官方工具（首次会 clone + 编译）：

```bash
bash ./scripts/build-geosite.sh
```

取数脚本会做体积下限与 gzip 完整性双重校验 —— IPInfo 取数失败时返回 HTTP 200 加一小段错误正文，只看状态码分辨不出来。

产物内容校验（看类别是否真在、覆盖量是否合理）：

```bash
python3 -m pip install "maxminddb==2.6.1"
python3 ./scripts/verify_mmdb.py ./output/Country.mmdb
```

与源文件逐点比对（抽样三万个源区间，要求每个答案都能由源文件或 `overwriteList` 解释）：

```bash
python3 ./scripts/verify_mmdb_vs_source.py ./output/Country.mmdb --csv ./data/ipinfo/country.csv.gz
```

## 可复现构建

同一份输入产出字节级一致的文件，可直接用校验和判断产物有没有变化。

`mmdb` 格式里唯一无法由数据决定的是 `build_epoch`（构建时间戳），`mmdbwriter` 默认填 `time.Now()`。本项目读 `SOURCE_DATE_EPOCH` 环境变量固定它：

```bash
export SOURCE_DATE_EPOCH=$(git log -1 --pretty=%ct)
```

CI 取当前修订的提交时间作为该值，因此重跑同一个提交不会改变产物校验和。不设该变量时行为与之前一致（用当前时间）。CI 里另有 `mmdbverify`（二进制结构）+ `verify_mmdb_vs_source.py`（内容与源一致）两道校验钉死这个性质。

## 自动化

每天北京时间 04:30 构建一次，流程是：取数 → 构建四个产物 → 三道校验 → 发布。

**发布到两个去处，都不占仓库体积。** Release 附件的 tag 固定为 `geoip`，每次覆盖同名附件，永远只有一条 Release、一个 tag；`release` 分支则是「干净目录 git init + 孤立 + force-push」，永远只有 1 个提交，供 jsDelivr 读。仓库另有 CNB 镜像同步。

**产物没变就不发布。** 构建出的 `Country.mmdb` 与线上那份逐字节相同时（改了文档或 workflow 后 push 是常见场景），后面推分支、发 Release、刷 jsDelivr 三步全部跳过。取不到线上校验和时一律按「有变化」处理 —— 短路是优化，不能变成漏发的原因。

## 与上游的关系

代码底座来自 [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip)（GPL-3.0），本项目在其基础上做了这些收敛：

- **国家维度换用 IPInfo**，不再依赖需要 license key、且分发受限的 MaxMind GeoLite2；ASN 维度换用免凭据的 iptoasn.com
- **产物聚焦四个格式**：`Country.mmdb` / `geoip.dat` / `GeoLite2-ASN.mmdb` / `geosite.dat`，移除了 sing-box srs / mihomo mrs / text / clash / surge 等其余输出形态
- **新增区间型输入**：数据源按「起止 IP」成对给出，直接以区间写入底层 IP 集合，不再逐行拆成 CIDR
- **可复现构建**：固定 `build_epoch`，同输入产出同字节流
- **取数逻辑收敛到 `lib/fetch.go`**：超时（5 分钟）、重试（3 次，仅 5xx）、固定 User-Agent 只写一遍。上游在不同插件里各写了一份，其中 `http.Get` 用的是零值 client —— 没有超时，源站卡住就会把 CI 挂到 6 小时上限才被 runner 杀掉
- **`Instance` 接口收缩到自己真正被调用的成员**，删掉零调用点的 `ResetInput` 与 `InitConfigFromBytes`

需要说明的是，本项目**不是** [JohnnySun/geoip](https://github.com/JohnnySun/geoip) 的复刻。两者共用 IPInfo 作为国家维度数据源这个思路，但代码库各自独立演进。

## 许可

- 代码：[GPL-3.0](LICENSE-GPL)
- 数据：[CC BY-SA 4.0](LICENSE)