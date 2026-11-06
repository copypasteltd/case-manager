"""Exercise the complete-wave boundary without running builds or native services."""
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import development_wave as wave


class CompleteWaveBoundary(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.app = self.root / "app"
        self.app.mkdir()
        self.addCleanup(patch.stopall)
        patch.object(wave, "ROOT", self.root).start()
        patch.object(wave, "APP", self.app).start()
        self.source = self.app / "source.go"
        self.source.write_bytes(b"package test\r\n")
        self.plan_path = self.root / "docs/development/168-batch-code-plan.json"
        self.plan_path.parent.mkdir(parents=True)
        scope = self.root / "docs/delivery/04-current-scope.json"
        scope.parent.mkdir()
        scope.write_text(json.dumps({"items": [{"id": "GAP-01", "status": "active"}, {"id": "GAP-02", "status": "active"}, {"id": "GAP-17", "status": "excluded"}]}), encoding="utf-8")
        packages = []
        for identifier in ("GAP-01", "GAP-02"):
            audit = self.root / f"docs/development/168-audit/{identifier}.md"
            audit.parent.mkdir(exist_ok=True)
            audit.write_text("independent source review\n", encoding="utf-8")
            packages.append({"id": identifier, "audit_state": "reviewed", "audit_record": audit.relative_to(self.root).as_posix(), "implementation_locations": ["app/source.go"], "acceptance_state": "open"})
        self.plan = {"increment": 168, "packages": packages, "code_wave_complete": True, "code_tasks": [{"id": "C01", "state": "implemented"}]}
        self.save_plan()
        patch.object(wave.Wave, "sources", return_value=["app/source.go", self.plan_path.relative_to(self.root).as_posix()]).start()
        self.wave = wave.Wave(168)

    def save_plan(self):
        self.plan_path.write_text(json.dumps(self.plan) + "\n", encoding="utf-8")

    def freeze(self):
        with contextlib.redirect_stdout(io.StringIO()):
            self.wave.freeze("initial")

    def test_last_numbered_review_blocks_freeze(self):
        self.plan["packages"][-1]["audit_state"] = "pending"
        self.save_plan()
        with self.assertRaisesRegex(ValueError, "source review"):
            self.freeze()
        self.assertFalse(self.wave.evidence.exists())

    def test_last_code_task_blocks_all_checks(self):
        self.plan["code_tasks"].append({"id": "C02", "state": "pending"})
        self.save_plan()
        with patch.object(wave.subprocess, "run") as child:
            with self.assertRaisesRegex(ValueError, "Finish every"):
                self.wave.execute("clients", "initial", "typecheck")
        child.assert_not_called()
        self.assertFalse(self.wave.private.exists())

    def test_missing_active_number_rejected_even_when_other_reviews_are_complete(self):
        self.plan["packages"].pop()
        self.save_plan()
        with self.assertRaisesRegex(ValueError, "entire current numbered"):
            self.freeze()

    def test_archive_preserves_exact_bytes_and_never_overwrites(self):
        self.freeze()
        target = self.wave.evidence / "freeze-code-initial.json"
        archive = self.wave.evidence / "code-wave-initial.zip"
        before = archive.read_bytes()
        record = json.loads(target.read_text(encoding="utf-8"))
        with zipfile.ZipFile(archive) as saved:
            self.assertEqual(saved.read("app/source.go"), b"package test\r\n")
        self.assertFalse(record["tests_executed"])
        self.assertEqual(record["reviewed_package_ids"], ["GAP-01", "GAP-02"])
        self.assertEqual(wave.sha(archive), record["archive"]["sha256"])
        with self.assertRaisesRegex(ValueError, "Preserve every"):
            self.freeze()
        self.assertEqual(before, archive.read_bytes())

    def test_line_ending_change_after_freeze_rejected_before_process_start(self):
        self.freeze()
        self.source.write_bytes(b"package test\n")
        with patch.object(wave.subprocess, "run") as child:
            with self.assertRaisesRegex(ValueError, "complete current code wave"):
                self.wave.execute("clients", "initial", "typecheck")
        child.assert_not_called()

    def test_local_batch_cannot_turn_formal_acceptance_into_pass(self):
        self.plan["packages"][0]["acceptance_state"] = "closed"
        self.save_plan()
        with self.assertRaisesRegex(ValueError, "formal acceptance"):
            self.freeze()


if __name__ == "__main__":
    unittest.main()
