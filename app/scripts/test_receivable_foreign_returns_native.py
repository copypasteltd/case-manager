"""Focused actual HTTP/PG checks; native full durability, no virtualization."""
from pathlib import Path
import hashlib
import os
import subprocess
import sys
import contract_body_native_cluster as cluster

APP = Path(__file__).resolve().parents[1]
cluster.OWNER = APP / '.local/receivable-foreign-returns-native-cluster.json'
cluster.E = APP.parent / 'docs/quality/evidence/receivable-foreign-returns'


def main():
    args = ['-run', '^(TestReceivableForeignReturns|TestForeignRefundExpenseAttribution|TestContractForeignRefundAttribution|TestContractFinancialReconciliation)$', '-p=1', '-timeout=12m', '-count=1', '-json']
    try:
        return subprocess.run([str(APP / '.tools/go/bin/go.exe'), 'test', './internal/httpapi', *args], cwd=APP / 'backend', env=cluster.test_environment(), stdout=sys.stdout.buffer, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0).returncode
    finally:
        if cluster.OWNER.exists():
            cluster.stop()
            identity = hashlib.sha256(cluster.OWNER.read_bytes()).hexdigest()[:16]
            cluster.OWNER.rename(cluster.OWNER.with_name('receivable-foreign-returns-native-closed-' + identity + '.json'))


if __name__ == '__main__':
    raise SystemExit(main())
