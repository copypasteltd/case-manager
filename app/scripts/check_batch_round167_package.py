"""Build native Linux artifact and run its installed Python CLI in an isolated path."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
from run_batch_round167 import APP, ROOT, EVIDENCE, PRIVATE, write_once
from build_production_release import build


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--attempt',required=True);args=parser.parse_args()
    import re
    if not re.fullmatch('[a-z0-9-]{1,48}',args.attempt):raise ValueError('Safe immutable attempt required.')
    output=PRIVATE/('production-'+args.attempt)
    report=build(output)
    package=output/'package'
    manifest=json.loads((package/'release.json').read_text(encoding='utf-8'))
    tools=package/'metadata/release-tools'
    program=('import runpy,sys;sys.path.insert(0,sys.argv[1]);'
             'sys.argv=[sys.argv[1]+"/controller.py","--help"];runpy.run_path(sys.argv[0],run_name="__main__")')
    result=subprocess.run([sys.executable,'-I','-B','-c',program,str(tools)],cwd=output,capture_output=True,text=True,timeout=30)
    from catalog import RELEASE_TOOLS, SERVICES, render, unit
    sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
    modules={n:sha(tools/n) for n in RELEASE_TOOLS}
    matched=all(modules[n]==sha(APP/'deploy/release'/n) for n in RELEASE_TOOLS)
    units=all((package/'systemd'/unit(n)).read_text(encoding='utf-8')==render(n) for n in SERVICES)
    assert (package/'bin/platform').read_bytes()[:4]==b'\x7fELF'
    assert len(manifest['schema'])==133
    passed=report['passed'] and result.returncode==0 and 'activate' in result.stdout and 'recover' in result.stdout and matched and units
    record={'passed':passed,'schema':133,'build':report,'installed_cli_exit_code':result.returncode,
            'isolated_import_without_repository':True,'installed_modules':modules,'all_controlled_units_match':units,
            'cleanup_unit':unit('original-cleanup'),'cleanup_command':SERVICES['original-cleanup'][1],
            'artifact_file_count':len(manifest['files']),'native_linux_executed':False,'production_deployed':False,
            'android_js_built':False,'apk_built':False,'virtualization_used':False}
    write_once(EVIDENCE/('production-'+args.attempt+'.json'),record)
    print(json.dumps({'passed':passed,'installed_modules':len(modules),'schema':133,'artifact_files':len(manifest['files'])}))
    return 0 if passed else 1

if __name__=='__main__':raise SystemExit(main())
