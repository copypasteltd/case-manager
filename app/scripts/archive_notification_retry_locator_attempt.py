"""Archive the closed H5 locator failure and its exact pre-fix Web/source evidence."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import shutil
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/notification-retry'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    target = E / 'attempts/11-h5-locator'
    private = APP / '.local/notification-retry-attempts/browser03'
    assert not target.exists() and not private.exists() and not (APP / '.local/business-browser-fixture.json').exists()
    directories = {client:E.parent / ('notification-retry-' + client) for client in ('web','mobile')}
    results = {client:read(directory / 'result.json') for client,directory in directories.items()}
    manifests = {client:read(directory / 'source-manifest.json') for client,directory in directories.items()}
    assert results['web']['passed'] and not results['mobile']['passed'] and not results['mobile']['checks']
    assert 'locator.click: Timeout' in results['mobile']['failure']
    assert all(value['native_fixture_exit_code'] == 0 and value['isolated_database_cleanup_passed'] and value['owned_browser_closed'] and value['owned_review_server_stopped'] for value in results.values())
    for client,manifest in manifests.items():
        digest = hashlib.sha256()
        for name in manifest['files']:
            digest.update(name.encode()+b'\0'+(APP/name).read_bytes()+b'\0')
        assert digest.hexdigest() == manifest['sha256'] == results[client]['source_sha256']
    target.mkdir(parents=True); private.mkdir(parents=True)
    source = private / 'source.zip'
    with zipfile.ZipFile(source,'w',zipfile.ZIP_DEFLATED) as archive:
        for name in sorted(set(name for value in manifests.values() for name in value['files'])):
            archive.write(APP / name,name)
    logs = []
    for client,directory in directories.items():
        for phase in ('native','build','browser'):
            original = APP / '.local' / ('notification-retry-' + client + '-' + phase + '.log')
            saved = private / original.name
            shutil.copyfile(original,saved)
            logs.append({'file':saved.relative_to(ROOT).as_posix(),'sha256':sha(saved)})
        if client == 'web':
            capture = read(directory/'capture.json')
            for shot in capture['screenshots']:
                assert sha(ROOT/shot['file']) == shot['sha256']
                shutil.copyfile(ROOT/shot['file'],target/Path(shot['file']).name)
            frames = APP / '.local/repository-showcase/notification-retry-web'
            assert frames.is_dir()
            shutil.move(str(frames),str(private/'web-frames'))
        shutil.move(str(directory),str(target/(client+'-before-locator-change')))
    record = {'passed':False,'archived_at':datetime.now(timezone.utc).isoformat(),
        'scope':'Shared-mobile H5 locator timed out before opening a failure row; both native fixture cleanups passed. Retain the pre-helper-change Web pass as historical, then rerun both clients at the corrected helper source.',
        'browser_contexts_closed':True,'native_fixture_cleanups_passed':True,'application_source_changed':False,
        'source_archive':source.relative_to(ROOT).as_posix(),'archive_sha256':sha(source),'raw_logs':logs,
        'web_source_sha256':manifests['web']['sha256'],'mobile_source_sha256':manifests['mobile']['sha256'],
        'verification_script_sha256':sha(Path(__file__))}
    (target/'attempt.json').write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':True,'archived_failed_h5':True,'preserved_pre_fix_web':True,'private_logs':len(logs)}))


if __name__ == '__main__':
    main()
