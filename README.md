# GeoIP

一份开箱可用的 **`Country.mmdb`**，供 Surge / Clash / sing-box 等客户端做 `GEOIP` 规则匹配。

每天自动构建一次，产物有两个去处，订阅地址都永不变化：

- **GitHub Release 附件**（首选）—— 附件不占 git 仓库，且 `releases/latest`
  是 GitHub 的魔法别名，永远指向最新一版
- **`release` 分支**（备用）—— 每次 force-push，永远只有 1 个提交，仓库零膨胀

> Release 的 tag 固定为 `geoip`，**每次构建覆盖同名附件** ——
> 所以这里永远只有一条 Release、一个 tag。
> （2026-09-17 之前是每版打一个时间戳 tag，三年攒了 1409 个，已清理并修正。）

## 订阅地址

首选 GitHub Release（`latest` 别名，永远指向最新）：

```
https://github.com/laincat/GeoIP/releases/latest/download/Country.mmdb
```

备用 GitHub Raw（读 `release` 分支，不限文件体积）：

```
https://github.com/laincat/GeoIP/raw/release/Country.mmdb
```

再备用 jsDelivr。它对单文件有 20MB 上限，产物接近该阈值时可能返回 403，此时请以上面两条为准：

```
https://cdn.jsdelivr.net/gh/laincat/GeoIP@release/Country.mmdb
```

校验产物完整性：

```
https://github.com/laincat/GeoIP/releases/latest/download/Country.mmdb.sha256sum
```

查看本次构建时间：

```
https://github.com/laincat/GeoIP/releases/latest/download/version
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

## 项目结构

```
cmd/geoip/        CLI 入口与子命令：convert / list / lookup / merge
lib/              核心库：配置解析、IP 集合容器、区间写入、取数（超时 + 重试）
plugin/           数据源与格式插件
  ipinfo/         IPInfo 免费 country.csv —— 国家维度
  iptoasn/        iptoasn.com IP-to-ASN 转储 —— 类别维度
  plaintext/      text / json / clash / surge 形式的列表
  special/        private、lookup、stdin、stdout
  maxmind/        mmdb 格式的读与写
config.json       唯一的构建配置：输入来源与覆盖优先级都在这
scripts/          fetch-data.sh 取数脚本 + 两个校验脚本
data/             下载的输入数据（不入库）
output/           构建产物 Country.mmdb（不入库）
```

## 本地构建

```bash
go build -o ./geoip ./cmd/geoip
```

```bash
./geoip convert -c ./config.json
```

配置里的输入路径都指向 `./data/`，所以构建前要先取数。下载逻辑写在脚本里，本地与 CI 调的是同一份，判据不会两头漂移：

```bash
IPINFO_TOKEN=你的token ./scripts/fetch-data.sh
```

脚本会做体积下限与 gzip 完整性双重校验。这两道是必要的 —— IPInfo 取数失败时返回的是 HTTP 200 加一小段错误正文，只看状态码分辨不出来。

产物内容校验（看类别是否真的在、覆盖量是否合理）：

```bash
python3 -m pip install "maxminddb==2.6.1"
```

```bash
python3 ./scripts/verify_mmdb.py ./output/Country.mmdb
```

与源文件逐点比对（抽样三万个源区间，要求每个答案都能由源文件或 `overwriteList` 解释）：

```bash
python3 ./scripts/verify_mmdb_vs_source.py ./output/Country.mmdb
```

## 可复现构建

同一份输入产出**字节级一致**的文件，可以直接用校验和判断产物有没有变化。

`mmdb` 格式里唯一无法由数据决定的是元数据字段 `build_epoch`（构建时间戳）——`mmdbwriter` 默认填 `time.Now()`。本项目读了 `SOURCE_DATE_EPOCH` 这个约定俗成的环境变量来固定它：

```bash
export SOURCE_DATE_EPOCH=$(git log -1 --pretty=%ct)
```

```bash
go build -o ./geoip ./cmd/geoip
```

```bash
./geoip convert -c ./config.json
```

CI 取当前修订的提交时间作为该值，因此**重跑同一个提交不会改变产物校验和**。不设这个变量时行为与之前一致（用当前时间）。

CI 里另有两道校验把这个性质钉死：`mmdbverify`（二进制结构）+ `verify_mmdb_vs_source.py`（内容与源一致）。

## 自动化

每天北京时间 04:30 构建一次，流程是：取数 → 构建 → 三道校验 → 发布。

**发布有两个去处，都不占仓库体积。** Release 附件的 tag 固定为 `geoip`，每次构建覆盖同名附件，所以那里永远只有一条 Release、一个 tag；`release` 分支则是「干净目录 git init + 孤立 + force-push」，永远只有 1 个提交。

**产物没变就不发布。** 构建出的 `Country.mmdb` 与线上那份逐字节相同时（改了文档或 workflow 后 push 是常见场景），后面推分支、发 Release、刷 jsDelivr 三步全部跳过。取不到线上校验和时一律按「有变化」处理 —— 短路是优化，不能变成漏发的原因。

**缓存两层**，都只影响耗时、不影响产物：

| 缓存对象 | 为什么 |
|---|---|
| `~/.cache/go-build` | `setup-go` 只缓存依赖**下载**，编译产物缓存要自己加 |
| `~/go/bin` | 校验工具 `mmdbverify` 装一次能长期复用，key 里带固定版本号 |

输入数据（约 20MB）**刻意不缓存**：全部下载实测只要 1 秒左右，而缓存本身的读写比这更贵，缓存的唯一效果是把 1 秒换成更慢的一步加一个失新鲜的隐患。

## 与上游的关系

代码底座来自 [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip)（GPL-3.0），本项目在其基础上做了这些收敛：

- **国家维度换用 IPInfo**，不再依赖需要 license key、且分发受限的 MaxMind GeoLite2；ASN 维度换用免凭据的 iptoasn.com
- **产物只保留 `Country.mmdb`**，移除了 v2ray dat / sing-box srs / mihomo mrs / text / clash / surge 等全部输出形态，对应的插件代码已从仓库删除
- **新增区间型输入**：数据源按「起止 IP」成对给出，直接以区间写入底层 IP 集合，不再逐行拆成 CIDR
- **可复现构建**：固定 `build_epoch`，同输入产出同字节流
- **取数逻辑收敛到 `lib/fetch.go`**：超时（5 分钟）、重试（3 次，仅 5xx）、固定 User-Agent 只写一遍。上游在不同插件里各写了一份，其中 `http.Get` 用的是零值 client —— 没有超时，源站卡住就会把 CI 挂到 6 小时上限才被 runner 杀掉
- **`Instance` 接口收缩到自己真正被调用的成员**，删掉零调用点的 `ResetInput` 与 `InitConfigFromBytes`

需要说明的是，本项目**不是** [JohnnySun/geoip](https://github.com/JohnnySun/geoip) 的复刻。两者共用 IPInfo 作为国家维度数据源这个思路，但代码库各自独立演进。

## 许可

- 代码：[GPL-3.0](LICENSE-GPL)
- 数据：[CC BY-SA 4.0](LICENSE)
