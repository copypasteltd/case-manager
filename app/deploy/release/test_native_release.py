"""Native filesystem/archive tests and injected process failures; no Linux emulation."""
from contextlib import contextmanager
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

import bundle
from bundle import ReleaseError, canonical, digest, inventory, verify_tree, write_archive
import catalog
from controller import Coordinator, NativeEffects, NativePointers, Paths, atomic_json, database_identity, parse_env
import sys
import subprocess
from unittest.mock import patch


def schema(n):
    return [{'name': f'{i:04d}_fixture.sql', 'sha256': digest(str(i).encode())} for i in range(1, n + 1)]


class Effects:
    def __init__(self):
        self.actual, self.calls = [], []
        self.fail_start, self.fail_stop, self.interrupt, self.partial = False, False, False, False
        self.fail_health = 0

    def preflight(self, root, config, paths):
        self.calls.append(('preflight', config['configuration_id']))

    def schema(self, environment):
        return list(self.actual)

    def stop(self, names):
        self.calls.append(('stop', list(names)))
        if self.fail_stop:
            raise ReleaseError('injected stop failure')

    def start(self, names):
        self.calls.append(('start', list(names)))
        if self.interrupt:
            self.interrupt = False
            raise KeyboardInterrupt()
        if self.fail_start:
            raise ReleaseError('injected start failure')

    def healthy(self, names):
        self.calls.append(('healthy', list(names)))
        if self.fail_health:
            self.fail_health -= 1
            raise ReleaseError('injected readiness failure')

    def migrate(self, root, environment):
        self.calls.append(('migrate', root.name))
        value = json.loads((root / 'release.json').read_text())['schema']
        self.actual = value[:len(self.actual) + 1] if self.partial else value
        if self.partial:
            raise ReleaseError('injected partial migration')


class FilePointers:
    """Explicit unit-test double only: Windows host lacks symlink privilege.

    Atomic JSON pointer records exercise coordinator states, not Linux symlinks.
    Linux CI uses NativePointers; the production CLI never selects this double.
    """
    def get(self, pointer):
        return Path(json.loads(pointer.read_bytes())['target']) if pointer.exists() else None

    def replace(self, pointer, target):
        atomic_json(pointer, {'target': str(target)})


