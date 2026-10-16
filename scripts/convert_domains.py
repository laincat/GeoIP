"""Preserve Surge exact/suffix semantics when writing v2fly input."""
import re
import sys
from pathlib import Path

LABEL = re.compile(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\Z")


def convert(lines):
    rules = set()
    for raw in lines:
        value = raw.strip().lower()
        if not value or value.startswith(('#', '//')) or '/' in value or ':' in value:
            continue
        suffix = value.startswith('.')
        domain = value[1:] if suffix else value
        labels = domain.split('.')
        if len(domain) > 253 or len(labels) < 2 or not all(LABEL.fullmatch(x) for x in labels):
            raise ValueError(f"invalid domain: {raw.rstrip()!r}")
        rules.add(('domain:' if suffix else 'full:') + domain)
    if not rules:
        raise ValueError('domain list is empty')
    return sorted(rules)


if __name__ == '__main__':
    Path(sys.argv[2]).write_text('\n'.join(convert(Path(sys.argv[1]).read_text().splitlines())) + '\n')
