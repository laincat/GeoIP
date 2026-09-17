#!/usr/bin/env bash
#
# 下载构建所需的全部输入数据到 ./data/。
#
# 为什么单独抽一个脚本：这份下载逻辑原先在 README 与 CI workflow 里各写了一遍，
# 改路径要改两处、判据也可能悄悄漂移。现在两边都调这一个脚本，
# 「本地怎么跑」与「CI 怎么跑」从此是同一条命令。
#
# 用法：
#     IPINFO_TOKEN=xxxx ./scripts/fetch-data.sh
#
# 幂等：文件已存在且体积校验通过时直接跳过，可用 --force 强制重下。
#
# 下载后的完整校验（体积下限 + gzip 完整性）在这里做，不放到下游 ——
# IPInfo 取数失败时会返回 HTTP 200 + 一小段错误正文，只看状态码分辨不出来。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATA_DIR="${ROOT}/data"

IPINFO_DIR="${DATA_DIR}/ipinfo"
IPTOASN_DIR="${DATA_DIR}/iptoasn"

# 低于这个体积基本可以断定不是真数据集（实测正品约 11MB）
IPINFO_MIN_BYTES=100000

FORCE=0
if [ "${1:-}" = "--force" ]; then
  FORCE=1
fi

log() { printf '%s\n' "$*"; }
die() { printf '::error::%s\n' "$*" >&2; exit 1; }

# 校验一个 gzip 文件：体积下限 + gzip -t
check_gzip() {
  local file="$1" min_bytes="${2:-1}" size
  [ -f "$file" ] || return 1
  size=$(wc -c < "$file" | tr -d ' ')
  [ "$size" -ge "$min_bytes" ] || return 1
  gzip -t "$file" 2>/dev/null || return 1
  return 0
}

fetch() {
  local url="$1" out="$2" min_bytes="${3:-1}"

  if [ "$FORCE" -eq 0 ] && check_gzip "$out" "$min_bytes"; then
    log "跳过（已存在且校验通过）：${out#${ROOT}/}"
    return 0
  fi

  mkdir -p "$(dirname "$out")"
  curl -fsSL --retry 3 --retry-delay 5 "$url" -o "$out"

  check_gzip "$out" "$min_bytes" || {
    log "可疑文件内容："
    head -c 400 "$out" || true
    die "${out#${ROOT}/} 体积或 gzip 结构异常，疑似返回了错误页"
  }

  log "已下载：${out#${ROOT}/}（$(wc -c < "$out" | tr -d ' ') 字节）"
}

# ── 1. 国家维度：IPInfo 免费 country.csv.gz（需要 token）────────────────────
if [ -z "${IPINFO_TOKEN:-}" ]; then
  die "缺少 IPINFO_TOKEN：IPInfo 数据集需要一个免费 token（本地用 IPINFO_TOKEN=xxx 传入，CI 用仓库 secret），见 README"
fi
fetch "https://ipinfo.io/data/free/country.csv.gz?token=${IPINFO_TOKEN}" \
  "${IPINFO_DIR}/country.csv.gz" "$IPINFO_MIN_BYTES"

# ── 2. 类别维度：iptoasn.com 的 IP-to-ASN 转储（免凭据）─────────────────────
for name in ip2asn-v4.tsv.gz ip2asn-v6.tsv.gz; do
  fetch "https://iptoasn.com/data/${name}" "${IPTOASN_DIR}/${name}"
done

log "输入数据就绪：${DATA_DIR}"
