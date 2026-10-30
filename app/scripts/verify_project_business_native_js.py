"""Inspect the actual current JS bundle/map without claiming APK acceptance."""
from datetime import datetime, timezone
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/project-business'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--feature', choices=('project-business', 'business-cost', 'cost-payment', 'file-batch', 'document-text', 'office-authoring'), default='project-business')
    args = parser.parse_args()
    evidence = APP.parent / 'docs/quality/evidence' / args.feature
    spec = importlib.util.spec_from_file_location('current_js_review', APP / 'scripts/verify_scoped_increment.py')
    current = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(current)
    config = current.plan('docs/quality/evidence/' + args.feature + '/verification-plan.json')
    build = json.loads((evidence / 'native-bundle.json').read_text(encoding='utf-8'))
    assert build['passed'] and build['source_unchanged'] and build['source_sha256'] == current.digest(config=config)
    bundle = APP / 'mobile/android/app/src/main/assets/index.android.bundle'
    mapping = APP / '.local/android/index.android.map'
    assert bundle.is_file() and 0 < mapping.stat().st_size <= 80 * 1024 * 1024
    data = json.loads(mapping.read_text(encoding='utf-8'))
    assert len(data['sources']) == len(data['sourcesContent'])
    paths = [name.replace('\\', '/') for name in data['sources']]
    records = []
    modules = ('mobile/src/pages/finance/subscriptions.tsx', 'mobile/src/pages/subscription-period/index.tsx', 'packages/contracts/src/project-business.ts') if args.feature == 'project-business' else (
        'mobile/src/pages/finance/editor.tsx', 'mobile/src/pages/finance/operations.tsx', 'mobile/src/pages/business/index.tsx', 'packages/contracts/src/cost-business.ts')
    if args.feature == 'cost-payment':
        modules = ('mobile/src/pages/finance/cost-payment.tsx', 'mobile/src/pages/finance/index.tsx', 'mobile/src/pages/finance/operations.tsx', 'packages/contracts/src/cost-payment.ts')
    if args.feature == 'file-batch':
        modules = ('mobile/src/pages/files/batches.tsx', 'mobile/src/pages/files/index.tsx', 'mobile/src/pages/files/scope-transfers.tsx', 'packages/contracts/src/file-batch.ts')
    if args.feature in ('document-text', 'office-authoring'):
        modules = ('mobile/src/pages/search/index.tsx', 'mobile/src/pages/files/comparison.tsx', 'packages/contracts/src/search.ts', 'packages/contracts/src/file-comparison.ts')
    if args.feature == 'office-authoring':
        modules = ('mobile/src/pages/files/office.tsx', 'mobile/src/platform/office-upload.rn.ts', 'mobile/src/platform/uploads.rn.ts', 'packages/contracts/src/office-author.ts')
    for name in modules:
        suffix = name if name.startswith('packages/') else name.removeprefix('mobile/')
        indices = [i for i, source in enumerate(paths) if source == suffix or source.endswith('/' + suffix)]
        assert len(indices) == 1, 'Expected one exact current native module: ' + name
        value = data['sourcesContent'][indices[0]]
        assert isinstance(value, str) and value.replace('\r\n', '\n') == (APP / name).read_text(encoding='utf-8').replace('\r\n', '\n'), 'Current module source differs: ' + name
        records.append({'file': name, 'file_sha256': sha(APP / name), 'map_source': paths[indices[0]],
            'source_content_sha256': hashlib.sha256(value.encode()).hexdigest(), 'source_content_matches_modulo_line_endings': True})
    record = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': current.digest(config=config),
        'build_record_sha256': sha(evidence / 'native-bundle.json'), 'bundle_sha256': sha(bundle), 'source_map_sha256': sha(mapping),
        'modules': records, 'verification_script_sha256': sha(Path(__file__)), 'read_only_artifact_inspection': True,
        'apk_built': args.feature == 'office-authoring', 'formal_android_acceptance': False, 'real_device_used': False, 'virtualization_used': False}
    (evidence / 'android-js-content.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': True, 'current_modules_verified': len(records), 'apk_built': args.feature == 'office-authoring'}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
