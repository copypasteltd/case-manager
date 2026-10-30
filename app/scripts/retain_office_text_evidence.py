"""Verify prior published media/evidence bytes; archive current source privately."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import zipfile
from office_text_sources import APP, sources, digest

ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/office-text'
BASE = 'ec2d2510b4ceb1b07459f66efca8477cb229ec1f'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    rows = []
    for line in subprocess.check_output(['git', 'ls-tree', '-r', BASE, '--', 'docs/media', 'docs/quality/evidence'], cwd=ROOT).decode().splitlines():
        meta, name = line.split('\t', 1)
        if name.startswith('docs/quality/evidence/') or Path(name).suffix.lower() in ('.png', '.gif', '.mp4'):
            rows.append((name, meta.split()[2]))
    # Compare raw Git blob identities directly: no long Windows command line,
    # stdin pipe or optional conversion filters. The repository is SHA-1 based.
    verified = []
    for name, expected in rows:
        data = (ROOT / name).read_bytes()
        actual = hashlib.sha1(b'blob ' + str(len(data)).encode() + b'\0' + data).hexdigest()
        assert actual == expected, 'Do not overwrite prior published evidence: ' + name
        verified.append({'file': name, 'git_blob': expected, 'sha256': hashlib.sha256(data).hexdigest()})
    media = [n for n, _ in rows if n.startswith('docs/media/')]
    assert len(media) == 316
    record = {'passed': True, 'published_commit': BASE, 'all_prior_evidence_git_objects_unchanged': True,
        'records': verified, 'prior_media_count': len(media), 'comparison': 'Raw Git SHA-1 blob identity and SHA-256, with no filters.',
        'verification_script_sha256': sha(Path(__file__)), 'new_dependency_audit': False,
        'package_lock_sha256': sha(APP / 'package-lock.json')}
    (E / 'reused-evidence.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    prior_audit = ROOT / 'docs/quality/evidence/payment-fee/dependency-audit.json'
    audit = json.loads(prior_audit.read_text(encoding='utf-8'))
    assert audit['package_lock_sha256'] == sha(APP / 'package-lock.json') and not audit['production_security_gate_passed']
    assert not (E / 'dependency-audit.json').exists() or sha(E / 'dependency-audit.json') == sha(prior_audit)
    shutil.copyfile(prior_audit, E / 'dependency-audit.json')
    names = sources()
    archive = APP / '.local/office-text-final-sources.zip'
    assert not archive.exists(), 'Preserve any existing source archive.'
    before = digest(names)
    with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as z:
        for name in names:
            z.write(APP / name, name)
    assert before == digest(names)
    (E / 'source-archive.json').write_text(json.dumps({'passed': True, 'source_sha256': before,
        'private_archive_sha256': sha(archive), 'files': [{'file': n, 'sha256': sha(APP / n)} for n in names],
        'credentials_included': False, 'source_only': True}, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'prior_media': len(media), 'prior_evidence_and_media_files': len(rows), 'source_files': len(names)}))


if __name__ == '__main__':
    main()
