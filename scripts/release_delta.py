"""Skip publication only when every required released file is present and identical.

The version timestamp is intentionally excluded from content comparison. Network or
metadata errors fail open to publication, never to skipping a needed update.
"""
import hashlib
import json
import sys
import urllib.request
from pathlib import Path

PRODUCTS = ('Country.mmdb', 'geoip.dat', 'GeoLite2-ASN.mmdb', 'geosite.dat')
REQUIRED = PRODUCTS + ('Country.mmdb.sha256sum', 'artifacts.sha256sum', 'inputs.json', 'version', 'snapshot-manifest.json')


def sha256_stream(stream):
    digest = hashlib.sha256()
    while block := stream.read(1024 * 1024):
        digest.update(block)
    return digest.hexdigest()


def unchanged(output, release, fetch):
    assets = {a['name']: a for a in release['assets']}
    if not set(REQUIRED) <= assets.keys():
        return False
    # A stale/partial seal must be repaired even when binary bytes are unchanged.
    with fetch(assets['snapshot-manifest.json']['browser_download_url']) as remote:
        raw = remote.read()
    if assets['snapshot-manifest.json'].get('digest') != 'sha256:' + hashlib.sha256(raw).hexdigest():
        return False
    seal = json.loads(raw)
    files = seal.get('files', {})
    if seal.get('schema') != 1 or set(files) != set(REQUIRED) - {'snapshot-manifest.json'}:
        return False
    if any(assets[name].get('digest') != 'sha256:' + digest for name, digest in files.items()):
        return False
    for name in REQUIRED:
        path = output / name
        if not path.is_file() or path.stat().st_size == 0 or assets[name]['size'] == 0:
            return False
        if name in ('version', 'snapshot-manifest.json'):
            continue
        if path.stat().st_size != assets[name]['size']:
            return False
        with path.open('rb') as local, fetch(assets[name]['browser_download_url']) as remote:
            if sha256_stream(local) != sha256_stream(remote):
                return False
    return True


def main():
    repo, directory = sys.argv[1:]
    def fetch(url):
        return urllib.request.urlopen(url, timeout=60)
    try:
        with fetch(f'https://api.github.com/repos/{repo}/releases/tags/geoip') as response:
            release = json.load(response)
        skip = unchanged(Path(directory), release, fetch)
    except Exception as exc:
        print(f'Published release comparison unavailable ({type(exc).__name__}); publish.', file=sys.stderr)
        skip = False
    print(f'skip={int(skip)}')


if __name__ == '__main__':
    main()
