"""Run the published schema89 contract journey, then the real schema90 migrator.

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
from contract_body_native_cluster import APP, test_environment
from verify_backend_batches import digest

BASE = '30c364a7794c3fe7049c6ab04c4ba5b41d306ec3'
E = APP.parent / 'docs/quality/evidence/contract-body'
WORK = APP / '.local/contract-body-upgrade-v3-116'

HELPER = r'''
func verifyLegacyContractBodyUpgrade(t *testing.T, f *fixture) {
 t.Helper()
 ctx := context.Background()
 type fact struct { Rows int `json:"rows"`; Hash string `json:"hash"` }
 snapshot := func() map[string]fact {
  result := map[string]fact{}
  for _, table := range []string{"contracts","contract_revisions","contract_lifecycle_events","file_version_references","payment_records","payment_revisions","payment_events","payment_revision_files","payment_allocations","payment_allocation_shares","actual_cost_entries","cost_revisions","cost_allocations","cost_links","agent_budget_policies","agent_budget_reservations","agent_budget_usage","agent_budget_reconciliations"} {
   expression := "to_jsonb(t)"
   var v fact
   if e:=f.owner.QueryRow(ctx,"SELECT count(*),md5(COALESCE(jsonb_agg("+expression+" ORDER BY "+expression+")::text,'[]')) FROM "+table+" t").Scan(&v.Rows,&v.Hash); e!=nil {t.Fatal(e)}
   result[table]=v
  }
  return result
 }
 var beforeCount,afterCount int
 if e:=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&beforeCount);e!=nil||beforeCount!=89 {t.Fatal(beforeCount,e)}
 before:=snapshot()
 if before["contracts"].Rows<2||before["contract_revisions"].Rows<2||before["file_version_references"].Rows<1 {t.Fatal("upgrade requires an actual populated prior contract journey")}
 command:=upgradeexec.Command(upgradeos.Getenv("CP_CONTRACT_BODY_MIGRATOR"),"migrate")
 command.Env=append(upgradeos.Environ(),"CP_MIGRATION_DATABASE_URL="+f.owner.Config().ConnConfig.ConnString())
 output,e:=command.CombinedOutput()
 if e!=nil {t.Fatal("actual native schema90 migration failed; private diagnostics retained",e)}
 _=output
 after:=snapshot()
 left,_:=json.Marshal(before);right,_:=json.Marshal(after)
 if string(left)!=string(right){t.Fatal("schema90 changed original contract or financial facts")}
 if e=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&afterCount);e!=nil||afterCount!=90 {t.Fatal(afterCount,e)}
 var accurate bool
 if e=f.owner.QueryRow(ctx,`SELECT (SELECT count(*) FROM agent_contract_body_intents)=0 AND (SELECT count(*) FROM agent_contract_body_adoptions)=0 AND (SELECT count(*)=4 FROM pg_trigger WHERE tgname IN ('agent_contract_body_intent_binding','agent_contract_body_adoption_binding','agent_contract_body_intent_immutable','agent_contract_body_adoption_immutable') AND tgenabled='O' AND NOT tgisinternal)`).Scan(&accurate);e!=nil||!accurate {t.Fatal("new contract body tables and immutable guards are not ready",e)}
 record,_:=json.Marshal(map[string]any{"passed":true,"before_schema":beforeCount,"after_schema":afterCount,"original_facts_unchanged":true,"new_tables_empty_and_guards_ready":true,"facts":before})
 t.Log("CONTRACT_BODY_UPGRADE:"+string(record))
}
'''


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    assert not WORK.exists() and not (E / 'schema-upgrade.json').exists(), 'Preserve an existing prior-source upgrade run.'
    source = digest()
    archive = APP / '.local/contract-body-upgrade-v3-116.zip'
    subprocess.run(['git', 'archive', '--format=zip', '--output=' + str(archive), BASE, 'app/backend'], cwd=APP.parent, check=True)
    WORK.mkdir()
    with zipfile.ZipFile(archive) as files:
        for item in files.infolist():
            assert (item.filename in ('app/', 'app/backend/') and item.is_dir() or item.filename.startswith('app/backend/')) and '..' not in Path(item.filename).parts
        files.extractall(WORK)
    backend = WORK / 'app/backend'
    target = backend / 'internal/httpapi/contracts_integration_test.go'
    original = target.read_bytes()
    adapted = original.replace(b'"context"', b'"context"\n upgradeos "os"\n upgradeexec "os/exec"', 1)
    needle = b'f := u.f'
    assert adapted.count(needle) == 1
    adapted = adapted.replace(needle, needle + b'\n t.Cleanup(func(){ verifyLegacyContractBodyUpgrade(t,f) })', 1) + HELPER.encode()
    target.write_bytes(adapted)
    binary = APP / '.local/contract-body-migrator.exe'
    buildlog = APP / '.local/contract-body-upgrade-v3-build.log'
    with buildlog.open('w', encoding='utf-8') as output:
        subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'build', '-o', str(binary), './cmd/platform'], cwd=APP / 'backend', stdout=output, stderr=subprocess.STDOUT, check=True, timeout=300)
    env = {**test_environment(), 'CP_CONTRACT_BODY_MIGRATOR': str(binary)}
    raw = APP / '.local/contract-body-upgrade-v3.log'
    with raw.open('w', encoding='utf-8') as output:
        result = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'test', './internal/httpapi', '-json', '-count=1', '-run', '^TestContractDraftRevisions$', '-timeout=20m'], cwd=backend, env=env, stdout=output, stderr=subprocess.STDOUT, timeout=1250)
    records, checked, chunks = [], None, []
    for line in raw.read_text(encoding='utf-8').splitlines():
        try: event = json.loads(line)
        except json.JSONDecodeError: continue
        if event.get('Test') and event.get('Action') in ('pass', 'fail', 'skip'): records.append(event)
        chunks.append(event.get('Output', ''))
    # Go splits long log lines into output frames. Parse only after joining
    # their exact bytes, rather than treating an individual frame as a line.
    for line in ''.join(chunks).splitlines():
        if 'CONTRACT_BODY_UPGRADE:' in line:
            checked = json.loads(line.split('CONTRACT_BODY_UPGRADE:', 1)[1])
    assert result.returncode == 0 and records and all(v['Action'] == 'pass' for v in records) and checked and checked['passed']
    assert source == digest()
    report = {**checked, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': source,
        'published_fixture_commit': BASE, 'prior_checkout_archive_sha256': sha(archive), 'original_test_sha256': hashlib.sha256(original).hexdigest(),
        'adapted_test_sha256': sha(target), 'adaptation': 'Only add one end-of-journey normal-upgrade assertion; existing prior contract cases are unchanged.',
        'actual_current_migrator_sha256': sha(binary), 'raw_log_sha256': sha(raw), 'prior_fixture_case_records': len(records),
        'counts_as_current_backend_tests': False, 'verification_script_sha256': sha(Path(__file__)), 'raw_log': 'ignored app/.local/' + raw.name}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'schema-upgrade.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'schema': [89, 90], 'original_fact_tables': len(checked['facts']), 'actual_prior_contract_revisions': checked['facts']['contract_revisions']['rows']}))


if __name__ == '__main__':
    main()
