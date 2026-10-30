"""Fixed service commands and identities; operator input cannot add executables."""
SERVICES = {
    'agent-native': ('root', None, None),
    'api': ('copypaste-api', 'api', 'api.env'),
    'notification': ('copypaste-notification', 'worker-notification', 'notification.env'),
    'subscription': ('copypaste-subscription', 'worker-subscription', 'subscription.env'),
    'report-export': ('copypaste-report', 'worker-report-export', 'report-export.env'),
    'file-text': ('copypaste-text', 'worker-file-text', 'file-text.env'),
    'multipart-cleanup': ('copypaste-cleanup', 'worker-multipart-cleanup', 'multipart-cleanup.env'),
    'upload': ('copypaste-upload', 'worker-upload', 'upload.env'),
    **{f'{mode}-{size}': (f'copypaste-{mode}', f'worker-{mode}-{size}', f'{mode}.env')
       for mode in ('verify', 'promote', 'replica', 'restore') for size in ('small', 'large')},
    'status-bridge': ('copypaste-status', None, None),
    'telegram': ('copypaste-telegram', 'worker-telegram', 'telegram.env'),
    'telegram-delivery': ('copypaste-delivery', 'worker-telegram-delivery', 'telegram-delivery.env'),
    'agent': ('copypaste-agent', 'worker-agent', 'agent.env'),
    'agent-reconcile': ('copypaste-agent', 'worker-agent-reconcile', 'agent.env'),
}


def unit(name):
    if name not in SERVICES:
        raise ValueError('Unknown service.')
    return 'copypaste-release-' + name + '.service'


def render(name):
    user, command, env_file = SERVICES[name]
    dependencies = ' copypaste-release-agent-native.service' if name in ('agent', 'agent-reconcile') else ''
    start = '/opt/copypaste/current/bin/platform ' + command if command else (
        '/opt/copypaste/current/bin/agent-native serve --config /etc/copypaste/active/agent-native.json' if name == 'agent-native'
        else '/opt/copypaste/current/bin/status-bridge --config %d/status-bridge.json')
    settings = f'EnvironmentFile=/etc/copypaste/active/{env_file}\n' if env_file else ''
    if name == 'status-bridge':
        settings += 'LoadCredential=status-bridge.json:/etc/copypaste/active/status-bridge.json\n'
    settings += ('ExecStartPre=/opt/copypaste/current/bin/agent-native check --config /etc/copypaste/active/agent-native.json\n'
                 if name == 'agent-native' else '')
    state = 'copypaste/agent-native' if name == 'agent-native' else 'copypaste-' + name
    writable = '/var/lib/copypaste/agent-native /var/lib/copypaste/agent-work' if name == 'agent-native' else '/var/lib/' + state
    return f'''# Generated from the fixed release catalog; install only after operator review.
[Unit]
Description=CopyPaste release {name}
After=network-online.target{dependencies}
Wants=network-online.target
RequiresMountsFor=/opt/copypaste /etc/copypaste
{'Requires=' + dependencies.strip() if dependencies else ''}
[Service]
Type=simple
User={user}
Group={user}
WorkingDirectory=/opt/copypaste/current/runtime/app
{settings}ExecStart={start}
Restart=on-failure
RestartSec=10
TimeoutStopSec=45
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
CacheDirectory=copypaste-{name}
ReadWritePaths={writable} /var/cache/copypaste-{name}
LimitCORE=0
UMask=0077
[Install]
WantedBy=multi-user.target
'''


def nginx_site(hostname):
    # Callers validate the hostname as one DNS name before rendering.
    return f'''# Reviewed CopyPaste TLS site; global nginx.conf must include conf.d/*.conf.
server {{
    listen 80;
    server_name {hostname};
    return 308 https://{hostname}$request_uri;
}}
server {{
    listen 443 ssl;
    server_name {hostname};
    ssl_certificate /etc/ssl/copypaste/fullchain.pem;
    ssl_certificate_key /etc/ssl/copypaste/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    root /opt/copypaste/current/web;
    index index.html;
    client_max_body_size 16m;
    add_header X-Content-Type-Options nosniff always;
    add_header Referrer-Policy same-origin always;
    add_header X-Frame-Options DENY always;
    if (-f /var/lib/copypaste-release/maintenance) {{ return 503; }}
    location /assets/ {{
        alias /opt/copypaste/assets/;
        expires 1y;
    }}
    location /v1/ {{
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_buffering off;
        proxy_read_timeout 125s;
    }}
    location /health/ {{ return 404; }}
    location / {{
        add_header Cache-Control no-store;
        try_files $uri $uri/ /index.html;
    }}
}}
'''
