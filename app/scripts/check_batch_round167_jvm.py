"""Compile and run the pure native upload JVM boundary; no Android JS, APK or emulator."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
from run_batch_round167 import APP, EVIDENCE, PRIVATE, write_once


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--attempt',required=True);args=parser.parse_args()
    import re
    if not re.fullmatch('[a-z0-9-]{1,48}',args.attempt):raise ValueError('Safe immutable attempt required.')
    javac=list((APP/'.tools/jdk17').glob('*/bin/javac.exe'))
    cache=APP/'.local/android/gradle-home/caches/modules-2/files-2.1'
    jars=[next((cache/n).glob('*/*.jar')) for n in ('junit/junit/4.13.2','org.hamcrest/hamcrest-core/1.3')]
    assert len(javac)==1
    source=APP/'mobile/android/app/src'
    names=[source/('main/java/hk/copypaste/workspace/'+n+'.java') for n in ('UploadLayout','UploadSource','UploadPermit')]+[source/'test/java/hk/copypaste/workspace/UploadBoundariesTest.java']
    before={p.relative_to(APP).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in names}
    output=PRIVATE/('jvm-'+args.attempt);output.mkdir(parents=True,exist_ok=False)
    classpath=os.pathsep.join(map(str,jars));log=output/'result.log'
    commands=[[str(javac[0]),'-encoding','UTF-8','-cp',classpath,'-d',str(output),*map(str,names)],
              [str(javac[0].with_name('java.exe')),'-cp',str(output)+os.pathsep+classpath,'org.junit.runner.JUnitCore','hk.copypaste.workspace.UploadBoundariesTest']]
    results=[]
    with log.open('xb') as handle:
        for command in commands:
            result=subprocess.run(command,stdout=handle,stderr=subprocess.STDOUT,timeout=90)
            results.append(result.returncode)
            if result.returncode:break
    raw=log.read_text(encoding='utf-8',errors='replace');matched=re.search(r'OK \((\d+) tests?\)',raw)
    unchanged=before=={p.relative_to(APP).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in names}
    record={'passed':results==[0,0] and bool(matched) and unchanged,'source_files':before,'source_unchanged':unchanged,'commands_exit_codes':results,'dependency_sha256':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in jars},'test_count':int(matched[1]) if matched else None,'private_log_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'android_js_built':False,'apk_built':False,'android_runtime_executed':False,'virtualization_used':False}
    write_once(EVIDENCE/('jvm-'+args.attempt+'.json'),record)
    print(json.dumps(record));return 0 if record['passed'] else 1

if __name__=='__main__':raise SystemExit(main())
