"""Independent wire-format/content checks for GeoIP DAT, geosite DAT and ASN MMDB."""
import gzip
import csv
import ipaddress
from pathlib import Path
from source_snapshot import Networks, ranges, config, load_overrides


def varint(data, offset):
    value = 0
    for shift in range(0, 70, 7):
        if offset >= len(data):
            raise ValueError('truncated protobuf varint')
        byte = data[offset]
        offset += 1
        value |= (byte & 127) << shift
        if not byte & 128:
            return value, offset
    raise ValueError('oversized protobuf varint')


def fields(data):
    offset = 0
    while offset < len(data):
        tag, offset = varint(data, offset)
        number, wire = tag >> 3, tag & 7
        if number == 0:
            raise ValueError('invalid protobuf field')
        if wire == 0:
            value, offset = varint(data, offset)
        elif wire == 2:
            length, offset = varint(data, offset)
            if length > len(data) - offset:
                raise ValueError('truncated protobuf field')
            value = data[offset:offset + length]
            offset += length
        else:
            raise ValueError(f'unsupported protobuf wire type {wire}')
        yield number, value


def lists(path):
    result = {}
    for number, raw in fields(Path(path).read_bytes()):
        if number != 1 or not isinstance(raw, bytes):
            raise ValueError('invalid DAT container')
        name = None
        entries = []
        for field, value in fields(raw):
            if field == 1:
                if name is not None:
                    raise ValueError('duplicate DAT name')
                name = value.decode().upper()
            elif field == 2:
                entries.append(value)
            else:
                raise ValueError('unexpected DAT entry field')
        if not name or not entries or name in result:
            raise ValueError('empty/duplicate DAT entry')
        result[name] = entries
    if not result:
        raise ValueError('empty DAT')
    return result


def decode_geoip(path):
    result = {}
    for name, entries in lists(path).items():
        net = Networks()
        for entry in entries:
            values = dict(fields(entry))
            ip = ipaddress.ip_address(values[1])
            prefix = values.get(2, 0)
            if not isinstance(prefix, int) or not 0 <= prefix <= ip.max_prefixlen:
                raise ValueError('invalid prefix length')
            network = ipaddress.ip_network(f'{ip}/{prefix}', strict=True)
            net.add_range(network.network_address, network.broadcast_address)
        result[name] = net.finish()
    return result


def canonical_domains(rules):
    # The pinned compiler removes semantically redundant children covered by a
    # suffix parent. Compare that normal form, preserving all exact-only rules.
    suffixes = {rule[7:] for rule in rules if rule.startswith('domain:')}
    result = set()
    for rule in rules:
        kind, domain = rule.split(':', 1)
        parts = domain.split('.')
        parents = ['.'.join(parts[i:]) for i in range(0 if kind == 'full' else 1, len(parts))]
        if not any(parent in suffixes for parent in parents):
            result.add(rule)
    return result


def verify_geosite(path, directory):
    actual = {}
    for name, entries in lists(path).items():
        rules = set()
        for entry in entries:
            values = dict(fields(entry))
            kind = values.get(1, 0)
            if kind not in (2, 3) or set(values) - {1, 2}:
                raise ValueError('geosite contains unexpected matching type/attributes')
            rule = ('domain:' if kind == 2 else 'full:') + values[2].decode()
            if rule in rules:
                raise ValueError('duplicate geosite rule')
            rules.add(rule)
        actual[name] = rules
    expected = {name.upper(): canonical_domains(set((directory / name).read_text().splitlines()))
                for name in ('category-ai', 'ozon', 'advertising')}
    if actual != expected:
        raise ValueError('geosite differs from exact/suffix input semantics')


def verify_geoip():
    actual = decode_geoip('output/geoip.dat')
    order, expected = load_overrides('config.snapshot.json')
    for code in order:
        if code not in actual or actual[code].intervals != expected[code].intervals:
            raise ValueError(f'GeoIP DAT override mismatch: {code}')
    count = 0
    with gzip.open('data/ipinfo/country.csv.gz', 'rt') as stream:
        for idx, row in enumerate(csv.DictReader(stream)):
            if idx % 47:
                continue
            code = row['country'].strip().upper()
            if not code or code == '-':
                continue
            for raw in (row['start_ip'], row['end_ip']):
                if code not in actual or not actual[code].contains(ipaddress.ip_address(raw)):
                    raise ValueError(f'GeoIP DAT missing country boundary: {code} {raw}')
                count += 1
    if not count:
        raise ValueError('no GeoIP DAT source samples')


def verify_asn():
    import maxminddb
    organizations = {}
    checked = 0
    families = set()
    with maxminddb.open_database('output/GeoLite2-ASN.mmdb') as reader:
        if reader.metadata().database_type != 'GeoLite2-ASN':
            raise ValueError('invalid ASN database type')
        for path in ('data/iptoasn/ip2asn-v4.tsv.gz', 'data/iptoasn/ip2asn-v6.tsv.gz'):
            for idx, (lo, hi, asn, org) in enumerate(ranges(path)):
                if asn == '0':
                    continue
                organizations.setdefault(asn, org)
                if idx % 47:
                    continue
                for ip in (lo, hi):
                    record = reader.get(str(ip)) or {}
                    if record.get('autonomous_system_number') != int(asn) or record.get('autonomous_system_organization') != organizations[asn]:
                        raise ValueError(f'ASN mismatch at {ip}')
                    families.add(ip.version)
                    checked += 1
    if checked < 1000 or families != {4, 6}:
        raise ValueError('insufficient ASN coverage')
    print(f'ASN source boundaries/org names: {checked} probes passed')


if __name__ == '__main__':
    verify_geosite(Path('output/geosite.dat'), Path('data/dlc'))
    verify_geoip()
    verify_asn()
    print('GeoIP DAT, exact/suffix geosite DAT and ASN MMDB: passed')
