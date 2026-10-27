"""Reconstruct increment86 capture sources from its published Git objects, without rebinding old media."""
import ast
import hashlib
import json
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
REF = '94081c0db039c07a04c382816e1e2a44eb9f079a'
EVIDENCE = ROOT / 'docs/quality/evidence/budget-comparison'


def blob(path):
    return subprocess.check_output(['git', 'show', REF + ':' + path], cwd=ROOT)


def objects(names):
    output = subprocess.check_output(['git', 'cat-file', '--batch'], cwd=ROOT,
        input=('\n'.join(REF + ':app/' + name for name in names) + '\n').encode())
    result, position = {}, 0
    for name in names:
        end = output.index(b'\n', position)
        header = output[position:end].split()
        assert len(header) == 3 and header[1] == b'blob'
        size = int(header[2]); position = end + 1
        result[name] = output[position:position + size]; position += size + 1
    assert position == len(output)
    return result


def digest(names, content):
    h = hashlib.sha256()
    for name in names:
        h.update(name.encode() + b'\0' + content[name] + b'\0')
    return h.hexdigest()


def reconstruct():
    source = ast.parse(blob('app/scripts/token_budget_sources.py'))
    scopes = next(ast.literal_eval(node.value) for node in ast.walk(source)
        if isinstance(node, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'scopes' for t in node.targets))
    full = sorted(p.removeprefix('app/') for p in subprocess.check_output(
        ['git', 'ls-tree', '-r', '-z', '--name-only', REF, '--', *['app/' + p for p in scopes]], cwd=ROOT).decode().split('\0') if p)
    clients = {}
    names = set(full)
    for client in ('web', 'mobile'):
        base = 'docs/quality/evidence/token-budgets-' + client + '/'
        manifest = json.loads(blob(base + 'source-manifest.json'))
        result = json.loads(blob(base + 'result.json'))
        names.update(manifest['files'])
        clients[client] = {'files': manifest['files'], 'recorded_sha256': manifest['sha256'],
            'recorded_startup_sha256': result['build_source_sha256']}
    content = objects(sorted(names))
    full_sha = digest(full, content)
    for value in clients.values():
        value['reconstructed_sha256'] = digest(value['files'], content)
        value['source_count'] = len(value.pop('files'))
        value['complete_capture_manifest_reconstructed'] = value['recorded_sha256'] == value['reconstructed_sha256']
        assert value['complete_capture_manifest_reconstructed'] and value['recorded_startup_sha256'] == full_sha
    paths = subprocess.check_output(['git', 'ls-tree', '-r', '-z', '--name-only', REF, '--',
        'docs/quality/evidence/token-budgets', 'docs/quality/evidence/token-budgets-web',
        'docs/quality/evidence/token-budgets-mobile'], cwd=ROOT).decode().split('\0')
    preserved = []
    for path in sorted(p for p in paths if p):
        original = blob(path)
        assert (ROOT / path).read_bytes() == original, 'Historical evidence changed: ' + path
        preserved.append({'path': path, 'sha256': hashlib.sha256(original).hexdigest()})
    return {'commit': REF, 'complete_startup_source': {'source_count': len(full), 'sha256': full_sha, 'reconstructed': True},
        'clients': clients, 'preserved_evidence': preserved,
        'scope': 'Published increment86 complete startup and both complete client capture manifests reconstruct from Git. Earlier increment84 cache limitations remain unchanged in its retained provenance.'}


if __name__ == '__main__':
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    value = reconstruct()
    (EVIDENCE / 'historical-86.json').write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'clients': len(value['clients']), 'preserved_evidence': len(value['preserved_evidence'])}))
