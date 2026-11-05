"""Inspect the successful native Windows JS build; preserve earlier failed steps."""
import argparse,hashlib,json,re
from datetime import datetime,timezone
from pathlib import Path
from run_batch_round import APP,EVIDENCE,write_once
from record_batch_client_inputs import client_names,client_digest

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--attempt',default='batch-3');args=parser.parse_args()
    assert re.fullmatch(r'batch-[0-9]+',args.attempt)
    log=APP/('.local/batch-round-157/android-adapter-retry.log' if args.attempt=='batch-3' else '.local/batch-round-157/android-bundle-'+args.attempt+'.log')
    build=json.loads(log.read_text(encoding='utf-8').splitlines()[-1])
    before=json.loads((EVIDENCE/('client-inputs-'+args.attempt+'-before.json')).read_text(encoding='utf-8'))
    assert before['source_files']==client_names() and before['source_sha256']==client_digest()
    assert build['passed'] and build['max_workers']==4 and build['native_invocation']['exit_code']==0 and not build['apk_created']
    bundle=APP/'mobile/android/app/src/main/assets/index.android.bundle';mapping=APP/'.local/android/index.android.map'
    assert sha(bundle)==build['bundle_sha256'] and sha(mapping)==build['source_map_sha256']
    data=json.loads(mapping.read_text(encoding='utf-8'));modules=[]
    assert len(data['sources'])==len(data['sourcesContent'])
    for source,content in zip(data['sources'],data['sourcesContent']):
        normalized=source.replace('\\','/')
        candidates=[APP/normalized,APP/'mobile'/normalized]
        if '/src/' in normalized:candidates.append(APP/'mobile/src'/normalized.split('/src/',1)[1])
        path=next((p for p in candidates if p.is_file() and p.resolve().is_relative_to(APP.resolve()) and 'node_modules' not in p.parts),None)
        if path is None or path.suffix not in ('.js','.jsx','.ts','.tsx','.json','.cjs','.mjs'):continue
        assert content.replace('\r\n','\n')==path.read_text(encoding='utf-8').replace('\r\n','\n'), str(path)
        modules.append({'file':path.relative_to(APP).as_posix(),'sha256':sha(path)})
    assert len(modules)>20
    value={'passed':True,'attempt':args.attempt,'checked_at':datetime.now(timezone.utc).isoformat(),'client_source_sha256':client_digest(),'client_source_files':client_names(),'build':build,'modules':modules,'private_log_sha256':sha(log),'recorder_sha256':sha(Path(__file__)),'replaces_failed_step':'client-checks-batch-3.json:android-bundle' if args.attempt=='batch-3' else None,'new_apk':False,'formal_android_acceptance':False}
    write_once(EVIDENCE/('android-js-supplement.json' if args.attempt=='batch-3' else 'android-js-supplement-'+args.attempt+'.json'),value)
    print(json.dumps({'passed':True,'matched_current_modules':len(modules),'new_apk':False}))
