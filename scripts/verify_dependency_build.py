"""Offline conversion regression: IPv4/IPv6, overrides and reproducibility."""
import hashlib
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

import maxminddb


def main():
    executable = str(Path(sys.argv[1]).resolve())
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        config = {
            "input": [
                {"type": "text", "action": "add",
                 "args": {"name": "US", "ipOrCIDR": ["8.8.8.0/24",
                                                    "2001:4860::/32"]}},
                {"type": "text", "action": "add",
                 "args": {"name": "GOOGLE", "ipOrCIDR": ["8.8.8.8/32"]}},
                {"type": "private", "action": "add"},
            ],
            "output": [
                {"type": "maxmindMMDB", "action": "output",
                 "args": {"outputDir": directory, "outputName": "Country.mmdb",
                          "overwriteList": ["GOOGLE", "PRIVATE"]}},
                {"type": "v2rayGeoIPDat", "action": "output",
                 "args": {"outputDir": directory, "outputName": "geoip.dat"}},
            ],
        }
        source = root / "config.json"
        source.write_text(json.dumps(config), encoding="utf-8")
        env = {**os.environ, "SOURCE_DATE_EPOCH": "1700000000"}
        hashes = []
        for _ in range(2):
            subprocess.run([executable, "convert", "-c", str(source)],
                           env=env, check=True)
            hashes.append([hashlib.sha256((root / name).read_bytes()).hexdigest()
                           for name in ("Country.mmdb", "geoip.dat")])
        assert hashes[0] == hashes[1], "identical inputs produced different bytes"
        with maxminddb.open_database(str(root / "Country.mmdb")) as reader:
            for ip, expected in {"8.8.8.8": "GOOGLE", "8.8.8.9": "US",
                                 "2001:4860::8888": "US",
                                 "192.168.1.1": "PRIVATE"}.items():
                assert reader.get(ip)["country"]["iso_code"] == expected, ip
            assert reader.get("9.9.9.9") is None
        print("IPv4/IPv6, override priority, MMDB/DAT reproducibility: passed")


if __name__ == "__main__":
    main()
