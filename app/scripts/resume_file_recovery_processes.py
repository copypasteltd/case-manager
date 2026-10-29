"""Restart only confirmed stopped owned native PG after a process interruption.

Never initializes a cluster, starts virtualization or terminates a live process.
An abandoned synthetic test database is preserved privately before removal.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import tempfile
from urllib.parse import urlsplit, unquote
import file_recovery_native_cluster as test_pg

APP = test_pg.APP
E = APP.parent / 'docs/quality/evidence/file-recovery'
FLAGS = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0

def listening(port):
    with socket.socket() as probe:
        probe.settimeout(1)
        return probe.connect_ex(('127.0.0.1', port)) == 0

def start_stopped(directory, port, log):
    status = subprocess.run([str(test_pg.NATIVE_APP / '.tools/pgsql/bin/pg_ctl.exe'), '-D', str(directory), 'status'],
        capture_output=True, creationflags=FLAGS, timeout=20)
    if status.returncode == 0:
        assert listening(port)
        return False
    assert status.returncode == 3 and not listening(port), 'Native ownership or port is ambiguous; nothing started.'
    test_pg.run('pg_ctl', ['-D', str(directory), '-l', str(log), '-o', '-h 127.0.0.1 -p ' + str(port), '-w', '-t', '180', 'start'], timeout=190)
    assert listening(port)
    return True

def main():
    target = E / 'process-restoration.json'
    assert not target.exists(), 'Inspect the original restoration before repeating it.'
    owner = json.loads(test_pg.OWNER.read_text(encoding='utf-8'))
    directory = Path(owner['directory']).resolve()
    assert directory.is_relative_to(Path(tempfile.gettempdir()).resolve()) and directory.name.startswith('cp-file-recovery-pg-')
    assert (directory / 'PG_VERSION').read_text().strip() == '18'
    test_started = start_stopped(directory, owner['port'], directory / 'server.log')
    test_pg.attest(owner)
    rows = json.loads(test_pg.sql(owner, "SELECT coalesce(json_agg(json_build_object('name',datname,'owner',pg_get_userbyid(datdba),'sessions',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid))),'[]'::json) FROM pg_database d WHERE datname ~ '^cp_(test|suite|batch|fixture)_'").strip())
    preserved = []
    private = APP / '.local/file-recovery-process-interruption'
    private.mkdir(exist_ok=True)
    for row in rows:
        name = row['name']
        assert re.fullmatch(r'cp_(test|suite|batch|fixture)_[a-z0-9_]+', name) and row['owner'] == 'postgres' and row['sessions'] == 0
        saved = private / (name + '.pgdump')
        assert not saved.exists()
        test_pg.run('pg_dump', ['-F', 'c', '-d', name, '-f', str(saved)], env={**os.environ,
            'PGHOST': '127.0.0.1', 'PGPORT': str(owner['port']), 'PGUSER': 'postgres', 'PGPASSWORD': owner['admin_password']}, timeout=180)
        assert saved.stat().st_size > 0
        preserved.append({'private_dump': saved.relative_to(APP).as_posix(), 'sha256': hashlib.sha256(saved.read_bytes()).hexdigest(), 'bytes': saved.stat().st_size})
        test_pg.sql(owner, 'DROP DATABASE "' + name + '"')
    assert int(test_pg.sql(owner, "SELECT count(*) FROM pg_database WHERE datname ~ '^cp_(test|suite|batch|fixture)_'").strip()) == 0
    values = json.loads((APP / '.local/environment.json').read_text(encoding='utf-8'))
    db = urlsplit(values['CP_DATABASE_URL'])
    assert values['CP_ENV'] == 'development' and db.hostname == '127.0.0.1' and db.port == 14660
    development = APP / '.local/postgres'
    assert development.resolve().is_relative_to((APP / '.local').resolve()) and (development / 'PG_VERSION').read_text().strip() == '18'
    opts = (development / 'postmaster.opts').read_text()
    assert '.local/postgres' in opts
    development_started = start_stopped(test_pg.NATIVE_APP / '.local/postgres', 14660, test_pg.NATIVE_APP / '.local/postgres.log')
    raw = test_pg.run('psql', ['-X', '-A', '-t', '-q', '-v', 'ON_ERROR_STOP=1', '-c',
        "SELECT json_build_object('port',current_setting('port')::int,'schema',(SELECT count(*) FROM application_schema_manifest()))"], env={**os.environ,
        'PGHOST': db.hostname, 'PGPORT': str(db.port), 'PGUSER': unquote(db.username), 'PGPASSWORD': unquote(db.password), 'PGDATABASE': unquote(db.path[1:])}, timeout=30)
    actual = json.loads(raw)
    postmaster = (development / 'postmaster.pid').read_text().splitlines()
    assert Path(postmaster[1]).resolve() == development.resolve() and int(postmaster[3]) == 14660
    assert actual['port'] == 14660 and actual['schema'] == 87
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
        'confirmed_stopped_before_restart': True, 'temporary_pg_started': test_started, 'development_pg_started': development_started,
        'clusters_reinitialized': False, 'live_process_terminated': False, 'virtualization_used': False,
        'abandoned_test_databases_preserved_then_removed': preserved, 'remaining_disposable_databases': 0,
        'development_schema': 87, 'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    target.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'owned_native_clusters_started': sum((test_started, development_started)), 'synthetic_databases_preserved_then_removed': len(preserved), 'clusters_reinitialized': False}))

if __name__ == '__main__':
    main()
