const { spawnSync } = require('node:child_process');
const { mkdirSync, readFileSync } = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
if (!process.env.CP_MOBILE_API_ORIGIN) throw new Error('An explicit API origin is required.');
mkdirSync(path.join(root, 'android/app/src/main/assets'), { recursive: true });
const result = spawnSync(process.execPath, [require.resolve('react-native/cli.js'), 'bundle',
  '--platform', 'android', '--dev', 'false', '--entry-file', 'index.js', '--max-workers', '2', '--reset-cache',
  '--bundle-output', 'android/app/src/main/assets/index.android.bundle',
  '--assets-dest', 'android/app/src/main/res',
  '--sourcemap-output', '../.local/android/index.android.map'], {
  cwd: root, stdio: 'inherit', windowsHide: true,
  env: { ...process.env, TARO_ENV: 'rn', NODE_ENV: 'production' },
});
if (result.error) throw new Error('Native bundling could not start.');
if (result.status === 0) {
  const map = JSON.parse(readFileSync(path.join(root, '../.local/android/index.android.map'), 'utf8'));
  // The native host deliberately has no Expo runtime. Fail the build when a
  // newly used Taro API introduces an extension that needs a native module.
  if (map.sources.some(file => /[/\\]node_modules[/\\](?:expo(?:-[^/\\]+)?|@unimodules)[/\\]/.test(file))) {
    throw new Error('This bundle requires an Expo native extension; update and validate the native host before packaging.');
  }
}
process.exit(result.status ?? 1);
