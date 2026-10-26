"""Metadata-only browser fixture in the owned native development database.

No objects are uploaded, scanned, copied or reported verified. Production is
refused. The folder and project must have been created through the real API.
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
parser.add_argument('--folder', required=True, type=uuid.UUID)
parser.add_argument('--owner', required=True, type=uuid.UUID)
args = parser.parse_args()
values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
source = urlsplit(values['CP_DATABASE_URL'])
if values.get('CP_ENV') != 'development' or source.hostname != '127.0.0.1' or source.port != values['_postgres_port']:
    raise SystemExit('Only the owned local development database is supported')
file_id, first, second = (str(uuid.uuid4()) for _ in range(3))
digest = hashlib.sha256(b'COPYPASTE-METADATA-ONLY-FIXTURE-NO-CLOUD-OBJECT').hexdigest()
sql = """
BEGIN;
DO $guard$ BEGIN
    IF NOT EXISTS(SELECT 1 FROM folders f JOIN projects p ON p.org_id=f.org_id AND p.id=f.project_id
        JOIN users u ON u.org_id=f.org_id AND u.id=f.creator_id
        WHERE f.id='%(folder)s' AND f.creator_id='%(owner)s' AND p.creator_id=f.creator_id
        AND f.name LIKE '界面验证资料 %%' AND p.name LIKE '存储验证项目 %%'
        AND u.login_key='project.manager' AND f.state='active') THEN
        RAISE EXCEPTION 'fixture scope check failed';
    END IF;
END $guard$;
INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence)
SELECT org_id,1,'oss',creator_id,'metadata fixture only; no cloud fence' FROM folders WHERE id='%(folder)s'
ON CONFLICT(org_id,epoch) DO NOTHING;
INSERT INTO files(id,org_id,folder_id,name,name_key,classification,current_version_id,state,creator_id)
SELECT '%(file)s',org_id,id,'元数据样本-120GB.bin','元数据样本-120gb.bin',classification,NULL,'active',creator_id
FROM folders WHERE id='%(folder)s';
INSERT INTO file_versions(id,org_id,file_id,seq,base_version_id,object_key,provider_source_id,storage_epoch,
    size_bytes,sha256,media_type,source,author_id,executor_id,scan_state,availability,original_name)
SELECT v.id::uuid,f.org_id,f.id,v.seq,v.base::uuid,'metadata-fixture/'||v.id,'metadata-only',1,
    v.bytes,'%(digest)s','application/octet-stream','local_metadata_fixture',f.creator_id,f.creator_id,'unscannable','missing','原始素材.bin'
FROM files f CROSS JOIN (VALUES
    ('%(first)s',1,NULL::text,120000000000::bigint),
    ('%(second)s',2,'%(first)s',120000000000::bigint)) v(id,seq,base,bytes)
WHERE f.id='%(file)s';
UPDATE files SET current_version_id='%(second)s' WHERE id='%(file)s';
INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,state,expected_sha256,last_error)
SELECT gen_random_uuid(),f.org_id,'%(second)s',v.provider,'metadata-fixture','metadata-fixture/%(second)s',
    v.state,'%(digest)s','test-internal-detail-must-not-be-disclosed'
FROM files f CROSS JOIN (VALUES('oss','queued'),('r2','failed')) v(provider,state) WHERE f.id='%(file)s';
COMMIT;
""" % dict(folder=str(args.folder), owner=str(args.owner), file=file_id, first=first, second=second, digest=digest)
env = {**os.environ, 'PGPASSWORD': unquote(source.password), 'PGCLIENTENCODING': 'UTF8'}
process = subprocess.run([
    str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-h', source.hostname, '-p', str(source.port),
    '-U', unquote(source.username), '-d', unquote(source.path.lstrip('/')),
    '-v', 'ON_ERROR_STOP=1', '-q', '-f', '-',
], input=sql.encode('utf-8'), stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
if process.returncode:
    # psql diagnostics can include private SQL parameters; keep them out of reports.
    (APP / '.local/storage-browser-fixture-error.log').write_bytes(process.stderr)
    raise SystemExit('Local metadata fixture failed; see ignored diagnostic log')
print(json.dumps({'file_id': file_id, 'version_ids': [first, second], 'metadata_only': True, 'cloud_objects_created': False}))
