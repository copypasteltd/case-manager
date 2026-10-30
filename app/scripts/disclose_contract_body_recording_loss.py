"""Disclose two lost obsolete recordings without replacing them or claiming recovery."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/contract-body'
PUBLIC = E.parent / 'contract-body-attempts/recording-retention-v1'
PRIVATE = ROOT / 'app/.local/contract-body-attempts/recording-retention-v1'
ORIGINAL = 'app/.local/repository-showcase/contract-body-web/frames.json'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def main():
    assert not PUBLIC.exists() and not PRIVATE.exists()
    failed = E / 'final-source-check.json'
    proof = read(failed)
    assert not proof['passed'] and len([v for v in proof['checks'] if not v['passed']]) == 1
    PUBLIC.mkdir(parents=True)
    PRIVATE.mkdir(parents=True)
    saved = PUBLIC / 'original-failed-source-check.json'
    shutil.copyfile(failed, saved)
    records = [{'file': saved.relative_to(ROOT).as_posix(), 'sha256': sha(saved)}]
    # The first verifier is stored before its release-only repair.
    original_verifier = PRIVATE / 'verify_contract_body_sources.py'
    shutil.copyfile(ROOT / 'app/scripts/verify_contract_body_sources.py', original_verifier)
    assert sha(original_verifier) == proof['verification_script_sha256']
    records.append({'file': original_verifier.relative_to(ROOT).as_posix(), 'sha256': sha(original_verifier)})
    missing = []
    for label in ('03-before-mobile-field-fix-web', '04-before-mobile-field-repair-web'):
        path = E.parent / 'contract-body-attempts' / label / 'preservation.json'
        prior = read(path)
        entries = [v for v in prior['records'] if v['file'] == ORIGINAL]
        assert len(entries) == 1 and sha(ROOT / ORIGINAL) != entries[0]['sha256']
        missing.append({'preservation_file': path.relative_to(ROOT).as_posix(),
            'preservation_sha256': sha(path), 'file': ORIGINAL, 'expected_sha256': entries[0]['sha256'],
            'observed_reused_path_sha256': sha(ROOT / ORIGINAL), 'original_bytes_recovered': False})
    report = {'status': 'historical_recording_retention_failed', 'disclosed_at': datetime.now(timezone.utc).isoformat(),
        'historical_raw_recordings_retained': False, 'original_bytes_recovered': False,
        'counts_as_current_pass': False, 'missing_records': missing, 'records': records,
        'reason': 'Two earlier successful Web attempts retained screenshots, source archives, logs and result metadata, but their recorder manifest only referenced a reused temporary directory. Later captures overwrote those original frames and manifests. They are unavailable and cannot be reverified or counted as preserved recordings. Original metadata remains byte-exact and is not rewritten.',
        'current_recordings': 'The final Web/mobile capture manifests and encoded media are independently verified by final-source-check.json; no missing earlier recording is substituted or counted.',
        'correction': 'The passed-browser archiver now copies the original manifest and every hash-verified frame into the immutable attempt directory before later captures.',
        'corrected_archiver_sha256': sha(ROOT / 'app/scripts/archive_contract_body_passed_browser.py'),
        'disclosure_script_sha256': sha(Path(__file__))}
    (PUBLIC / 'disclosure.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'disclosed_lost_prior_recordings': len(missing), 'recovery_claim': False, 'final_pass_credit': False}))


if __name__ == '__main__':
    main()
