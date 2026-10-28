import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { mkdir, mkdtemp, readFile, realpath, rm, stat, writeFile } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(path.join(app, 'package.json'));
const local = await realpath(path.join(app, '.local'));
const workspace = await mkdtemp(path.join(local, 'dependency-maintenance-'));
after(async () => {
  const target = await realpath(workspace);
  assert.equal(path.dirname(target), local);
  assert.ok(path.basename(target).startsWith('dependency-maintenance-'));
  await rm(target, { recursive: true, force: true });
});

test('Taro Doctor glob CLI passes filename metacharacters as literal child arguments', async () => {
  const mobileRequire = createRequire(path.join(app, 'mobile/package.json'));
  const doctorRequire = createRequire(mobileRequire.resolve('@tarojs/plugin-doctor/package.json'));
  const globPackage = doctorRequire.resolve('glob/package.json');
  const manifest = JSON.parse(await readFile(globPackage, 'utf8'));
  const bin = path.resolve(path.dirname(globPackage), typeof manifest.bin === 'string' ? manifest.bin : manifest.bin.glob);
  const area = path.join(workspace, 'glob');
  await mkdir(area);
  const names = ['ordinary.fixture', 'with space.fixture', 'bracket [one].fixture', 'a.fixture&mkdir injected.fixture'];
  for (const name of names) await writeFile(path.join(area, name), 'synthetic input');
  const collector = path.join(area, 'collect.cjs'), output = path.join(area, 'argv.json');
  await writeFile(collector, "require('node:fs').writeFileSync(process.argv[2], JSON.stringify(process.argv.slice(3)));\n");
  const run = spawnSync(process.execPath, [bin, '--cwd', area, '--cmd', process.execPath,
    '--cmd-arg', collector, '--cmd-arg', output, '--', '*.fixture'],
  { cwd: area, encoding: 'utf8', timeout: 10000, windowsHide: true });
  assert.equal(run.error, undefined);
  assert.equal(run.status, 0, run.stderr);
  assert.deepEqual(JSON.parse(await readFile(output, 'utf8')).sort(), names.sort());
  await assert.rejects(stat(path.join(area, 'injected.fixture')), { code: 'ENOENT' });
});

test('Legacy native codegen codemod honors actual ignore patterns with the maintained matcher', async () => {
  const codegenRequire = createRequire(require.resolve('react-native-codegen/package.json'));
  const { run } = codegenRequire('jscodeshift/src/Runner');
  const area = path.join(workspace, 'codemod'), input = path.join(area, 'input');
  await mkdir(path.join(input, 'excluded'), { recursive: true });
  const selected = ['selected.js', '.visible.js'];
  const ignored = ['excluded/payload.js', 'config-hidden.js', 'pattern.skip.js', 'pattern.keep.js'];
  for (const name of [...selected, ...ignored]) await writeFile(path.join(input, name), 'const value = "BEFORE";\n');
  const transform = path.join(area, 'transform.cjs'), config = path.join(area, '.ignore');
  await writeFile(transform, "module.exports = file => file.source.replace('BEFORE', 'AFTER');\n");
  await writeFile(config, '# synthetic ignore file\nexcluded/\nconfig-hidden.js\n');
  const result = await run(transform, [input], { runInBand: true, babel: false,
    silent: true, verbose: 0, extensions: 'js', ignorePattern: ['**/pattern.{skip,keep}.js'], ignoreConfig: [config] });
  assert.equal(result.ok, selected.length);
  assert.equal(result.error, 0);
  for (const name of selected) assert.match(await readFile(path.join(input, name), 'utf8'), /AFTER/);
  for (const name of ignored) assert.match(await readFile(path.join(input, name), 'utf8'), /BEFORE/);
});
