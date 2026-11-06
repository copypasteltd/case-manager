"""Strict operator-authored monitoring plan. No network or secret-file reads."""
from __future__ import annotations

import ipaddress
import json
import re
from pathlib import Path
from urllib.parse import urlsplit


class PlanError(ValueError):
    pass


def require(ok: bool, code: str) -> None:
    if not ok:
        raise PlanError(code)


def fields(value, required, optional=()):
    require(type(value) is dict and set(required) <= set(value) <= set(required) | set(optional), 'PLAN_FIELDS')


def identifier(value):
    require(type(value) is str and re.fullmatch(r'[a-z][a-z0-9_-]{0,47}', value) is not None, 'IDENTIFIER')
    return value


def private_path(value):
    require(type(value) is str and re.fullmatch(r'/etc/copypaste-monitoring/secrets/[a-z][a-z0-9_.-]{0,79}', value) is not None, 'SECRET_PATH')
    return value


def hostname(value):
    require(type(value) is str and len(value) <= 253 and re.fullmatch(r'[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?', value) is not None and '..' not in value, 'HOSTNAME')
    return value


def load(path: Path):
    require(path.is_file() and path.stat().st_size <= 65536 and not path.is_symlink(), 'PLAN_FILE')
    def unique(pairs):
        out = {}
        for key, value in pairs:
            require(key not in out, 'DUPLICATE_KEY')
            out[key] = value
        return out
    try:
        value = json.loads(path.read_text('utf-8'), object_pairs_hook=unique)
    except (UnicodeError, json.JSONDecodeError):
        raise PlanError('PLAN_JSON') from None
    return validate(value)


def validate(plan):
    fields(plan, ('schema', 'deployment_id', 'monitor_host_id', 'application_host_id', 'separate_host_reviewed',
                  'targets', 'ssh', 'required_units', 'receiver', 'watchdog_url_file'))
    require(type(plan['schema']) is int and plan['schema'] == 1, 'PLAN_SCHEMA')
    for name in ('deployment_id', 'monitor_host_id', 'application_host_id'):
        identifier(plan[name])
    require(plan['separate_host_reviewed'] is True and plan['monitor_host_id'] != plan['application_host_id'], 'SEPARATE_HOST_REQUIRED')
    targets = plan['targets']
    require(type(targets) is list and 1 <= len(targets) <= 16, 'TARGET_COUNT')
    seen = {'api_ready'}
    for target in targets:
        fields(target, ('id', 'url', 'approved_ip', 'expected_status_codes'))
        identifier(target['id'])
        require(target['id'] not in seen, 'TARGET_DUPLICATE')
        seen.add(target['id'])
        require(type(target['url']) is str and len(target['url']) <= 2048
                and not any(ord(c) <= 32 or ord(c) == 127 for c in target['url']), 'TARGET_URL')
        try:
            url = urlsplit(target['url'])
            ip = ipaddress.ip_address(target['approved_ip'])
            valid_url = url.scheme == 'https' and url.port in (None, 443) and not url.username and not url.password and not url.query and not url.fragment
        except (ValueError, TypeError):
            raise PlanError('TARGET_URL') from None
        require(valid_url and url.hostname is not None, 'TARGET_URL')
        hostname(url.hostname)
        try:
            ipaddress.ip_address(url.hostname)
        except ValueError:
            pass
        else:
            raise PlanError('DNS_NAME_REQUIRED')
        require(ip.is_global and str(ip) == target['approved_ip'], 'PUBLIC_ADDRESS_REQUIRED')
        codes = target['expected_status_codes']
        require(type(codes) is list and 1 <= len(codes) <= 8
                and all(type(code) is int and 200 <= code < 300 for code in codes)
                and len(set(codes)) == len(codes), 'EXPECTED_STATUS')
    ssh = plan['ssh']
    fields(ssh, ('host', 'port', 'user', 'identity_file', 'known_hosts_file', 'api_port', 'node_port'))
    hostname(ssh['host'])
    identifier(ssh['user'])
    for name in ('port', 'api_port', 'node_port'):
        require(type(ssh[name]) is int and 1 <= ssh[name] <= 65535, 'SSH_PORT')
    require(ssh['api_port'] != ssh['node_port'], 'SSH_DISTINCT_PORTS')
    for name in ('identity_file', 'known_hosts_file'):
        private_path(ssh[name])
    units = plan['required_units']
    require(type(units) is list and 1 <= len(units) <= 40, 'UNIT_COUNT')
    for unit in units:
        require(type(unit) is str and re.fullmatch(r'copypaste-release-[a-z][a-z0-9-]{0,63}\.service', unit) is not None, 'UNIT_NAME')
    require(len(set(units)) == len(units), 'UNIT_DUPLICATE')
    require('copypaste-release-api.service' in units, 'API_UNIT_REQUIRED')
    receiver = plan['receiver']
    require(type(receiver) is dict and receiver.get('kind') in ('telegram', 'webhook'), 'RECEIVER_KIND')
    if receiver['kind'] == 'telegram':
        fields(receiver, ('kind', 'bot_token_file', 'chat_id_file'))
        private_path(receiver['bot_token_file'])
        private_path(receiver['chat_id_file'])
    else:
        fields(receiver, ('kind', 'url_file'))
        private_path(receiver['url_file'])
    private_path(plan['watchdog_url_file'])
    # Operator declarations are not proof of host independence or actual delivery.
    return plan
