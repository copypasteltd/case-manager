"""Retain published media/Go records with original bytes and explicit old scope."""
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/dependency-remediation'
PUBLISHED = '52899b6531d118b02770d2a40842d4e56622872c'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def main():
    E.mkdir(parents=True, exist_ok=True)
    assert not (E / 'reused-evidence.json').exists(), 'Preserve previous record before rerunning.'
    listing = subprocess.check_output(['git', 'ls-tree', '-r', PUBLISHED, '--', 'docs/media'], cwd=ROOT).decode().splitlines()
    media = []
    for line in listing:
        meta, name = line.split('\t', 1)
        if Path(name).suffix.lower() not in ['.png', '.mp4', '.gif']:
            continue
        data = (ROOT / name).read_bytes()
        original = subprocess.check_output(['git', 'show', PUBLISHED + ':' + name], cwd=ROOT)
        assert data == original
        media.append({'file': name, 'sha256': sha(data), 'git_blob': meta.split()[2]})
    counts = dict(Counter(Path(row['file']).suffix[1:] for row in media))
    assert counts == {'gif': 41, 'png': 107, 'mp4': 41}
    record_names = ['docs/quality/evidence/finance-fx-backend/result.json',
                    'docs/quality/evidence/finance-fx/linux-build.json',
                    'docs/quality/evidence/finance-fx-web/result.json',
                    'docs/quality/evidence/finance-fx-mobile/result.json',
                    'docs/quality/evidence/finance-fx/media.json',
                    'docs/quality/evidence/finance-fx/visual-review.json']
    records = []
    for name in record_names:
        data = (ROOT / name).read_bytes()
        assert data == subprocess.check_output(['git', 'show', PUBLISHED + ':' + name], cwd=ROOT)
        records.append({'file': name, 'sha256': sha(data)})
    ui_paths = ['app/web/src', 'app/web/public', 'app/mobile/src', 'app/mobile/native', 'app/mobile/android', 'app/packages/design', 'app/packages/contracts']
    diff = subprocess.check_output(['git', 'diff', '--name-only', PUBLISHED, '--', *ui_paths], cwd=ROOT).decode().splitlines()
    assert diff == []
    result = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'published_commit': PUBLISHED,
              'media_counts': counts, 'media': media, 'retained_records': records,
              'ui_sources_unchanged': True, 'ui_source_paths': ui_paths,
              'new_browser_capture': False, 'new_backend_test_execution': False, 'new_linux_cross_compile': False,
              'scope': 'Original102 UI captures remain historical. Source UI and media bytes are unchanged; dependency-sensitive clients/Android are freshly checked separately. Unchanged Go evidence keeps its original execution/retry history.'}
    (E / 'reused-evidence.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'retained_media': counts, 'retained_records': len(records)}))


if __name__ == '__main__':
    main()
