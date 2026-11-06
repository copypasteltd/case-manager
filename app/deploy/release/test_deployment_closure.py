"""Exercise the installed tool boundary and all real worker consumers, natively."""
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

import catalog
from controller import Coordinator, Paths
from test_native_release import Fixture

APP = Path(__file__).resolve().parents[2]


class InstalledToolTests(unittest.TestCase):
    def test_standalone_packaged_controller_has_its_dependency_closure(self):
        spec = importlib.util.spec_from_file_location('batch167_packager', APP / 'scripts/build_production_release.py')
        packager = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(packager)
        with tempfile.TemporaryDirectory() as temporary:
            tools = Path(temporary) / 'release-tools'
            packager.copy_release_tools(tools)
            # The child has no repository or ambient PYTHONPATH on its path.
            # Run the actual CLI help, which must import before parsing options.
            program = ('import runpy,sys;sys.path.insert(0,sys.argv[1]);'
                       'sys.argv=[sys.argv[1]+"/controller.py","--help"];'
                       'runpy.run_path(sys.argv[0],run_name="__main__")')
            child = subprocess.run([sys.executable, '-I', '-B', '-c', program, str(tools)],
                                   cwd=temporary, capture_output=True, text=True, timeout=30)
            self.assertEqual(child.returncode, 0, child.stderr)
            self.assertIn('activate', child.stdout)
            self.assertIn('recover', child.stdout)
            # Reproduce the omitted-module boundary: it must really be needed.
            (tools / 'telegram_config.py').unlink()
            missing = subprocess.run([sys.executable, '-I', '-B', '-c', program, str(tools)],
                                     cwd=temporary, capture_output=True, text=True, timeout=30)
            self.assertNotEqual(missing.returncode, 0)
            self.assertIn('telegram_config', missing.stderr)

    def test_every_dispatched_background_worker_has_one_controlled_unit(self):
        # Read dispatch consumers, independent of the catalog's declared count.
        main = (APP / 'backend/cmd/platform/main.go').read_text(encoding='utf-8')
        declared = {value for value in re.findall(r'"(worker-[a-z-]+)"', main) if not value.endswith('-')}
        for mode in ('verify', 'promote', 'replica', 'restore'):
            declared.update(f'worker-{mode}-{size}' for size in ('small', 'large'))
        commands = [entry[1] for entry in catalog.SERVICES.values() if entry[1] and entry[1].startswith('worker-')]
        self.assertEqual(len(commands), len(set(commands)))
        self.assertEqual(set(commands), declared)


class OriginalCleanupReleaseTests(Fixture):
    def test_cleanup_is_configuration_bound_and_uses_a_separate_identity(self):
        root = self.configuration('cleanup')
        descriptor = json.loads((root / 'deployment.json').read_text())
        descriptor['units'].append('original-cleanup')
        (root / 'deployment.json').write_text(json.dumps(descriptor))
        (root / 'original-cleanup.env').write_text(
            'CP_ENV=production\nCP_DATABASE_URL=postgres://retirer:fixture@db.example.invalid/company?sslmode=verify-full\n'
            'CP_ORIGINAL_CLEANUP_PROVIDERS=oss,r2\nCP_R2_CLEANUP_EXCLUSIVE_NAMESPACE=true\n')
        checked, _ = self.coordinator.configuration('cleanup')
        self.assertIn('original-cleanup', checked['units'])
        before = checked['configuration_sha256']
        with (root / 'original-cleanup.env').open('a') as output:
            output.write('CP_ORIGINAL_CLEANUP_OSS_BUCKET=another-fixture\n')
        after, _ = self.coordinator.configuration('cleanup')
        self.assertNotEqual(before, after['configuration_sha256'])
        unit = catalog.render('original-cleanup')
        self.assertIn('User=copypaste-retire', unit)
        self.assertIn('ExecStart=/opt/copypaste/current/bin/platform worker-original-cleanup', unit)
        self.assertIn('EnvironmentFile=/etc/copypaste/active/original-cleanup.env', unit)
        self.assertNotIn('CP_CURSOR_KEY', unit)
        self.assertNotEqual(catalog.SERVICES['original-cleanup'][0], catalog.SERVICES['original-observation'][0])


if __name__ == '__main__':
    unittest.main()
