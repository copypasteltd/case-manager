"""Native independent Prometheus stack, separate from platform API/PG/workers."""
from __future__ import annotations

import json
from urllib.parse import urlsplit

from config import validate

CONFIG = '/etc/copypaste-monitoring/current'
BIN = '/opt/copypaste-monitoring/bin'
CREDS = '/run/credentials/copypaste-monitor-alertmanager.service'


def encoded(value):
    # JSON is also a YAML document; no string interpolation into YAML syntax.
    return json.dumps(value, ensure_ascii=False, indent=2) + '\n'


def rule(name, expression, labels=None, duration='2m'):
    item = {'alert': name, 'expr': expression, 'labels': {'severity': 'critical', **(labels or {})},
            'annotations': {'summary': 'Independent infrastructure check requires operator review.'}}
    if duration:
        item['for'] = duration
    return item


def unit(name, command, *, credentials=None, extra='', application=False):
    credential_lines = ''.join(f'LoadCredential={key}:{path}\n' for key, path in (credentials or {}).items())
    state = f'copypaste-monitoring/{name}'
    return f'''[Unit]
Description=CopyPaste independent {name}
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=cpmon-{name}
Group=cpmon-{name}
WorkingDirectory={'/' if application else CONFIG}
{credential_lines}ExecStart={command}
Restart=on-failure
RestartSec=10
TimeoutStopSec=30
KillMode=control-group
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
LockPersonality=true
StateDirectory={state}
UMask=0077
LimitCORE=0
{extra}[Install]
WantedBy=multi-user.target
'''


