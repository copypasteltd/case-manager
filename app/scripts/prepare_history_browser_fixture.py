"""Owned metadata-only restoration UI fixture; no cloud or scan evidence.

Requires the explicitly labelled file produced by prepare_storage_browser_fixture.
The queued row has an unavailable source, no job, and no successful scan/copy.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlsplit
import uuid

APP = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--file', required=True, type=uuid.UUID)
parser.add_argument('--owner', required=True, type=uuid.UUID)
args = parser.parse_args()
values = json.loads((APP / '.local/environment.json').read_text('utf-8'))
source = urlsplit(values['CP_DATABASE_URL'])
if values.get('CP_ENV') != 'development' or source.hostname != '127.0.0.1' or source.port != values['_postgres_port']:
    raise SystemExit('Only the owned local development database is supported')
restore_id, replica_id, planned = (str(uuid.uuid4()) for _ in range(3))
fixed = json.dumps({'key': 'metadata-fixture/unavailable', 'etag': 'metadata-only', 'size': 120000000000}, separators=(',', ':'))
sql = """
BEGIN;
DO $guard$ BEGIN
    IF NOT EXISTS(SELECT 1 FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
        JOIN projects p ON p.org_id=d.org_id AND p.id=d.project_id
        JOIN users u ON u.org_id=f.org_id AND u.id=f.creator_id
        WHERE f.id='%(file)s' AND f.creator_id='%(owner)s' AND u.login_key='project.manager'
        AND p.name LIKE '存储验证项目 %%' AND d.name LIKE '界面验证资料 %%'
        AND f.name='元数据样本-120GB.bin' AND f.state='active'
        AND (SELECT count(*) FROM file_versions WHERE file_id=f.id AND source='local_metadata_fixture'
            AND scan_state='unscannable' AND availability='missing')=2) THEN
        RAISE EXCEPTION 'fixture scope check failed';
    END IF;
END $guard$;
INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,state,expected_sha256)
SELECT '%(replica)s',org_id,id,'oss','metadata-fixture','metadata-fixture/unavailable','missing',sha256
FROM file_versions WHERE file_id='%(file)s' AND seq=1;
INSERT INTO file_history_restores(id,org_id,file_id,owner_id,owner_auth_version,org_auth_version,source_channel,folder_id,classification,
    base_file_version,base_version_id,source_version_id,planned_version_id,source_replica_id,source_provider,source_bucket,fixed_source,
    source_digest,sha256,size_bytes,storage_epoch,reason)
SELECT '%(restore)s',f.org_id,f.id,u.id,u.auth_version,o.authorization_version,'web',f.folder_id,f.classification,f.version,
    f.current_version_id,v.id,'%(planned)s','%(replica)s','oss','metadata-fixture','%(fixed)s'::jsonb,
    '%(digest)s',v.sha256,v.size_bytes,1,'界面元数据任务：无对象、无扫描、无复制'
FROM files f JOIN file_versions v ON v.org_id=f.org_id AND v.file_id=f.id AND v.seq=1
JOIN users u ON u.org_id=f.org_id AND u.id=f.creator_id JOIN organizations o ON o.id=f.org_id WHERE f.id='%(file)s';
COMMIT;
""" % dict(file=str(args.file), owner=str(args.owner), replica=replica_id, restore=restore_id, planned=planned,
           fixed=fixed, digest=hashlib.sha256(fixed.encode()).hexdigest())
env = {**os.environ, 'PGPASSWORD': unquote(source.password), 'PGCLIENTENCODING': 'UTF8'}
process = subprocess.run([
    str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-h', source.hostname, '-p', str(source.port),
    '-U', unquote(source.username), '-d', unquote(source.path.lstrip('/')),
    '-v', 'ON_ERROR_STOP=1', '-q', '-f', '-',
], input=sql.encode('utf-8'), stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
if process.returncode:
    (APP / '.local/history-browser-fixture-error.log').write_bytes(process.stderr)
    raise SystemExit('Local metadata fixture failed; see ignored diagnostic log')
print(json.dumps({'restore_id': restore_id, 'metadata_only': True, 'cloud_objects_created': False, 'scan_reports_created': False}))
