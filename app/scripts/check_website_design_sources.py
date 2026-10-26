"""Compare public website resources with the saved design reference.

Uses native Python HTTPS, opens no browser, and never replaces local brand assets.
Run from any directory. Evidence belongs in docs/design/evidence.
"""

import hashlib
import json
import sys
import urllib.request
from datetime import datetime, timezone
from pathlib import Path


DESIGN = Path(__file__).resolve().parents[2] / 'docs' / 'design'
ORIGIN = 'https://copypaste.hk'
PATHS = (
    '/', '/styles.css', '/app.js',
    '/assets/logo/CopyPaste_Light.png', '/assets/logo/CopyPaste_Dark.png',
)


def main():
    reference = json.loads((DESIGN / 'evidence/website-reference.json').read_text(encoding='utf-8'))
    baseline = {asset['url']: asset for asset in reference['assets']}
    report = {
        'checked_at': datetime.now(timezone.utc).isoformat(),
        'method': 'Windows native Python HTTPS; SHA-256 of actual response bytes',
        'previous_capture_at': reference['captured_at'],
        'browser_invoked': False,
        'scope': 'Homepage, shared stylesheet/script and both currently used PNG logos; not a new browser or full-site audit',
        'script': 'app/scripts/check_website_design_sources.py',
        'resources': [], 'errors': [],
    }
    for path in PATHS:
        url = ORIGIN + path
        previous = baseline[url]
        item = {'url': url, 'previous_sha256': previous['sha256']}
        try:
            request = urllib.request.Request(url, headers={'User-Agent': 'CopyPaste-design-reference-check/1.0'})
            with urllib.request.urlopen(request, timeout=30) as response:
                if not response.url.startswith(ORIGIN + '/'):
                    raise ValueError('Unexpected website origin')
                data = response.read(2 * 1024 * 1024 + 1)
                if len(data) > 2 * 1024 * 1024:
                    raise ValueError('Reference resource exceeds the 2 MiB inspection limit')
                digest = hashlib.sha256(data).hexdigest()
                item.update(status=response.status, bytes=len(data), sha256=digest,
                            unchanged=digest == previous['sha256'])
            if not item['unchanged']:
                report['errors'].append({'url': url, 'error': 'Resource differs from the saved reference; review the visual baseline'})
            if previous.get('local_file'):
                local = (DESIGN / previous['local_file']).resolve()
                local.relative_to(DESIGN.resolve())
                local_digest = hashlib.sha256(local.read_bytes()).hexdigest()
                item.update(local_file=previous['local_file'], local_sha256=local_digest,
                            local_matches_reference=local_digest == previous['sha256'],
                            local_matches_current=local_digest == digest)
                if not item['local_matches_reference'] or not item['local_matches_current']:
                    report['errors'].append({'url': url, 'error': 'Local brand asset differs; no asset was overwritten'})
        except Exception as error:
            report['errors'].append({'url': url, 'error': str(error)})
        report['resources'].append(item)
    report['all_sources_unchanged'] = all(item.get('unchanged', False) for item in report['resources'])
    logos = [item for item in report['resources'] if item.get('local_file')]
    report['all_local_logos_match'] = len(logos) == 2 and all(
        item['local_matches_reference'] and item['local_matches_current'] for item in logos)
    (DESIGN / 'evidence/website-source-refresh.json').write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({key: report[key] for key in (
        'checked_at', 'browser_invoked', 'all_sources_unchanged', 'all_local_logos_match', 'errors')}, ensure_ascii=False))
    return 1 if report['errors'] else 0


if __name__ == '__main__':
    sys.exit(main())
