"""Preserve passed pre-correction evidence before removing one Web EOF blank line."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import zipfile
from payment_fx_sources import APP, sources, digest

ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/payment-fx'
TARGET = E / 'attempts/06-web-eof-whitespace'
PRIVATE = APP / '.local/payment-fx-before-eof'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def module(name):
    spec = importlib.util.spec_from_file_location('eof_archive_' + name, APP / 'scripts' / (name + '.py'))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def main():
    assert not TARGET.exists() and not PRIVATE.exists()
    client = read(E / 'client-checks.json')
    publication = read(E / 'repository-check.json')
    before = digest()
    assert client['passed'] and read(E / 'final-source-check.json')['passed']
    assert not publication['passed'] and publication['publication_whitespace_findings'] == ['app/web/src/Payments.tsx']
    assert all(v['passed'] for v in publication['checks'] if v['name'] != 'Publication code and guides have no trailing whitespace or extra EOF blank lines')
    source = APP / 'web/src/Payments.tsx'
    old = source.read_bytes()
    new = old.rstrip(b'\r\n') + b'\r\n'
    assert old == new + b'\r\n'
    android_before = module('build_android').sources()[0]
    backend_before = module('verify_backend_batches').digest()
    assert android_before == read(E / 'build.json')['source_sha256']
    TARGET.mkdir(parents=True)
    PRIVATE.mkdir()
    archive = PRIVATE / 'sources.zip'
    with zipfile.ZipFile(archive, 'x', compression=zipfile.ZIP_DEFLATED) as output:
        for name in sorted(set(sources() + client['source_files'])):
            output.write(APP / name, name)
    public_records = []
    private_records = []

    def preserve(file, target, records):
        assert file.is_file() and not target.exists()
        assert file.resolve().is_relative_to(ROOT.resolve()) and target.resolve().is_relative_to(ROOT.resolve())
        target.parent.mkdir(parents=True, exist_ok=True)
        value = {'original_file': file.relative_to(ROOT).as_posix(), 'file': target.relative_to(ROOT).as_posix(), 'sha256': sha(file)}
        shutil.move(str(file), str(target))
        assert sha(target) == value['sha256']
        records.append(value)

    for name in ('client-checks.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json',
                 'mobile-tests.log', 'media.json', 'web-capture-manifest.json', 'mobile-capture-manifest.json',
                 'visual-review.json', 'final-source-check.json', 'scope-check.json',
                 'acceptance-applicability-check.json', 'repository-check.json', 'native-cluster-closed.json'):
        preserve(E / name, TARGET / name, public_records)
    for target in ('web', 'mobile'):
        folder = E.parent / ('payment-fx-' + target)
        for file in sorted(folder.iterdir()):
            preserve(file, TARGET / target / file.name, public_records)
        for file in sorted((APP / '.local').glob('payment-fx-' + target + '-*.log')):
            preserve(file, PRIVATE / 'logs' / file.name, private_records)
        capture = APP / '.local/repository-showcase' / ('payment-fx-' + target)
        replacement = PRIVATE / ('capture-' + target)
        assert capture.resolve().is_relative_to((APP / '.local').resolve()) and not replacement.exists()
        for file in sorted(capture.rglob('*')):
            if file.is_file():
                private_records.append({'original_file': file.relative_to(ROOT).as_posix(),
                    'file': (replacement / file.relative_to(capture)).relative_to(ROOT).as_posix(), 'sha256': sha(file)})
        shutil.move(str(capture), str(replacement))
        for suffix in ('quote', 'narrow', 'ledger'):
            name = 'payment-fx-' + target + '-' + suffix + '.png'
            preserve(ROOT / 'docs/media/screenshots' / name, PRIVATE / 'media' / name, private_records)
        for folder_name, suffix in (('videos', 'mp4'), ('gifs', 'gif')):
            name = 'payment-fx-' + target + '.' + suffix
            preserve(ROOT / 'docs/media' / folder_name / name, PRIVATE / 'media' / name, private_records)
    for item in client['checks']:
        file = APP / '.local' / Path(item['raw_log']).name
        assert sha(file) == item['raw_log_sha256']
        preserve(file, PRIVATE / 'logs' / file.name, private_records)
    skill = APP / '.local/payment_fx-skill-check.log'
    preserve(skill, PRIVATE / 'logs' / skill.name, private_records)
    source.write_bytes(new)
    assert module('build_android').sources()[0] == android_before
    assert module('verify_backend_batches').digest() == backend_before
    result = {'passed_original_checks': True, 'counts_as_final_current_source': False,
        'archived_at': datetime.now(timezone.utc).isoformat(),
        'reason': 'A final publication gate found one extra Web EOF blank line. Remove it, then rerun clients and both actual UI flows; preserve original passed evidence and the failed publication record.',
        'source_archive': archive.relative_to(ROOT).as_posix(), 'source_archive_sha256': sha(archive),
        'old_interaction_source_sha256': before, 'new_interaction_source_sha256': digest(),
        'corrected_file': 'web/src/Payments.tsx', 'old_file_sha256': hashlib.sha256(old).hexdigest(),
        'new_file_sha256': sha(source), 'removed_bytes': 2, 'code_tokens_changed': False,
        'android_inputs_unchanged': True, 'backend_inputs_unchanged': True,
        'public_records': public_records, 'private_records': private_records,
        'archive_script_sha256': sha(Path(__file__))}
    (TARGET / 'preservation.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'removed_bytes': 2, 'public_records': len(public_records), 'private_records': len(private_records), 'android_inputs_unchanged': True, 'backend_inputs_unchanged': True}))


if __name__ == '__main__':
    main()
