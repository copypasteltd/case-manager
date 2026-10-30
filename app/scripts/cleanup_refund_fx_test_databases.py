"""Retain interrupted synthetic fixtures before releasing this task's test cluster.

This handles only the attested temporary refund-fx cluster after all current Go
cases pass. Production/development databases and historical migration/DR are
outside this operation. Archives and credentials remain private and ignored.
"""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re

import refund_fx_native_cluster as cluster

APP = cluster.APP
E = APP.parent / 'docs/quality/evidence/refund-fx'
PRIVATE = APP / '.local/refund-fx-failed-fixtures'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    backend = json.loads((E.parent / 'refund-fx-backend/result.json').read_text(encoding='utf-8'))
    assert backend['passed'] and backend['all_discovered_tests_executed']
    owner = json.loads(cluster.OWNER.read_text(encoding='utf-8'))
    cluster.attest(owner)
    query = "SELECT COALESCE(json_agg(datname ORDER BY datname),'[]'::json) FROM pg_database WHERE datname ~ '^cp_(test|suite|batch|fixture)_'"
    names = json.loads(cluster.sql(owner, query))
    assert all(re.fullmatch(r'cp_test_[0-9a-f]{32}', name) for name in names), 'Keep unexpected fixture names intact.'
    records = []
    PRIVATE.mkdir(parents=True, exist_ok=True)
    assert PRIVATE.resolve().is_relative_to((APP / '.local').resolve())
    for name in names:
        # Exact identifiers originate in the attested task-only cluster and
        # must match setup(t)'s generated synthetic name. Never force sessions.
        active = "SELECT count(*) FROM pg_stat_activity WHERE datname='" + name + "'"
        assert int(cluster.sql(owner, active).strip()) == 0, 'Keep an active fixture intact.'
        snapshot = PRIVATE / (name + '.dump')
        assert not snapshot.exists(), 'Do not replace an earlier retained fixture archive.'
        env = {**os.environ, 'PGHOST': '127.0.0.1', 'PGPORT': str(owner['port']),
               'PGUSER': 'postgres', 'PGDATABASE': name, 'PGPASSWORD': owner['admin_password']}
        cluster.run('pg_dump', ['--format=custom', '--file', str(snapshot)], env=env, timeout=120)
        assert snapshot.read_bytes()[:5] == b'PGDMP'
        catalog = PRIVATE / (name + '.catalog.txt')
        catalog.write_text(cluster.run('pg_restore', ['--list', str(snapshot)], timeout=30), encoding='utf-8')
        before = {'database': name, 'file': snapshot.relative_to(APP).as_posix(), 'bytes': snapshot.stat().st_size,
                  'sha256': sha(snapshot), 'catalog_file': catalog.relative_to(APP).as_posix(),
                  'catalog_sha256': sha(catalog), 'complete_archive_readable': True, 'database_released': False}
        journal = PRIVATE / (name + '.json')
        journal.write_text(json.dumps(before, indent=2) + '\n', encoding='utf-8')
        cluster.attest(owner)
        assert int(cluster.sql(owner, active).strip()) == 0
        # No FORCE: a newly active connection makes cleanup fail closed.
        cluster.sql(owner, 'DROP DATABASE "' + name + '"')
        before['database_released'] = True
        journal.write_text(json.dumps(before, indent=2) + '\n', encoding='utf-8')
        records.append(before)
    remaining = json.loads(cluster.sql(owner, query))
    assert not remaining
    report = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(),
              'owned_temporary_cluster_attested': True, 'backend_source_sha256': backend['source_sha256'],
              'retained_fixture_count': len(records), 'archives': records, 'remaining_disposable_databases': 0,
              'development_or_production_database_modified': False, 'restoration_executed': False,
              'historical_migration_or_dr_acceptance': False, 'archives_private': True,
              'verification_script_sha256': sha(Path(__file__))}
    (E / 'test-database-cleanup.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'private_fixtures_retained': len(records), 'remaining': 0}))


if __name__ == '__main__':
    main()
