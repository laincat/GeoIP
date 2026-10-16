import io
import ipaddress
import tempfile
import unittest
from pathlib import Path
from convert_domains import convert
from release_delta import REQUIRED, unchanged
from source_snapshot import Networks, expected_code, json_prefixes, ranges
from verify_products import fields, lists, canonical_domains, verify_geosite
import gzip


class Regressions(unittest.TestCase):
    def test_exact_suffix(self):
        self.assertEqual(convert(['exact.example', '.suffix.example']), ['domain:suffix.example', 'full:exact.example'])
        self.assertNotIn('domain:exact.example', convert(['exact.example']))

    def test_semantic_redundancy(self):
        self.assertEqual(canonical_domains({'full:a.example', 'domain:a.example', 'full:b.a.example', 'domain:c.a.example', 'full:other.example'}),
                         {'domain:a.example', 'full:other.example'})

    def test_geosite_wire_roundtrip(self):
        def wire(n, value):
            self.assertLess(len(value), 128)
            return bytes([n * 8 + 2, len(value)]) + value
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            data = b''
            for name in ('category-ai', 'ozon', 'advertising'):
                (root / name).write_text('full:exact.example\ndomain:suffix.example\n')
                domains = wire(2, bytes([8, 3]) + wire(2, b'exact.example')) + wire(2, bytes([8, 2]) + wire(2, b'suffix.example'))
                data += wire(1, wire(1, name.upper().encode()) + domains)
            path = root / 'geosite.dat'
            path.write_bytes(data)
            verify_geosite(path, root)
            path.write_bytes(data.replace(bytes([8, 3]), bytes([8, 2])))
            with self.assertRaises(ValueError):
                verify_geosite(path, root)

    def test_bad_domains(self):
        for domain in ('com', '-bad.example', 'bad-.example', 'a..example', 'a.*.example', 'a.' + 'x' * 64):
            with self.subTest(domain=domain), self.assertRaises(ValueError):
                convert([domain])

    def test_domain_comments_and_deduplication(self):
        self.assertEqual(convert(['# comment', 'EXAMPLE.COM', 'example.com', '192.0.2.0/24']), ['full:example.com'])

    def test_release_all_products(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            published = {name: b'old' for name in REQUIRED}
            for name, content in published.items():
                (root / name).write_bytes(content)
            release = {'assets': [{'name': n, 'size': 3, 'browser_download_url': n} for n in REQUIRED]}
            fetch = lambda name: io.BytesIO(published[name])
            self.assertTrue(unchanged(root, release, fetch))
            for name in ('geoip.dat', 'GeoLite2-ASN.mmdb', 'geosite.dat'):
                (root / name).write_bytes(b'new')
                self.assertFalse(unchanged(root, release, fetch))
                (root / name).write_bytes(b'old')
            (root / 'version').write_bytes(b'new timestamp')
            self.assertTrue(unchanged(root, release, fetch))
            release['assets'].pop()
            self.assertFalse(unchanged(root, release, fetch))

    def test_override_membership_and_precedence(self):
        google, private = Networks(), Networks()
        google.add('8.8.8.0/24')
        private.add('8.8.8.8/32')
        nets = {'GOOGLE': google.finish(), 'PRIVATE': private.finish()}
        order = ['GOOGLE', 'PRIVATE']
        for ip, expected in [('1.1.1.1', 'US'), ('8.8.8.9', 'GOOGLE'), ('8.8.8.8', 'PRIVATE')]:
            self.assertEqual(expected_code(ipaddress.ip_address(ip), 'US', order, nets), expected)
        self.assertNotEqual(expected_code(ipaddress.ip_address('1.1.1.1'), 'US', order, nets), 'GOOGLE')

    def test_json_paths(self):
        data = {'prefixes': [{'service': 'CLOUDFRONT', 'ip_prefix': '1.1.1.0/24'}, {'service': 'EC2', 'ip_prefix': '2.2.2.0/24'}]}
        self.assertEqual(json_prefixes(data, 'prefixes.#(service=="CLOUDFRONT")#.ip_prefix'), ['1.1.1.0/24'])
        with self.assertRaises(ValueError):
            json_prefixes(data, 'unsupported')

    def test_malformed_wire(self):
        for wire in (b'\x0a\x08short', b'\x00', b'\x80' * 12):
            with self.assertRaises(ValueError):
                list(fields(wire))

    def test_asn_rows(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'asn.gz'
            with gzip.open(path, 'wt') as stream:
                stream.write('1.1.1.0\t1.1.1.255\t13335\tUS\tCloudflare\n')
            self.assertEqual(list(ranges(path))[0][2:], ('13335', 'Cloudflare'))
            for row in ('bad\trow\n', '1.1.1.2\t1.1.1.1\t1\tUS\torg\n', '1.1.1.1\t::1\t1\tUS\torg\n'):
                with gzip.open(path, 'wt') as stream:
                    stream.write(row)
                with self.assertRaises(ValueError):
                    list(ranges(path))
            path.write_bytes(b'not gzip')
            with self.assertRaises(OSError):
                list(ranges(path))


if __name__ == '__main__':
    unittest.main()
