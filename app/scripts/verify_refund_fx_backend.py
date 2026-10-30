"""Resume every discovered case against owned native PG, selecting its actual packages.

Completed exact-source batches remain reusable. The current interrupted batch
runs again in full. No case, lease, session or package deadline is relaxed.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import verify_backend_batches as base
from refund_fx_native_cluster import APP, test_environment

E = APP.parent / 'docs/quality/evidence/refund-fx-backend'
assert '--resume' in sys.argv, 'This source-bound runner resumes the existing discovery manifest.'
state = json.loads((E / 'batch-progress.json').read_text(encoding='utf-8'))
source = base.digest()
assert state['source_sha256'] == source
expected = sorted(map(tuple, state['expected_tests']))
base.discover = lambda: expected
plans = {}
for index, batch in enumerate(state['batches']):
    pattern = '^(' + '|'.join(re.escape(name) for name in batch['names']) + ')$'
    packages = sorted({package for package, name in expected if name in batch['names']})
    assert packages and all(package.startswith('copypaste.hk/platform/') for package in packages)
    plans[pattern] = {'batch': index + 1, 'names': batch['names'], 'packages': packages}
retained = [index + 1 for index, batch in enumerate(state['batches']) if batch['status'] == 'passed']
invocations = []


def command(*args):
    assert '-run' in args and '-timeout=20m' in args
    pattern = args[args.index('-run') + 1]
    plan = plans[pattern]
    assert base.digest() == source
    actual = [str(APP / '.tools/go/bin/go.exe'), '-C', str(APP / 'backend'),
              'test', *plan['packages'], '-count=1', '-json', '-p', '1', *args]
    invocations.append({**plan, 'arguments': actual})
    return actual


# The same durable native fixture environment as test_refund_fx_native.py.
# Keep credentials in the process environment; never include them in evidence.
os.environ.update(test_environment())
base.command = command
sys.argv += ['--evidence-group','refund-fx-backend']
result = base.main()
after = base.digest()
record = {'passed': result == 0 and after == source, 'source_sha256': source,
          'source_after_sha256': after, 'all_discovered_test_names_preserved': True,
          'retained_completed_batches': retained, 'invocations': invocations,
          'count_flag': 1, 'package_parallelism': 1, 'package_deadline': '20m',
          'selection': 'All packages containing each selected name, from the exact unchanged discovery manifest; no test names removed.',
          'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
(E / 'package-execution-plan.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
raise SystemExit(result)
