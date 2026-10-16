"""Snapshot remote service inputs once and validate source ranges independently."""
from __future__ import annotations
import bisect
import csv
import gzip
import hashlib
import ipaddress
import json
import re
import sys
import urllib.request
from pathlib import Path
from verify_mmdb_vs_source import strip_jsonc

PRIVATE = ('0.0.0.0/8 10.0.0.0/8 100.64.0.0/10 127.0.0.0/8 169.254.0.0/16 '
           '172.16.0.0/12 192.0.0.0/24 192.0.2.0/24 192.88.99.0/24 192.168.0.0/16 '
           '198.18.0.0/15 198.51.100.0/24 203.0.113.0/24 224.0.0.0/4 240.0.0.0/4 '
           '255.255.255.255/32 ::/128 ::1/128 fc00::/7 ff00::/8 fe80::/10').split()


def config(path):
    return json.loads(strip_jsonc(Path(path).read_text()))


def ranges(path):
    """TSV: reject malformed/mixed-family/reversed intervals; ASN 0 is unassigned."""
    with gzip.open(path, 'rt') as stream:
        previous = None
        count = 0
        for row in csv.reader(stream, delimiter='\t'):
            if not row or row[0].startswith('#'):
                continue
            if len(row) < 5:
                raise ValueError(f'{path}: short ASN row')
            lo, hi = ipaddress.ip_address(row[0]), ipaddress.ip_address(row[1])
            asn = int(row[2])
            if lo.version != hi.version or int(lo) > int(hi) or not 0 <= asn <= 4294967295:
                raise ValueError(f'{path}: invalid ASN interval')
            if previous is not None and int(lo) <= previous:
                raise ValueError(f'{path}: unsorted/overlapping ASN intervals')
            previous = int(hi)
            count += 1
            yield lo, hi, str(asn), row[4].strip()
        if count == 0:
            raise ValueError(f'{path}: empty ASN source')


class Networks:
    def __init__(self):
        self.intervals = {4: [], 6: []}
        self.starts = {}

    def add_range(self, lo, hi):
        if lo.version != hi.version or int(lo) > int(hi):
            raise ValueError('invalid IP interval')
        self.intervals[lo.version].append((int(lo), int(hi)))

    def add(self, raw, only=''):
        net = ipaddress.ip_network(raw, strict=False)
        if only and only.lower() != f'ipv{net.version}':
            return
        self.add_range(net.network_address, net.broadcast_address)

    def finish(self):
        for family, intervals in self.intervals.items():
            merged = []
            for lo, hi in sorted(intervals):
                if merged and lo <= merged[-1][1] + 1:
                    merged[-1] = (merged[-1][0], max(hi, merged[-1][1]))
                else:
                    merged.append((lo, hi))
            self.intervals[family] = merged
            self.starts[family] = [lo for lo, _ in merged]
        return self

    def contains(self, ip):
        pos = bisect.bisect_right(self.starts[ip.version], int(ip)) - 1
        return pos >= 0 and int(ip) <= self.intervals[ip.version][pos][1]


def json_prefixes(data, expression):
    # Deliberately support the reviewed config grammar and fail on unknown paths.
    if expression in ('addresses', 'ipv6_addresses'):
        return data[expression]
    match = re.fullmatch(r'(prefixes|ipv6_prefixes)\.#(?:(\(service=="CLOUDFRONT"\))#)?\.(\w+)', expression)
    if not match:
        raise ValueError(f'unsupported JSON path: {expression}')
    array, filter_, key = match.groups()
    return [item[key] for item in data[array] if key in item and (not filter_ or item.get('service') == 'CLOUDFRONT')]


