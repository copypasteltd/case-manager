"""Build exact frozen server inputs with the declared native Node on PATH.

Windows executable lookup may use the parent's PATH even when a different child
environment is passed. Set the process PATH before invoking the original builder.
Keep its real dirty-workspace flag; independently verify every input against Git.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess

import build_production_release as server
from build_candidate_146 import APP, ROOT, E, SOURCE, VERSION, sha

OUTPUT = APP / '.local/frozen-releases' / (VERSION + '-server-node22-r2')


def main():
    assert not OUTPUT.exists() and not (E / 'server-build-scope.json').exists()
    assert subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip() == SOURCE
    entries = {}
    for row in subprocess.check_output(['git', 'ls-tree', '-rz', SOURCE], cwd=ROOT).split(b'\0'):
        if row:
            metadata, name = row.split(b'\t', 1)
            entries[name.decode('utf-8')] = metadata.decode().split()[2]
    before = server.inputs()
    for row in before:
        raw = (ROOT / row['path']).read_bytes()
        actual = hashlib.sha1(b'blob ' + str(len(raw)).encode() + b'\0' + raw).hexdigest()
        assert entries.get(row['path']) == actual, 'A build input differs from frozen source.'
    node = APP / '.tools/node-v22.23.4-win-x64/node.exe'
    original_path = os.environ.get('PATH', '')
    try:
        os.environ['PATH'] = str(node.parent) + os.pathsep + original_path
        assert subprocess.check_output(['node', '--version']).decode().strip() == 'v22.23.4'
        report = server.build(OUTPUT)
    finally:
        os.environ['PATH'] = original_path
    assert report['passed'] and report['tool_versions']['node'] == 'v22.23.4'
    assert before == report['source_files'] == server.inputs()
    scope = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
        'source_commit': SOURCE, 'frozen_input_bytes_verified': True,
        'source_files': before, 'inputs_sha256': report['source']['inputs_sha256'],
        'whole_workspace_dirty_at_build': report['source']['working_tree_dirty'],
        'workspace_note': 'Release146 helpers/docs are pending; every actual server input matches frozen145 Git bytes.',
        'node': report['tool_versions']['node'], 'wrapper_sha256': sha(Path(__file__)),
        'original_builder_sha256': sha(APP / 'scripts/build_production_release.py'),
        'first_attempt_published': False, 'production_ready': False}
    (E / 'server-build-scope.json').write_text(json.dumps(scope, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'node': scope['node'], 'frozen_inputs': len(before),
        'workspace_dirty': scope['whole_workspace_dirty_at_build'], 'release_id': report['release_id']}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
