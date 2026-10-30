"""Manual native Linux CI: disposable local PG, full checks, sealed server bundle.

Only the artifacts subdirectory is uploaded. No deployment credentials, SSH,
production activation, virtualization or Android signing are used by this job.
"""
import argparse
import base64
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import signal
import socket
import subprocess
import sys
from urllib.parse import quote

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
PRIVATE = APP / '.local/ci-native/private'
ARTIFACTS = APP / '.local/ci-native/artifacts'
sys.path.insert(0, str(APP / 'deploy/release'))
from bundle import canonical, file_sha, require
from build_production_release import build


def child_env():
    # A build runner must hold no production secrets. These names never reach builds.
    return child_env_from(os.environ)


def native_run(command, name, env=None, cwd=ROOT, stdin=None):
    log = PRIVATE / (name + '.log')
    with log.open('xb') as stream:
        result = subprocess.run(command, cwd=cwd, env={**child_env(), **(env or {})}, input=stdin,
                                stdout=stream, stderr=subprocess.STDOUT)
    require(result.returncode == 0, 'Native CI command failed; private log retained.')
    return {'name': name, 'exit_code': 0, 'raw_log_sha256': file_sha(log)}


def cleanup():
    state = PRIVATE / 'cluster.json'
    if not state.exists():
        return {'cluster_started': False, 'stopped': True}
    value = json.loads(state.read_text(encoding='utf-8'))
    data = Path(value['data'])
    require(data.resolve() == (PRIVATE / 'postgres-data').resolve() and not data.is_symlink(), 'Refusing unrelated PG cleanup path.')
    executable = Path(value['pg_bin']) / 'pg_ctl'
    status = subprocess.run([str(executable), 'status', '-D', str(data)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if status.returncode == 0:
        lines = (data / 'postmaster.pid').read_text().splitlines()
        require(Path(lines[1]).resolve() == data.resolve(), 'PostgreSQL data identity differs.')
        pid = int(lines[0])
        require(Path('/proc/' + str(pid) + '/exe').resolve() == (Path(value['pg_bin']) / 'postgres').resolve(), 'PostgreSQL process identity differs.')
        subprocess.run([str(executable), 'stop', '-D', str(data), '-m', 'fast', '-w', '-t', '60'], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    else:
        require(status.returncode == 3, 'Could not establish test-cluster state.')
    final = subprocess.run([str(executable), 'status', '-D', str(data)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    require(final.returncode == 3, 'Owned test PostgreSQL remains active.')
    receipt = {'cluster_started': True, 'stopped': True, 'production_database_used': False}
    (PRIVATE / 'cleanup.json').write_bytes(canonical(receipt))
    return receipt


def postgres(pg_bin):
    pg_bin = Path(pg_bin).resolve(strict=True)
    require((pg_bin / 'initdb').is_file() and (pg_bin / 'pg_ctl').is_file() and (pg_bin / 'psql').is_file(), 'Native PostgreSQL utilities missing.')
    version = subprocess.check_output([str(pg_bin / 'postgres'), '--version']).decode()
    require(re.search(r'PostgreSQL\) 18\.', version) is not None, 'Native PostgreSQL18 is required for this CI profile.')
    password, app_password = secrets.token_hex(32), secrets.token_hex(32)
    pwfile = PRIVATE / 'init-password'
    pwfile.write_text(password + '\n', encoding='ascii')
    pwfile.chmod(0o600)
    data = PRIVATE / 'postgres-data'
    (PRIVATE / 'cluster.json').write_bytes(canonical({'pg_bin': str(pg_bin), 'data': str(data)}))
    native_run([str(pg_bin / 'initdb'), '-D', str(data), '-U', 'postgres', '--auth-local=scram-sha-256',
                '--auth-host=scram-sha-256', '--pwfile=' + str(pwfile), '--encoding=UTF8', '--no-locale'], 'initdb')
    with socket.socket() as probe:
        probe.bind(('127.0.0.1', 0))
        port = probe.getsockname()[1]
    # TCP loopback only; native Unix sockets disabled so no outside directory is needed.
    native_run([str(pg_bin / 'pg_ctl'), 'start', '-D', str(data), '-w', '-t', '60', '-l', str(PRIVATE / 'postgres.log'),
                '-o', f'-h 127.0.0.1 -p {port} -k "" -c fsync=on -c synchronous_commit=on -c full_page_writes=on'], 'pg-start')
    admin = f'postgresql://postgres:{quote(password)}@127.0.0.1:{port}/postgres?sslmode=disable'
    native_run([str(pg_bin / 'psql'), '-X', '-w', '--set=ON_ERROR_STOP=1'], 'test-role', {'PGDATABASE': admin},
               stdin=("CREATE ROLE copypaste_app LOGIN PASSWORD '" + app_password + "';\n").encode())
    return {'CP_TEST_ADMIN_URL': admin, 'CP_DATABASE_URL': f'postgresql://copypaste_app:{quote(app_password)}@127.0.0.1:{port}/postgres?sslmode=disable',
            'CP_ENV': 'test', 'CP_CURSOR_KEY': secrets.token_hex(32), 'CP_MFA_KEY': base64.b64encode(secrets.token_bytes(32)).decode(),
            'CP_PUBLIC_ORIGIN': 'http://127.0.0.1:8080', 'CP_HTTP_ADDR': '127.0.0.1:8080'}


def run_ci(pg_bin):
    require(sys.platform == 'linux' and os.geteuid() != 0, 'Native CI requires a dedicated non-root Linux runner.')
    require(not subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=all'], cwd=ROOT), 'CI requires a clean committed checkout.')
    require(not (APP / '.local/environment.json').exists() and not PRIVATE.exists() and not ARTIFACTS.exists(), 'Use a fresh runner checkout without development identities.')
    node = subprocess.check_output(['node', '--version']).decode().strip()
    require(re.fullmatch(r'v22\.(?:1[5-9]|[2-9][0-9])\.[0-9]+', node), 'Node22.15 through22.x is required.')
    os.umask(0o077)
    PRIVATE.mkdir(parents=True, mode=0o700)
    ARTIFACTS.mkdir(mode=0o700)
    results, cleanup_receipt = [], None
    try:
        results.append(native_run(['npm', 'ci'], 'npm-ci', cwd=APP))
        results.append(native_run([sys.executable, '-m', 'unittest', 'discover', '-s', 'app/deploy/release', '-p', 'test_native_release.py', '-v'], 'release-tests'))
        env = postgres(pg_bin)
        results.append(native_run(['go', 'test', './...', '-count=1', '-json', '-p', '1', '-parallel', '1', '-timeout', '90m'], 'backend', env, APP / 'backend'))
        records = [json.loads(line) for line in (PRIVATE / 'backend.log').read_text().splitlines() if line.startswith('{')]
        tests = [row for row in records if row.get('Test') and row.get('Action') in ('pass', 'fail', 'skip')]
        require(tests and all(v['Action'] == 'pass' for v in tests), 'Full backend CI must not skip or replace discovered tests.')
        for command in ('typecheck', 'test', 'build:weapp', 'build:tt'):
            results.append(native_run(['npm', 'run', command], command.replace(':', '-'), cwd=APP))
        # Provider keys and test DB identities are absent from artifact builds.
        original_environment = dict(os.environ)
        try:
            os.environ.clear()
            os.environ.update(child_env_from(original_environment))
            report = build(APP / '.local/ci-native/build')
        finally:
            os.environ.clear()
            os.environ.update(original_environment)
        require(not report['source']['working_tree_dirty'], 'Artifact source changed during CI.')
        for name in ('copypaste-linux-amd64.tar.gz', 'manifest.sha256', 'build.json'):
            shutil.copyfile(APP / '.local/ci-native/build' / name, ARTIFACTS / name)
        cleanup_receipt = cleanup()
        receipt = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': results,
                   'backend_result_records': len(tests), 'source_commit': report['source']['base_commit'],
                   'native_linux_ci_executed': True, 'native_pointer_operations_tested': True, 'cleanup': cleanup_receipt,
                   'native_posix_release_permissions_tested': True,
                   'production_deployed': False, 'linux_isolation_accepted': False, 'real_model_invoked': False}
        (ARTIFACTS / 'ci.json').write_bytes(canonical(receipt))
        return receipt
    finally:
        if cleanup_receipt is None:
            cleanup()


def child_env_from(value):
    denied = ('CP_', 'VITE_', 'TARO_APP_', 'OPENAI_', 'AWS_', 'ALIYUN_', 'CLOUDFLARE_', 'TELEGRAM_')
    return {k: v for k, v in value.items() if not k.startswith(denied)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pg-bin', type=Path)
    parser.add_argument('--cleanup', action='store_true')
    args = parser.parse_args()
    require(sys.platform == 'linux', 'This CI entry point runs only on native Linux.')
    def interrupted(*_):
        raise KeyboardInterrupt()
    signal.signal(signal.SIGTERM, interrupted)
    if args.cleanup:
        print(json.dumps(cleanup()))
        return
    pg_bin = args.pg_bin or (Path(shutil.which('initdb')).parent if shutil.which('initdb') else None)
    require(pg_bin is not None, 'Install native PostgreSQL18 and provide --pg-bin.')
    report = run_ci(pg_bin)
    print(json.dumps({'passed': report['passed'], 'native_linux_ci_executed': True, 'production_deployed': False}))


if __name__ == '__main__':
    main()
