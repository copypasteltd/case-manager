"""Private signing boundary checks; no signing key, APK or device is created."""
import json
import tempfile
from pathlib import Path
import unittest
from android_release import APP, certificate, load_release, lock_release

class AndroidReleaseInputs(unittest.TestCase):
    def test_private_config_and_increasing_versions(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);store=root/'private.keystore';store.write_bytes(b'opaque test input')
            config={'keystore':str(store),'alias':'release','store_password_env':'CP_ANDROID_SECRET_STORE','key_password_env':'CP_ANDROID_SECRET_KEY','certificate_sha256':'a'*64,'version_code':2,'version_name':'0.2.0','previous_version_code':1}
            path=root/'config.json';env={'CP_ANDROID_SECRET_STORE':'fixture-only','CP_ANDROID_SECRET_KEY':'fixture-only'}
            def write(value):path.write_text(json.dumps(value),encoding='utf-8')
            write(config);self.assertEqual(load_release(path,'https://portal.example.test',False,env)[1]['version_code'],2)
            for changes in [{'version_code':1},{'previous_version_code':2},{'version_code':True},{'keystore':str(APP/'package.json')},{'certificate_sha256':'wrong'},{'store_password_env':'PATH'},{'version_name':'latest'},{'password':'not-supported'}]:
                write({**config,**changes})
                with self.assertRaises(ValueError):load_release(path,'https://portal.example.test',False,env)
            write(config)
            for origin,local in [('http://127.0.0.1',True),('http://portal.example.test',False)]:
                with self.assertRaises(ValueError):load_release(path,origin,local,env)
            with self.assertRaises(ValueError):load_release(path,'https://portal.example.test',False,{})
    def test_certificate_is_pinned_without_passwords(self):
        self.assertEqual(certificate('SHA256: '+':'.join(['AB']*32)),'ab'*32)
        self.assertEqual(certificate('Signer #1 certificate SHA-256 digest: '+'cd'*32),'cd'*32)
        for value in ['none','SHA256: '+'a'*64+'\nSHA256: '+'b'*64]:
            with self.assertRaises(ValueError):certificate(value)
    def test_signing_version_ledger_has_one_native_writer(self):
        with tempfile.TemporaryDirectory() as directory:
            first=lock_release(Path(directory))
            with self.assertRaises(ValueError):lock_release(Path(directory))
            first.close();second=lock_release(Path(directory));second.close()

if __name__=='__main__':unittest.main()
