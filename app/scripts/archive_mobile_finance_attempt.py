"""Retain an exact frozen startup and failed local capture before any correction."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import zipfile

ROOT = Path(__file__).resolve().parents[2]
APP = ROOT / 'app'
E = ROOT / 'docs/quality/evidence/mobile-finance-operations'
BROWSER = ROOT / 'docs/quality/evidence/mobile-finance-operations-mobile'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('name')
    parser.add_argument('--reason', required=True)
    parser.add_argument('--native-fixture-exit', type=int)
    args = parser.parse_args()
    if not re.fullmatch(r'[a-z][a-z0-9-]{0,63}', args.name):
        raise SystemExit('Use a short plain attempt name.')
    target = E / 'attempts' / args.name
    target.mkdir(parents=True, exist_ok=False)
    startup = APP / '.local/mobile-finance-startup.zip'
    shutil.copyfile(startup, target / 'sources.zip')
    source_digest = hashlib.sha256()
    with zipfile.ZipFile(startup) as archive:
        names = sorted(archive.namelist())
        for name in names:
            source_digest.update(name.encode() + b'\0' + archive.read(name) + b'\0')
    browser = None
    if (BROWSER / 'result.json').exists():
        browser = json.loads((BROWSER / 'result.json').read_text(encoding='utf-8'))
        if browser['passed']:
            raise SystemExit('Passing evidence requires a separately reviewed supersession; nothing was removed.')
        for file in BROWSER.iterdir():
            if file.is_file():
                shutil.copyfile(file, target / file.name)
        assert browser.get('build_source_sha256') == source_digest.hexdigest()
    media = []
    for file in sorted((ROOT / 'docs/media/screenshots').glob('mobile-finance-*.png')):
        destination = target / 'media' / file.name
        destination.parent.mkdir(exist_ok=True)
        shutil.copyfile(file, destination)
        media.append({'original_path': file.relative_to(ROOT).as_posix(), 'sha256': sha(file)})
    report = {'passed': False, 'diagnostic_only': True, 'reason': args.reason,
        'source_count': len(names), 'complete_startup_source_sha256': source_digest.hexdigest(),
        'source_zip_sha256': sha(startup), 'browser_was_started': browser is not None,
        'browser_closed': browser['owned_browser_closed'] if browser else True,
        'native_fixture_exit_code': browser['native_fixture_exit_code'] if browser else args.native_fixture_exit,
        'database_cleanup_passed': browser['isolated_database_cleanup_passed'] if browser else args.native_fixture_exit == 0 and not (APP / '.local/business-browser-fixture.json').exists(),
        'media': media}
    (target / 'attempt.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    if browser:
        for file in BROWSER.iterdir():
            if file.is_file():
                file.unlink()
    print(json.dumps({'archived': args.name, 'complete_startup_sources': len(names), 'browser_closed': report['browser_closed']}))


if __name__ == '__main__':
    main()
