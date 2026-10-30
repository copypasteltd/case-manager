"""Keep an unsuccessful check and its exact input/log bytes before any repair."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def archive_client(label):
    record = ROOT / 'docs/quality/evidence/contract-body/client-checks.json'
    value = json.loads(record.read_text(encoding='utf-8'))
    assert not value['passed'] and value['source_unchanged']
    names = value['source_files']
    digest = hashlib.sha256()
    for name in names:
        digest.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    assert digest.hexdigest() == value['source_sha256']
    private = APP / '.local/contract-body-attempts' / label
    assert not private.exists()
    private.mkdir(parents=True)
    source = private / 'source.zip'
    with zipfile.ZipFile(source, 'w', zipfile.ZIP_DEFLATED) as output:
        for name in names:
            output.write(APP / name, 'app/' + name)
    logs = []
    for item in value['checks']:
        original = APP / '.local' / Path(item['raw_log']).name
        assert sha(original) == item['raw_log_sha256']
        saved = private / original.name
        shutil.move(original, saved)
        logs.append({'name': saved.name, 'sha256': sha(saved)})
    public = ROOT / 'docs/quality/evidence/contract-body-attempts' / (label + '.json')
    assert not public.exists()
    public.parent.mkdir(parents=True, exist_ok=True)
    value['preservation'] = {'archived_at': datetime.now(timezone.utc).isoformat(),
        'source_zip_sha256': sha(source), 'source_entries': len(names), 'source_and_logs_private': True,
        'logs': logs, 'counted_as_current_pass': False, 'script_sha256': sha(Path(__file__))}
    public.write_text(json.dumps(value, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')
    record.unlink()
    print(json.dumps({'archived': label, 'input_entries': len(names), 'current_pass_claim': False}))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('label', choices=['client-types-v1'])
    args = parser.parse_args()
    archive_client(args.label)
