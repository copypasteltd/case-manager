"""Current development APK/JVM checks, native tools only; no device acceptance."""
import json
import os
from pathlib import Path
import subprocess
import sys
import xml.etree.ElementTree as ET
import build_android as base
import build_refund_fx_android as adapter

APP=base.APP

def run(command,*args,**kwargs):
    if command==['npm.cmd','run','build:rn']:
        bridge=APP/'.local/android/refund-fx-fs-diagnostic.cjs'
        kwargs['env']={**kwargs['env'],'NODE_OPTIONS':'--require="'+bridge.as_posix()+'"'}
    return adapter.run(command,*args,**kwargs)

def main():
    if len(sys.argv)!=1:raise SystemExit('This command builds the complete Office development APK.')
    temp=APP/'.local/android/office-authoring-metro-temp';temp.mkdir(parents=True,exist_ok=True)
    os.environ.update({'TEMP':str(temp),'TMP':str(temp)})
    original=subprocess.run
    subprocess.run=run
    sys.argv+=['--evidence-group','office-authoring','--google-mirror']
    try:result=base.main()
    finally:subprocess.run=original
    if result:return result
    native=base.native_app()
    jdk=next((native/'.tools/jdk17').glob('*/bin/java.exe')).parents[1]
    env={**os.environ,'JAVA_HOME':str(jdk),'GRADLE_USER_HOME':str(native/'.local/android/gradle-home')}
    log=APP/'.local/android/office-authoring-jvm.log'
    with log.open('wb') as output:
        r=original(['cmd.exe','/d','/c','gradlew.bat','--no-daemon','--console=plain','-p','../native/jvm-tests','test'],cwd=native/'mobile/android',env=env,stdout=output,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW)
    if r.returncode:return r.returncode
    suites={}
    for label,folder in [('jvm','mobile/native/jvm-tests/build/test-results/test'),('android','mobile/android/app/build/test-results/testDebugUnitTest')]:
        rows=[ET.parse(p).getroot()for p in (APP/folder).glob('TEST-*.xml')]
        suites[label]={key:sum(int(row.get(key,'0'))for row in rows)for key in ('tests','failures','errors','skipped')}
    assert all(v['tests']>=27 and v['failures']==v['errors']==v['skipped']==0 for v in suites.values())
    report={'passed':True,'suites':suites,'native_invocations':adapter.invocations,'apk_created':True,'development_only':True,'formal_android_acceptance':False,'real_device_used':False,'virtualization_used':False}
    (APP.parent/'docs/quality/evidence/office-authoring/native-suites.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(report));return 0

if __name__=='__main__':raise SystemExit(main())
