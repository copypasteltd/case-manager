"""Execute populated published schema92 finance cases, then the real schema93 migrator.

Only an end-of-journey assertion is added to the archived prior fixture. This
normal additive upgrade does not count as a current-source business test.
"""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile
from refund_fx_native_cluster import APP, test_environment
from verify_backend_batches import digest

BASE = '794406d51cad29ba66ff8d8cc7385b6441edb2e5'
E = APP.parent / 'docs/quality/evidence/refund-fx'
WORK = APP / '.local/refund-fx-upgrade-120-history-fixed'
HELPER = r'''
func verifyLegacyRefundUpgrade(t *testing.T, f *fixture) {
 t.Helper();ctx:=context.Background()
 type fact struct { Rows int `json:"rows"`; Hash string `json:"hash"` }
 snapshot:=func()map[string]fact{out:=map[string]fact{};for _,table:=range []string{"payment_records","payment_revisions","payment_events","payment_allocations","payment_allocation_shares","actual_cost_entries","cost_revisions","contracts","contract_revisions","file_versions","file_replicas","agent_budget_policies","agent_budget_reservations","agent_budget_usage","agent_budget_reconciliations"}{var v fact;if e:=f.owner.QueryRow(ctx,"SELECT count(*),md5(COALESCE(jsonb_agg(to_jsonb(t)-'refund_original_amount'-'refund_fx_acknowledged' ORDER BY to_jsonb(t)-'refund_original_amount'-'refund_fx_acknowledged')::text,'[]')) FROM "+table+" t").Scan(&v.Rows,&v.Hash);e!=nil{t.Fatal(e)};out[table]=v};return out}
 var beforeCount,afterCount int
 if e:=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&beforeCount);e!=nil||beforeCount!=92{t.Fatal(beforeCount,e)}
 before:=snapshot();if before["payment_allocations"].Rows<5||before["payment_revisions"].Rows<5{t.Fatal("upgrade requires populated prior actual finance journey")}
 command:=upgradeexec.Command(upgradeos.Getenv("CP_REFUND_FX_MIGRATOR"),"migrate");command.Env=append(upgradeos.Environ(),"CP_MIGRATION_DATABASE_URL="+f.owner.Config().ConnConfig.ConnString());if output,e:=command.CombinedOutput();e!=nil{_=output;t.Fatal("actual schema93 migrator failed",e)}
 after:=snapshot();left,_:=upgradejson.Marshal(before);right,_:=upgradejson.Marshal(after);if string(left)!=string(right){t.Fatal("normal upgrade rewrote original cash/cost evidence")}
 if e:=f.owner.QueryRow(ctx,"SELECT count(*) FROM schema_migrations").Scan(&afterCount);e!=nil||afterCount!=93{t.Fatal(afterCount,e)}
 var accurate bool
 if e:=f.owner.QueryRow(ctx,`SELECT NOT EXISTS(SELECT 1 FROM payment_revisions WHERE refund_original_amount IS DISTINCT FROM amount) AND NOT EXISTS(SELECT 1 FROM payment_events WHERE refund_fx_acknowledged) AND EXISTS(SELECT 1 FROM pg_trigger WHERE tgname='payment_foreign_refund_no_attribution' AND tgenabled='O')`).Scan(&accurate);e!=nil||!accurate{t.Fatal("additive principal columns or attribution guard incorrect",e)}
 record,_:=upgradejson.Marshal(map[string]any{"passed":true,"before_schema":beforeCount,"after_schema":afterCount,"original_facts_unchanged":true,"new_principal_matches_legacy_cash":true,"new_confirmation_defaults_false":true,"facts":before});t.Log("REFUND_FX_UPGRADE:"+string(record))
}
'''
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    assert not WORK.exists() and not (E / 'schema-upgrade.json').exists()
    source = digest()
    archive = APP / '.local/refund-fx-upgrade-120.zip'
    subprocess.run(['git', 'archive', '--format=zip', '--output=' + str(archive), BASE, 'app/backend'], cwd=APP.parent, check=True)
    WORK.mkdir()
    with zipfile.ZipFile(archive) as files:
        assert all((v.filename in ('app/', 'app/backend/') and v.is_dir() or v.filename.startswith('app/backend/')) and '..' not in Path(v.filename).parts for v in files.infolist())
        files.extractall(WORK)
    backend = WORK / 'app/backend'
    target = backend / 'internal/httpapi/payment_fx_settlement_integration_test.go'
    original = target.read_bytes()
    adapted = original.replace(b'"context"', b'"context"\n upgradeos "os"\n upgradeexec "os/exec"\n upgradejson "encoding/json"', 1)
    end = adapted.rfind(b'\n}')
    assert end > 0
    adapted = adapted[:end] + b'\n verifyLegacyRefundUpgrade(t,f)' + adapted[end:] + HELPER.encode()
    target.write_bytes(adapted)
    binary = APP / '.local/refund-fx-migrator.exe'
    with (APP / '.local/refund-fx-upgrade-build.log').open('w', encoding='utf-8') as output:
        subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'build', '-o', str(binary), './cmd/platform'], cwd=APP / 'backend', stdout=output, stderr=subprocess.STDOUT, check=True, timeout=300)
    raw = APP / '.local/refund-fx-upgrade.log'
    with raw.open('w', encoding='utf-8') as output:
        result = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'test', './internal/httpapi', '-json', '-count=1', '-run', '^TestCrossCurrencySettlement$', '-timeout=20m'], cwd=backend, env={**test_environment(), 'CP_REFUND_FX_MIGRATOR': str(binary)}, stdout=output, stderr=subprocess.STDOUT, timeout=1250)
    chunks, records = [], []
    for line in raw.read_text(encoding='utf-8').splitlines():
        try: event = json.loads(line)
        except json.JSONDecodeError: continue
        chunks.append(event.get('Output', ''))
        if event.get('Test') and event.get('Action') in ('pass', 'fail', 'skip'): records.append(event)
    checked = None
    for line in ''.join(chunks).splitlines():
        if 'REFUND_FX_UPGRADE:' in line: checked = json.loads(line.split('REFUND_FX_UPGRADE:', 1)[1])
    assert result.returncode == 0 and records and all(v['Action'] == 'pass' for v in records) and checked and checked['passed'] and digest() == source
    report = {**checked, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': source,
        'published_fixture_commit': BASE, 'prior_checkout_archive_sha256': sha(archive), 'original_test_sha256': hashlib.sha256(original).hexdigest(),
        'adapted_test_sha256': sha(target), 'adaptation': 'Append only an end-of-journey schema92 to93 assertion; prior finance cases remain unchanged.',
        'actual_current_migrator_sha256': sha(binary), 'raw_log_sha256': sha(raw), 'prior_fixture_case_records': len(records),
        'counts_as_current_backend_tests': False, 'verification_script_sha256': sha(Path(__file__)), 'raw_log': 'ignored app/.local/' + raw.name}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'schema-upgrade.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'schema': [92, 93], 'original_fact_tables': len(checked['facts'])}))


if __name__ == '__main__': main()
