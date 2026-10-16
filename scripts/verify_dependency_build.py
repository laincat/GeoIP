"""Offline conversion regression: IPv4/IPv6, overrides and reproducibility."""
import gzip
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
        from verify_products import decode_geoip
        dat = decode_geoip(root / "geoip.dat")
        import ipaddress
        assert dat['GOOGLE'].contains(ipaddress.ip_address('8.8.8.8'))
        assert not dat['GOOGLE'].contains(ipaddress.ip_address('8.8.8.9'))
        assert dat['US'].contains(ipaddress.ip_address('2001:4860::8888'))
        asn_file = root / "asn.tsv.gz"
        with gzip.open(asn_file, "wt") as stream:
            stream.write("8.8.8.0\t8.8.8.255\t15169\tUS\tExample Organization\n")
            stream.write("2001:4860::\t2001:4860::ffff\t15169\tUS\tExample Organization\n")
        asn_config = {
            "input": [{"type": "ipToASNFull", "action": "add", "args": {"ipv4": str(asn_file)}}],
            "output": [{"type": "asnMMDB", "action": "output", "args": {"outputDir": directory}}],
        }
        source.write_text(json.dumps(asn_config))
        hashes = []
        for _ in range(2):
            subprocess.run([executable, "convert", "-c", str(source)], env=env, check=True)
            hashes.append(hashlib.sha256((root / "GeoLite2-ASN.mmdb").read_bytes()).hexdigest())
        assert hashes[0] == hashes[1], "ASN output is not reproducible"
        with maxminddb.open_database(str(root / "GeoLite2-ASN.mmdb")) as reader:
            for ip in ("8.8.8.0", "8.8.8.255", "2001:4860::", "2001:4860::ffff"):
                record = reader.get(ip)
                assert record["autonomous_system_number"] == 15169, ip
                assert record["autonomous_system_organization"] == "Example Organization", ip
            assert reader.get("8.8.9.0") is None
        asn_file.write_bytes(b"corrupt gzip")
        failed = subprocess.run([executable, "convert", "-c", str(source)], env=env, capture_output=True)
        assert failed.returncode != 0, "corrupt gzip accepted"
        print("IPv4/IPv6, overrides, DAT semantics, ASN organizations, corruption and reproducibility: passed")


if __name__ == "__main__":
    main()

