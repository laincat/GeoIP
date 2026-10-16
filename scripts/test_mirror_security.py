import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import urllib.request

ROOT = Path(__file__).resolve().parents[1]

def module(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'tools' / (name + '.py'))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result

cnb = module('publish_cnb_release')
snapshot = module('release_snapshot')

class MirrorSecurity(unittest.TestCase):
    def test_token_destination(self):
        for url in ('http://api.cnb.cool/x', 'https://evil.example/x', 'https://api.cnb.cool.evil.example/x',
                    'https://user@api.cnb.cool/x', 'https://api.cnb.cool:444/x', 'https://api.cnb.cool/x#bad'):
            with self.subTest(url=url), patch.object(cnb.urllib.request, 'build_opener') as opener:
                with self.assertRaises(ValueError):
                    cnb._api('POST', url, 'DUMMY')
                opener.assert_not_called()

    def test_upload_hosts(self):
        for url in ('https://evil.example/file', 'https://cnb.cool.evil.example/file'):
            with self.assertRaises(ValueError):
                cnb.secure_url(url)
        cnb.secure_url('https://bucket.cos.ap-shanghai.myqcloud.com/file')

    def test_no_token_redirect(self):
        request = urllib.request.Request('https://api.cnb.cool/x', headers={'Authorization': 'Bearer DUMMY'})
        with self.assertRaises(ValueError):
            cnb.NoAuthRedirect().redirect_request(request, None, 302, 'Found', {}, 'https://evil.example/x')

    def test_upload_no_http(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'file'
            path.write_bytes(b'test')
            with patch.object(cnb.http.client, 'HTTPConnection') as http:
                with self.assertRaises(ValueError):
                    cnb._put_file('http://upload.example/file', str(path))
                http.assert_not_called()

    def test_release_snapshot_rejects_missing_or_changed_assets(self):
        files = {name: 'a' * 64 for name in ('Country.mmdb', 'Country.mmdb.sha256sum', 'geoip.dat',
                 'GeoLite2-ASN.mmdb', 'geosite.dat', 'artifacts.sha256sum', 'inputs.json', 'version')}
        manifest = {'schema': 1, 'source_sha': 'b' * 40, 'files': files}
        assets = {name: {'digest': 'sha256:' + value} for name, value in files.items()}
        snapshot.validate(manifest, assets)
        assets['geosite.dat']['digest'] = 'sha256:' + 'c' * 64
        with self.assertRaises(ValueError):
            snapshot.validate(manifest, assets)
        del assets['geosite.dat']
        with self.assertRaises(ValueError):
            snapshot.validate(manifest, assets)

if __name__ == '__main__':
    unittest.main()
