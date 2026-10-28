"""Build and exercise the real native CLI with isolated synthetic source files."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import uuid

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/file-imports'


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def backend_digest():
    spec = importlib.util.spec_from_file_location('import_backend', APP / 'scripts/verify_backend_batches.py')
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value.digest()


def main():
    before = backend_digest()
    E.mkdir(parents=True, exist_ok=True)
    checks, binaries = [], []
    log = APP / '.local/file-import-cli-build.log'
    stamp = datetime.now(timezone.utc).isoformat()
    exe = APP / '.local/file-import.exe'
    with log.open('w', encoding='utf-8') as output:
        for target, artifact in [('windows', exe), ('linux', APP / '.local/file-import-linux-amd64')]:
            env = {**os.environ, 'GOOS': target, 'GOARCH': 'amd64', 'CGO_ENABLED': '0', 'GOMAXPROCS': '2', 'GOFLAGS': '-p=1'}
            status = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'build', '-o', str(artifact), './cmd/file-import'],
                cwd=APP / 'backend', env=env, stdout=output, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW).returncode
            assert status == 0
            binaries.append({'target': target + '/amd64', 'sha256': sha(artifact), 'bytes': artifact.stat().st_size,
                'artifact_executed': target == 'windows', 'deployed': False})
    with tempfile.TemporaryDirectory(prefix='cp-import-cli-', dir=APP / '.local') as temporary:
        base = Path(temporary)
        source = base / 'source'
        private = base / 'private'
        source.mkdir()
        private.mkdir()
        (source / 'synthetic-original.txt').write_bytes(b'Actual native CLI synthetic source\n')
        inventory, template, mapping, plan = [private / (n + '.jsonl') for n in ('inventory', 'template', 'mapping', 'plan')]
        def invoke(name, args, expected):
            # The complete output is deliberately not published: only bounded codes.
            result = subprocess.run([str(exe), *args], capture_output=True, text=True, encoding='utf-8',
                env={k:v for k,v in os.environ.items() if k != 'CP_FILE_IMPORT_TOKEN'}, creationflags=subprocess.CREATE_NO_WINDOW)
            assert (result.returncode == 0) == expected, name
            checks.append({'name': name, 'passed': True, 'exit_code': result.returncode,
                'stdout_sha256': hashlib.sha256(result.stdout.encode()).hexdigest(),
                'stderr_sha256': hashlib.sha256(result.stderr.encode()).hexdigest()})
        ns = str(uuid.uuid4())
        invoke('actual source inventory', ['inventory','--root',str(source),'--namespace',ns,'--out',str(inventory)], True)
        invoke('mapping template', ['template','--inventory',str(inventory),'--out',str(template)], True)
        invoke('unreviewed template rejected', ['plan','--inventory',str(inventory),'--mapping',str(template),'--reason','unreviewed source','--out',str(plan)], False)
        assert not plan.exists()
        item = json.loads(template.read_text())
        item.update(source_owner='Reviewed synthetic source owner',mapped_owner_id=str(uuid.uuid4()),folder_id=str(uuid.uuid4()),folder_version=1,classification='internal')
        mapping.write_text(json.dumps(item,ensure_ascii=False)+'\n', encoding='utf-8')
        invoke('reviewed plan', ['plan','--inventory',str(inventory),'--mapping',str(mapping),'--reason','reviewed source mapping','--out',str(plan)], True)
        header = json.loads(plan.read_text().splitlines()[0])
        assert header['entry_count'] == 1 and header['total_bytes'] == str((source/'synthetic-original.txt').stat().st_size)
        invoke('existing inventory preserved', ['inventory','--root',str(source),'--namespace',ns,'--out',str(inventory)], False)
        invoke('inside-source output rejected', ['inventory','--root',str(source),'--namespace',ns,'--out',str(source/'bad.jsonl')], False)
        invoke('missing token rejected', ['report','--api','https://uncontacted.invalid/v1','--batch',str(uuid.uuid4()),'--out',str(private/'report.jsonl')], False)
    after = backend_digest()
    report = {'passed': before == after and all(c['passed'] for c in checks), 'started_at': stamp,
        'finished_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': before, 'source_after_sha256': after,
        'source_unchanged': before == after, 'binaries': binaries, 'checks': checks, 'raw_build_log_sha256': sha(log),
        'synthetic_only': True, 'real_cloud': False, 'real_2tb_inventory': False, 'real_120gb_transport': False,
        'private_temp_source_and_outputs_removed': not base.exists(), 'verification_script_sha256': sha(Path(__file__))}
    (E/'native-cli.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'native_cli_checks':len(checks),'temporary_source_removed':not base.exists()}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
