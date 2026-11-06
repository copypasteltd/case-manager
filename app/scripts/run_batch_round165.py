"""Freeze a complete code wave, then execute source-bound checks as one batch.

This records local evidence only. It never changes GAP acceptance, starts a
virtualized runtime, reads deployment secrets, or publishes an application.
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

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
PLAN = ROOT / "docs/development/165-batch-code-plan.json"
EVIDENCE = ROOT / "docs/quality/evidence/batch-round-165"
PRIVATE = APP / ".local/batch-round-165"


def source_names() -> list[str]:
    paths = ["app/backend", "app/web", "app/mobile", "app/packages", "app/agent-runtime", "app/skills", "app/deploy", "docs/skills", "app/scripts", "app/package.json", "app/package-lock.json", "docs/development/165-batch-code-plan.json"]
    out = subprocess.check_output(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", *paths], cwd=ROOT)
    return sorted({p for p in out.decode("utf-8").split("\0") if p and (ROOT / p).is_file()})



def client_source_names() -> list[str]:
    # The full code wave is fixed globally; client checks bind their actual inputs.
    # A later backend-only fixture/SQL fix does not require replaying mobile builds.
    return [n for n in source_names() if not n.startswith(("app/backend/", "app/deploy/"))
            and n != "docs/development/165-batch-code-plan.json"]


def source_digest(names: list[str]) -> str:
    h = hashlib.sha256()
    for name in names:
        h.update(name.encode("utf-8") + b"\0" + (ROOT / name).read_bytes() + b"\0")
    return h.hexdigest()


def digest(names: list[str] | None = None) -> str:
    return source_digest(names or source_names())


def load_plan() -> dict:
    plan = json.loads(PLAN.read_text(encoding="utf-8"))
    scope = json.loads((ROOT / "docs/delivery/04-current-scope.json").read_text(encoding="utf-8"))
    actual = [i["id"] for i in scope["items"] if i["status"] == "active"]
    if [i["id"] for i in plan["packages"]] != actual:
        raise ValueError("The complete active package register must be reviewed in the code wave.")
    if not plan["code_wave_complete"] or any(i["state"] != "implemented" for i in plan["round_tasks"]):
        raise ValueError("Finish every declared round implementation before starting checks.")
    if any(not i["implementation_locations"] for i in plan["packages"]):
        raise ValueError("Every active package needs actual implementation locations.")
    for item in plan["packages"]:
        if any(not (ROOT / p).exists() for p in item["implementation_locations"]):
            raise ValueError("Implementation location missing for " + item["id"])
    return plan


def write_once(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("x", encoding="utf-8", newline="\n") as handle:
        json.dump(value, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


def native_tests(log: Path, selector: str, attempt: str, package: str) -> tuple[int, list[dict]]:
    import contract_body_native_cluster as cluster
    cluster.OWNER = APP / ('.local/batch-round-165-native-' + attempt + '-owner.json')
    cluster.E = EVIDENCE / ('native-' + attempt)
    try:
        env = cluster.test_environment()
        command = [str(APP / ".tools/go/bin/go.exe"), "test", package, "-p=1", "-count=1", "-timeout=30m", "-json"]
        if selector:
            command += ['-run', selector]
        with log.open("xb") as handle:
            code = subprocess.run(command, cwd=APP / "backend", env=env, stdout=handle, stderr=subprocess.STDOUT,
                                  creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0).returncode
    finally:
        if cluster.OWNER.exists():
            cluster.stop()
            stamp = hashlib.sha256(cluster.OWNER.read_bytes()).hexdigest()[:16]
            cluster.OWNER.rename(cluster.OWNER.with_name('batch-round-165-' + attempt + '-closed-' + stamp + '.json'))
    events = []
    for line in log.read_text(encoding="utf-8", errors="replace").splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if event.get("Action") in ("pass", "fail", "skip"):
            events.append({k: event[k] for k in ("Action", "Package", "Test", "Elapsed") if k in event})
    return code, events


def client_checks(attempt: str, selected: str) -> tuple[int, list[dict]]:
    node = APP / '.tools/node-v22.23.4-win-x64/node.exe'
    npm = node.parent / 'node_modules/npm/bin/npm-cli.js'
    env = {**os.environ, 'PATH': str(node.parent) + os.pathsep + os.environ['PATH'], 'CI': 'true',
           'TARO_CHECK_UPDATE': 'false', 'CP_MOBILE_BUILD_PROFILE': 'local', 'CP_MOBILE_API_ORIGIN': 'http://127.0.0.1:18080'}
    commands = [('typecheck', APP, [node, npm, 'run', 'typecheck']),
                ('web-tests', APP, [node, npm, 'run', 'test', '--workspace', '@copypaste/web']),
                ('mobile-tests', APP, [node, npm, 'run', 'test', '--workspace', '@copypaste/mobile']),
                ('mobile-new-contracts', APP, [node, '--test', '.local/mobile-concern-tests/mobile/src/platform/telegram-grants.test.js', '.local/mobile-concern-tests/mobile/src/platform/fixed-delivery-navigation.test.js']),



                *[(name, APP, [node, npm, 'run', command]) for name, command in
                  [('web-build', 'build:web'), ('weapp-build', 'build:weapp'), ('douyin-build', 'build:tt')]],
                ]
    allowed = {name for name, _, _ in commands}
    chosen = set(selected.split(',')) if selected else allowed
    if not chosen <= allowed: raise ValueError('Unknown client check')
    results = []
    for name, cwd, command in commands:
        if name not in chosen: continue
        if name.endswith('-build') or name == 'android-bundle':
            if any(not i['passed'] for i in results):
                results.append({'step':name,'exit_code':None,'passed':False,'executed':False,'reason':'Fix the unified client check failures before building artifacts.'})
                continue
        log = PRIVATE / (name + '-' + attempt + '.log')
        start = time.monotonic()
        with log.open('xb') as handle:
            code = subprocess.run(list(map(str, command)), cwd=cwd, env=env, stdout=handle, stderr=subprocess.STDOUT,
                                  creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0).returncode
        result = {'step': name, 'exit_code': code, 'passed': code == 0, 'elapsed_seconds': round(time.monotonic()-start, 3),
                  'private_log_sha256': hashlib.sha256(log.read_bytes()).hexdigest()}
        results.append(result)
        print(json.dumps(result), flush=True)
    return (0 if all(i['passed'] for i in results) else 1), results


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("phase", choices=("freeze-code", "native-tests", "client-checks"))
    parser.add_argument("--attempt", default="initial")
    parser.add_argument('--run-pattern', default='^Test(Round165SubscriptionReminders|SubscriptionBilling|SubscriptionJobLeases)$')
    parser.add_argument('--native-package', choices=('./...', './internal/httpapi'), default='./internal/httpapi')
    parser.add_argument('--client-steps', default='')
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", args.attempt):
        raise ValueError("Use an immutable, safe attempt name.")
    plan = load_plan()
    names = client_source_names() if args.phase == "client-checks" else source_names()
    before = source_digest(names)
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    PRIVATE.mkdir(parents=True, exist_ok=True)
    target = EVIDENCE / (args.phase + "-" + args.attempt + ".json")
    if target.exists():
        raise ValueError("Preserve earlier attempts; choose a new name.")
    record = {"phase": args.phase, "attempt": args.attempt, "started_at": datetime.now(timezone.utc).isoformat(),
              "source_files": names, "source_file_sha256": {name:hashlib.sha256((ROOT/name).read_bytes()).hexdigest() for name in names}, "source_sha256": before, "reviewed_package_ids": [i["id"] for i in plan["packages"]],
              "code_wave_only": args.phase == "freeze-code", "input_scope": "complete-client-consumers" if args.phase == "client-checks" else "complete-code-wave", "formal_acceptance": False, "real_integrations_executed": False,
              "virtualization_used": False}
    record['runner_sha256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    if args.phase == 'native-tests':
        record['native_test_selector'] = args.run_pattern
        record['native_test_package'] = args.native_package
        record['full_backend_suite_executed'] = not bool(args.run_pattern)
    if args.phase == "freeze-code":
        record.update({"passed": True, "tests_executed": False, "meaning": "All declared round code is present; business acceptance remains open."})
        write_once(target, record)
        import zipfile
        archive=EVIDENCE/('code-wave-'+args.attempt+'.zip')
        with zipfile.ZipFile(archive,'x',zipfile.ZIP_DEFLATED) as saved:
            for name in names: saved.writestr(name,(ROOT/name).read_bytes())
        write_once(EVIDENCE/('code-wave-'+args.attempt+'-archive.json'),{'file':archive.relative_to(ROOT).as_posix(),'sha256':hashlib.sha256(archive.read_bytes()).hexdigest(),'source_sha256':before})
        print(json.dumps({"record": str(target), "packages": len(plan["packages"]), "tests_executed": False}))
        return 0
    start = time.monotonic()
    log = PRIVATE / ("native-" + args.attempt + ".jsonl")
    code, events = client_checks(args.attempt, args.client_steps) if args.phase == 'client-checks' else native_tests(log, args.run_pattern, args.attempt, args.native_package)
    after_names = client_source_names() if args.phase == "client-checks" else source_names()
    after = source_digest(after_names)
    record.update({"exit_code": code, "elapsed_seconds": round(time.monotonic() - start, 3), "test_results": events,
                   "source_after_sha256": after, "source_unchanged": names == after_names and before == after,
                   "private_log_sha256": hashlib.sha256(log.read_bytes()).hexdigest() if log.exists() else None, "tests_executed": True,
                   "passed": code == 0 and before == after and names == after_names})
    write_once(target, record)
    print(json.dumps({"record": str(target), "passed": record["passed"], "result_records": len(events)}))
    return 0 if record["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
