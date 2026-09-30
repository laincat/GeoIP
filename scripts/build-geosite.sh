#!/usr/bin/env bash
#
# 从 laincat/Rules 仓库拉域名列表，转成 v2fly dlc 数据目录，再用 v2fly
# domain-list-community 的官方工具编译成 geosite.dat。
#
# 这是「方案 A」：geosite 是域名数据，与 geoip 的 IP 数据模型不同，
# 不进 geoip convert 的 Go 管线，而是复用 v2fly 官方构建工具，本脚本只做编排。
#
# 产物输出到 ./output/geosite.dat，分类名：category-ai / ozon / advertising。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/output"
DLC_DIR="${ROOT}/data/dlc"
RULES_BRANCH="${RULES_BRANCH:-main}"
RULES_REPO="https://raw.githubusercontent.com/laincat/Rules/${RULES_BRANCH}"

log() { printf '%s\n' "$*"; }

# 域名列表 → dlc 文件（一行 domain:xxx）。
# 输入是 Surge 的 .list（前导 . 后缀 + 裸域名），这里只取域名，忽略 keyword/IP 行。
convert_list() {
  local src="$1" dst="$2"
  : > "$dst"
  local domain
  while IFS= read -r domain; do
    case "$domain" in
      */*) continue ;;              # 忽略 CIDR
      *:*) continue ;;              # 忽略 classical 规则行（DOMAIN-KEYWORD 等）
      .*) domain="${domain#.}" ;;   # 前导 . = 后缀匹配 → 裸域名
    esac
    # 只保留合法域名：至少一个点，无空白，无通配
    case "$domain" in
      *[!a-zA-Z0-9.-]*) continue ;;
      *..*) continue ;;
      .*|*.) continue ;;
      *) ;;
    esac
    if [ -n "$domain" ]; then
      printf 'domain:%s\n' "$domain" >> "$dst"
    fi
  done < "$src"
}

mkdir -p "$DLC_DIR"
rm -rf "${DLC_DIR:?}"/*

# 拉取三类域名列表并转 dlc 格式。
# AI：用 AI.list（主文件，纯域名）；Ozon：Ozon.list；去广告：Advertising.list。
curl -fsSL --retry 3 "${RULES_REPO}/Surge/Ruleset/AI.list" -o "${DLC_DIR}/ai.raw"
curl -fsSL --retry 3 "${RULES_REPO}/Surge/Ruleset/Ozon.list" -o "${DLC_DIR}/ozon.raw"
curl -fsSL --retry 3 "${RULES_REPO}/Surge/Advertising/Advertising.list" -o "${DLC_DIR}/advertising.raw"

convert_list "${DLC_DIR}/ai.raw" "${DLC_DIR}/category-ai"
convert_list "${DLC_DIR}/ozon.raw" "${DLC_DIR}/ozon"
convert_list "${DLC_DIR}/advertising.raw" "${DLC_DIR}/advertising"

rm -f "${DLC_DIR}"/*.raw

# v2fly 构建工具：clone 官方仓库，跑它的 main.go 编译。
# allowlist 模式挑出三个分类，输出 geosite.dat。
V2FLY_DIR="${ROOT}/data/v2fly-domain-list"
if [ ! -d "$V2FLY_DIR" ]; then
  git clone --depth 1 https://github.com/v2fly/domain-list-community.git "$V2FLY_DIR"
fi

cat > "${DLC_DIR}/geosite.json" <<EOF
[
  {"name": "geosite.dat", "mode": "allowlist", "lists": ["category-ai", "ozon", "advertising"]}
]
EOF

# 把我们的 data 目录作为 datapath 传入（v2fly 的 main.go 默认 ./data）。
# 这里直接在 v2fly 目录里建软链指向我们的 dlc 目录，避免污染官方仓库。
ln -sfn "$DLC_DIR" "${V2FLY_DIR}/data-laincat"

cd "$V2FLY_DIR"
go run . \
  -datapath "$DLC_DIR" \
  -outputname "geosite.dat" \
  -outputdir "$OUT" \
  -datprofile "${DLC_DIR}/geosite.json"

log "✅ geosite.dat 已生成：${OUT}/geosite.dat"