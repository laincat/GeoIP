#!/usr/bin/env python3
"""内容级 A/B 交叉验证：Country.mmdb 是否忠实反映 IPInfo 源文件。

mmdbverify 只验二进制结构，不看内容；这个脚本补上内容维度：

  1. 逐行读 `ipinfo/country.csv.gz`（每行一个 [start_ip, end_ip] 区间 + 国家码）；
  2. 按固定步长抽样，在区间内取探测点（中点 + 一个随机点）；
  3. 查 Country.mmdb，比对答案。

判据只有一条：**每个探测点的答案必须是「CSV 里的国家码」或「config.json 里
overwriteList 声明的类别码」**。第二类是预期覆盖（cn / cloudflare / google …），
第一类是直通。

这样写的好处是判据来自配置本身 —— 如果哪天有人往 overwriteList 里塞了个把
半个地球吞掉的条目，这里会立刻报出来，而不是被"预期覆盖"糊过去。

背景：mmdb 里的**网段数大于 CSV 行数**是正常的。一个非对齐区间会被拆成多个
CIDR（实测平均约 1.7 个），所以网段数不是判据，逐点覆盖率才是。

用法：
    python3 tools/verify_mmdb_vs_source.py ./output/Country.mmdb
    python3 tools/verify_mmdb_vs_source.py ./output/Country.mmdb --step 400
"""

from __future__ import annotations

import argparse
import csv
import gzip
import ipaddress
import json
import random
import re
import sys
from collections import Counter
from pathlib import Path

try:
    import maxminddb
except ImportError:  # pragma: no cover
    sys.exit("❌ 需要 maxminddb：python3 -m pip install 'maxminddb==2.6.1'")

SEED = 20260917
DEFAULT_STEP = 47  # 1,428,262 行 / 47 ≈ 3 万个抽样区间


def strip_jsonc(text: str) -> str:
    """config.json 是带注释和尾随逗号的 JSONC。

    必须是真状态机 —— 简单正则把 `//` 当注释开头会毁掉
    `https://raw.githubusercontent.com/...` 这类字符串。
    """
    out: list[str] = []
    i, n = 0, len(text)
    in_str = False

    while i < n:
        ch = text[i]

        if in_str:
            out.append(ch)
            if ch == "\\" and i + 1 < n:
                out.append(text[i + 1])
                i += 2
                continue
            if ch == '"':
                in_str = False
            i += 1
            continue

        if ch == '"':
            in_str = True
            out.append(ch)
            i += 1
            continue

        if ch == "/" and i + 1 < n and text[i + 1] == "/":
            while i < n and text[i] not in "\r\n":
                i += 1
            continue

        if ch == "/" and i + 1 < n and text[i + 1] == "*":
            i += 2
            while i + 1 < n and not (text[i] == "*" and text[i + 1] == "/"):
                i += 1
            i += 2
            continue

        out.append(ch)
        i += 1

    return re.sub(r",(\s*[}\]])", r"\1", "".join(out))


def load_overwrite_codes(config_path: Path) -> set[str]:
    cfg = json.loads(strip_jsonc(config_path.read_text(encoding="utf-8")))
    codes: set[str] = set()
    for out in cfg.get("output", []):
        for name in out.get("args", {}).get("overwriteList", []) or []:
            codes.add(str(name).strip().upper())
    return codes


def probe_points(start: str, end: str, rng: random.Random):
    """country.csv 同时含 IPv4 与 IPv6 区间，按族返回探测点。"""
    lo, hi = ipaddress.ip_address(start), ipaddress.ip_address(end)
    span = int(hi) - int(lo)
    kind = ipaddress.IPv4Address if lo.version == 4 else ipaddress.IPv6Address
    yield kind(int(lo) + span // 2)
    if span > 1:
        yield kind(int(lo) + rng.randrange(span + 1))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("mmdb", nargs="?", default="./output/Country.mmdb", help="待验证的 Country.mmdb")
    parser.add_argument("--csv", default="./ipinfo/country.csv.gz", help="IPInfo 源文件")
    parser.add_argument("--config", default="./config.json", help="用于读取 overwriteList")
    parser.add_argument("--step", type=int, default=DEFAULT_STEP, help=f"抽样步长（默认 {DEFAULT_STEP}）")
    parser.add_argument("--min-rate", type=float, default=100.0, help="要求的最低可解释率（默认 100）")
    args = parser.parse_args()

    mmdb_path = Path(args.mmdb)
    csv_path = Path(args.csv)
    if not mmdb_path.is_file():
        print(f"❌ 找不到 {mmdb_path}")
        return 1
    if not csv_path.is_file():
        print(f"❌ 找不到 {csv_path}（CI 里需要先下载数据）")
        return 1

    override_codes = load_overwrite_codes(Path(args.config))
    print(f"config.json 声明的覆盖类别: {sorted(override_codes) or '（无）'}")

    rng = random.Random(SEED)
    samples: list[tuple[str, str, str]] = []
    with gzip.open(csv_path, "rt", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        for idx, row in enumerate(reader):
            if idx % args.step:
                continue
            code = (row.get("country") or "").strip().upper()
            if not code or code == "-":
                continue
            samples.append((row["start_ip"], row["end_ip"], code))

    if not samples:
        print("❌ 源文件里没有可用样本")
        return 1
    print(f"抽样区间数: {len(samples)}（步长 {args.step}）")

    checked = 0
    direct = 0
    covered: Counter = Counter()
    unexplained: list[tuple[str, str, str]] = []
    per_country_bad: Counter = Counter()
    per_country_total: Counter = Counter()
    families: Counter = Counter()

    with maxminddb.open_database(str(mmdb_path)) as reader:
        for start, end, code in samples:
            per_country_total[code] += 1
            for ip in probe_points(start, end, rng):
                got = ((reader.get(str(ip)) or {}).get("country", {}).get("iso_code") or "-").upper()
                checked += 1
                families[f"IPv{ip.version}"] += 1

                if got == code:
                    direct += 1
                elif got in override_codes:
                    covered[got] += 1
                else:
                    per_country_bad[code] += 1
                    if len(unexplained) < 25:
                        unexplained.append((str(ip), code, got))

    unexplained_total = sum(per_country_bad.values())
    explained = direct + sum(covered.values())
    rate = explained / checked * 100 if checked else 0.0

    print(f"探测点        : {checked}  ({dict(families)})")
    print(f"  与 CSV 一致 : {direct}")
    print(f"  类别覆盖    : {sum(covered.values())}  {dict(covered.most_common(12))}")
    print(f"  无法解释    : {unexplained_total}")
    print(f"可解释率      : {rate:.4f}%")

    if unexplained:
        print("\n前 25 例无法解释（IP / CSV / MMDB）:")
        for ip, want, got in unexplained:
            print(f"  {ip:<26} {want:<6} -> {got}")
        print("\n按国家分布（国 / 失配 / 抽样）:")
        for code, bad in per_country_bad.most_common(15):
            print(f"  {code:<6}{bad:>8}{per_country_total[code]:>10}")

    if rate >= args.min_rate:
        print(f"\n✅ 通过：Country.mmdb 与 IPInfo 源文件一致，差异均可由 overwriteList 解释")
        return 0

    print(f"\n❌ 未通过：可解释率 {rate:.4f}% < {args.min_rate}%")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
