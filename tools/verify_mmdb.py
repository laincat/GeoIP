#!/usr/bin/env python3
"""对构建出的 Country.mmdb 做内容级校验。

mmdbverify 只检查文件结构，不看内容 —— 一个「结构完全合法但内容为空」
的产物同样能通过它的校验。本脚本补上内容侧的把关：

  1. 关键类别是否真的存在（不存在的类别等于订阅方规则永远不命中）
  2. 覆盖量是否落在合理区间（防止数据源静默退化）
  3. 若干知名 IP 的解析结果（防止整库错位）

用法：
    python3 tools/verify_mmdb.py [Country.mmdb 路径]
"""

from __future__ import annotations

import sys
from collections import Counter

# 必须存在的条目。前三个是国家/私有段，后面是类别维度，
# 任何一个缺失都说明对应的数据源在这次构建里没生效。
REQUIRED = [
    "CN",
    "US",
    "PRIVATE",
    "CLOUDFLARE",
    "GOOGLE",
    "TELEGRAM",
    "TOR",
    "NETFLIX",
    "FACEBOOK",
    "TWITTER",
    "FASTLY",
    "CLOUDFRONT",
]

# 覆盖量下限。明显低于这些值说明上游数据源出了问题，
# 而不是「今天恰好少了一点」。
MIN_NETWORKS = 1_000_000
MIN_CODES = 200

# 抽查用。只打印结果，不断言 —— 这些地址落在哪个类别取决于
# 数据源的 ASN 归属，写死期望值反而容易误报。
SAMPLES = [
    "114.114.114.114",
    "223.5.5.5",
    "1.1.1.1",
    "8.8.8.8",
    "192.168.1.1",
    "10.0.0.1",
]


def fail(message: str) -> None:
    print("::error::" + message)
    sys.exit(1)


def main() -> None:
    path = sys.argv[1] if len(sys.argv) > 1 else "./output/Country.mmdb"

    try:
        import maxminddb
    except ImportError:
        fail("缺少 maxminddb，请先 `pip install maxminddb==2.6.1`")

    try:
        db = maxminddb.open_database(path)
    except Exception as exc:  # noqa: BLE001
        fail(f"无法打开 {path}: {exc}")

    meta = db.metadata()
    print(f"文件            : {path}")
    print(f"  database_type : {meta.database_type}")
    print(f"  record_size   : {meta.record_size}")
    print(f"  ip_version    : {meta.ip_version}")

    codes: Counter[str] = Counter()
    networks = 0
    for _network, record in db:
        networks += 1
        code = ((record or {}).get("country") or {}).get("iso_code")
        codes[code or "<empty>"] += 1

    print(f"  网段总数      : {networks:,}")
    print(f"  不同 iso_code : {len(codes):,}")

    problems: list[str] = []

    if networks < MIN_NETWORKS:
        problems.append(f"网段总数只有 {networks:,}，低于下限 {MIN_NETWORKS:,}")

    if len(codes) < MIN_CODES:
        problems.append(f"iso_code 只有 {len(codes)} 种，低于下限 {MIN_CODES}")

    for name in REQUIRED:
        if not codes.get(name):
            problems.append(f"缺少必需条目 {name}")

    print("\n  条目覆盖量 Top 15：")
    for name, count in codes.most_common(15):
        print(f"    {name:<14} {count:>12,}")

    print("\n  抽查解析：")
    for ip in SAMPLES:
        record = db.get(ip)
        code = ((record or {}).get("country") or {}).get("iso_code", "<none>")
        print(f"    {ip:<18} -> {code}")

    db.close()

    if problems:
        print()
        for item in problems:
            print("::error::" + item)
        sys.exit(1)

    print("\n产物内容校验通过 ✓")


if __name__ == "__main__":
    main()
