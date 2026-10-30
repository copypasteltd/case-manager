"""Run the published schema91 file/search journey, then the real schema92 migrator.

The prior checkout stays private. Its only test adaptation is an end-of-test
upgrade assertion, whose bytes and prior Git archive are disclosed below.
This separate upgrade check is never counted as a current-source backend test.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import zipfile
from office_text_native_cluster import APP, test_environment
from verify_backend_batches import digest

BASE = 'ec2d2510b4ceb1b07459f66efca8477cb229ec1f'
E = APP.parent / 'docs/quality/evidence/office-text'
WORK = APP / '.local/office-text-upgrade-v1-119'

HELPER = r'''
func verifyLegacyOfficeUpgrade(t *testing.T, f *fixture) {
 t.Helper();ctx:=context.Background()
 type fact struct { Rows int `json:"rows"`; Hash string `json:"hash"` }
 snapshot:=func()map[string]fact{out:=map[string]fact{};for _,table:=range []string{"file_text_derivations","files","file_versions","file_replicas","contracts","contract_revisions","actual_cost_entries","payment_records","agent_budget_policies","agent_budget_reservations","agent_budget_usage","agent_budget_reconciliations"}{var v fact;if e:=f.owner.QueryRow(ctx,"SELECT count(*),md5(COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t))::text,'[]')) FROM "+table+" t").Scan(&v.Rows,&v.Hash);e!=nil{t.Fatal(e)};out[table]=v};return out}
 var beforeCount,afterCount int
 if e:=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&beforeCount);e!=nil||beforeCount!=91{t.Fatal(beforeCount,e)}
 before:=snapshot();if before["file_text_derivations"].Rows<8||before["file_versions"].Rows<8{t.Fatal("upgrade requires populated prior real file/index journey")}
 command:=upgradeexec.Command(upgradeos.Getenv("CP_OFFICE_TEXT_MIGRATOR"),"migrate");command.Env=append(upgradeos.Environ(),"CP_MIGRATION_DATABASE_URL="+f.owner.Config().ConnConfig.ConnString());if output,e:=command.CombinedOutput();e!=nil{_=output;t.Fatal("actual schema92 migrator failed",e)}
 after:=snapshot();left,_:=upgradejson.Marshal(before);right,_:=upgradejson.Marshal(after);if string(left)!=string(right){t.Fatal("normal upgrade rewrote formal facts or old text evidence")}
 if e:=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&afterCount);e!=nil||afterCount!=92{t.Fatal(afterCount,e)}
 var accurate bool
 if e:=f.owner.QueryRow(ctx,`SELECT file_text_extractor('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet')='office-v1' AND file_text_extractor('text/plain')='text-v1' AND NOT EXISTS(SELECT 1 FROM file_text_derivations WHERE extractor='office-v1') AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='office_text_source' AND tgenabled='O') AND EXISTS(SELECT 1 FROM file_text_derivations d JOIN file_versions v ON v.org_id=d.org_id AND v.id=d.version_id WHERE v.media_type='application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' AND d.extractor='text-v1' AND d.state='unsupported')`).Scan(&accurate);e!=nil||!accurate{t.Fatal("new selector/guard or preserved unsupported row incorrect",e)}
 record,_:=upgradejson.Marshal(map[string]any{"passed":true,"before_schema":beforeCount,"after_schema":afterCount,"original_facts_unchanged":true,"old_text_evidence_unchanged":true,"no_body_backfill":true,"selector_and_guard_ready":true,"facts":before});t.Log("OFFICE_TEXT_UPGRADE:"+string(record))
}
'''


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    assert not WORK.exists() and not (E / 'schema-upgrade.json').exists(), 'Preserve an existing prior-source upgrade run.'
    source = digest()
    archive = APP / '.local/office-text-upgrade-v1-119.zip'
    subprocess.run(['git', 'archive', '--format=zip', '--output=' + str(archive), BASE, 'app/backend'], cwd=APP.parent, check=True)
    WORK.mkdir()
    with zipfile.ZipFile(archive) as files:
        for item in files.infolist():
            assert (item.filename in ('app/', 'app/backend/') and item.is_dir() or item.filename.startswith('app/backend/')) and '..' not in Path(item.filename).parts
        files.extractall(WORK)
    backend = WORK / 'app/backend'
    target = backend / 'internal/httpapi/file_text_search_integration_test.go'
    original = target.read_bytes()
    adapted = original.replace(b'"context"', b'"context"\n upgradeos "os"\n upgradeexec "os/exec"\n upgradejson "encoding/json"', 1)
    needle = b'if os.Getenv("CP_BROWSER_FILE_SEARCH_FIXTURE")'
    assert adapted.count(needle) == 1
    extra = r'''
 legacy:=add("Prior Office.xlsx","application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",[]byte("prior unsupported object"),nil)
 _=legacy
 if _,e:=service.Schedule(ctx);e!=nil{t.Fatal(e)}
 for run()!=nil{}
 verifyLegacyOfficeUpgrade(t,f)
 '''.encode()
    adapted = adapted.replace(needle, extra+needle, 1) + HELPER.encode()
    target.write_bytes(adapted)
    binary = APP / '.local/office-text-migrator.exe'
    buildlog = APP / '.local/office-text-upgrade-v1-build.log'
    with buildlog.open('w', encoding='utf-8') as output:
        subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'build', '-o', str(binary), './cmd/platform'], cwd=APP / 'backend', stdout=output, stderr=subprocess.STDOUT, check=True, timeout=300)
    env = {**test_environment(), 'CP_OFFICE_TEXT_MIGRATOR': str(binary)}
    raw = APP / '.local/office-text-upgrade-v1.log'
    with raw.open('w', encoding='utf-8') as output:
        result = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'test', './internal/httpapi', '-json', '-count=1', '-run', '^TestFileTextSearch$', '-timeout=20m'], cwd=backend, env=env, stdout=output, stderr=subprocess.STDOUT, timeout=1250)
    records, checked, chunks = [], None, []
    for line in raw.read_text(encoding='utf-8').splitlines():
        try: event = json.loads(line)
        except json.JSONDecodeError: continue
        if event.get('Test') and event.get('Action') in ('pass', 'fail', 'skip'): records.append(event)
        chunks.append(event.get('Output', ''))
    # Go splits long log lines into output frames. Parse only after joining
    # their exact bytes, rather than treating an individual frame as a line.
    for line in ''.join(chunks).splitlines():
        if 'OFFICE_TEXT_UPGRADE:' in line:
            checked = json.loads(line.split('OFFICE_TEXT_UPGRADE:', 1)[1])
    assert result.returncode == 0 and records and all(v['Action'] == 'pass' for v in records) and checked and checked['passed']
    assert source == digest()
    report = {**checked, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': source,
        'published_fixture_commit': BASE, 'prior_checkout_archive_sha256': sha(archive), 'original_test_sha256': hashlib.sha256(original).hexdigest(),
        'adapted_test_sha256': sha(target), 'adaptation': 'Add one prior-source unsupported XLSX and end-of-journey schema92 assertion; existing prior file/search cases are unchanged.',
        'actual_current_migrator_sha256': sha(binary), 'raw_log_sha256': sha(raw), 'prior_fixture_case_records': len(records),
        'counts_as_current_backend_tests': False, 'verification_script_sha256': sha(Path(__file__)), 'raw_log': 'ignored app/.local/' + raw.name}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'schema-upgrade.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'schema': [91, 92], 'original_fact_tables': len(checked['facts']), 'actual_prior_derivations': checked['facts']['file_text_derivations']['rows']}))


if __name__ == '__main__':
    main()
