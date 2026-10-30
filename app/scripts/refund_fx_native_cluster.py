"""Own a disposable native Windows PG cluster; retain full durability and deadlines."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
from urllib.parse import quote
from windows_native_path import native_app

APP = Path(__file__).resolve().parents[1]
NATIVE_APP = native_app()
OWNER = APP / '.local/refund_fx-native-cluster.json'
E = APP.parent / 'docs/quality/evidence/refund-fx'
FLAGS = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(name, args, **kwargs):
    # A postmaster must never inherit a pipe which would keep the caller waiting.
    with tempfile.TemporaryFile() as output, tempfile.TemporaryFile() as error:
        status = subprocess.run([str(NATIVE_APP / '.tools/pgsql/bin' / (name + '.exe')), *args],
            stdout=output, stderr=error, creationflags=FLAGS, **kwargs)
        if status.returncode:
            error.seek(0)
            (APP / '.local/refund_fx-native-pg-error.log').write_bytes(error.read())
            raise RuntimeError('Owned native PG command failed; diagnostics remain private.')
        output.seek(0)
        return output.read().decode('utf-8')


def sql(owner, query):
    return run('psql', ['-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c', query],
        env={**os.environ, 'PGHOST':'127.0.0.1', 'PGPORT':str(owner['port']), 'PGUSER':'postgres',
             'PGDATABASE':'postgres', 'PGPASSWORD':owner['admin_password']}, timeout=30)


def attest(owner):
    path = Path(owner['directory']).resolve()
    assert path.is_relative_to(Path(tempfile.gettempdir()).resolve()) and path.name.startswith('cp-refund_fx-pg-')
    actual = json.loads(sql(owner, "SELECT json_build_object('directory',current_setting('data_directory'),'port',current_setting('port')::int,'fsync',current_setting('fsync'),'synchronous_commit',current_setting('synchronous_commit'),'full_page_writes',current_setting('full_page_writes'),'version',current_setting('server_version'))"))
    assert Path(actual['directory']).resolve() == path and actual['port'] == owner['port']
    assert all(actual[key] == 'on' for key in ('fsync','synchronous_commit','full_page_writes'))
    return actual


def record_attestation(owner, actual):
    E.mkdir(parents=True,exist_ok=True)
    (E/'native-cluster.json').write_text(json.dumps({'passed':True,'created_at':owner['created_at'],
        'native_postgresql_version':actual['version'],'loopback_only':True,'temporary_cluster':True,
        'fsync':True,'synchronous_commit':True,'full_page_writes':True,'original_development_cluster_modified':False,
        'fixture_cleanup_deadline_changed':False,'virtualization_used':False,'verification_script_sha256':sha(Path(__file__))},indent=2)+'\n',encoding='utf-8')


def prepare():
    if OWNER.exists():
        owner = json.loads(OWNER.read_text(encoding='utf-8'))
        actual = attest(owner)
        record_attestation(owner, actual)
        return owner
    directory = Path(tempfile.mkdtemp(prefix='cp-refund_fx-pg-')).resolve()
    assert directory.is_relative_to(Path(tempfile.gettempdir()).resolve())
    with socket.socket() as probe:
        probe.bind(('127.0.0.1',0))
        port = probe.getsockname()[1]
    owner = {'directory':str(directory),'port':port,'admin_password':secrets.token_hex(32),
        'application_password':secrets.token_hex(32),'migration_password':secrets.token_hex(32),
        'created_at':datetime.now(timezone.utc).isoformat()}
    # Write ownership before mutation so an interrupted initialization is recoverable.
    OWNER.write_text(json.dumps(owner,indent=2)+'\n',encoding='utf-8')
    password = NATIVE_APP / '.local/refund_fx-native-init-password.tmp'
    assert not password.exists()
    password.write_text(owner['admin_password'],encoding='utf-8')
    try:
        run('initdb',['-D',str(directory),'-U','postgres','-A','scram-sha-256','--pwfile='+str(password),'--encoding=UTF8','--locale=C'],timeout=180)
    finally:
        password.unlink(missing_ok=True)
    with (directory / 'postgresql.conf').open('a',encoding='utf-8') as config:
        config.write("\nlisten_addresses='127.0.0.1'\nport="+str(port)+"\nfsync=on\nsynchronous_commit=on\nfull_page_writes=on\n")
    run('pg_ctl',['-D',str(directory),'-l',str(directory/'server.log'),'-w','-t','180','start'],timeout=190)
    # Passwords go through stdin, never process arguments or public output.
    statements = ("CREATE ROLE copypaste_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '"+owner['migration_password']+"';\n"
        +"CREATE ROLE copypaste_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '"+owner['application_password']+"';\n"
        +"CREATE DATABASE copypaste OWNER copypaste_migrator;\n")
    run('psql',['-X','-q','-v','ON_ERROR_STOP=1'], input=statements.encode(),
        env={**os.environ,'PGHOST':'127.0.0.1','PGPORT':str(port),'PGUSER':'postgres','PGDATABASE':'postgres','PGPASSWORD':owner['admin_password']},timeout=30)
    actual = attest(owner)
    record_attestation(owner, actual)
    return owner


def test_environment():
    owner = prepare()
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    base = 'postgres://'
    suffix = '@127.0.0.1:'+str(owner['port'])+'/copypaste?sslmode=disable'
    return {**os.environ,**{k:v for k,v in values.items() if k.startswith('CP_')},
        'CP_DATABASE_URL':base+'copypaste_app:'+quote(owner['application_password'])+suffix,
        'CP_MIGRATION_DATABASE_URL':base+'copypaste_migrator:'+quote(owner['migration_password'])+suffix,
        'CP_TEST_ADMIN_URL':base+'postgres:'+quote(owner['admin_password'])+'@127.0.0.1:'+str(owner['port'])+'/postgres?sslmode=disable',
        'CP_ENV':'test','GOPROXY':'https://goproxy.cn,https://proxy.golang.org,direct','GODEBUG':'http2client=0'}


def stop():
    owner = json.loads(OWNER.read_text(encoding='utf-8'))
    actual = attest(owner)
    count = int(sql(owner,"SELECT count(*) FROM pg_database WHERE datname ~ '^cp_(test|suite|batch|fixture)_'").strip())
    assert count == 0, 'Keep failed disposable database evidence intact.'
    run('pg_ctl',['-D',owner['directory'],'-m','fast','-w','-t','60','stop'],timeout=70)
    record = {'passed':True,'stopped_at':datetime.now(timezone.utc).isoformat(),'remaining_disposable_databases':count,
        'owned_cluster_stopped':True,'development_cluster_preserved':True,'native_postgresql_version':actual['version'],
        'verification_script_sha256':sha(Path(__file__)),'data_retained_private':True}
    (E/'native-cluster-closed.json').write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')


if __name__ == '__main__':
    import sys
    if sys.argv[1:] == ['stop']:
        stop()
    elif sys.argv[1:] == ['prepare']:
        prepare()
    else:
        raise SystemExit('Use prepare or stop; credentials are never printed.')
