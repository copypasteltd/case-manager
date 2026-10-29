"""Preserve the passed Web check and failed mobile navigation attempt before a checker-only repair."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import zipfile
from payment_fx_sources import APP, sources, digest

ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/payment-fx'
TARGET = E / 'attempts/07-mobile-navigation-race'
PRIVATE = APP / '.local/payment_fx-attempts'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def main():
    web = E.parent / 'payment-fx-web'
    result, manifest = [read(web / name) for name in ('result.json', 'source-manifest.json')]
    failed = read(TARGET / 'mobile/result.json')
    assert result['passed'] and result['owned_browser_closed'] and result['isolated_database_cleanup_passed']
    assert not failed['passed'] and len(failed['checks']) == 4 and failed['owned_browser_closed']
    assert not (APP / '.local/business-browser-fixture.json').exists()
    archive = PRIVATE / '07-before-navigation-check.zip'
    assert not archive.exists() and not (TARGET / 'navigation-preservation.json').exists()
    names = sorted(set(sources() + manifest['files'] + read(TARGET / 'mobile/source-manifest.json')['files']))
    with zipfile.ZipFile(archive, 'x', compression=zipfile.ZIP_DEFLATED) as output:
        for name in names:
            output.write(APP / name, name)
    public, private = [], []

    def move(file, target, records):
        assert file.resolve().is_relative_to(ROOT.resolve()) and target.resolve().is_relative_to(ROOT.resolve())
        assert file.is_file() and not target.exists()
        target.parent.mkdir(parents=True, exist_ok=True)
        records.append({'original_file': file.relative_to(ROOT).as_posix(), 'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(file)})
        shutil.move(str(file), str(target))

    for file in sorted(web.iterdir()):
        move(file, TARGET / 'passed-web' / file.name, public)
    for suffix in ('native', 'browser', 'build'):
        file = APP / '.local' / ('payment-fx-web-' + suffix + '.log')
        move(file, PRIVATE / ('07-web-' + suffix + '.log'), private)
    for target in ('web', 'mobile'):
        capture = APP / '.local/repository-showcase' / ('payment-fx-' + target)
        replacement = PRIVATE / ('07-' + target + '-capture')
        assert capture.resolve().is_relative_to((APP / '.local').resolve()) and not replacement.exists()
        for file in sorted(capture.rglob('*')):
            if file.is_file():
                private.append({'original_file': file.relative_to(ROOT).as_posix(), 'file': (replacement / file.relative_to(capture)).relative_to(ROOT).as_posix(), 'sha256': sha(file)})
        shutil.move(str(capture), str(replacement))
        for suffix in ('quote', 'narrow', 'ledger'):
            file = ROOT / 'docs/media/screenshots' / ('payment-fx-' + target + '-' + suffix + '.png')
            if file.exists():
                move(file, PRIVATE / ('07-' + file.name), private)
    record = {'passed_original_web': True, 'mobile_passed': False, 'counts_as_final_current_source': False,
        'archived_at': datetime.now(timezone.utc).isoformat(), 'source_archive': archive.relative_to(ROOT).as_posix(),
        'source_archive_sha256': sha(archive), 'interaction_source_sha256': digest(),
        'checker_sha256': sha(APP / 'scripts/check_payment_fx.mjs'),
        'public_records': public, 'private_records': private, 'archive_script_sha256': sha(Path(__file__)),
        'diagnosis': 'The first four mobile groups commit and verify both axes. After release, the checker forces page.goto while useCommand still validates the receipt and schedules Taro.redirectTo. The failed screenshot shows a loading detail with two active pages. Repair only the checker: wait for the actual completed detail navigation and ledger before the next action. Keep all six assertions, their deadlines and all application inputs.'}
    (TARGET / 'navigation-preservation.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'source_preserved': True, 'public_records': len(public), 'private_records': len(private)}))


if __name__ == '__main__':
    main()
