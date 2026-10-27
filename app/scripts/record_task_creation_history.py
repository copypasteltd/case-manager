"""Reconstruct retained increment84 captures from their original committed blobs."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import ast

ROOT = Path(__file__).resolve().parents[2]
COMMIT = '24ad850940f5840803fabf325ae33bff92f20ef1'
RETAINED = ['docs/quality/evidence/project-concerns/' + name for name in
    ('final-source-check.json', 'result.json', 'media.json', 'verification.md', 'android-test-scope.json')]


def reconstruct():
    manifests = {}
    names = list(RETAINED)
    for client in ('web', 'mobile'):
        folder = 'docs/quality/evidence/project-concerns-' + client + '/'
        manifest_file = folder + 'source-manifest.json'
        manifest = json.loads(subprocess.check_output(['git', 'show', COMMIT + ':' + manifest_file], cwd=ROOT))
        manifests[client] = (manifest, folder)
        names += ['app/' + file for file in manifest['files']] + [manifest_file, folder + 'result.json', folder + 'capture.json']
    original_collector = subprocess.check_output(['git', 'show', COMMIT + ':app/scripts/project_concern_sources.py'], cwd=ROOT).decode()
    tree = ast.parse(original_collector)
    scopes = next(ast.literal_eval(node.value) for node in ast.walk(tree) if isinstance(node, ast.Assign)
        and any(isinstance(target, ast.Name) and target.id == 'scopes' for target in node.targets))
    startup = subprocess.check_output(['git', 'ls-tree', '-r', '--name-only', COMMIT, '--', *['app/' + scope for scope in scopes]], cwd=ROOT).decode().splitlines()
    names += startup
    # The old generic Web collector included one ignored compiler cache which
    # was never frozen. Do not fabricate that artifact or a complete replay.
    absent = 'app/web/tsconfig.tsbuildinfo'
    names = sorted(set(name for name in names if name != absent))
    output = subprocess.run(['git', 'cat-file', '--batch'], cwd=ROOT,
        input=''.join(COMMIT + ':' + name + '\n' for name in names).encode(), stdout=subprocess.PIPE, check=True).stdout
    offset, blobs = 0, {}
    for name in names:
        end = output.index(b'\n', offset)
        header = output[offset:end].split()
        assert len(header) == 3 and header[1] == b'blob'
        size = int(header[2]); offset = end + 1
        blobs[name] = output[offset:offset + size]; offset += size + 1
    assert offset == len(output)
    clients = {}
    retained = []
    for client, (manifest, folder) in manifests.items():
        h = hashlib.sha256()
        files = [name for name in manifest['files'] if 'app/' + name != absent]
        for name in files:
            h.update(name.encode() + b'\0' + blobs['app/' + name] + b'\0')
        result = json.loads(blobs[folder + 'result.json'])
        complete = len(files) == len(manifest['files'])
        assert result['passed'] and manifest['sha256'] == result['source_sha256']
        if complete:
            assert h.hexdigest() == manifest['sha256']
        clients[client] = {'captured_source_count': len(manifest['files']), 'reconstructed_source_count': len(files),
            'committed_source_sha256': h.hexdigest(), 'recorded_capture_sha256': manifest['sha256'],
            'complete_capture_manifest_reconstructed': complete,
            'unfrozen_ignored_artifacts': [] if complete else ['web/tsconfig.tsbuildinfo']}
        for suffix in ('source-manifest.json', 'result.json', 'capture.json'):
            name = folder + suffix
            assert (ROOT / name).read_bytes() == blobs[name]
            retained.append({'file': name, 'sha256': hashlib.sha256(blobs[name]).hexdigest()})
    for name in RETAINED:
        assert (ROOT / name).read_bytes() == blobs[name]
        retained.append({'file': name, 'sha256': hashlib.sha256(blobs[name]).hexdigest()})
    h = hashlib.sha256()
    for name in sorted(startup):
        h.update(name.removeprefix('app/').encode() + b'\0' + blobs[name] + b'\0')
    assert all(json.loads(blobs[folder + 'result.json'])['build_source_sha256'] == h.hexdigest() for _, folder in manifests.values())
    return {'passed': True, 'commit': COMMIT, 'clients': clients, 'retained_files': retained,
        'complete_startup_source': {'source_count': len(startup), 'source_sha256': h.hexdigest(), 'reconstructed': True},
        'scope': 'Original complete startup sources and mobile capture reconstructed from Git; Web committed source subset and original evidence retained. The unfrozen ignored Web compiler cache cannot be reconstructed. No current-source or new-execution claim.'}


def main():
    value = reconstruct()
    value['recorded_at'] = datetime.now(timezone.utc).isoformat()
    target = ROOT / 'docs/quality/evidence/task-creation/historical-84.json'
    target.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'clients': value['clients']}))


if __name__ == '__main__':
    main()
