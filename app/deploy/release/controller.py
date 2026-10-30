"""Native single-host release coordinator. CLI writes require Linux root.

Portable tests inject process effects; they do not execute/emulate systemd/Linux.
The database is never rolled back. Interrupted operations retain a closed gate.
"""
import argparse
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
sys.dont_write_bytecode = True
import tarfile
import time
from urllib.parse import urlsplit, parse_qs, unquote
from urllib.request import build_opener, ProxyHandler, HTTPRedirectHandler
import uuid

from bundle import ReleaseError, canonical, digest, extract_archive, file_sha, read_json, require, verify_tree
from catalog import SERVICES, nginx_site, unit


def release_id(value):
    require(isinstance(value, str) and re.fullmatch(r'r-[a-f0-9]{24}', value), 'Invalid release identity.')
    return value


def configuration_id(value):
    require(isinstance(value, str) and re.fullmatch(r'[a-z][a-z0-9-]{2,63}', value), 'Invalid configuration identity.')
    return value


def secure(path, private=False):
    path = Path(path)
    require(path.is_absolute(), 'Absolute operator path required.')
    for part in [*reversed(path.parents), path]:
        s = part.lstat()
        require(not stat.S_ISLNK(s.st_mode) and s.st_uid == 0 and not s.st_mode & 0o022,
                'Operator path must be root-owned, non-writable and contain no symlinks.')
    if private:
        require(not path.stat().st_mode & 0o077, 'Private configuration permissions required.')


def sync_directory(path):
    if os.name != 'nt':
        descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)


def atomic_json(path, value):
    temporary = path.with_name('.' + path.name + '-' + uuid.uuid4().hex)
    with temporary.open('xb') as stream:
        os.chmod(temporary, 0o600)
        stream.write(canonical(value))
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)
    sync_directory(path.parent)


@dataclass
class Paths:
    root: Path = Path('/opt/copypaste')
    config_root: Path = Path('/etc/copypaste')
    state: Path = Path('/var/lib/copypaste-release')
    units: Path = Path('/etc/systemd/system')
    nginx: Path = Path('/etc/nginx/conf.d/copypaste-release.conf')


class NativePointers:
    def get(self, pointer):
        if not pointer.exists() and not pointer.is_symlink():
            return None
        require(pointer.is_symlink(), 'Unmanaged current pointer refused.')
        return pointer.resolve(strict=True)

    def replace(self, pointer, target):
        temporary = pointer.with_name('.pointer-' + uuid.uuid4().hex)
        temporary.symlink_to(target, target_is_directory=True)
        os.replace(temporary, pointer)
        sync_directory(pointer.parent)


def database_identity(connection):
    parsed = urlsplit(connection)
    query = parse_qs(parsed.query, keep_blank_values=True)
    allowed = {'sslmode', 'sslrootcert', 'sslcert', 'sslkey', 'ssl_min_protocol_version',
               'ssl_max_protocol_version', 'application_name', 'connect_timeout'}
    host = unquote(parsed.hostname or '').lower()
    require(parsed.scheme in ('postgres', 'postgresql') and parsed.hostname and parsed.path not in ('', '/')
            and re.fullmatch(r'[a-z0-9][a-z0-9.:-]{0,252}', host) and not parsed.fragment
            and set(query) <= allowed and all(len(values) == 1 and values[0] for values in query.values())
            and query.get('sslmode') == ['verify-full'], 'Production database identities require verified TLS and no address/login overrides.')
    return digest(canonical({'host': host, 'port': parsed.port or 5432, 'database': unquote(parsed.path[1:])}))


def parse_env(path):
    result = {}
    for line in path.read_text(encoding='utf-8').splitlines():
        if not line or line.startswith('#'):
            continue
        key, separator, value = line.partition('=')
        require(separator and re.fullmatch(r'(?:CP_[A-Z0-9_]+|PGSSLROOTCERT|PGSSLCERT|PGSSLKEY|SSL_CERT_FILE|TZ)', key)
                and key not in result and value and value == value.strip()
                and not any(c in value for c in '\x00\r\n\"\'`\\'), 'Use unique, plain KEY=value environment entries.')
        result[key] = value
    return result


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


