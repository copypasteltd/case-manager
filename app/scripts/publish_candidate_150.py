"""Prepare new immutable Git refs with a separate index; never pushes or resets.

Only a subsequent explicit Git push publishes these already sealed objects.
Existing branches/tags are refused rather than moved or force-pushed.
"""
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import subprocess
from build_candidate_150 import APP, ROOT, E, VERSION, SOURCE, sha
from seal_candidate_150 import OUT, RELEASE


def main():
    assert not (RELEASE / 'git-publication.json').exists()
    manifest = json.loads((RELEASE / 'release-manifest.json').read_text(encoding='utf-8'))
    assert manifest['source']['commit'] == SOURCE
    branch = 'refs/heads/codex/release-' + VERSION
    tag = 'refs/tags/' + VERSION

    def git(*args, data=None, env=None):
        return subprocess.run(['git', *args], cwd=ROOT, input=data, env=env, capture_output=True, check=True).stdout.decode().strip()

    for ref in (branch, tag):
        result = subprocess.run(['git', 'show-ref', '--verify', '--quiet', ref], cwd=ROOT)
        assert result.returncode == 1, 'A candidate ref already exists; never overwrite it.'
    assert git('rev-parse', 'HEAD') == SOURCE
    for asset in manifest['assets']:
        path = OUT / asset['name']
        assert sha(path) == asset['sha256'] and path.stat().st_size == asset['bytes']
    index = OUT.parent / (VERSION + '-artifact.index')
    assert not index.exists()
    env = {**os.environ, 'GIT_INDEX_FILE': str(index)}
    git('read-tree', SOURCE, env=env)
    paths = [(path, 'docs/releases/' + VERSION + '/' + path.name) for path in OUT.iterdir() if path.is_file()]
    paths += [(path, path.relative_to(ROOT).as_posix()) for path in RELEASE.iterdir() if path.is_file()]
    rows = {}
    for path, name in paths:
        blob = git('hash-object', '-w', '--no-filters', '--stdin', data=path.read_bytes())
        if name in rows:
            assert rows[name] == blob
        rows[name] = blob
    git('update-index', '--index-info', env=env,
        data=''.join('100644 ' + blob + '\t' + name + '\n' for name, blob in sorted(rows.items())).encode())
    tree = git('write-tree', env=env)
    body = ('release: seal ' + VERSION + ' candidate artifacts\n\n'
        'Fixed business source ' + SOURCE + '; schema111.\n'
        'Native server, miniapps, development APK, complete source and media archives.\n'
        'Reduced validation is explicit;23 work packages and production acceptance remain open.\n').encode()
    commit = git('commit-tree', tree, '-p', SOURCE, data=body)
    git('update-ref', branch, commit, '0' * 40)
    git('tag', '-a', VERSION, SOURCE, '-F', '-', data=(
        VERSION + ' frozen candidate / 冻结候选\n\n'
        'Schema111; includes all business changes through149, including verified replica recovery notifications and exact version routing. Development Android only.\n'
        'Full-source/media assets on codex/release-' + VERSION + '.\n'
        'Current packaging/artifact inspection; prior149 business checks retained without replay. Broader validation deferred by user; not production acceptance.\n').encode('utf-8'))
    record = {'format': 1, 'prepared_at': datetime.now(timezone.utc).isoformat(),
        'version': VERSION, 'source_commit': SOURCE, 'source_tag': tag, 'tag_object': git('rev-parse', tag),
        'artifact_ref': branch, 'artifact_commit': commit, 'artifact_tree': tree,
        'manifest_sha256': sha(RELEASE / 'release-manifest.json'),
        'publisher_script_sha256': sha(Path(__file__)),
        'transport': 'Prepared immutable refs; authorized SSH push and remote verification are performed separately.',
        'force_push_used': False, 'user_index_modified': False, 'github_release_page_created': False}
    (RELEASE / 'git-publication.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'source_tag': VERSION, 'source_commit': SOURCE, 'artifact_commit': commit}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
