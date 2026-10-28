"""Packaging safety checks use isolated native temporary files, not installed packages."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('native_release', Path(__file__).with_name('build_native_runtime_release.py'))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class NativeReleaseTests(unittest.TestCase):
    def test_outside_source_copy_is_refused(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            allowed = root / 'allowed'
            allowed.mkdir()
            source = root / 'private.key'
            source.write_text('synthetic private fixture', encoding='utf-8')
            with self.assertRaises(ValueError):
                release.copy_file(source, root / 'result', allowed)
            self.assertFalse((root / 'result').exists())

    def test_private_caches_native_binaries_and_tests_do_not_enter_package(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            source, out = root / 'source', root / 'release'
            source.mkdir()
            for path in ('package.json', 'index.js', 'LICENSE', 'tests/fixture.js', '.local/database.json', '.env.secret', 'native.exe', 'vendor/codex'):
                file = source / path
                file.parent.mkdir(parents=True, exist_ok=True)
                file.write_text('{}', encoding='utf-8')
            release.copy_tree(source, out, source, package=True)
            self.assertEqual({p.relative_to(out).as_posix() for p in out.rglob('*') if p.is_file()}, {'package.json', 'index.js', 'LICENSE'})

    def test_existing_outputs_and_source_locations_are_not_overwritten(self):
        with tempfile.TemporaryDirectory() as name:
            app = Path(name) / 'app'
            (app / 'node_modules').mkdir(parents=True)
            existing = app / '.local/release'
            existing.mkdir(parents=True)
            (existing / 'keep.txt').write_text('preserve', encoding='utf-8')
            for path in (existing, app / 'skills/new-release', app.parent / 'docs/new-release'):
                with self.assertRaises(ValueError):
                    release.create_release(path, app)
            self.assertEqual((existing / 'keep.txt').read_text(encoding='utf-8'), 'preserve')

    def test_runtime_dependency_resolution_obeys_node_nesting(self):
        with tempfile.TemporaryDirectory() as name:
            app = Path(name) / 'app'
            modules = app / 'node_modules'
            start = modules / 'parent'
            for base in (modules / 'child', start / 'node_modules/child'):
                base.mkdir(parents=True)
                (base / 'package.json').write_text('{}', encoding='utf-8')
            self.assertEqual(release.resolve_package('child', start, modules), start / 'node_modules/child')
            with self.assertRaises(ValueError):
                release.resolve_package('../private', start, modules)


if __name__ == '__main__':
    unittest.main()