class Fixture(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        self.paths = Paths(*[self.base / p for p in ('installation', 'configuration', 'state', 'units')], self.base / 'nginx.conf')
        for directory in (self.paths.root, self.paths.config_root, self.paths.state, self.paths.units):
            directory.mkdir()
        self.effects = Effects()
        self.pointers = NativePointers() if sys.platform == 'linux' else FilePointers()
        self.coordinator = Coordinator(self.paths, self.effects, strict=False, pointers=self.pointers)

    def package(self, marker='first', migrations=1):
        root = self.base / ('input-' + marker)
        root.mkdir()
        for name in ['bin/platform', 'bin/agent-native', 'bin/status-bridge', 'web/index.html',
                     'web/assets/main-' + marker + '.js', 'runtime/app/agent-runtime/dist/worker.js', 'metadata/runtime.json']:
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(marker, encoding='utf-8')
        for name in catalog.SERVICES:
            path = root / 'systemd' / catalog.unit(name)
            path.parent.mkdir(exist_ok=True)
            path.write_text(catalog.render(name), encoding='utf-8')
        value = {'format': 1, 'target': 'linux-amd64', 'source': {'base_commit': 'a' * 40, 'inputs_sha256': 'b' * 64, 'working_tree_dirty': True},
                 'schema': schema(migrations), 'files': inventory(root)}
        (root / 'release.json').write_bytes(canonical(value))
        archive = self.base / (marker + '.tar.gz')
        receipt = write_archive(root, archive)
        return root, archive, receipt

    def configuration(self, name, units=('api', 'notification'), database='company'):
        root = self.paths.config_root / 'configurations' / name
        root.mkdir(parents=True)
        (root / 'deployment.json').write_bytes(canonical({'format': 1, 'units': list(units), 'hostname': 'case.example.invalid'}))
        connection = 'postgresql://fixture@db.example.invalid/' + database + '?sslmode=verify-full'
        (root / 'migration.env').write_text('CP_MIGRATION_DATABASE_URL=' + connection.replace('fixture@', 'migrator@') + '\n')
        for name in units:
            env = catalog.SERVICES.get(name, (None, None, None))[2]
            if env:
                (root / env).write_text('CP_ENV=production\nCP_DATABASE_URL=' + connection + '\n' +
                                      ('CP_HTTP_ADDR=127.0.0.1:8080\nCP_PUBLIC_ORIGIN=https://case.example.invalid\n' if name == 'api' else ''))
        return root

    def staged(self, marker='first', migrations=1, config=None):
        _, archive, receipt = self.package(marker, migrations)
        result = self.coordinator.stage(archive, receipt['manifest_sha256'])
        config = config or 'cfg-' + marker
        self.configuration(config)
        return result['release_id'], receipt['manifest_sha256'], config

    def installed(self):
        values = self.staged()
        result = self.coordinator.activate(*values, 'none', migrate=True)
        self.assertEqual(result['state'], 'committed')
        self.effects.calls.clear()
        return values

    @contextmanager
    def writable_release(self, root):
        # Tests preserve/finally restore private temporary package permissions only.
        paths = [root, *root.rglob('*')]
        for path in paths:
            path.chmod(0o755 if path.is_dir() else 0o644)
        try:
            yield
        finally:
            for path in reversed(paths):
                if path.exists():
                    path.chmod(0o755 if path.is_dir() else 0o644)

    def tearDown(self):
        # Windows read-only attributes need explicit clearing before TemporaryDirectory cleanup.
        for path in self.base.rglob('*'):
            if not path.is_symlink():
                path.chmod(0o755 if path.is_dir() else 0o644)


class BundleTests(Fixture):
    def test_inventory_order_is_canonical_across_directory_prefixes_and_host_case_rules(self):
        root, _, _ = self.package()
        for name in ('metadata/a/z', 'metadata/a-plugin/B', 'metadata/Upper', 'metadata/lower'):
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text('public')
        files = inventory(root)
        self.assertEqual([row['path'] for row in files], sorted(row['path'] for row in files))

    def test_public_dependency_dotfiles_round_trip_without_allowing_private_names(self):
        root, _, _ = self.package()
        file = root / 'runtime/app/node_modules/ajv/.runkit_example.js'
        file.parent.mkdir(parents=True)
        file.write_text('public package example')
        row = json.loads((root / 'release.json').read_text())
        row['files'] = inventory(root)
        (root / 'release.json').write_bytes(canonical(row))
        archive = self.base / 'public-metadata.tar.gz'
        result = write_archive(root, archive)
        target = self.base / 'public-metadata'
        bundle.extract_archive(archive, target, result['manifest_sha256'])
        self.assertEqual((target / file.relative_to(root)).read_bytes(), file.read_bytes())
        for name in ('runtime/.npmrc', 'runtime/.env', 'runtime/.secret.json'):
            with self.assertRaises(ReleaseError):
                bundle.safe_name(name)

    def test_actual_portable_inspect_cli_checks_the_pinned_archive(self):
        _, archive, receipt = self.package()
        result = subprocess.run([sys.executable, str(Path(bundle.__file__)), 'inspect', '--archive', str(archive),
                                 '--manifest-sha256', receipt['manifest_sha256'], '--output', str(self.base / 'cli-inspection')],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertFalse(json.loads(result.stdout)['deployment_executed'])

    def test_deterministic_round_trip_and_complete_inventory(self):
        root, archive, receipt = self.package()
        second = self.base / 'second.tar.gz'
        self.assertEqual(write_archive(root, second), receipt)
        self.assertEqual(archive.read_bytes(), second.read_bytes())
        out = self.base / 'unpacked'
        self.assertEqual(bundle.extract_archive(archive, out, receipt['manifest_sha256']), verify_tree(root, receipt['manifest_sha256']))

    def test_changed_missing_extra_and_wrong_pinned_digest_are_refused(self):
        root, _, receipt = self.package()
        with self.assertRaises(ReleaseError):
            verify_tree(root, '0' * 64)
        (root / 'web/index.html').write_text('changed')
        with self.assertRaises(ReleaseError):
            verify_tree(root, receipt['manifest_sha256'])
        (root / 'web/index.html').unlink()
        with self.assertRaises(ReleaseError):
            verify_tree(root, receipt['manifest_sha256'])
        (root / 'web/index.html').write_text('first')
        (root / 'metadata/unlisted').write_text('unexpected')
        with self.assertRaises(ReleaseError):
            verify_tree(root, receipt['manifest_sha256'])

    def test_json_duplicates_nonfinite_and_unsafe_names_fail_closed(self):
        for raw in ('{"format":1,"format":1}', '{"v":NaN}', '{"v":Infinity}'):
            with self.assertRaises(ReleaseError):
                bundle.read_json(raw)
        for name in ('../escape', '/absolute', 'web//x', 'web/./x', 'C:/secret', 'web\\file', '.env', 'web/../../../x'):
            with self.assertRaises(ReleaseError):
                bundle.safe_name(name)

    def test_archive_traversal_links_duplicates_and_devices_are_rejected(self):
        for i, kind in enumerate(('traversal', 'symlink', 'hardlink', 'device', 'duplicate')):
            with self.subTest(kind=kind):
                archive = self.base / (kind + '.tar.gz')
                with tarfile.open(archive, 'w:gz') as out:
                    member = tarfile.TarInfo('../outside' if kind == 'traversal' else 'web/member')
                    member.type = {'symlink': tarfile.SYMTYPE, 'hardlink': tarfile.LNKTYPE, 'device': tarfile.CHRTYPE}.get(kind, tarfile.REGTYPE)
                    member.linkname = '/outside'
                    out.addfile(member, io.BytesIO())
                    if kind == 'duplicate':
                        out.addfile(member, io.BytesIO())
                with self.assertRaises(ReleaseError):
                    bundle.extract_archive(archive, self.base / ('out-' + str(i)), '0' * 64)
        self.assertFalse((self.base / 'outside').exists())

    def test_extraction_never_overwrites_existing_directory(self):
        _, archive, receipt = self.package()
        target = self.base / 'keep'
        target.mkdir()
        (target / 'private').write_text('preserve')
        with self.assertRaises(ReleaseError):
            bundle.extract_archive(archive, target, receipt['manifest_sha256'])
        self.assertEqual((target / 'private').read_text(), 'preserve')

    def test_declared_size_and_archive_total_bounds(self):
        archive = self.base / 'oversize.tar.gz'
        with tarfile.open(archive, 'w:gz') as out:
            member = tarfile.TarInfo('web/large')
            member.size = bundle.MAX_FILE_BYTES + 1
            # Stream creation stops after its header; reader must reject before copying bytes.
            out.fileobj.write(member.tobuf())
        with self.assertRaises(ReleaseError):
            bundle.extract_archive(archive, self.base / 'out', '0' * 64)

    def test_stage_preserves_existing_assets_and_blocks_collisions(self):
        values = self.staged()
        with self.assertRaises(ReleaseError):
            _, archive, receipt = self.package('duplicate')
            self.coordinator.stage(archive, values[1])
        assets = self.paths.root / 'assets/main-first.js'
        self.assertEqual(assets.read_text(), 'first')
        assets.chmod(0o644)
        assets.write_text('tampered')
        _, archive, receipt = self.package('again')
        # Give a second artifact the same asset name with conflicting bytes.
        source = self.base / 'input-again'
        (source / 'web/assets/main-again.js').rename(source / 'web/assets/main-first.js')
        manifest = json.loads((source / 'release.json').read_text())
        manifest['files'] = inventory(source)
        (source / 'release.json').write_bytes(canonical(manifest))
        fresh = self.base / 'collision.tar.gz'
        receipt = write_archive(source, fresh)
        with self.assertRaises(ReleaseError):
            self.coordinator.stage(fresh, receipt['manifest_sha256'])


class TransitionTests(Fixture):
    def test_stage_public_ancestors_remain_traversable_under_private_umask(self):
        import os
        previous = os.umask(0o077)
        try:
            values = self.staged()
        finally:
            os.umask(previous)
        for path in (self.paths.root / 'releases', self.paths.root / 'assets'):
            # Windows reports DOS-derived mode flags; real POSIX enforcement is Linux CI only.
            self.assertEqual(path.stat().st_mode & 0o005, 0o005)
        self.assertEqual(self.coordinator.staged(values[0], values[1])[1]['schema'], schema(1))

    def test_percent_encoded_runtime_login_is_not_a_separate_migration_identity(self):
        root = self.configuration('cfg-encoded')
        path = root / 'migration.env'
        path.write_text(path.read_text().replace('migrator@', '%66ixture@'))
        with self.assertRaises(ReleaseError):
            self.coordinator.configuration('cfg-encoded')

    def test_connection_queries_cannot_override_host_database_or_login(self):
        base = 'postgresql://fixture@db.example.invalid/company?sslmode=verify-full'
        for query in ('host=other.invalid', 'port=5440', 'dbname=other', 'user=postgres', 'password=unexpected',
                      'service=other', 'options=-csearch_path%3Dother', 'sslmode=disable', 'sslrootcert=a&sslrootcert=b', 'connect_timeout='):
            with self.subTest(query=query), self.assertRaises(ReleaseError):
                database_identity(base + '&' + query)

    def test_percent_encoded_database_and_safe_tls_options_share_actual_identity(self):
        base = 'postgresql://fixture@db.example.invalid/company?sslmode=verify-full'
        encoded = 'postgresql://other@db.example.invalid/%63ompany?sslmode=verify-full&connect_timeout=10&application_name=release'
        self.assertEqual(database_identity(base), database_identity(encoded))
        self.assertNotEqual(database_identity(base), database_identity(base.replace('.invalid/', '.invalid:5440/')))

    def test_durable_event_tail_recovers_after_pending_record_interrupt(self):
        old = self.installed()
        new = self.staged('second')
        def interrupt_pending(path, value):
            if path.name == 'pending.json' and value.get('phase') == 'switched':
                raise KeyboardInterrupt()
            return atomic_json(path, value)
        with patch('controller.atomic_json', side_effect=interrupt_pending), self.assertRaises(KeyboardInterrupt):
            self.coordinator.activate(*new, old[0])
        before = {p.name: p.read_bytes() for p in (self.paths.state / 'events').glob('*.json')}
        persisted = (self.paths.state / 'pending.json').read_bytes()
        self.assertEqual(json.loads(persisted)['phase'], 'schema_verified')
        self.assertEqual(self.coordinator.status()['pending_operation']['phase'], 'switched')
        self.assertEqual((self.paths.state / 'pending.json').read_bytes(), persisted)
        operation = json.loads(persisted)['operation_id']
        self.coordinator.recover(operation, 'forward')
        self.assertEqual(self.coordinator.current()['release_id'], new[0])
        self.assertTrue(all((self.paths.state / 'events' / name).read_bytes() == raw for name, raw in before.items()))

    def test_recovery_intent_in_durable_tail_is_not_replaced_by_stale_pending(self):
        old = self.installed()
        new = self.staged('second')
        self.effects.interrupt = True
        with self.assertRaises(KeyboardInterrupt):
            self.coordinator.activate(*new, old[0])
        operation = self.coordinator.status()['pending_operation']['operation_id']
        def interrupt_pending(path, value):
            if path.name == 'pending.json' and value.get('phase') == 'recovery_previous':
                raise KeyboardInterrupt()
            return atomic_json(path, value)
        with patch('controller.atomic_json', side_effect=interrupt_pending), self.assertRaises(KeyboardInterrupt):
            self.coordinator.recover(operation, 'previous')
        self.assertEqual(self.coordinator.status()['pending_target_release_id'], old[0])
        self.coordinator.recover(operation, 'forward')
        self.assertEqual(self.coordinator.current()['release_id'], old[0])

    def test_interrupted_temporary_event_never_becomes_a_partial_journal_member(self):
        old = self.installed()
        new = self.staged('second')
        import os
        original = os.rename
        def interrupt_event(source, target):
            if source.name.startswith('.event-') and json.loads(source.read_bytes())['phase'] == 'switched':
                raise KeyboardInterrupt()
            return original(source, target)
        with patch('controller.os.rename', side_effect=interrupt_event), self.assertRaises(KeyboardInterrupt):
            self.coordinator.activate(*new, old[0])
        self.assertTrue(list((self.paths.state / 'events').glob('.event-*')))
        self.assertEqual(self.coordinator.status()['pending_operation']['phase'], 'schema_verified')
        operation = self.coordinator.status()['pending_operation']['operation_id']
        self.coordinator.recover(operation, 'forward')
        self.assertEqual(self.coordinator.current()['release_id'], new[0])

    def test_broken_pending_event_chain_refuses_before_recovery_service_effects(self):
        old = self.installed()
        new = self.staged('second')
        self.effects.interrupt = True
        with self.assertRaises(KeyboardInterrupt):
            self.coordinator.activate(*new, old[0])
        operation = self.coordinator.status()['pending_operation']['operation_id']
        event = sorted((self.paths.state / 'events').glob(operation + '-*.json'))[1]
        value = json.loads(event.read_bytes())
        value['previous_sha256'] = '0' * 64
        event.write_bytes(canonical(value))
        self.effects.calls.clear()
        with self.assertRaises(ReleaseError):
            self.coordinator.recover(operation, 'forward')
        self.assertFalse(self.effects.calls)
        self.assertTrue((self.paths.state / 'maintenance').exists())

    def test_status_reports_interrupted_operation_without_mutating_release_records(self):
        old = self.installed()
        self.assertFalse(self.coordinator.status()['maintenance_closed'])
        new = self.staged('second')
        self.effects.interrupt = True
        with self.assertRaises(KeyboardInterrupt):
            self.coordinator.activate(*new, old[0])
        before = {p.name: p.read_bytes() for p in self.paths.state.glob('*.json')}
        result = self.coordinator.status()
        self.assertTrue(result['maintenance_closed'])
        self.assertFalse(result['pointers_match_committed'])
        self.assertEqual(result['pending_operation']['phase'], 'switched')
        self.assertEqual(result['committed_release_id'], old[0])
        self.assertEqual(before, {p.name: p.read_bytes() for p in self.paths.state.glob('*.json')})

    def test_first_install_explicit_migration_and_ordered_service_start(self):
        values = self.staged()
        with self.assertRaises(ReleaseError):
            self.coordinator.activate(*values, 'none')
        self.assertFalse((self.paths.state / 'maintenance').exists())
        result = self.coordinator.activate(*values, 'none', migrate=True)
        self.assertEqual(result['state'], 'committed')
        self.assertEqual(self.effects.actual, schema(1))
        self.assertFalse((self.paths.state / 'pending.json').exists())
        calls = [v[0] for v in self.effects.calls]
        self.assertLess(calls.index('stop'), calls.index('migrate'))
        self.assertLess(calls.index('migrate'), calls.index('start'))
        self.assertEqual(self.coordinator.current()['release_id'], values[0])

    def test_same_schema_switch_and_explicit_application_only_rollback(self):
        old = self.installed()
        new = self.staged('second')
        self.coordinator.activate(*new, old[0])
        self.coordinator.activate(*old, new[0])
        self.assertEqual(self.coordinator.current()['release_id'], old[0])
        self.assertNotIn('migrate', [v[0] for v in self.effects.calls])

    def test_readiness_failure_restores_exact_previous_program_and_configuration(self):
        old = self.installed()
        new = self.staged('second')
        self.effects.fail_health = 1
        with self.assertRaisesRegex(ReleaseError, 'restored'):
            self.coordinator.activate(*new, old[0])
        self.assertEqual(self.coordinator.current()['release_id'], old[0])
        self.assertFalse((self.paths.state / 'maintenance').exists())

    def test_upgraded_schema_failure_never_restarts_incompatible_old_program(self):
        old = self.installed()
        new = self.staged('second', migrations=2)
        self.effects.fail_health = 1
        with self.assertRaisesRegex(ReleaseError, 'incomplete'):
            self.coordinator.activate(*new, old[0], migrate=True)
        pending = json.loads((self.paths.state / 'pending.json').read_text())
        self.assertEqual(pending['phase'], 'forward_repair_required')
        self.assertTrue((self.paths.state / 'maintenance').exists())
        with self.assertRaises(ReleaseError):
            self.coordinator.recover(pending['operation_id'], 'previous')
        self.coordinator.recover(pending['operation_id'], 'forward')
        self.assertEqual(self.coordinator.current()['release_id'], new[0])

    def test_partial_migration_requires_explicit_forward_recovery(self):
        old = self.installed()
        new = self.staged('second', migrations=3)
        self.effects.partial = True
        with self.assertRaises(ReleaseError):
            self.coordinator.activate(*new, old[0], migrate=True)
        pending = json.loads((self.paths.state / 'pending.json').read_text())
        self.assertEqual(self.effects.actual, schema(2))
        with self.assertRaises(ReleaseError):
            self.coordinator.recover(pending['operation_id'], 'forward')
        with self.assertRaises(ReleaseError):
            self.coordinator.recover(pending['operation_id'], 'previous')
        self.effects.partial = False
        self.coordinator.recover(pending['operation_id'], 'forward', migrate=True)
        self.assertEqual(self.effects.actual, schema(3))

    def test_process_interrupt_retains_gate_and_can_restore_previous_exact_release(self):
        old = self.installed()
        new = self.staged('second')
        self.effects.interrupt = True
        with self.assertRaises(KeyboardInterrupt):
            self.coordinator.activate(*new, old[0])
        pending = json.loads((self.paths.state / 'pending.json').read_text())
        self.assertTrue((self.paths.state / 'maintenance').exists())
        with self.assertRaises(ReleaseError):
            self.coordinator.activate(*new, old[0])
        self.coordinator.recover(pending['operation_id'], 'previous')
        self.assertEqual(self.coordinator.current()['release_id'], old[0])

    def test_failed_stop_keeps_gate_and_does_not_start_any_program(self):
        old = self.installed()
        new = self.staged('second')
        self.effects.fail_stop = True
        with self.assertRaises(ReleaseError):
            self.coordinator.activate(*new, old[0])
        self.assertNotIn('start', [v[0] for v in self.effects.calls])
        self.assertTrue((self.paths.state / 'maintenance').exists())

    def test_wrong_current_schema_and_recovery_identity_have_no_service_effects(self):
        old = self.installed()
        new = self.staged('second')
        for expected in ('none', 'r-' + '0' * 24):
            with self.assertRaises(ReleaseError):
                self.coordinator.activate(*new, expected)
        self.effects.actual = schema(2)
        with self.assertRaises(ReleaseError):
            self.coordinator.activate(*new, old[0])
        self.assertNotIn('stop', [v[0] for v in self.effects.calls])

    def test_tampered_artifact_and_changed_configuration_block_recovery(self):
        old = self.installed()
        new = self.staged('second')
        self.effects.interrupt = True
        with self.assertRaises(KeyboardInterrupt):
            self.coordinator.activate(*new, old[0])
        pending = json.loads((self.paths.state / 'pending.json').read_text())
        path = self.paths.config_root / 'configurations' / new[2] / 'api.env'
        path.write_text(path.read_text() + 'CP_CHANGED=true\n')
        with self.assertRaises(ReleaseError):
            self.coordinator.recover(pending['operation_id'], 'forward')
        with self.assertRaises(ReleaseError):
            self.coordinator.recover('0' * 32, 'previous')
        root = self.paths.root / 'releases' / old[0]
        with self.writable_release(root):
            (root / 'web/index.html').write_text('tampered')
            with self.assertRaises(ReleaseError):
                self.coordinator.recover(pending['operation_id'], 'previous')
        self.assertTrue((self.paths.state / 'maintenance').exists())

    def test_audit_events_form_preserved_chains_without_configuration_values(self):
        self.installed()
        previous = None
        for path in sorted((self.paths.state / 'events').glob('*.json')):
            raw = path.read_bytes()
            record = json.loads(raw)
            self.assertEqual(record['previous_sha256'], previous)
            self.assertNotIn(b'postgresql:', raw)
            previous = digest(raw)
        self.assertEqual(record['phase'], 'committed')

    def test_unrelated_pointer_cannot_be_adopted_by_recovery(self):
        old = self.installed()
        new = self.staged('second')
        self.effects.interrupt = True
        with self.assertRaises(KeyboardInterrupt):
            self.coordinator.activate(*new, old[0])
        pending = json.loads((self.paths.state / 'pending.json').read_text())
        unrelated = self.base / 'unrelated'
        unrelated.mkdir()
        self.pointers.replace(self.paths.root / 'current', unrelated)
        self.effects.calls.clear()
        with self.assertRaises(ReleaseError):
            self.coordinator.recover(pending['operation_id'], 'forward')
        self.assertNotIn('stop', [v[0] for v in self.effects.calls])

    def test_configuration_changed_after_prepare_cannot_be_silently_adopted(self):
        old = self.installed()
        new = self.staged('second')
        original_stop = self.effects.stop
        changed = False
        def change_after_stop(names):
            nonlocal changed
            original_stop(names)
            if not changed:
                changed = True
                path = self.paths.config_root / 'configurations' / new[2] / 'api.env'
                path.write_text(path.read_text() + 'CP_CHANGED=true\n')
        self.effects.stop = change_after_stop
        with self.assertRaisesRegex(ReleaseError, 'restored'):
            self.coordinator.activate(*new, old[0])
        self.assertEqual(self.coordinator.current()['configuration_id'], old[2])

    def test_native_file_lock_excludes_a_concurrent_operator(self):
        with self.coordinator.locked():
            with self.assertRaises(OSError):
                with Coordinator(self.paths, self.effects, strict=False, pointers=self.pointers).locked():
                    self.fail('concurrent operator acquired lock')


class ConfigurationTests(Fixture):
    def test_runtime_and_migration_logins_cannot_be_the_same(self):
        root = self.configuration('cfg-valid')
        path = root / 'migration.env'
        path.write_text(path.read_text().replace('migrator@', 'fixture@'))
        with self.assertRaises(ReleaseError):
            self.coordinator.configuration('cfg-valid')

    def test_no_shell_evaluation_or_migration_identity_in_runtime(self):
        root = self.configuration('cfg-valid')
        path = root / 'notification.env'
        path.write_text(path.read_text() + 'CP_MIGRATION_DATABASE_URL=forbidden\n')
        with self.assertRaises(ReleaseError):
            self.coordinator.configuration('cfg-valid')
        path.write_text('CP_ENV=production\nCP_DATABASE_URL="quoted"\n')
        with self.assertRaises(ReleaseError):
            parse_env(path)

    def test_unknown_files_units_duplicate_env_and_mismatched_database_rejected(self):
        root = self.configuration('cfg-valid')
        extra = root / 'extra-private.key'
        extra.write_text('synthetic')
        with self.assertRaises(ReleaseError):
            self.coordinator.configuration('cfg-valid')
        extra.unlink()
        path = root / 'notification.env'
        original = path.read_text()
        path.write_text(original + 'CP_ENV=production\n')
        with self.assertRaises(ReleaseError):
            self.coordinator.configuration('cfg-valid')
        path.write_text(original.replace('/company?', '/other?'))
        with self.assertRaises(ReleaseError):
            self.coordinator.configuration('cfg-valid')
        (root / 'deployment.json').write_bytes(canonical({'format': 1, 'units': ['api', 'shell'], 'hostname': 'case.example.invalid'}))
        with self.assertRaises(ReleaseError):
            self.coordinator.configuration('cfg-valid')

    def test_releasing_to_another_database_is_not_an_application_upgrade(self):
        old = self.installed()
        new = self.staged('second')
        root = self.paths.config_root / 'configurations' / new[2]
        for file in root.glob('*.env'):
            file.write_text(file.read_text().replace('/company?', '/other?'))
        with self.assertRaises(ReleaseError):
            self.coordinator.activate(*new, old[0])
        self.assertNotIn('stop', [v[0] for v in self.effects.calls])

    def test_status_configuration_uses_pid1_credentials_and_separate_roles(self):
        units = [catalog.render(name) for name in catalog.SERVICES]
        self.assertIn('LoadCredential=status-bridge.json:', catalog.render('status-bridge'))
        self.assertIn('--config %d/status-bridge.json', catalog.render('status-bridge'))
        self.assertTrue(all('NoNewPrivileges=true' in text and 'ProtectSystem=strict' in text for text in units))
        self.assertIn('return 503', catalog.nginx_site('case.example.invalid'))
        self.assertIn('proxy_buffering off', catalog.nginx_site('case.example.invalid'))


class NativeCommandBoundaryTests(unittest.TestCase):
    def test_migration_password_is_not_a_process_argument_or_public_error(self):
        effect = NativeEffects()
        calls = []
        responses = iter(['t\n', json.dumps(schema(1))])
        def run(command, env):
            calls.append((command, env))
            return next(responses)
        effect.run = run
        connection = 'postgresql://migrator:synthetic-only-password@db.example.invalid/company?sslmode=verify-full'
        self.assertEqual(effect.schema({'CP_MIGRATION_DATABASE_URL': connection, 'CP_IRRELEVANT': 'no'}), schema(1))
        self.assertTrue(all(connection not in ' '.join(command) and environment['PGDATABASE'] == connection
                            and 'CP_IRRELEVANT' not in environment for command, environment in calls))

    def test_native_process_failures_emit_only_fixed_non_secret_diagnoses(self):
        class Failed:
            returncode, stdout, stderr = 1, b'synthetic private output', b'synthetic password'
        with patch('controller.subprocess.run', return_value=Failed()) as invoked:
            with self.assertRaises(ReleaseError) as caught:
                NativeEffects().run(['/usr/bin/systemctl', 'start', 'fixed.service'])
            self.assertNotIn('synthetic', str(caught.exception))
            self.assertNotIn('CP_MIGRATION_DATABASE_URL', invoked.call_args.kwargs['env'])

    def test_ci_does_not_forward_provider_or_frontend_injection_values(self):
        scripts = Path(__file__).resolve().parents[2] / 'scripts'
        sys.path.insert(0, str(scripts))
        from native_ci_release import child_env_from
        clean = child_env_from({'PATH': '/usr/bin', 'CP_DATABASE_URL': 'private', 'OPENAI_API_KEY': 'private',
                                'VITE_SECRET': 'private', 'TARO_APP_SECRET': 'private', 'AWS_TOKEN': 'private'})
        self.assertEqual(clean, {'PATH': '/usr/bin'})


if __name__ == '__main__':
    unittest.main(verbosity=2)
