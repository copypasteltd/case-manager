"""Retain published UI/media and unchanged Go evidence without changing their provenance."""
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/dependency-integrity'
PUBLISHED = 'a7f323f631eb07d6d6852d3d666df8e085af02a3'


def main():
    target = E / 'reused-evidence.json'
    assert not target.exists(), 'Preserve earlier evidence records.'
    listing = subprocess.check_output(['git', 'ls-tree', '-r', PUBLISHED, '--', 'docs/media'], cwd=ROOT).decode().splitlines()
    media = []
    for line in listing:
        metadata, name = line.split('\t', 1)
        if Path(name).suffix.lower() not in ['.png', '.mp4', '.gif']:
            continue
        data = (ROOT / name).read_bytes()
        blob = hashlib.sha1(b'blob ' + str(len(data)).encode() + b'\0' + data).hexdigest()
        assert blob == metadata.split()[2], name
        media.append({'file': name, 'sha256': hashlib.sha256(data).hexdigest(), 'git_blob': blob})
    assert len(media) == 211
    paths = ['app/backend', 'app/web/src', 'app/web/public', 'app/mobile/src', 'app/mobile/native',
             'app/mobile/android', 'app/packages/design', 'app/packages/contracts', 'app/agent-runtime', 'app/skills']
    changed = subprocess.check_output(['git', 'diff', '--name-only', PUBLISHED, '--', *paths], cwd=ROOT).decode().splitlines()
    assert not changed, changed
    records = []
    for name in ['docs/quality/evidence/cost-amortization-backend/result.json',
                 'docs/quality/evidence/cost-amortization-backend/raw-log-archive.json',
                 'docs/quality/evidence/cost-amortization/linux-build.json',
                 'docs/quality/evidence/cost-amortization-web/result.json',
                 'docs/quality/evidence/cost-amortization-mobile/result.json',
                 'docs/quality/evidence/cost-amortization/media.json',
                 'docs/quality/evidence/cost-amortization/visual-review.json']:
        data = (ROOT / name).read_bytes()
        assert data == subprocess.check_output(['git', 'show', PUBLISHED + ':' + name], cwd=ROOT)
        records.append({'file': name, 'sha256': hashlib.sha256(data).hexdigest()})
    result = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'published_commit': PUBLISHED,
              'media_counts': dict(Counter(Path(row['file']).suffix[1:] for row in media)), 'media': media,
              'retained_records': records, 'ui_sources_unchanged': True, 'ui_source_paths': paths,
              'new_browser_capture': False, 'new_backend_test_execution': False, 'new_linux_cross_compile': False,
              'scope': 'Original106 UI/business captures, media and Go execution/cross-build records remain historical. Product UI and Go/SQL sources retain exact published bytes. Dependency-sensitive clients/Android and a synthetic local Taro development-server smoke check are verified separately.'}
    target.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'retained_media': result['media_counts'], 'retained_records': len(records)}))


if __name__ == '__main__':
    main()
