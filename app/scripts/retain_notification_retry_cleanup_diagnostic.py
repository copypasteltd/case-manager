"""Retain only two safe native checkpoint lines explaining a failed fixture cleanup."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/notification-retry'


def main():
    target = E / 'native-cleanup-diagnostic.json'
    private = APP / '.local/notification-retry-attempts/backend02-cleanup/checkpoint-excerpt.log'
    assert not target.exists() and not private.exists()
    lines = (APP / '.local/postgres.log').read_text(encoding='utf-8').splitlines()
    start = next(line for line in lines if line.startswith('2026-10-29 17:01:32.796 CST [19588] LOG:  checkpoint starting: immediate force wait'))
    end = next(line for line in lines if line.startswith('2026-10-29 17:03:44.814 CST [19588] LOG:  checkpoint complete:'))
    assert 'sync=128.318 s, total=132.018 s; sync files=1314' in end
    private.write_text(start + '\n' + end + '\n', encoding='utf-8')
    record = {'passed':True, 'retained_at':datetime.now(timezone.utc).isoformat(),
        'scope':'Diagnostic of the original failed local fixture cleanup, not a network or capacity benchmark.',
        'checkpoint_start':'2026-10-29T17:01:32.796+08:00', 'checkpoint_end':'2026-10-29T17:03:44.814+08:00',
        'checkpoint_total_seconds':132.018, 'checkpoint_sync_seconds':128.318, 'sync_files':1314,
        'unchanged_drop_context_seconds':120,
        'interpretation':'The native server checkpoint overlaps the failed DROP and exceeds its existing cleanup deadline. The same unchanged test family subsequently passes on a separate fully durable native cluster.',
        'private_excerpt':private.relative_to(APP.parent).as_posix(),
        'private_excerpt_sha256':hashlib.sha256(private.read_bytes()).hexdigest(),
        'verification_script_sha256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'full_server_log_published':False, 'database_durability_changed':False,
        'test_deadline_changed':False, 'network_performance_test':False}
    target.write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':True,'retained_checkpoint_lines':2,'full_server_log_published':False}))


if __name__ == '__main__':
    main()
