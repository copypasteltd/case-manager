"""Freeze a complete numbered code wave, then run immutable native check batches.

The increment is data, rather than another copied runner. No production secrets,
virtualization, devices, Android rebuild or external integration is activated.
"""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
RUNNER = Path(__file__).relative_to(ROOT).as_posix()


def sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write_once(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("x", encoding="utf-8", newline="\n") as handle:
        json.dump(value, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


class Wave:
    def __init__(self, increment: int):
        if increment < 168:
            raise ValueError("Historical runners and evidence remain unchanged.")
        self.increment = increment
        self.plan_path = ROOT / f"docs/development/{increment}-batch-code-plan.json"
        self.evidence = ROOT / f"docs/quality/evidence/batch-round-{increment}"
        self.private = APP / f".local/batch-round-{increment}"

    def plan(self) -> dict:
        plan = json.loads(self.plan_path.read_text(encoding="utf-8"))
        scope = json.loads((ROOT / "docs/delivery/04-current-scope.json").read_text(encoding="utf-8"))
        active = [row["id"] for row in scope["items"] if row["status"] == "active"]
        packages = plan["packages"]
        if plan["increment"] != self.increment or [row["id"] for row in packages] != active:
            raise ValueError("Review the entire current numbered register before checks.")
        if not plan["code_wave_complete"] or any(row["state"] != "implemented" for row in plan["code_tasks"]):
            raise ValueError("Finish every declared code/consumer/fixture/tool task first.")
        for row in packages:
            if row["audit_state"] != "reviewed" or not (ROOT / row["audit_record"]).is_file():
                raise ValueError("An independent numbered source review is missing: " + row["id"])
            if not row["implementation_locations"] or any(not (ROOT / name).exists() for name in row["implementation_locations"]):
                raise ValueError("A numbered implementation location is missing: " + row["id"])
            if row["acceptance_state"] != "open":
                raise ValueError("Local batches must not silently close formal acceptance.")
        return plan

    def sources(self) -> list[str]:
        roots = ["app/backend", "app/web", "app/mobile", "app/packages", "app/agent-runtime", "app/skills", "app/deploy", "app/scripts", "docs/skills", "app/package.json", "app/package-lock.json", self.plan_path.relative_to(ROOT).as_posix()]
        out = subprocess.check_output(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", *roots], cwd=ROOT)
        return sorted({name for name in out.decode("utf-8").split("\0") if name and (ROOT / name).is_file()})

    def consumers(self, phase: str, step: str = "") -> list[str]:
        names = self.sources()
        if phase == "freeze":
            return names
        if phase == "native":
            prefixes = ("app/backend/",)
            exact = {RUNNER, "app/scripts/contract_body_native_cluster.py"}
        elif phase == "clients":
            prefixes = ("app/packages/", "app/web/") if step in {"web-tests", "web-build"} else ("app/packages/", "app/mobile/") if step != "typecheck" else ("app/packages/", "app/web/", "app/mobile/")
            exact = {RUNNER, "app/package.json", "app/package-lock.json"}
        elif phase == "runtime":
            prefixes = ("app/agent-runtime/", "app/packages/", "app/skills/", "docs/skills/")
            exact = {RUNNER, "app/package.json", "app/package-lock.json"}
        elif phase == "release":
            prefixes = ("app/deploy/release/",)
            exact = {RUNNER, "app/scripts/build_production_release.py", "app/scripts/test_development_wave.py"}
        else:
            raise ValueError("Unknown consumer phase")
        return [name for name in names if name.startswith(prefixes) or name in exact]

    @staticmethod
    def binding(names: list[str]) -> dict[str, str]:
        return {name: sha(ROOT / name) for name in names}

    @staticmethod
    def digest(binding: dict[str, str]) -> str:
        value = hashlib.sha256()
        for name, file_hash in sorted(binding.items()):
            value.update(name.encode("utf-8") + b"\0" + file_hash.encode("ascii") + b"\0")
        return value.hexdigest()

    def freeze(self, attempt: str) -> None:
        plan = self.plan()
        binding = self.binding(self.sources())
        target = self.evidence / f"freeze-code-{attempt}.json"
        archive = self.evidence / f"code-wave-{attempt}.zip"
        self.evidence.mkdir(parents=True, exist_ok=True)
        if target.exists() or archive.exists():
            raise ValueError("Preserve every earlier source wave; use a new attempt.")
        with zipfile.ZipFile(archive, "x", zipfile.ZIP_DEFLATED) as saved:
            for name in binding:
                data = (ROOT / name).read_bytes()
                if hashlib.sha256(data).hexdigest() != binding[name]:
                    raise ValueError("Source changed while preserving the code wave.")
                saved.writestr(name, data)
        if binding != self.binding(self.sources()):
            raise ValueError("Source changed while preserving the code wave.")
        write_once(target, {"passed": True, "increment": self.increment, "attempt": attempt, "started_at": datetime.now(timezone.utc).isoformat(), "code_wave_complete": True, "tests_executed": False, "reviewed_package_ids": [row["id"] for row in plan["packages"]], "source_file_sha256": binding, "source_sha256": self.digest(binding), "archive": {"file": archive.relative_to(ROOT).as_posix(), "sha256": sha(archive)}, "runner_sha256": sha(Path(__file__)), "formal_acceptance": False})
        print(json.dumps({"phase": "freeze", "packages": len(plan["packages"]), "source_files": len(binding), "tests_executed": False}))

    def execute(self, phase: str, attempt: str, selected: str) -> int:
        plan = self.plan()
        frozen = json.loads((self.evidence / f"freeze-code-{attempt}.json").read_text(encoding="utf-8"))
        if not frozen["passed"] or frozen["tests_executed"] or frozen["source_file_sha256"] != self.binding(self.sources()):
            raise ValueError("Preserve the complete current code wave before any checks.")
        target = self.evidence / f"{phase}-{attempt}.json"
        if target.exists():
            raise ValueError("Preserve every earlier execution; use a new attempt.")
        self.private.mkdir(parents=True, exist_ok=True)
        node = APP / ".tools/node-v22.23.4-win-x64/node.exe"
        npm = node.parent / "node_modules/npm/bin/npm-cli.js"
        environment = {**os.environ, "PATH": str(node.parent) + os.pathsep + os.environ["PATH"], "CI": "true", "TARO_CHECK_UPDATE": "false", "CP_MOBILE_BUILD_PROFILE": "local", "CP_MOBILE_API_ORIGIN": "http://127.0.0.1:18080"}
        commands = {
            "clients": [("typecheck", [node, npm, "run", "typecheck"]), ("web-tests", [node, npm, "run", "test", "--workspace", "@copypaste/web"]), ("mobile-tests", [node, npm, "run", "test", "--workspace", "@copypaste/mobile"]), ("mobile-concern-extra", [node, "--test", ".local/mobile-concern-tests/mobile/src/platform/telegram-grants.test.js", ".local/mobile-concern-tests/mobile/src/platform/fixed-delivery-navigation.test.js"]), ("web-build", [node, npm, "run", "build:web"]), ("weapp-build", [node, npm, "run", "build:weapp"]), ("douyin-build", [node, npm, "run", "build:tt"])],
            "runtime": [("runtime-tests", [node, npm, "run", "test", "--workspace", "@copypaste/agent-runtime"])],
            "release": [("wave-tool-tests", [sys.executable, "-B", "-m", "unittest", "discover", "-s", "scripts", "-p", "test_development_wave.py", "-v"]), ("release-tests", [sys.executable, "-B", "-m", "unittest", "discover", "-s", "deploy/release", "-p", "test*.py", "-v"])],
        }
        events = []
        if phase == "native":
            import contract_body_native_cluster as cluster
            selector = plan["validation"]["native_selector"]
            if not selector:
                raise ValueError("Declare the selected native business cases explicitly.")
            commands[phase] = [("native-tests", [APP / ".tools/go/bin/go.exe", "test", *plan["validation"].get("native_packages", ["./internal/httpapi"]), "-p=1", "-count=1", "-timeout=30m", "-json", "-run", selector])]
            cluster.OWNER = self.private / f"native-{attempt}-owner.json"
            cluster.E = self.evidence / f"native-{attempt}"
            try:
                environment = cluster.test_environment()
            except BaseException:
                if cluster.OWNER.exists():
                    cluster.stop()
                raise
        chosen = set(selected.split(",")) if selected else {name for name, _ in commands[phase]}
        allowed = {name for name, _ in commands[phase]}
        if not chosen or not chosen <= allowed:
            if phase == "native" and cluster.OWNER.exists():
                cluster.stop()
            raise ValueError("Choose known, explicit check steps.")
        try:
            for step, command in commands[phase]:
                if step not in chosen:
                    continue
                names = self.consumers(phase, step)
                before = self.binding(names)
                log = self.private / f"{step}-{attempt}.log"
                started = time.monotonic()
                with log.open("xb") as handle:
                    result = subprocess.run(list(map(str, command)), cwd=APP / "backend" if phase == "native" else APP, env=environment, stdout=handle, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
                record = {"step": step, "exit_code": result.returncode, "passed": result.returncode == 0 and names == self.consumers(phase, step) and before == self.binding(names), "elapsed_seconds": round(time.monotonic() - started, 3), "source_file_sha256": before, "private_log_sha256": sha(log)}
                if phase == "native":
                    rows = [json.loads(line) for line in log.read_text(encoding="utf-8").splitlines() if line.startswith("{")]
                    records = [row for row in rows if row.get("Test") and row.get("Action") in {"pass", "fail", "skip"}]
                    record["test_results"] = records
                    record["passed"] = record["passed"] and bool(records) and all(row["Action"] == "pass" for row in records)
                events.append(record)
                print(json.dumps({key: value for key, value in record.items() if key not in {"source_file_sha256", "test_results"}}), flush=True)
        finally:
            if phase == "native" and cluster.OWNER.exists():
                cluster.stop()
        same = frozen["source_file_sha256"] == self.binding(self.sources())
        report = {"passed": bool(events) and same and all(row["passed"] for row in events), "phase": phase, "attempt": attempt, "increment": self.increment, "source_unchanged": same, "source_sha256": frozen["source_sha256"], "freeze_record_sha256": sha(self.evidence / f"freeze-code-{attempt}.json"), "steps": events, "tests_executed": True, "virtualization_used": False, "android_apk_built": False, "formal_acceptance": False, "real_integrations_executed": False}
        write_once(target, report)
        return 0 if report["passed"] else 1


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--increment", required=True, type=int)
    parser.add_argument("phase", choices=("freeze", "native", "clients", "runtime", "release"))
    parser.add_argument("--attempt", default="initial")
    parser.add_argument("--steps", default="")
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", args.attempt):
        raise ValueError("Use a safe immutable attempt name.")
    wave = Wave(args.increment)
    if args.phase == "freeze":
        wave.freeze(args.attempt)
        return 0
    return wave.execute(args.phase, args.attempt, args.steps)


if __name__ == "__main__":
    raise SystemExit(main())
