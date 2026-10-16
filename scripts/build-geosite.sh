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

# Exact entries remain full:, leading-dot suffix entries become domain:.
convert_list() {
  python3 "${ROOT}/scripts/convert_domains.py" "$1" "$2"
}

# Resolve one Rules revision for all three lists; keep daily data updates.
if [ "${1:-}" != "--reuse-inputs" ]; then
  RULES_BRANCH=$(git ls-remote https://github.com/laincat/Rules.git "$RULES_BRANCH" | awk 'NR==1 {print $1}')
  [[ "$RULES_BRANCH" =~ ^[0-9a-f]{40}$ ]] || { echo "Cannot resolve Rules revision" >&2; exit 1; }
  RULES_REPO="https://raw.githubusercontent.com/laincat/Rules/${RULES_BRANCH}"
mkdir -p "$DLC_DIR" "${ROOT}/data/geosite-raw"
rm -rf "${DLC_DIR:?}"/*

# 拉取三类域名列表并转 dlc 格式。
# AI：用 AI.list（主文件，纯域名）；Ozon：Ozon.list；
# 去广告：Advertising.Reject.list（2026-10 分片重构后的纯域名主库，
# 旧的合并版 Advertising.list 已删除）。
curl -fsSL --retry 3 "${RULES_REPO}/Surge/Ruleset/AI.list" -o "${ROOT}/data/geosite-raw/ai.raw"
curl -fsSL --retry 3 "${RULES_REPO}/Surge/Ruleset/Ozon.list" -o "${ROOT}/data/geosite-raw/ozon.raw"
curl -fsSL --retry 3 "${RULES_REPO}/Surge/Advertising/Advertising.Reject.list" -o "${ROOT}/data/geosite-raw/advertising.raw"

convert_list "${ROOT}/data/geosite-raw/ai.raw" "${DLC_DIR}/category-ai"
convert_list "${ROOT}/data/geosite-raw/ozon.raw" "${DLC_DIR}/ozon"
convert_list "${ROOT}/data/geosite-raw/advertising.raw" "${DLC_DIR}/advertising"

mkdir -p "${ROOT}/output"
printf '%s\n' "$RULES_BRANCH" > "${ROOT}/data/rules-revision"
fi


# v2fly 构建工具：clone 官方仓库，跑它的 main.go 编译。
# allowlist 模式挑出三个分类，输出 geosite.dat。
V2FLY_DIR="${ROOT}/data/v2fly-domain-list"
# Verified upstream revision. Dependencies are locked by its go.mod/go.sum.
DLC_REVISION=dd4e3c080cee602512768e8109f89fafaa94b8ca
if [ ! -d "$V2FLY_DIR/.git" ]; then
  git init -q "$V2FLY_DIR"
  git -C "$V2FLY_DIR" remote add origin https://github.com/v2fly/domain-list-community.git
fi
if ! git -C "$V2FLY_DIR" cat-file -e "${DLC_REVISION}^{commit}" 2>/dev/null; then
  git -C "$V2FLY_DIR" fetch --depth 1 origin "$DLC_REVISION"
fi
git -C "$V2FLY_DIR" checkout --detach "$DLC_REVISION"
[ "$(git -C "$V2FLY_DIR" rev-parse HEAD)" = "$DLC_REVISION" ]

# profile 文件必须放在 datapath 目录之外，否则 v2fly 会把它当作一个
# 列表名解析（报 "invalid list name: GEOSITE.JSON"）。
PROFILE="${ROOT}/data/geosite-profile.json"
cat > "$PROFILE" <<EOF
[
  {"name": "geosite.dat", "mode": "allowlist", "lists": ["category-ai", "ozon", "advertising"]}
]
EOF

cd "$V2FLY_DIR"
go run -mod=readonly . \
  -datapath "$DLC_DIR" \
  -outputname "geosite.dat" \
  -outputdir "$OUT" \
  -datprofile "$PROFILE"

log "✅ geosite.dat 已生成：${OUT}/geosite.dat"

