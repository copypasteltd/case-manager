"""Run the native build with disclosed four-worker Metro indexing.

Only the npm bundle subprocess is replaced by the identical underlying Metro
CLI with max-workers 4. Preserve the native host's no-Expo source-map guard.
Application inputs, Gradle commands, signature/profile and APK checks are intact.
"""
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys

import build_android as base

APP = base.APP
original_run = subprocess.run
invocations = []


def run(command, *args, **kwargs):
    if command == ['npm.cmd', 'run', 'build:rn'] and Path(kwargs['cwd']).resolve() == APP / 'mobile':
        executable = shutil.which('node.exe')
        if not executable:
            raise RuntimeError('The checkout native Node22 must be on PATH.')
        node = Path(executable).resolve()
        if node.parent != (APP / '.tools/node-v22.23.4-win-x64').resolve():
            raise RuntimeError('Use the checkout native Node22 for this scoped build.')
        (APP / 'mobile/android/app/src/main/assets').mkdir(parents=True, exist_ok=True)
        # Keep generated native outputs and Taro review outputs out of file
        # discovery. These directories are not application inputs. Preserve
        # the real config, resolver, entry, source folders and signature checks.
        config = APP / '.local/android/refund-fx-metro.config.cjs'
        skipped = [APP / 'mobile' / name for name in ('android/.gradle', 'android/app/build', 'android/build', 'dist', '.taro', '.swc')]
        config.write_text("const source = require(" + json.dumps(str(APP / 'mobile/metro.config.js')) + ");\n"
            "module.exports = Promise.resolve(source).then(config => {\n"
            " const escape = value => value.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\\\$&');\n"
            " const generated = " + json.dumps([str(p) for p in skipped]) + ".map(p => new RegExp('^' + escape(p) + '(?:[/\\\\\\\\]|$)'));\n"
            " const old = config.resolver.blockList;\n"
            " return { ...config, resolver: { ...config.resolver, blockList: [...(Array.isArray(old) ? old : old ? [old] : []), ...generated] } };\n"
            "});\n", encoding='utf-8')
        trace = APP / '.local/android/refund-fx-fs-diagnostic.cjs'
        trace.write_text("const root = " + json.dumps(str(APP)) + ";\n" + """const fs = require('node:fs'), path = require('node:path');
const within = value => { const relative=path.relative(root,path.resolve(String(value))); return relative==='' || !relative.startsWith('..'+path.sep)&&relative!=='..'&&!path.isAbsolute(relative); };
// Work around stalled concurrent Windows filesystem calls only in this checkout.
// Use native synchronous filesystem results; never synthesize a path or source.
for (const name of ['readlink','realpath','stat','lstat','readdir','readFile','access']) {
 const original=fs[name], synchronous=name==='realpath'?fs.realpathSync.native:fs[name+'Sync'];
 const promised=fs.promises[name];
 fs.promises[name]=async function(value,...args){return within(value)?synchronous(value,...args):promised.call(this,value,...args);};
 function bridge(...args){const cb=args[args.length-1];if(typeof cb!=='function'||!within(args[0]))return original.apply(this,args);setImmediate(()=>{let error=null,value;try{value=synchronous(...args.slice(0,-1));}catch(e){error=e;}cb(error,value);});}
 Object.assign(bridge,original);if(name==='realpath')bridge.native=bridge;fs[name]=bridge;
}
""".replace('\n+', '\n'), encoding='utf-8')
        actual = [str(node), '--require', str(trace), str(APP / 'node_modules/react-native/cli.js'), 'bundle',
            '--platform', 'android', '--dev', 'false', '--entry-file', 'index.js', '--max-workers', '4', '--reset-cache',
            '--config', str(config),
            '--bundle-output', 'android/app/src/main/assets/index.android.bundle',
            '--assets-dest', 'android/app/src/main/res', '--sourcemap-output', '../.local/android/index.android.map']
        kwargs['env'] = {**kwargs['env'], 'DEBUG':'Metro:NodeCrawler,Metro:FileMap', 'DEBUG_COLORS':'0', 'UV_THREADPOOL_SIZE':'128'}
        result = original_run(actual, *args, **kwargs)
        invocations.append({'command': actual, 'max_workers': 4, 'exit_code': result.returncode,
            'generated_output_exclusions':[str(p.relative_to(APP)) for p in skipped],
            'derived_config_sha256':hashlib.sha256(config.read_bytes()).hexdigest(), 'filesystem_threads':128,
            'windows_checkout_link_bridge_sha256':hashlib.sha256(trace.read_bytes()).hexdigest(),
            'windows_checkout_link_bridge':'Native synchronous link, metadata and read results for checkout paths only; callbacks remain asynchronous, with no source or resolution substitution'})
        if result.returncode == 0:
            mapping = json.loads((APP / '.local/android/index.android.map').read_text(encoding='utf-8'))
            if any(re.search(r'[/\\]node_modules[/\\](?:expo(?:-[^/\\]+)?|@unimodules)[/\\]', name) for name in mapping['sources']):
                raise RuntimeError('The bundle requires an Expo native extension; update and validate the native host.')
        return result
    return original_run(command, *args, **kwargs)


def main():
    if '--evidence-group' in sys.argv or '--bundle-only' in sys.argv:
        raise SystemExit('This wrapper is limited to the complete refund-fx development APK.')
    subprocess.run = run
    try:
        sys.argv += ['--evidence-group', 'refund-fx']
        result = base.main()
    finally:
        subprocess.run = original_run
    if result == 0:
        assert len(invocations) == 1 and invocations[0]['exit_code'] == 0
        evidence = APP.parent / 'docs/quality/evidence/refund-fx/build.json'
        record = json.loads(evidence.read_text(encoding='utf-8'))
        record['scoped_bundle_invocation'] = invocations[0]
        record['scoped_build_script'] = 'app/scripts/build_refund_fx_android.py'
        record['scoped_build_script_sha256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        record['bundle_source_map_guard_applied'] = True
        record['source_map_guard'] = 'Same no-Expo/no-unimodules rule as mobile/native/build-bundle.cjs.'
        evidence.write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    return result


if __name__ == '__main__':
    raise SystemExit(main())
