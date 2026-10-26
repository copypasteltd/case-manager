"""Reconstruct the retained pre-LF proof bytes without changing historical evidence."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
from mobile_knowledge_sources import APP, digest, sources
from verify_backend_batches import digest as backend_digest

EVIDENCE = APP.parent / 'docs/quality/evidence/mobile-knowledge'
PREVIOUS = EVIDENCE / 'attempts/before-lf-restoration'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    scope = read(PREVIOUS / 'scope.json')
    previous = read(PREVIOUS / 'browser/source-manifest.json')
    backend = read(PREVIOUS / 'result.json')
    wait_attempt = EVIDENCE / 'attempts/browser-04'
    wait_fix = read(wait_attempt / 'scope.json')
    assert wait_fix['file'] == 'scripts/check_mobile_knowledge.mjs' and not wait_fix['application_code_changed']
    assert sha(APP / wait_fix['file']) == wait_fix['after_sha256']
    assert sha(wait_attempt / wait_fix['retained_source']) == wait_fix['before_sha256']
    changed = {item['file']: item for item in scope['changed_files']}
    assert len(changed) == 4 and not scope['code_tokens_changed']
    assert previous['files'] == sources()

    def retained(name):
        if name == wait_fix['file']:
            return (wait_attempt / wait_fix['retained_source']).read_bytes()
        value = (APP / name).read_bytes()
        if name in changed:
            assert b'\r' not in value and sha(APP / name) == changed[name]['after_sha256']
            value = value.replace(b'\n', b'\r\n')
            assert hashlib.sha256(value).hexdigest() == changed[name]['before_sha256']
        return value

    capture = hashlib.sha256()
    for name in previous['files']:
        capture.update(name.encode() + b'\0' + retained(name) + b'\0')
    assert capture.hexdigest() == previous['sha256']
    old_backend = hashlib.sha256()
    for path in sorted((APP / 'backend').rglob('*')):
        if path.is_file() and (path.suffix in ('.go', '.sql') or path.name in ('go.mod', 'go.sum') or path.relative_to(APP / 'backend').as_posix() == 'internal/contract/openapi.json'):
            old_backend.update(path.relative_to(APP / 'backend').as_posix().encode() + b'\0')
            old_backend.update(retained(path.relative_to(APP).as_posix()) + b'\0')
    assert old_backend.hexdigest() == backend['source_sha256']
    retained_media = read(PREVIOUS / 'artifact-map.json')
    assert retained_media['passed'] and len(retained_media['artifacts']) == 5
    assert all(sha(PREVIOUS / item['retained']) == item['sha256'] for item in retained_media['artifacts'])
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
        'changed_files': scope['changed_files'], 'all_other_capture_bytes_unchanged': True,
        'capture_wait_fix': wait_fix,
        'before_browser_source_sha256': capture.hexdigest(), 'after_browser_source_sha256': digest(),
        'before_backend_source_sha256': old_backend.hexdigest(), 'after_backend_source_sha256': backend_digest(),
        'archived_manifest_sha256': sha(PREVIOUS / 'browser/source-manifest.json'),
        'archived_media_exact': True, 'archived_artifact_map_sha256': sha(PREVIOUS / 'artifact-map.json'),
        'archived_backend_result_sha256': sha(PREVIOUS / 'result.json'),
        'verification_script_sha256': sha(Path(__file__)), 'historical_evidence_modified': False,
        'scope': 'Four Go CRLF-to-original-LF byte restorations and one declared browser-checker wait correction. Reconstruct the original complete proofs from retained bytes; current backend/Linux/browser checks run afresh.'}
    (EVIDENCE / 'source-normalization.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'lf_files': 4, 'previous_capture_reconstructed': True}))


if __name__ == '__main__':
    main()
