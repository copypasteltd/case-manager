"""Revalidate unchanged UI inputs and retain published media with original provenance."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/native-agent-runtime'
COMMIT = '3517ecfefe359d898f0ec40ac5c618fd99b6c29f'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('native_reuse_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    E.mkdir(parents=True, exist_ok=True)
    previous = E.parent / 'multipart-cleanup'
    mobile, android, web = [read(previous / name) for name in ('mobile-build.json', 'build.json', 'web-build.json')]
    assert mobile['passed'] and mobile['source_sha256'] == module('verify_mobile').digest()[0]
    assert android['passed'] and android['source_sha256'] == digest(android['sources'])
    assert web['passed'] and web['source_sha256'] == digest(web['sources'])
    assert mobile['tests']['pass'] == 261 and web['tests']['numPassedTests'] == 159
    records = []
    for source, copy in [(previous / 'mobile-build.json', E / 'mobile-build.json'), (previous / 'build.json', E / 'build.json')]:
        assert not copy.exists() or sha(copy) == sha(source)
        if not copy.exists():
            shutil.copyfile(source, copy)
        records.append({'file': source.relative_to(ROOT).as_posix(), 'copied_to': copy.relative_to(ROOT).as_posix(), 'sha256': sha(source), 'new_execution': False})
    for group, names in [('multipart-cleanup', ['web-build.json', 'android-test-scope.json', 'android-feature-content.json']), ('dependency-remediation', ['dependency-audit.json', 'remaining-findings.json'])]:
        for name in names:
            source = E.parent / group / name
            assert source.is_file()
            records.append({'file': source.relative_to(ROOT).as_posix(), 'sha256': sha(source), 'new_execution': False})
    tree = subprocess.check_output(['git', 'ls-tree', '-r', '-z', COMMIT, '--', 'docs/media'], cwd=ROOT)
    media = []
    for raw in tree.split(b'\0'):
        if not raw:
            continue
        header, name = raw.split(b'\t', 1)
        name = name.decode('utf-8')
        if Path(name).suffix.lower() not in {'.png', '.mp4', '.gif'}:
            continue
        blob = header.decode().split()[2]
        assert len(blob)==40, 'This retained Git object inventory must use the repository SHA1 format.'
        data=(ROOT/name).read_bytes()
        current=hashlib.sha1(b'blob '+str(len(data)).encode()+b'\0'+data).hexdigest()
        assert current == blob
        media.append({'file': name, 'git_blob': blob, 'sha256': sha(ROOT / name), 'bytes': (ROOT / name).stat().st_size})
    counts = {suffix: sum(Path(v['file']).suffix == '.' + suffix for v in media) for suffix in ('png', 'mp4', 'gif')}
    assert counts == {'png': 113, 'mp4': 43, 'gif': 43}
    baseline = subprocess.check_output(['git', 'show', COMMIT + ':docs/quality/current-functional-baseline.json'], cwd=ROOT)
    archive = E / 'retained104-functional-baseline.json'
    assert not archive.exists() or archive.read_bytes() == baseline
    if not archive.exists():
        archive.write_bytes(baseline)
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'published_commit': COMMIT,
              'verification_script_sha256': sha(Path(__file__)), 'records': records, 'media': media, 'media_counts': counts,
              'original_baseline_sha256': sha(archive), 'web_source_sha256': web['source_sha256'], 'mobile_source_sha256': mobile['source_sha256'], 'android_source_sha256': android['source_sha256'],
              'new_web_mobile_android_build_execution': False, 'new_browser_capture': False, 'new_jvm_execution': False,
              'original_scope_preserved': True, 'current_backend_browser_flow_claimed': False,
              'scope': 'Unchanged UI/build inputs and exact199 published media; original104 browser/API fixtures and JVM cache scope remain historical.'}
    (E / 'reused-evidence.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'media': counts, 'unchanged_client_inputs': True, 'new_capture': False}))


if __name__ == '__main__':
    main()
