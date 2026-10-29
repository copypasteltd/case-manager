"""Remove only inactive synthetic databases attributable to the archived109 cleanup failures."""
from datetime import datetime, timedelta, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
from urllib.parse import urlsplit

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/audit-console'
ORIGINAL = E.parent / 'audit-console-backend/attempts/02-native-cleanup-timeout'


def read(p):
    return json.loads(p.read_text(encoding='utf-8'))


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    assert not (E / 'fixture-cleanup.json').exists(), 'Preserve prior cleanup evidence.'
    values = read(APP / '.local/environment.json')
    url = urlsplit(values['CP_DATABASE_URL'])
    assert values['CP_ENV'] == 'development' and url.hostname == '127.0.0.1' and url.port == 14660 and int(values['_postgres_port']) == 14660
    initial = read(ORIGINAL / 'result.json'); archive = read(ORIGINAL / 'attempt.json')
    assert not initial['passed'] and archive['failed_batches'] == [8,9,10]
    windows = []
    for row in archive['raw_logs']:
        if row['batch'] not in archive['failed_batches']:
            continue
        file = APP / row['file']; assert sha(file) == row['sha256']
        events = []
        for line in file.read_text(encoding='utf-8').splitlines():
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                pass
        failed = [v for v in events if v.get('Action') == 'fail' and v.get('Test')]
        for v in failed:
            test = v['Test']; start = next(x['Time'] for x in events if x.get('Action') == 'run' and x.get('Test') == test)
            windows.append({'test':test, 'from':datetime.fromisoformat(start)-timedelta(seconds=2), 'to':datetime.fromisoformat(v['Time'])+timedelta(seconds=2)})
    environment = {**os.environ, 'PGHOST':'127.0.0.1', 'PGPORT':'14660', 'PGUSER':'postgres',
        'PGPASSWORD':values['_superuser_password'], 'PGCONNECT_TIMEOUT':'5'}
    def sql(query, database='postgres', parse=True):
        result = subprocess.run([str(APP / '.tools/pgsql/bin/psql.exe'), '-X','-A','-t','-q','-v','ON_ERROR_STOP=1','-c',query],
            env={**environment,'PGDATABASE':database}, capture_output=True, text=True, encoding='utf-8', timeout=300,
            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
        if result.returncode:
            (APP / '.local/audit-console-cleanup-error.log').write_text(query + '\n' + result.stderr,encoding='utf-8')
            raise SystemExit('Owned fixture inspection/cleanup failed; private database diagnostics withheld.')
        return json.loads(result.stdout) if parse else None
    query = "SELECT coalesce(json_agg(json_build_object('oid',d.oid::text,'name',datname,'owner',pg_get_userbyid(datdba),'invalid_drop_marker',datconnlimit=-2,'template',datistemplate,'active_connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid),'created',(pg_stat_file('base/'||d.oid)).creation)),'[]'::json) FROM pg_database d WHERE datname ~ '^cp_test_[a-f0-9]{32}$'"
    candidates = sql(query); assert {v['name'] for v in candidates} == {'cp_test_de00478af8864c199105d045d16a9321','cp_test_88358ad8533d4fe9b4728e6e33e83c74','cp_test_108c9b5ef7df487690cc8793c14185bb'}
    for row in candidates:
        assert re.fullmatch(r'cp_test_[a-f0-9]{32}',row['name']) and re.fullmatch(r'[0-9]+',row['oid']) and row['owner'] == 'postgres' and row['active_connections'] == 0 and row['invalid_drop_marker'] and not row['template']
        created = datetime.fromisoformat(row['created'])
        matched = [v['test'] for v in windows if v['from'] <= created <= v['to']]
        assert matched, 'A database cannot be attributed to this failed attempt; preserve it.'
        row.update(matched_failed_tests=matched, contents_inspected=False, attribution='Exact discovered fixture names, local owned test cluster, creation within archived failing cleanup windows, postgres owner, invalid DROP marker and zero active connections. Invalid databases cannot be connected to inspect organization/schema contents.')
    before = E / 'attempts/05-before-owned-fixture-cleanup'; before.mkdir(parents=True,exist_ok=False)
    for name in ('development-runtime.json','development-project-resources.json'):
        shutil.copyfile(E / name,before / name)
    (E / 'fixture-cleanup-plan.json').write_text(json.dumps({'candidates':candidates,'initial_attempt_sha256':sha(ORIGINAL / 'attempt.json'),'force':False,'production_database_selected':False},indent=2)+'\n',encoding='utf-8')
    for row in candidates:
        current = sql("SELECT json_build_object('name',datname,'invalid_drop_marker',datconnlimit=-2,'active_connections',(SELECT count(*) FROM pg_stat_activity WHERE datid=d.oid)) FROM pg_database d WHERE oid=" + row['oid'])
        assert current['name'] == row['name'] and current['active_connections'] == 0 and current['invalid_drop_marker']
        # No FORCE: a racing connection causes DROP to fail rather than terminate another session.
        sql('DROP DATABASE "' + row['name'] + '"',parse=False)
        print('Removed one verified inactive synthetic fixture database.',flush=True)
    remaining = sql(query)
    record = {'passed':remaining == [],'checked_at':datetime.now(timezone.utc).isoformat(),
        'removed':candidates,'remaining_disposable_databases':len(remaining),'force_used':False,
        'development_database_preserved':True,'database_cluster_restarted':False,
        'initial_attempt_sha256':sha(ORIGINAL / 'attempt.json'),'plan_sha256':sha(E / 'fixture-cleanup-plan.json'),
        'verification_script_sha256':sha(Path(__file__))}
    (E / 'fixture-cleanup.json').write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':record['passed'],'removed_owned_fixtures':len(candidates),'remaining':len(remaining)}))
    return 0 if record['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
