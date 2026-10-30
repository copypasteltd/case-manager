"""Freeze the union of all final declared source inputs; this is preservation, not a test."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/payment-fee'


def module(name):
    spec = importlib.util.spec_from_file_location('body_archive_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    assert not (E / 'source-archive.json').exists()
    client, android = [read(E / n) for n in ('client-checks.json', 'build.json')]
    assert client['passed'] and android['passed']
    interaction = module('payment_fee_sources')
    names = set(interaction.sources()) | set(client['source_files']) | set(android['sources'])
    names.update(read(E / 'web-build.json')['sources'])
    for target in ('web', 'mobile'):
        folder = E.parent / ('payment-fee-' + target)
        assert read(folder / 'result.json')['passed']
        names.update(read(folder / 'source-manifest.json')['files'])
    names.update(module('verify_agent_runtime').sources())
    names.update(p.relative_to(APP).as_posix() for p in (APP / 'scripts').glob('*payment_fee*') if p.suffix in ('.py', '.mjs'))
    names.update('scripts/' + name for name in ('verify_backend_batches.py', 'verify_web.py', 'verify_mobile.py', 'verify_agent_runtime.py', 'build_android.py', 'inspect_android_apk.py', 'setup_android_native.py'))
    paths = set()
    for name in names:
        path = (APP / name).resolve()
        assert path.is_relative_to(ROOT.resolve()) and path.is_file()
        paths.add(path.relative_to(ROOT.resolve()).as_posix())
    archive = APP / '.local/payment-fee-final-source.zip'
    assert not archive.exists()
    receipts = [{'file': name, 'sha256': sha(ROOT / name)} for name in sorted(paths)]
    with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as output:
        for item in receipts:
            output.write(ROOT / item['file'], item['file'])
    with zipfile.ZipFile(archive) as saved:
        assert all(hashlib.sha256(saved.read(v['file'])).hexdigest() == v['sha256'] == sha(ROOT / v['file']) for v in receipts)
    report = {'passed': True, 'preserved_at': datetime.now(timezone.utc).isoformat(),
        'private_archive': archive.relative_to(ROOT).as_posix(), 'archive_sha256': sha(archive),
        'files': receipts, 'all_declared_inputs_included': True, 'counts_as_test_execution': False,
        'interaction_source_sha256': interaction.digest(), 'archive_script_sha256': sha(Path(__file__))}
    (E / 'source-archive.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'preserved': True, 'source_files': len(receipts), 'test_execution_claim': False}))


if __name__ == '__main__':
    main()
