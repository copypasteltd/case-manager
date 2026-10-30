"""Run the published schema90 contract journey, then the real schema91 migrator.

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
from payment_fee_native_cluster import APP, test_environment
from verify_backend_batches import digest

BASE = 'e4a66e32cc4a2bf1a663506381a2be6f3ebeb15c'
E = APP.parent / 'docs/quality/evidence/payment-fee'
WORK = APP / '.local/payment-fee-upgrade-v1-117'

HELPER = r'''
func verifyLegacyPaymentFeeUpgrade(t *testing.T, f *fixture) {
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
 if e:=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&beforeCount);e!=nil||beforeCount!=90 {t.Fatal(beforeCount,e)}
 before:=snapshot()
 if before["payment_records"].Rows<2||before["actual_cost_entries"].Rows<2||before["payment_allocations"].Rows<2||before["payment_allocation_shares"].Rows<2 {t.Fatal("upgrade requires an actual populated prior cross-currency finance journey")}
 command:=upgradeexec.Command(upgradeos.Getenv("CP_PAYMENT_FEE_MIGRATOR"),"migrate")
 command.Env=append(upgradeos.Environ(),"CP_MIGRATION_DATABASE_URL="+f.owner.Config().ConnConfig.ConnString())
 output,e:=command.CombinedOutput()
 if e!=nil {t.Fatal("actual native schema91 migration failed; private diagnostics retained",e)}
 _=output
 after:=snapshot()
 left,_:=json.Marshal(before);right,_:=json.Marshal(after)
 if string(left)!=string(right){t.Fatal("schema91 changed original contract or financial facts")}
 if e=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&afterCount);e!=nil||afterCount!=91 {t.Fatal(afterCount,e)}
 var accurate bool
 if e=f.owner.QueryRow(ctx,`SELECT (SELECT count(*) FROM payment_fee_links)=0 AND (SELECT count(*) FROM payment_fee_revocations)=0 AND (SELECT count(*)=4 FROM pg_trigger WHERE tgname IN ('payment_fee_link_evidence','payment_fee_revocation_evidence','payment_fee_link_immutable','payment_fee_revocation_immutable') AND tgenabled='O' AND NOT tgisinternal)`).Scan(&accurate);e!=nil||!accurate {t.Fatal("new contract body tables and immutable guards are not ready",e)}
 record,_:=json.Marshal(map[string]any{"passed":true,"before_schema":beforeCount,"after_schema":afterCount,"original_facts_unchanged":true,"new_tables_empty_and_guards_ready":true,"facts":before})
 t.Log("PAYMENT_FEE_UPGRADE:"+string(record))
}
'''


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    assert not WORK.exists() and not (E / 'schema-upgrade.json').exists(), 'Preserve an existing prior-source upgrade run.'
    source = digest()
    archive = APP / '.local/payment-fee-upgrade-v1-117.zip'
    subprocess.run(['git', 'archive', '--format=zip', '--output=' + str(archive), BASE, 'app/backend'], cwd=APP.parent, check=True)
    WORK.mkdir()
    with zipfile.ZipFile(archive) as files:
        for item in files.infolist():
            assert (item.filename in ('app/', 'app/backend/') and item.is_dir() or item.filename.startswith('app/backend/')) and '..' not in Path(item.filename).parts
        files.extractall(WORK)
    backend = WORK / 'app/backend'
    target = backend / 'internal/httpapi/payment_fx_settlement_integration_test.go'
    original = target.read_bytes()
    adapted = original.replace(b'"context"', b'"context"\n upgradeos "os"\n upgradeexec "os/exec"', 1)
    needle = b'f, ctx := u.f, context.Background()'
    assert adapted.count(needle) == 1
    adapted = adapted.replace(needle, needle + b'\n t.Cleanup(func(){ verifyLegacyPaymentFeeUpgrade(t,f) })', 1) + HELPER.encode()
    target.write_bytes(adapted)
    binary = APP / '.local/payment-fee-migrator.exe'
    buildlog = APP / '.local/payment-fee-upgrade-v1-build.log'
    with buildlog.open('w', encoding='utf-8') as output:
        subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'build', '-o', str(binary), './cmd/platform'], cwd=APP / 'backend', stdout=output, stderr=subprocess.STDOUT, check=True, timeout=300)
    env = {**test_environment(), 'CP_PAYMENT_FEE_MIGRATOR': str(binary)}
    raw = APP / '.local/payment-fee-upgrade-v1.log'
    with raw.open('w', encoding='utf-8') as output:
        result = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'test', './internal/httpapi', '-json', '-count=1', '-run', '^TestCrossCurrencySettlement$', '-timeout=20m'], cwd=backend, env=env, stdout=output, stderr=subprocess.STDOUT, timeout=1250)
    records, checked, chunks = [], None, []
    for line in raw.read_text(encoding='utf-8').splitlines():
        try: event = json.loads(line)
        except json.JSONDecodeError: continue
        if event.get('Test') and event.get('Action') in ('pass', 'fail', 'skip'): records.append(event)
        chunks.append(event.get('Output', ''))
    # Go splits long log lines into output frames. Parse only after joining
    # their exact bytes, rather than treating an individual frame as a line.
    for line in ''.join(chunks).splitlines():
        if 'PAYMENT_FEE_UPGRADE:' in line:
            checked = json.loads(line.split('PAYMENT_FEE_UPGRADE:', 1)[1])
    assert result.returncode == 0 and records and all(v['Action'] == 'pass' for v in records) and checked and checked['passed']
    assert source == digest()
    report = {**checked, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': source,
        'published_fixture_commit': BASE, 'prior_checkout_archive_sha256': sha(archive), 'original_test_sha256': hashlib.sha256(original).hexdigest(),
        'adapted_test_sha256': sha(target), 'adaptation': 'Only add one end-of-journey normal-upgrade assertion; existing prior cross-currency cases are unchanged.',
        'actual_current_migrator_sha256': sha(binary), 'raw_log_sha256': sha(raw), 'prior_fixture_case_records': len(records),
        'counts_as_current_backend_tests': False, 'verification_script_sha256': sha(Path(__file__)), 'raw_log': 'ignored app/.local/' + raw.name}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'schema-upgrade.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'schema': [90, 91], 'original_fact_tables': len(checked['facts']), 'actual_prior_allocations': checked['facts']['payment_allocations']['rows']}))


if __name__ == '__main__':
    main()