class NativeEffects:
    def run(self, command, env=None, timeout=60):
        # Never inherit a deployment identity, provider token, proxy or loader variable.
        clean = {'PATH': '/usr/bin:/bin', 'LANG': 'C.UTF-8', 'HOME': '/nonexistent'}
        result = subprocess.run(command, env={**clean, **(env or {})}, stdin=subprocess.DEVNULL,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout)
        require(result.returncode == 0 and len(result.stdout) < 8 << 20, 'Native release command failed; inspect restricted service diagnostics.')
        return result.stdout.decode('utf-8')

    def schema(self, environment):
        connection = environment['CP_MIGRATION_DATABASE_URL']
        parsed = urlsplit(connection)
        require(parsed.scheme in ('postgres', 'postgresql') and parsed.hostname
                and parse_qs(parsed.query).get('sslmode') == ['verify-full'], 'Production migration identity requires verified TLS.')
        env = {k: v for k, v in environment.items() if k.startswith('PGSSL')}
        env['PGDATABASE'] = connection  # not a command argument or log value
        base = ['/usr/bin/psql', '-X', '-w', '-qAt', '--set=ON_ERROR_STOP=1', '-c']
        present = self.run([*base, "SELECT to_regclass('public.schema_migrations') IS NOT NULL"], env).strip()
        require(present in ('t', 'f'), 'Cannot identify database migration state.')
        if present == 'f':
            return []
        raw = self.run([*base, "SELECT COALESCE(json_agg(v),'[]'::json) FROM (SELECT name,sha256 FROM public.schema_migrations ORDER BY name) v"], env)
        value = read_json(raw)
        require(isinstance(value, list) and len(value) <= 1000, 'Invalid database schema response.')
        return value

    def stop(self, names):
        for name in reversed(names):
            self.run(['/usr/bin/systemctl', 'stop', unit(name)], timeout=90)

    def start(self, names):
        for name in names:
            self.run(['/usr/bin/systemctl', 'start', unit(name)], timeout=90)

    def preflight(self, root, configuration, paths):
        for name in configuration['units']:
            installed = paths.units / unit(name)
            secure(installed)
            require(file_sha(installed) == file_sha(root / 'systemd' / unit(name)), 'Installed unit differs from reviewed package unit.')
        secure(paths.nginx)
        require(paths.nginx.read_text(encoding='utf-8') == nginx_site(configuration['hostname']), 'Installed TLS/maintenance site differs from the reviewed site.')
        self.run(['/usr/sbin/nginx', '-t'])

    def migrate(self, root, environment):
        self.run([str(root / 'bin/platform'), 'migrate'], environment, timeout=1800)

    def healthy(self, names):
        opener = build_opener(ProxyHandler({}), NoRedirect())
        for _ in range(30):
            try:
                # is-active across multiple units succeeds if ANY unit is active; check each.
                for name in names:
                    self.run(['/usr/bin/systemctl', 'is-active', '--quiet', unit(name)])
                with opener.open('http://127.0.0.1:8080/health/ready', timeout=2) as response:
                    require(response.status == 200 and read_json(response.read(4096)) == {'status': 'ready'}, 'API is not ready.')
                return
            except (ReleaseError, OSError, ValueError):
                time.sleep(1)
        raise ReleaseError('Candidate did not become ready; maintenance remains closed unless exact rollback succeeds.')


