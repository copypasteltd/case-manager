"""Run the selected unchanged HTTP suites on an owned native temporary PG cluster.

Keep full PostgreSQL durability, the original per-fixture cleanup deadline and
normal schema migrations. Never restart or reconfigure the development cluster.
This is a native process, without Docker, WSL, a VM or an emulator.
"""
from pathlib import Path
import hashlib
import os
import subprocess
import sys
import contract_body_native_cluster as cluster

APP = Path(__file__).resolve().parents[1]
cluster.OWNER = APP / '.local/contract-refunds-native-cluster.json'
cluster.E = APP.parent / 'docs/quality/evidence/contract-refunds'


def main():
    args = sys.argv[1:]
    assert len(args) == 5 and args[0] == '-run' and args[2:] == ['-p=1', '-timeout=12m', '-json']
    assert args[1] == '^(TestContractForeignRefundAttribution|TestContractFinancialReconciliation|TestActualForeignRefunds|TestContractCompletionAndTermination|TestCrossCurrencySettlement)$'
    try:
        result = subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'test', './internal/httpapi', '-count=1', *args],
            cwd=APP / 'backend', env=cluster.test_environment(),
            stdout=sys.stdout.buffer, stderr=subprocess.STDOUT,
            creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
        return result.returncode
    finally:
        # The helper refuses to stop a cluster containing a failed disposable
        # fixture. Preserve its exact ownership and private data for inspection.
        if cluster.OWNER.exists():
            cluster.stop()
            # Keep private ownership recoverable while allowing a later
            # separately archived check to create a new temporary cluster.
            identity = hashlib.sha256(cluster.OWNER.read_bytes()).hexdigest()[:16]
            cluster.OWNER.rename(cluster.OWNER.with_name('contract-refunds-native-cluster-closed-' + identity + '.json'))


if __name__ == '__main__':
    raise SystemExit(main())