def load_overrides(config_path):
    cfg = config(config_path)
    outputs = [x for x in cfg['output'] if x['type'].lower() == 'maxmindmmdb']
    order = [x.upper() for x in outputs[0]['args']['overwriteList']]
    nets = {name: Networks() for name in order}
    for entry in cfg['input']:
        kind, args = entry['type'].lower(), entry.get('args', {})
        if entry['action'] != 'add':
            raise ValueError('source verifier supports add-only snapshot configurations')
        only = args.get('onlyIPType', '')
        if kind == 'ipinfocountrycsv':
            with gzip.open(args['uri'], 'rt') as stream:
                reader = csv.DictReader(stream)
                if not {'start_ip', 'end_ip', 'country'} <= set(reader.fieldnames or []):
                    raise ValueError('missing country CSV columns')
                count = 0
                for row in reader:
                    lo, hi = ipaddress.ip_address(row['start_ip']), ipaddress.ip_address(row['end_ip'])
                    if lo.version != hi.version or int(lo) > int(hi):
                        raise ValueError('invalid country interval')
                    code = row['country'].strip().upper()
                    if code not in ('', '-') and not re.fullmatch('[A-Z]{2}', code):
                        raise ValueError('invalid country code')
                    if code in nets:
                        nets[code].add_range(lo, hi)
                    count += 1
                if not count:
                    raise ValueError('empty country CSV')
        elif kind == 'iptoasn':
            wanted = {}
            for name, asns in args['wantedList'].items():
                for asn in asns:
                    wanted.setdefault(asn.removeprefix('AS'), []).append(name.upper())
            for path in (args['ipv4'], args['ipv6']):
                for lo, hi, asn, _ in ranges(path):
                    for name in wanted.get(asn, []):
                        if name in nets:
                            nets[name].add_range(lo, hi)
        elif kind in ('text', 'json'):
            name = args['name'].upper()
            if name not in nets:
                continue
            path = args.get('uri')
            if path and path.startswith(('http:', 'https:')):
                raise ValueError('use config.snapshot.json: verifier must not refetch live sources')
            if kind == 'json':
                data = json.loads(Path(path).read_text())
                prefixes = [prefix for expr in args['jsonPath'] for prefix in json_prefixes(data, expr)]
            else:
                prefixes = list(args.get('ipOrCIDR', []))
                if path:
                    prefixes += [re.split(r'#|//|/\*', x)[0].strip() for x in Path(path).read_text().splitlines()]
            usable = [x for x in prefixes if x]
            if not usable:
                raise ValueError(f'empty service source: {name}')
            for prefix in usable:
                nets[name].add(prefix, only)
        elif kind == 'private':
            for prefix in PRIVATE:
                nets['PRIVATE'].add(prefix, only)
        else:
            raise ValueError(f'unsupported source type: {kind}')
    for net in nets.values():
        net.finish()
    return order, nets


def expected_code(ip, source_code, order, networks):
    for code in reversed(order):
        if networks[code].contains(ip):
            return code
    return source_code


def snapshot():
    cfg = config('config.json')
    directory = Path('data/snapshots')
    directory.mkdir(parents=True, exist_ok=True)
    for n, entry in enumerate(cfg['input']):
        args = entry.get('args', {})
        uri = args.get('uri', '')
        if uri.startswith('https://'):
            path = directory / f'{n}.input'
            with urllib.request.urlopen(uri, timeout=60) as response:
                data = response.read(64 * 1024 * 1024 + 1)
            if not data or len(data) > 64 * 1024 * 1024:
                raise ValueError('empty/oversized service response')
            path.write_bytes(data)
            args['uri'] = str(path)
    Path('config.snapshot.json').write_text(json.dumps(cfg, indent=2) + '\n')
    load_overrides('config.snapshot.json')  # strict source gate before conversion


def manifest():
    paths = [Path('config.snapshot.json'), Path('config-asn.json'), Path('data/rules-revision')]
    for directory in ('data/ipinfo', 'data/iptoasn', 'data/snapshots', 'data/dlc', 'data/geosite-raw'):
        paths.extend(p for p in Path(directory).rglob('*') if p.is_file())
    for name in ('go.mod', 'go.sum'):
        paths.append(Path('data/v2fly-domain-list') / name)
    result = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(paths)}
    result['compiler_revision'] = 'dd4e3c080cee602512768e8109f89fafaa94b8ca'
    result['rules_revision'] = Path('data/rules-revision').read_text().strip()
    Path('output/inputs.json').write_text(json.dumps(result, sort_keys=True, indent=2) + '\n')


if __name__ == '__main__':
    {'snapshot': snapshot, 'manifest': manifest}[sys.argv[1]]()