class Coordinator:
    def __init__(self, paths=None, effects=None, strict=True, pointers=None):
        self.p = paths or Paths()
        self.effects = effects or NativeEffects()
        self.strict = strict
        self.pointers = pointers or NativePointers()

    def check_path(self, path, private=False):
        if self.strict:
            secure(path, private)

    @contextmanager
    def locked(self):
        for path in (self.p.root, self.p.config_root, self.p.state, self.p.units):
            self.check_path(path)
        require(not self.strict or self.p.state.stat().st_mode & 0o005 == 0o005
                and self.p.root.stat().st_mode & 0o005 == 0o005, 'Maintenance and public asset ancestors require Nginx traversal.')
        with (self.p.state / 'operation.lock').open('a+b') as stream:
            if os.name == 'nt':
                import msvcrt
                stream.seek(0)
                if not stream.read(1):
                    stream.write(b'0')
                    stream.flush()
                stream.seek(0)
                msvcrt.locking(stream.fileno(), msvcrt.LK_NBLCK, 1)
            else:
                import fcntl
                fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
            try:
                yield
            finally:
                if os.name == 'nt':
                    stream.seek(0)
                    msvcrt.locking(stream.fileno(), msvcrt.LK_UNLCK, 1)

    def configuration(self, name):
        root = self.p.config_root / 'configurations' / configuration_id(name)
        self.check_path(root, True)
        files = sorted(root.iterdir())
        require(0 < len(files) <= 30 and all(f.is_file() and not f.is_symlink() and f.stat().st_size <= 1 << 20 for f in files), 'Invalid configuration directory.')
        for file in files:
            self.check_path(file, True)
        value = read_json((root / 'deployment.json').read_bytes())
        require(isinstance(value, dict) and set(value) == {'format', 'units', 'hostname'} and type(value['format']) is int and value['format'] == 1
                and isinstance(value['units'], list) and 'api' in value['units']
                and len(value['units']) == len(set(value['units'])) and set(value['units']) <= SERVICES.keys(), 'Invalid enabled-service set.')
        require(isinstance(value['hostname'], str) and len(value['hostname']) <= 253
                and re.fullmatch(r'(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}', value['hostname']), 'One explicit DNS hostname is required.')
        require(not set(value['units']) & {'agent', 'agent-reconcile'} or 'agent-native' in value['units'], 'Agent worker requires its broker.')
        ordered = [n for n in SERVICES if n in value['units']]
        required = {'deployment.json', 'migration.env'}
        database_ids, runtime_users = set(), set()
        for name in ordered:
            env_file = SERVICES[name][2]
            if env_file:
                required.add(env_file)
                env = parse_env(root / env_file)
                require(env.get('CP_ENV') == 'production' and 'CP_MIGRATION_DATABASE_URL' not in env, 'Runtime identities must be production and exclude migration credentials.')
                if name == 'api':
                    require(env.get('CP_HTTP_ADDR') == '127.0.0.1:8080' and env.get('CP_PUBLIC_ORIGIN') == 'https://' + value['hostname'], 'API must bind loopback and its exact public HTTPS origin.')
                else:
                    require(not {'CP_MFA_KEY', 'CP_CURSOR_KEY'} & env.keys(), 'Worker must not receive browser security keys.')
                database_ids.add(database_identity(env.get('CP_DATABASE_URL', '')))
                runtime_user = unquote(urlsplit(env['CP_DATABASE_URL']).username or '')
                require(runtime_user and runtime_user not in ('postgres', 'root'), 'Runtime must use a dedicated non-admin database login.')
                runtime_users.add(runtime_user)
            else:
                required.add(name + '.json')
        require(required == {p.name for p in files}, 'Configuration contains missing or unapproved files.')
        migration = parse_env(root / 'migration.env')
        require('CP_MIGRATION_DATABASE_URL' in migration and set(migration) <= {'CP_MIGRATION_DATABASE_URL', 'PGSSLROOTCERT', 'PGSSLCERT', 'PGSSLKEY'}, 'Dedicated migration environment required.')
        database_ids.add(database_identity(migration['CP_MIGRATION_DATABASE_URL']))
        require(len(database_ids) == 1, 'Runtime and migration identities must address the same database.')
        migration_user = unquote(urlsplit(migration['CP_MIGRATION_DATABASE_URL']).username or '')
        require(migration_user and migration_user not in runtime_users, 'Migration and runtime database logins must be separate.')
        return {'configuration_id': root.name, 'configuration_sha256': digest(b''.join(f.name.encode() + b'\0' + f.read_bytes() + b'\0' for f in files)),
                'hostname': value['hostname'], 'database_identity_sha256': next(iter(database_ids)), 'units': ordered}, migration

    def staged(self, identity, expected):
        root = self.p.root / 'releases' / release_id(identity)
        self.check_path(root)
        require(identity == 'r-' + expected[:24], 'Release identity does not match pinned manifest.')
        value = verify_tree(root, expected)
        if self.strict:
            require(stat.S_IMODE(root.stat().st_mode) == 0o555, 'Release root must remain frozen.')
            modes = {v['path']: v['mode'] for v in value['files']}
            modes['release.json'] = 0o444
            for path in root.rglob('*'):
                self.check_path(path)
                expected_mode = 0o555 if path.is_dir() else modes[path.relative_to(root).as_posix()]
                require(stat.S_IMODE(path.stat().st_mode) == expected_mode, 'Staged release permissions differ from the frozen inventory.')
        return root, value

    def stage(self, archive, expected):
        require(not (self.p.state / 'pending.json').exists(), 'Recover the pending operation before staging.')
        releases = self.p.root / 'releases'
        self.public_directory(releases)
        identity = 'r-' + expected[:24]
        destination = releases / release_id(identity)
        require(not destination.exists(), 'Release identity already staged; verify it instead of overwriting.')
        temporary = releases / ('.incoming-' + uuid.uuid4().hex)
        value = extract_archive(archive, temporary, expected)
        # Keep content-addressed Web assets for already-open clients across switches.
        assets = self.p.root / 'assets'
        self.public_directory(assets)
        for row in value['files']:
            if not row['path'].startswith('web/assets/'):
                continue
            relative = row['path'].removeprefix('web/assets/')
            require('/' not in relative, 'Only flat Vite assets are supported.')
            target = assets / relative
            if target.exists() or target.is_symlink():
                self.check_path(target)
                require(target.is_file() and file_sha(target) == row['sha256'], 'Existing static asset collision.')
            else:
                with target.open('xb') as stream:
                    stream.write((temporary / row['path']).read_bytes())
                    stream.flush()
                    os.fsync(stream.fileno())
                target.chmod(0o444)
        sync_directory(assets)
        for row in value['files']:
            (temporary / row['path']).chmod(row['mode'])
        (temporary / 'release.json').chmod(0o444)
        for path in sorted(temporary.rglob('*'), reverse=True):
            if path.is_dir():
                path.chmod(0o555)
        temporary.chmod(0o555)
        temporary.rename(destination)
        sync_directory(releases)
        return {'release_id': identity, 'manifest_sha256': expected, 'files': len(value['files'])}

    def public_directory(self, path):
        created = not path.exists()
        if created:
            path.mkdir(mode=0o700)
        self.check_path(path)
        require(path.is_dir(), 'Public release ancestor must be a directory.')
        if created:
            # CLI deliberately uses umask077; service-readable ancestors need an explicit mode.
            path.chmod(0o755)
            sync_directory(path.parent)
        require(not self.strict or stat.S_IMODE(path.stat().st_mode) == 0o755,
                'Public release and asset ancestors require explicit0755 permissions.')

    def current(self):
        path = self.p.state / 'current.json'
        current = read_json(path.read_bytes()) if path.exists() else None
        pointers = [(self.p.root / 'current', 'release_id', self.p.root / 'releases'),
                    (self.p.config_root / 'active', 'configuration_id', self.p.config_root / 'configurations')]
        for pointer, key, base in pointers:
            if current:
                (release_id if key == 'release_id' else configuration_id)(current[key])
                require(self.pointers.get(pointer) == base / current[key], 'Managed pointers and committed state differ; recover first.')
            else:
                require(self.pointers.get(pointer) is None, 'Unmanaged current pointer refused.')
        return current

    def candidate(self, identity, expected, configuration):
        root, value = self.staged(identity, expected)
        config, environment = self.configuration(configuration)
        record = {'release_id': identity, 'manifest_sha256': expected, **config, 'schema': value['schema']}
        if 'agent-native' in config['units']:
            broker = read_json((self.p.config_root / 'configurations' / configuration / 'agent-native.json').read_bytes())
            runtime = read_json((root / 'metadata/runtime.json').read_bytes())
            require(broker.get('release') == str(root / 'runtime') and broker.get('release_sha256') == runtime['release_sha256']
                    and broker.get('broker') == {'path': str(root / 'bin/agent-native'), 'sha256': file_sha(root / 'bin/agent-native')}
                    and broker.get('entry') == str(root / 'runtime/app/agent-runtime/dist/worker.js'), 'Broker configuration must pin this exact runtime and executable.')
            self.effects.run([str(root / 'bin/agent-native'), 'check', '--config', str(self.p.config_root / 'configurations' / configuration / 'agent-native.json')])
        self.effects.preflight(root, config, self.p)
        return record, environment

    def event(self, pending, phase):
        events = self.p.state / 'events'
        events.mkdir(mode=0o700, exist_ok=True)
        self.check_path(events, True)
        index = pending.get('event_count', 0) + 1
        require(1 <= index <= 9999, 'Operation journal requires explicit operator review before more events.')
        value = {'operation_id': pending['operation_id'], 'phase': phase, 'sequence': index,
                 'at': datetime.now(timezone.utc).isoformat(), 'previous_sha256': pending.get('event_sha256'),
                 'old': pending['old'], 'target': pending['target']}
        raw = canonical(value)
        path = events / (pending['operation_id'] + '-' + str(index).zfill(4) + '.json')
        temporary = events / ('.event-' + uuid.uuid4().hex)
        with temporary.open('xb') as stream:
            os.chmod(temporary, 0o600)
            stream.write(raw)
            stream.flush()
            os.fsync(stream.fileno())
        require(not path.exists() and not path.is_symlink(), 'An immutable operation event already exists.')
        os.rename(temporary, path)
        sync_directory(events)
        pending.update(event_count=index, event_sha256=digest(raw), phase=phase)
        atomic_json(self.p.state / 'pending.json', pending)

    def pending(self):
        path = self.p.state / 'pending.json'
        if not path.exists():
            return None
        self.check_path(path, True)
        require(path.stat().st_size <= 2 << 20, 'Pending operation record is too large.')
        value = read_json(path.read_bytes())
        operation = value['operation_id']
        require(isinstance(operation, str) and re.fullmatch(r'[a-f0-9]{32}', operation), 'Invalid pending operation identity.')
        files = sorted((self.p.state / 'events').glob(operation + '-*.json'))
        count = value['event_count']
        require(type(count) is int and 1 <= count <= len(files) <= 9999, 'Pending operation journal is incomplete.')
        previous = None
        for sequence, event in enumerate(files, 1):
            self.check_path(event, True)
            require(event.name == operation + '-' + str(sequence).zfill(4) + '.json'
                    and event.is_file() and event.stat().st_size <= 2 << 20, 'Invalid operation event member.')
            raw = event.read_bytes()
            row = read_json(raw)
            require(isinstance(row, dict) and set(row) == {'operation_id', 'phase', 'sequence', 'at', 'previous_sha256', 'old', 'target'}
                    and row['operation_id'] == operation and type(row['sequence']) is int and row['sequence'] == sequence
                    and row['previous_sha256'] == previous, 'Operation event chain differs from its durable history.')
            previous = digest(raw)
            if sequence == count:
                require(previous == value['event_sha256'] and all(row[key] == value[key] for key in ('phase', 'old', 'target')),
                        'Pending operation differs from its journal anchor.')
        # An event rename may survive a process interruption before pending.json.
        # Adopt only the complete, anchored tail; status never writes repairs.
        return {**value, 'event_count': len(files), 'event_sha256': previous,
                **{key: row[key] for key in ('phase', 'old', 'target')}}

    def switch(self, record):
        for pointer, target in [(self.p.root / 'current', self.p.root / 'releases' / release_id(record['release_id'])),
                                (self.p.config_root / 'active', self.p.config_root / 'configurations' / configuration_id(record['configuration_id']))]:
            self.pointers.replace(pointer, target)

    def finish(self, pending, record, phase):
        atomic_json(self.p.state / 'current.json', record)
        self.event(pending, phase)
        (self.p.state / 'maintenance').unlink()
        sync_directory(self.p.state)
        (self.p.state / 'pending.json').unlink()
        sync_directory(self.p.state)

    def execute(self, pending, environment, migrate):
        old, target = pending['old'], pending['target']
        names = [n for n in SERVICES if n in set(target['units']) | set(old['units'] if old else [])]
        gate = self.p.state / 'maintenance'
        if not gate.exists():
            with gate.open('xb') as stream:
                stream.write(b'controlled release\n')
                stream.flush()
                os.fsync(stream.fileno())
            gate.chmod(0o644)
            sync_directory(self.p.state)
        self.event(pending, 'maintenance')
        try:
            self.effects.stop(names)
            self.event(pending, 'stopped')
            actual = self.effects.schema(environment)
            require(actual == target['schema'] or migrate and target['schema'][:len(actual)] == actual, 'Database schema requires forward repair.')
            root = self.p.root / 'releases' / target['release_id']
            if actual != target['schema']:
                self.event(pending, 'migration_started')
                self.effects.migrate(root, environment)
            require(self.effects.schema(environment) == target['schema'], 'Database differs from the exact candidate schema.')
            self.event(pending, 'schema_verified')
            # Detect a changed private configuration or artifact before pointer changes.
            now, _ = self.candidate(target['release_id'], target['manifest_sha256'], target['configuration_id'])
            require(now == target, 'Candidate configuration changed during operation.')
            self.switch(target)
            self.event(pending, 'switched')
            self.effects.start(target['units'])
            self.effects.healthy(target['units'])
            self.finish(pending, target, 'committed')
            return {'state': 'committed', 'operation_id': pending['operation_id'], 'release_id': target['release_id']}
        except Exception as error:
            # No compensation across schema changes, incomplete stops or changed secrets.
            try:
                self.effects.stop(names)
                if old:
                    previous, old_env = self.candidate(old['release_id'], old['manifest_sha256'], old['configuration_id'])
                    require(previous == old and self.effects.schema(old_env) == old['schema'], 'Previous version is not compatible with the current database.')
                    self.switch(old)
                    self.effects.start(old['units'])
                    self.effects.healthy(old['units'])
                    self.finish(pending, old, 'rolled_back')
                    raise ReleaseError('Candidate failed; exact previous application/configuration restored. No database rollback occurred.')
            except ReleaseError as recovery_error:
                if not (self.p.state / 'pending.json').exists():
                    raise recovery_error from error
            except Exception:
                pass
            self.event(pending, 'forward_repair_required')
            raise ReleaseError('Release incomplete; maintenance remains closed. Inspect the operation and explicitly recover.') from error

    def activate(self, identity, expected, configuration, expected_current, migrate=False):
        require(not (self.p.state / 'pending.json').exists() and not (self.p.state / 'maintenance').exists(), 'Recover pending maintenance before activation.')
        old = self.current()
        require((old['release_id'] if old else 'none') == expected_current, 'Current release changed.')
        target, environment = self.candidate(identity, expected, configuration)
        require(old != target, 'This exact application/configuration is already current.')
        require(not old or old['database_identity_sha256'] == target['database_identity_sha256'], 'Database switching is outside normal application release.')
        actual = self.effects.schema(environment)
        require(not old or actual == old['schema'], 'Current database and committed application differ.')
        require(actual == target['schema'] or migrate and target['schema'][:len(actual)] == actual, 'Explicit upgrade consent and unchanged schema prefix required.')
        pending = {'operation_id': uuid.uuid4().hex, 'old': old, 'target': target}
        self.event(pending, 'prepared')
        return self.execute(pending, environment, migrate)

    def status(self):
        current_path = self.p.state / 'current.json'
        current = read_json(current_path.read_bytes()) if current_path.exists() else None
        pending = self.pending()
        try:
            self.current()
            consistent = True
        except (ReleaseError, OSError):
            consistent = False
        return {'committed_release_id': current['release_id'] if current else None,
                'configuration_id': current['configuration_id'] if current else None,
                'maintenance_closed': (self.p.state / 'maintenance').exists(), 'pointers_match_committed': consistent,
                'pending_operation': {key: pending[key] for key in ('operation_id', 'phase')} if pending else None,
                'pending_target_release_id': pending['target']['release_id'] if pending else None}

    def recover(self, operation, mode, migrate=False):
        require(re.fullmatch(r'[a-f0-9]{32}', operation), 'Exact operation identity required.')
        pending = self.pending()
        require(pending is not None, 'No pending operation exists.')
        require(pending['operation_id'] == operation, 'Pending operation changed.')
        target = pending['old'] if mode == 'previous' else pending['target']
        require(target is not None, 'No previous application exists.')
        # A process interruption may leave either pointer combination, never accept an unrelated one.
        for pointer, key, base in [(self.p.root / 'current', 'release_id', self.p.root / 'releases'),
                                   (self.p.config_root / 'active', 'configuration_id', self.p.config_root / 'configurations')]:
            allowed = {base / r[key] for r in (pending['old'], pending['target']) if r}
            require(self.pointers.get(pointer) is None or self.pointers.get(pointer) in allowed,
                    'Unrelated pointer blocks recovery.')
        now, environment = self.candidate(target['release_id'], target['manifest_sha256'], target['configuration_id'])
        require(now == target, 'Recovery candidate/configuration changed.')
        actual = self.effects.schema(environment)
        require(actual == target['schema'] or mode == 'forward' and migrate and target['schema'][:len(actual)] == actual,
                'Recovery cannot downgrade or silently change the database.')
        # For previous recovery retain the union of affected services, even when the target shrinks.
        if mode == 'previous':
            self.effects.stop([n for n in SERVICES if n in set(pending['target']['units']) | set(target['units'])])
            pending = {**pending, 'old': pending['target'], 'target': target}
        self.event(pending, 'recovery_' + mode)
        return self.execute(pending, environment, migrate and mode == 'forward')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    stage = commands.add_parser('stage')
    stage.add_argument('--archive', type=Path, required=True)
    stage.add_argument('--manifest-sha256', required=True)
    commands.add_parser('status')
    activate = commands.add_parser('activate')
    activate.add_argument('--release-id', required=True)
    activate.add_argument('--manifest-sha256', required=True)
    activate.add_argument('--configuration', required=True)
    activate.add_argument('--expected-current', required=True)
    activate.add_argument('--migrate', action='store_true')
    recover = commands.add_parser('recover')
    recover.add_argument('--operation-id', required=True)
    recover.add_argument('--mode', choices=('previous', 'forward'), required=True)
    recover.add_argument('--migrate', action='store_true')
    args = parser.parse_args()
    require(sys.platform == 'linux' and os.geteuid() == 0, 'Deployment writes require native Linux root; no local substitute is started.')
    os.umask(0o077)
    coordinator = Coordinator()
    with coordinator.locked():
        if args.command == 'stage':
            result = coordinator.stage(args.archive, args.manifest_sha256)
        elif args.command == 'status':
            result = coordinator.status()
        elif args.command == 'activate':
            result = coordinator.activate(args.release_id, args.manifest_sha256, args.configuration, args.expected_current, args.migrate)
        else:
            result = coordinator.recover(args.operation_id, args.mode, args.migrate)
    print(json.dumps(result))


if __name__ == '__main__':
    try:
        main()
    except (ReleaseError, OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError, tarfile.TarError):
        # Native process outputs/secret-bearing file contents are deliberately not printed.
        print('Controlled release failed. Check the root-only operation records and current maintenance state.', file=sys.stderr)
        raise SystemExit(1)
