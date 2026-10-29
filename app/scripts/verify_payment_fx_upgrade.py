"""Run the published schema88 cash journey, then the real schema89 migrator.

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
from payment_fx_native_cluster import APP, test_environment
from verify_backend_batches import digest

BASE = 'd1a31a03d1487776920b61aa946e5cd8c49e9f45'
E = APP.parent / 'docs/quality/evidence/payment-fx'
WORK = APP / '.local/payment-fx-upgrade-115'

HELPER = r'''
func verifyLegacyPaymentFXUpgrade(t *testing.T, f *fixture) {
 t.Helper()
 ctx := context.Background()
 type fact struct { Rows int `json:"rows"`; Hash string `json:"hash"` }
 snapshot := func() map[string]fact {
  result := map[string]fact{}
  for _, table := range []string{"payment_records","payment_revisions","payment_events","payment_revision_files","payment_allocations","payment_allocation_shares","actual_cost_entries","cost_revisions","cost_allocations","cost_links","agent_budget_policies","agent_budget_reservations","agent_budget_usage","agent_budget_reconciliations"} {
   expression := "to_jsonb(t)"
   if table=="payment_allocations" { expression+="-'cost_amount'-'fx_rate'-'fx_source'-'fx_quoted_on'" }
   if table=="payment_allocation_shares" { expression+="-'cost_amount'" }
   var v fact
   if e:=f.owner.QueryRow(ctx,"SELECT count(*),md5(COALESCE(jsonb_agg("+expression+" ORDER BY "+expression+")::text,'[]')) FROM "+table+" t").Scan(&v.Rows,&v.Hash); e!=nil {t.Fatal(e)}
   result[table]=v
  }
  return result
 }
 var beforeCount,afterCount int
 if e:=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&beforeCount);e!=nil||beforeCount!=88 {t.Fatal(beforeCount,e)}
 before:=snapshot()
 if before["payment_allocations"].Rows<10||before["payment_allocation_shares"].Rows<10 {t.Fatal("upgrade requires actual populated prior cash journey")}
 command:=upgradeexec.Command(upgradeos.Getenv("CP_FX_MIGRATOR"),"migrate")
 command.Env=append(upgradeos.Environ(),"CP_MIGRATION_DATABASE_URL="+f.owner.Config().ConnConfig.ConnString())
 output,e:=command.CombinedOutput()
 if e!=nil {t.Fatal("actual native schema89 migration failed; private diagnostics retained",e)}
 _=output
 after:=snapshot()
 left,_:=json.Marshal(before);right,_:=json.Marshal(after)
 if string(left)!=string(right){t.Fatal("schema89 changed original immutable financial facts")}
 if e=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&afterCount);e!=nil||afterCount!=89 {t.Fatal(afterCount,e)}
 var accurate bool
 if e=f.owner.QueryRow(ctx,`SELECT NOT EXISTS(SELECT 1 FROM payment_allocations WHERE cost_amount<>amount OR fx_rate IS NOT NULL OR fx_source IS NOT NULL OR fx_quoted_on IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM payment_allocation_shares WHERE cost_amount<>amount)`).Scan(&accurate);e!=nil||!accurate {t.Fatal("old same currency ledger backfill differs",e)}
 record,_:=json.Marshal(map[string]any{"passed":true,"before_schema":beforeCount,"after_schema":afterCount,"original_facts_unchanged":true,"legacy_dual_units_exact":true,"facts":before})
 t.Log("PAYMENT_FX_UPGRADE:"+string(record))
}
'''


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    assert not WORK.exists() and not (E / 'schema-upgrade.json').exists(), 'Preserve an existing prior-source upgrade run.'
    source = digest()
    archive = APP / '.local/payment-fx-upgrade-115.zip'
    subprocess.run(['git', 'archive', '--format=zip', '--output=' + str(archive), BASE, 'app/backend'], cwd=APP.parent, check=True)
    WORK.mkdir()
    with zipfile.ZipFile(archive) as files:
        for item in files.infolist():
            assert (item.filename in ('app/', 'app/backend/') and item.is_dir() or item.filename.startswith('app/backend/')) and '..' not in Path(item.filename).parts
        files.extractall(WORK)
    backend = WORK / 'app/backend'
    target = backend / 'internal/httpapi/payments_integration_test.go'
    original = target.read_bytes()
    adapted = original.replace(b'"context"', b'"context"\n upgradeos "os"\n upgradeexec "os/exec"', 1)
    needle = b'f := u.f'
    assert adapted.count(needle) == 1
    adapted = adapted.replace(needle, needle + b'\n t.Cleanup(func(){ verifyLegacyPaymentFXUpgrade(t,f) })', 1) + HELPER.encode()
    target.write_bytes(adapted)
    binary = APP / '.local/payment-fx-migrator.exe'
    buildlog = APP / '.local/payment-fx-upgrade-build.log'
    with buildlog.open('w', encoding='utf-8') as output:
        subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'build', '-o', str(binary), './cmd/platform'], cwd=APP / 'backend', stdout=output, stderr=subprocess.STDOUT, check=True, timeout=300)
    env = {**test_environment(), 'CP_FX_MIGRATOR': str(binary)}
    raw = APP / '.local/payment-fx-upgrade.log'
    with raw.open('w', encoding='utf-8') as output:
        result = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'test', './internal/httpapi', '-json', '-count=1', '-run', '^TestPaymentsAndSettlement$', '-timeout=20m'], cwd=backend, env=env, stdout=output, stderr=subprocess.STDOUT, timeout=1250)
    records, checked, chunks = [], None, []
    for line in raw.read_text(encoding='utf-8').splitlines():
        try: event = json.loads(line)
        except json.JSONDecodeError: continue
        if event.get('Test') and event.get('Action') in ('pass', 'fail', 'skip'): records.append(event)
        chunks.append(event.get('Output', ''))
    # Go splits long log lines into output frames. Parse only after joining
    # their exact bytes, rather than treating an individual frame as a line.
    for line in ''.join(chunks).splitlines():
        if 'PAYMENT_FX_UPGRADE:' in line:
            checked = json.loads(line.split('PAYMENT_FX_UPGRADE:', 1)[1])
    assert result.returncode == 0 and records and all(v['Action'] == 'pass' for v in records) and checked and checked['passed']
    assert source == digest()
    report = {**checked, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': source,
        'published_fixture_commit': BASE, 'prior_checkout_archive_sha256': sha(archive), 'original_test_sha256': hashlib.sha256(original).hexdigest(),
        'adapted_test_sha256': sha(target), 'adaptation': 'Only add one end-of-journey normal-upgrade assertion; existing prior cash cases are unchanged.',
        'actual_current_migrator_sha256': sha(binary), 'raw_log_sha256': sha(raw), 'prior_fixture_case_records': len(records),
        'counts_as_current_backend_tests': False, 'verification_script_sha256': sha(Path(__file__)), 'raw_log': 'ignored app/.local/' + raw.name}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'schema-upgrade.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'schema': [88, 89], 'original_fact_tables': len(checked['facts']), 'actual_prior_allocations': checked['facts']['payment_allocations']['rows']}))


if __name__ == '__main__':
    main()