def build(plan):
    validate(plan)
    modules = {'api_ready': {'prober': 'http', 'timeout': '8s', 'http': {
        'method': 'GET', 'valid_status_codes': [200], 'follow_redirects': False, 'preferred_ip_protocol': 'ip4'}}}
    configs = [{'targets': ['http://127.0.0.1:18080/health/ready'], 'labels': {'target_id': 'api_ready', 'probe_module': 'api_ready'}}]
    addresses = []
    for target in plan['targets']:
        url = urlsplit(target['url'])
        address = target['approved_ip']
        addresses.append(address)
        host = f'[{address}]' if ':' in address else address
        modules[target['id']] = {'prober': 'http', 'timeout': '8s', 'http': {
            'method': 'GET', 'valid_status_codes': target['expected_status_codes'], 'follow_redirects': False,
            'headers': {'Host': url.hostname}, 'tls_config': {'server_name': url.hostname, 'min_version': 'TLS12'},
            'preferred_ip_protocol': 'ip6' if ':' in address else 'ip4', 'ip_protocol_fallback': False}}
        # Connect to the fixed approved public IP, preserving TLS SNI and Host.
        # Target DNS/rebinding, redirects and user/model-supplied probe URLs are absent.
        configs.append({'targets': [f'https://{host}{url.path or "/"}'], 'labels': {'target_id': target['id'], 'probe_module': target['id']}})
    relabel = [{'source_labels': ['__address__'], 'target_label': '__param_target'},
               {'source_labels': ['probe_module'], 'target_label': '__param_module'},
               {'source_labels': ['target_id'], 'target_label': 'instance'},
               {'target_label': '__address__', 'replacement': '127.0.0.1:9115'}]
    prometheus = {'global': {'scrape_interval': '60s', 'scrape_timeout': '10s', 'evaluation_interval': '60s',
                            'external_labels': {'deployment': plan['deployment_id']}},
                  'rule_files': ['rules.yml'], 'alerting': {'alertmanagers': [{'static_configs': [{'targets': ['127.0.0.1:9093']}]}]},
                  'scrape_configs': [
                      {'job_name': 'copypaste_blackbox', 'metrics_path': '/probe', 'static_configs': configs, 'relabel_configs': relabel},
                      {'job_name': 'copypaste_host', 'static_configs': [{'targets': ['127.0.0.1:19100']}]},
                      {'job_name': 'copypaste_monitor', 'static_configs': [
                          {'targets': ['127.0.0.1:9090'], 'labels': {'component': 'prometheus'}},
                          {'targets': ['127.0.0.1:9093'], 'labels': {'component': 'alertmanager'}},
                          {'targets': ['127.0.0.1:9115'], 'labels': {'component': 'blackbox'}}]}]}
    rules = []
    for target_id in ['api_ready', *[t['id'] for t in plan['targets']]]:
        selector = f'probe_success{{job="copypaste_blackbox",target_id="{target_id}"}}'
        rules += [rule('CopyPasteEndpointDown', selector + ' == 0', {'target_id': target_id}),
                  rule('CopyPasteObservationUnknown', f'absent_over_time({selector}[3m])', {'target_id': target_id}, duration='')]
    for name in plan['required_units']:
        selector = 'node_systemd_unit_state{job="copypaste_host",name=' + json.dumps(name) + ',state="active"}'
        rules += [rule('CopyPasteRequiredUnitDown', selector + ' == 0', {'unit': name}),
                  rule('CopyPasteRequiredUnitUnknown', f'absent_over_time({selector}[3m])', {'unit': name}, duration='')]
    rules += [rule('CopyPasteMonitorComponentDown', 'up{job=~"copypaste_monitor|copypaste_host"} == 0'),
              rule('CopyPasteExternalWatchdog', 'vector(1)', {'severity': 'none'}, duration='')]
    credentials = {'watchdog-url': plan['watchdog_url_file']}
    receiver = plan['receiver']
    if receiver['kind'] == 'telegram':
        credentials.update({'telegram-token': receiver['bot_token_file'], 'telegram-chat': receiver['chat_id_file']})
        primary = {'name': 'operators', 'telegram_configs': [{'bot_token_file': CREDS + '/telegram-token', 'chat_id_file': CREDS + '/telegram-chat',
                  'send_resolved': True, 'parse_mode': '', 'message': '{{ .Status }} / {{ .CommonLabels.deployment }} / {{ .CommonLabels.alertname }}. Review the private monitoring console.'}]}
    else:
        credentials['receiver-url'] = receiver['url_file']
        primary = {'name': 'operators', 'webhook_configs': [{'url_file': CREDS + '/receiver-url', 'send_resolved': True, 'max_alerts': 20}]}
    manager = {'global': {'resolve_timeout': '5m'}, 'route': {'receiver': 'operators', 'group_by': ['deployment', 'alertname', 'target_id', 'unit'],
               'group_wait': '10s', 'group_interval': '1m', 'repeat_interval': '30m',
               'routes': [{'receiver': 'watchdog', 'matchers': ['alertname="CopyPasteExternalWatchdog"'], 'group_wait': '0s', 'group_interval': '30s', 'repeat_interval': '1m'}]},
               'receivers': [primary, {'name': 'watchdog', 'webhook_configs': [{'url_file': CREDS + '/watchdog-url', 'send_resolved': False, 'max_alerts': 1}]}]}
    ssh = plan['ssh']
    tunnel = f'/usr/bin/ssh -F /dev/null -N -T -o BatchMode=yes -o IdentitiesOnly=yes -o IdentityAgent=none -o ExitOnForwardFailure=yes -o ServerAliveInterval=15 -o ServerAliveCountMax=2 -o StrictHostKeyChecking=yes -o UserKnownHostsFile=%d/known-hosts -i %d/ssh-key -p {ssh["port"]} -L 127.0.0.1:18080:127.0.0.1:{ssh["api_port"]} -L 127.0.0.1:19100:127.0.0.1:{ssh["node_port"]} {ssh["user"]}@{ssh["host"]}'
    include = '|'.join(name.replace('.', '[.]') for name in plan['required_units'])
    result = {'config/prometheus.yml': encoded(prometheus), 'config/blackbox.yml': encoded({'modules': modules}),
              'config/rules.yml': encoded({'groups': [{'name': 'copypaste-independent', 'rules': rules}]}), 'config/alertmanager.yml': encoded(manager),
              'monitor-host/copypaste-monitor-prometheus.service': unit('prometheus', f'{BIN}/prometheus --config.file={CONFIG}/prometheus.yml --web.listen-address=127.0.0.1:9090 --storage.tsdb.path=/var/lib/copypaste-monitoring/prometheus --storage.tsdb.retention.time=15d --storage.tsdb.retention.size=2GB'),
              'monitor-host/copypaste-monitor-blackbox.service': unit('blackbox', f'{BIN}/blackbox_exporter --config.file={CONFIG}/blackbox.yml --web.listen-address=127.0.0.1:9115', extra='IPAddressDeny=any\nIPAddressAllow=localhost\n' + ''.join(f'IPAddressAllow={ip}\n' for ip in sorted(set(addresses)))),
              'monitor-host/copypaste-monitor-alertmanager.service': unit('alertmanager', f'{BIN}/alertmanager --config.file={CONFIG}/alertmanager.yml --web.listen-address=127.0.0.1:9093 --cluster.listen-address= --storage.path=/var/lib/copypaste-monitoring/alertmanager', credentials=credentials),
              'monitor-host/copypaste-monitor-tunnel.service': unit('tunnel', tunnel, credentials={'ssh-key': ssh['identity_file'], 'known-hosts': ssh['known_hosts_file']}),
              'application-host/copypaste-monitor-node.service': unit('node', f'{BIN}/node_exporter --web.listen-address=127.0.0.1:{ssh["node_port"]} --collector.disable-defaults --collector.systemd --collector.cpu --collector.meminfo --collector.filesystem --collector.systemd.unit-include={include}', application=True)}
    return result
