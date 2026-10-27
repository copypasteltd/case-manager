"""Reconstruct published90 capture sources and preserve its evidence/media without rebinding."""
import ast
import hashlib
import json
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
REF = 'e45c347a31cc7496b77585dc5328bcfdec92ec6d'
EVIDENCE = ROOT / 'docs/quality/evidence/task-dependencies'


def blob(path):
    return subprocess.check_output(['git', 'show', REF + ':' + path], cwd=ROOT)


def names(scopes):
    return sorted(p for p in subprocess.check_output(
        ['git', 'ls-tree', '-r', '-z', '--name-only', REF, '--', *scopes], cwd=ROOT).decode().split('\0') if p)


def objects(paths):
    output = subprocess.check_output(['git', 'cat-file', '--batch'], cwd=ROOT,
        input=('\n'.join(REF + ':' + name for name in paths) + '\n').encode())
    result, position = {}, 0
    for name in paths:
        end = output.index(b'\n', position)
        header = output[position:end].split()
        assert len(header) == 3 and header[1] == b'blob'
        size = int(header[2]); position = end + 1
        result[name] = output[position:position + size]; position += size + 1
    assert position == len(output)
    return result


def digest(paths, content):
    value = hashlib.sha256()
    for name in paths:
        value.update(name.encode() + b'\0' + content['app/' + name] + b'\0')
    return value.hexdigest()


def reconstruct():
    tree = ast.parse(blob('app/scripts/dependency_hardening_sources.py'))
    scopes = next(ast.literal_eval(node.value) for node in ast.walk(tree)
        if isinstance(node, ast.Assign) and any(isinstance(t, ast.Name) and t.id == 'scopes' for t in node.targets))
    full = [p.removeprefix('app/') for p in names(['app/' + p for p in scopes])]
    base = 'docs/quality/evidence/dependency-hardening-mobile/'
    manifest = json.loads(blob(base + 'source-manifest.json'))
    capture = json.loads(blob(base + 'result.json'))
    paths = sorted(set(full + manifest['files']))
    content = objects(['app/' + p for p in paths])
    full_sha, capture_sha = digest(full, content), digest(manifest['files'], content)
    assert full_sha == capture['build_source_sha256'] == capture['build_source_after_sha256']
    assert capture_sha == manifest['sha256'] == capture['source_sha256'] == capture['source_after_sha256']
    preserved = names(['docs/quality/evidence/dependency-hardening', 'docs/quality/evidence/dependency-hardening-mobile'])
    media = [p for p in names(['docs/media/screenshots','docs/media/videos','docs/media/gifs'])
        if Path(p).name.startswith('dependency-hardening-')]
    originals = objects(preserved + media)
    records = []
    for path in preserved + media:
        original = originals[path]
        assert (ROOT / path).read_bytes() == original, 'Historical bytes changed: ' + path
        records.append({'path': path, 'sha256': hashlib.sha256(original).hexdigest()})
    assert len(media) == 8
    return {'commit': REF, 'complete_startup_source': {'source_count': len(full), 'sha256': full_sha, 'reconstructed': True},
        'clients': {'mobile': {'source_count': len(manifest['files']), 'recorded_sha256': capture_sha,
            'complete_capture_manifest_reconstructed': True}},
        'preserved_evidence': records, 'preserved_media_count': len(media),
        'scope': 'Published90 complete startup/capture sources reconstructed from exact Git objects. All its evidence and eight original media files remain unchanged. Earlier limitations stay at their original records.'}


if __name__ == '__main__':
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    value = reconstruct()
    (EVIDENCE / 'historical-90.json').write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'preserved': len(value['preserved_evidence']), 'media': value['preserved_media_count']}))
