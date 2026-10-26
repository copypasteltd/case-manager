"""Owned development UI fixtures only; no bytes, cloud calls or scan evidence.

The caller creates a dedicated project/folder through the real API. Optional
MFA expiry applies only to the caller's current development browser session.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import unquote, urlsplit
import uuid

APP = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--folder', type=uuid.UUID)
parser.add_argument('--owner', type=uuid.UUID)
parser.add_argument('--expire-session-hash')
args = parser.parse_args()
values = json.loads((APP / '.local/environment.json').read_text('utf-8'))
source = urlsplit(values['CP_DATABASE_URL'])
if values.get('CP_ENV') != 'development' or source.hostname != '127.0.0.1' or source.port != values['_postgres_port']:
    raise SystemExit('Only the owned native development database is supported')
if args.expire_session_hash:
    if not re.fullmatch('[0-9a-f]{64}', args.expire_session_hash):
        raise SystemExit('Invalid session digest')
    sql = """UPDATE sessions s SET mfa_verified_at=now()-interval '1 hour'
        FROM users u WHERE u.org_id=s.org_id AND u.id=s.user_id AND u.login_key='system.admin'
        AND s.access_hash='%s' AND s.device_type='web' AND s.revoked_at IS NULL AND s.expires_at>now();""" % args.expire_session_hash
    result = {'current_development_session_proof_expired': True}
else:
    if not args.folder or not args.owner:
        raise SystemExit('Provide the caller-owned fixture folder and owner')
    waiting, transfer, failed = (str(uuid.uuid4()) for _ in range(3))
    sql = """
BEGIN;
DO $guard$ BEGIN
    IF NOT EXISTS(SELECT 1 FROM folders f JOIN projects p ON p.org_id=f.org_id AND p.id=f.project_id
        JOIN users u ON u.org_id=f.org_id AND u.id=f.creator_id
        WHERE f.id='%(folder)s' AND f.creator_id='%(owner)s' AND p.creator_id=f.creator_id
        AND f.name LIKE '上传界面验证资料 %%' AND p.name LIKE '上传队列验证项目 %%'
        AND u.login_key='project.manager' AND f.state='active') THEN
        RAISE EXCEPTION 'fixture scope check failed';
    END IF;
END $guard$;
INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence)
SELECT org_id,1,'oss',creator_id,'metadata fixture only; no cloud fence' FROM folders WHERE id='%(folder)s'
ON CONFLICT(org_id,epoch) DO NOTHING;
INSERT INTO upload_sessions(id,org_id,owner_id,target_folder_id,name,media_type,size_bytes,source_fingerprint,
    part_size,part_count,draft_key,storage_epoch,state,expires_at,provider,bucket,classification,source_channel,last_error)
SELECT v.id::uuid,f.org_id,f.creator_id,f.id,v.name,'application/octet-stream',v.bytes,
    'metadata-only-not-a-real-source',33554432,v.parts,'metadata-fixture/'||v.id,1,v.state,now()+interval '2 hours',
    'oss','metadata-fixture',f.classification,'web',v.error
FROM folders f CROSS JOIN (VALUES
    ('%(waiting)s','等待准备-元数据样本.bin',1::bigint,1,'created',NULL::text),
    ('%(transfer)s','120GB-元数据样本.bin',120000000000::bigint,3577,'uploading',NULL::text),
    ('%(failed)s','失败状态-元数据样本.bin',1::bigint,1,'failed','STORAGE_NOT_CONFIGURED')
) v(id,name,bytes,parts,state,error) WHERE f.id='%(folder)s';
COMMIT;
""" % dict(folder=str(args.folder), owner=str(args.owner), waiting=waiting, transfer=transfer, failed=failed)
    result = {'upload_ids': [waiting, transfer, failed], 'metadata_only': True, 'cloud_objects_created': False, 'scan_reports_created': False}
env = {**os.environ, 'PGPASSWORD': unquote(source.password), 'PGCLIENTENCODING': 'UTF8'}
process = subprocess.run([
    str(APP / '.tools/pgsql/bin/psql.exe'), '-X', '-h', source.hostname, '-p', str(source.port),
    '-U', unquote(source.username), '-d', unquote(source.path.lstrip('/')),
    '-v', 'ON_ERROR_STOP=1', '-q', '-f', '-',
], input=sql.encode('utf-8'), stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
if process.returncode:
    (APP / '.local/upload-browser-fixture-error.log').write_bytes(process.stderr)
    raise SystemExit('Development fixture failed; see ignored diagnostic log')
print(json.dumps(result))
