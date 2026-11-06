"""Native plan and deployment-boundary tests; no real target or receiver I/O."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from config import PlanError, load, validate
from render import build

HERE = Path(__file__).resolve().parent


class MonitoringTests(unittest.TestCase):
    def setUp(self):
        self.plan = json.loads((HERE / 'example.json').read_text('utf-8'))

    def test_independent_loopback_stack_with_fixed_network_targets(self):
        files = build(self.plan)
        self.assertEqual(len(files), 9)
        prom = json.loads(files['config/prometheus.yml'])
        blackbox = json.loads(files['config/blackbox.yml'])
        manager = json.loads(files['config/alertmanager.yml'])
        self.assertEqual(prom['rule_files'], ['rules.yml'])
        self.assertIn('93.184.216.34', json.dumps(prom))
        self.assertNotIn('https://workspace.example.com', json.dumps(prom))
        self.assertEqual(blackbox['modules']['workspace_web']['http']['tls_config']['server_name'], 'workspace.example.com')
        self.assertFalse(blackbox['modules']['workspace_web']['http']['follow_redirects'])
        self.assertIn('watchdog', json.dumps(manager))
        self.assertIn('chat_id_file', json.dumps(manager))
        self.assertIn('LoadCredential=telegram-token:', files['monitor-host/copypaste-monitor-alertmanager.service'])
        self.assertIn('StrictHostKeyChecking=yes', files['monitor-host/copypaste-monitor-tunnel.service'])
        self.assertIn('-L 127.0.0.1:18080:127.0.0.1:8080', files['monitor-host/copypaste-monitor-tunnel.service'])
        self.assertNotIn('LocalForward *', files['monitor-host/copypaste-monitor-tunnel.service'])
        self.assertIn('--web.listen-address=127.0.0.1:9100', files['application-host/copypaste-monitor-node.service'])

    def test_target_and_service_input_cannot_inject_or_probe_private_addresses(self):
        invalid = []
        for url in ('http://workspace.example.com', 'https://user:secret@workspace.example.com',
                    'https://127.0.0.1', 'https://workspace.example.com:444/',
                    'https://workspace.example.com/?secret=x', 'https://workspace.example.com/#x',
                    'https://workspace.example.com/\nadmin'):
            p=copy.deepcopy(self.plan);p['targets'][0]['url']=url;invalid.append(p)
        for address in ('127.0.0.1', '10.0.0.1', '169.254.169.254', '::1', []):
            p=copy.deepcopy(self.plan);p['targets'][0]['approved_ip']=address;invalid.append(p)
        for key, value in (('expected_status_codes', [[]]), ('expected_status_codes', [True]), ('id', 'x; rm')):
            p=copy.deepcopy(self.plan);p['targets'][0][key]=value;invalid.append(p)
        for value in ([[]], ['copypaste-release-api.service\nExecStart=bad'], []):
            p=copy.deepcopy(self.plan);p['required_units']=value;invalid.append(p)
        p=copy.deepcopy(self.plan);p['monitor_host_id']=p['application_host_id'];invalid.append(p)
        p=copy.deepcopy(self.plan);p['receiver']['bot_token_file']='/tmp/secret';invalid.append(p)
        p=copy.deepcopy(self.plan);p['ssh']['port']=True;invalid.append(p)
        for p in invalid:
            with self.subTest(case=len(str(p))):
                with self.assertRaises(PlanError):validate(p)

    def test_generation_reads_no_secrets_and_refuses_overwrite_and_duplicate_json(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);source=root/'plan.json';out=root/'reviewed'
            source.write_text(json.dumps(self.plan),encoding='utf-8')
            command=[sys.executable,'-B',str(HERE/'monitoring.py'),'--config',str(source),'--output',str(out)]
            first=subprocess.run(command,capture_output=True,text=True)
            self.assertEqual(first.returncode,0,first.stdout)
            manifest=json.loads((out/'manifest.json').read_text('utf-8'))
            self.assertFalse(manifest['services_started'])
            self.assertFalse(manifest['real_delivery_verified'])
            second=subprocess.run(command,capture_output=True,text=True)
            self.assertNotEqual(second.returncode,0)
            self.assertNotIn(str(source),second.stdout+second.stderr)
            source.write_text('{"schema":1,"schema":2}',encoding='utf-8')
            with self.assertRaises(PlanError):load(source)
            source.write_text('x'*65537,encoding='utf-8')
            with self.assertRaises(PlanError):load(source)

    def test_webhook_and_watchdog_have_distinct_controlled_credential_slots(self):
        self.plan['receiver']={'kind':'webhook','url_file':'/etc/copypaste-monitoring/secrets/receiver-url'}
        files=build(self.plan);manager=json.loads(files['config/alertmanager.yml'])
        self.assertEqual(manager['receivers'][0]['name'],'operators')
        unit=files['monitor-host/copypaste-monitor-alertmanager.service']
        self.assertIn('LoadCredential=receiver-url:',unit)
        self.assertIn('LoadCredential=watchdog-url:',unit)
        self.assertNotIn('telegram-token',unit)


if __name__=='__main__':unittest.main(verbosity=2)
