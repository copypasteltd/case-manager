import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { mkdtemp, mkdir, readFile, realpath, rm, writeFile, lstat, readdir } from 'node:fs/promises';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { execFile, spawnSync } from 'node:child_process';
import { promisify } from 'node:util';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(path.join(app, 'package.json'));
const cliRequire = createRequire(require.resolve('@tarojs/cli/package.json'));
const metroRequire = createRequire(require.resolve('metro/package.json'));
const imageSize = metroRequire('image-size');
const download = promisify(cliRequire('download-git-repo'));
const latestVersion = cliRequire('latest-version');
const run = promisify(execFile);
const local = await realpath(path.join(app, '.local'));
const workspace = await mkdtemp(path.join(local, 'dependency-remediation-'));
after(async () => {
  const target = await realpath(workspace);
  assert.equal(path.dirname(target), local);
  assert.ok(path.basename(target).startsWith('dependency-remediation-'));
  await rm(target, { recursive: true, force: true });
});
let sequence = 0;
async function area() { const dir = path.join(workspace, String(++sequence)); await mkdir(dir); return dir; }
async function exists(file) { return lstat(file).then(() => true, error => { if (error.code === 'ENOENT') return false; throw error; }); }
async function http(handler, fn) {
  const server = createServer(handler); server.listen(0, '127.0.0.1'); await once(server, 'listening');
  try { await fn('http://127.0.0.1:' + server.address().port); }
  finally { server.closeAllConnections(); await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve())); }
  assert.equal(server.listening, false);
}
function png(width, height) {
  const bytes = Buffer.alloc(33); Buffer.from('89504e470d0a1a0a0000000d49484452', 'hex').copy(bytes);
  bytes.writeUInt32BE(width, 16); bytes.writeUInt32BE(height, 20); bytes[24] = 8; bytes[25] = 6;
  return bytes;
}
function zip(text = 'synthetic template content') {
  const value = new (require('adm-zip'))(); value.addFile('package/package.json', Buffer.from('{"name":"synthetic-template"}'));
  value.addFile('package/content.txt', Buffer.from(text)); return value.toBuffer();
}

test('Actual Metro resolution keeps the callable parser and exact PNG/SVG dimensions', () => {
  assert.equal(typeof imageSize, 'function');
  const p = imageSize(png(7, 5)); assert.equal(p.width, 7); assert.equal(p.height, 5); assert.equal(p.type, 'png');
  const svg = imageSize(Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" width="13" height="17"></svg>'));
  assert.equal(svg.width, 13); assert.equal(svg.height, 17); assert.equal(svg.type, 'svg');
});
test('Filesystem and callback image reads remain usable; disableFS still prevents file reads', async () => {
  const dir = await area(), file = path.join(dir, 'image with spaces.png'); await writeFile(file, png(11, 9));
  assert.equal(imageSize(file).width, 11);
  assert.equal((await promisify(imageSize)(file)).height, 9);
  imageSize.disableFS();
  try { assert.throws(() => imageSize(file), /disabled/); assert.equal(imageSize(png(3, 2)).width, 3); }
  finally { imageSize.disableFS(false); }
  await assert.rejects(promisify(imageSize)(path.join(dir, 'missing.png')), { code: 'ENOENT' });
});
test('Real Metro asset discovery reads dimensions, scales and hashes through the new parser', async () => {
  const dir = await area(), file = path.join(dir, 'icon@2x.png'); await writeFile(file, png(14, 10));
  const assets = metroRequire('./src/Assets.js');
  const data = await assets.getAssetData(file, 'assets/icon@2x.png', [], 'android', '/assets');
  assert.equal(data.width, 7); assert.equal(data.height, 5); assert.deepEqual(data.scales, [2]);
  assert.equal(data.files[0], file); assert.match(data.hash, /^[a-f0-9]+$/);
});
for (const kind of ['icns', 'heif', 'jxl']) test('Patched parser rejects zero-length ' + kind + ' structures in a bounded child', () => {
  const malformed = Buffer.alloc(kind === 'jxl' ? 40 : kind === 'heif' ? 32 : 24);
  if (kind === 'icns') { malformed.write('icns'); malformed.writeUInt32BE(24, 4); malformed.write('ic07', 8); }
  else if (kind === 'heif') { malformed.writeUInt32BE(24, 0); malformed.write('ftyp', 4); malformed.write('heic', 8); malformed.write('free', 28); }
  else { Buffer.from('0000000c4a584c200d0a870a', 'hex').copy(malformed); malformed.writeUInt32BE(20, 12); malformed.write('ftyp', 16); malformed.write('jxl ', 20); malformed.write('free', 36); }
  const script = 'const size=require(' + JSON.stringify(metroRequire.resolve('image-size')) + '); try {size(Buffer.from(process.argv[1],"hex"));process.exitCode=2;} catch(e) { if (!(e instanceof TypeError || e instanceof RangeError)) throw e; process.stdout.write("rejected"); }';
  const result = spawnSync(process.execPath, ['-e', script, malformed.toString('hex')], { timeout: 5000, windowsHide: true, encoding: 'utf8' });
  assert.ifError(result.error); assert.equal(result.status, 0, result.stderr); assert.equal(result.stdout, 'rejected');
});
test('Actual upstream Taro fetchTemplate callback caller publishes the extracted package', async () => {
  const dir = await area(), bytes = zip();
  await http((request, response) => { assert.equal(request.url, '/fixture.zip'); response.writeHead(200, { 'Content-Type': 'application/zip' }); response.end(bytes); }, async origin => {
    const fetchTemplate = cliRequire('./dist/create/fetchTemplate.js').default;
    const result = await fetchTemplate('direct:' + origin + '/fixture.zip', dir, false);
    assert.equal(result.length, 1); assert.equal(result[0].name, 'fixture.zip');
    assert.equal(await readFile(path.join(dir, 'fixture.zip/content.txt'), 'utf8'), 'synthetic template content');
    assert.equal(await exists(path.join(dir, 'taro-temp')), false);
  });
});
test('Existing template destinations retain every original byte and do not receive a request', async () => {
  const dir = await area(), destination = path.join(dir, 'existing'); await mkdir(destination); await writeFile(path.join(destination, 'sentinel.txt'), 'retained');
  let requests = 0;
  await http((request, response) => { requests++; response.end(zip()); }, async origin => {
    await assert.rejects(download('direct:' + origin + '/fixture.zip', destination), /already exists/);
  });
  assert.equal(requests, 0); assert.equal(await readFile(path.join(destination, 'sentinel.txt'), 'utf8'), 'retained');
});
test('HTTP failures and malformed archives leave no destination or temporary extraction', async () => {
  const dir = await area();
  await http((request, response) => { response.writeHead(request.url === '/missing' ? 404 : 200); response.end('invalid archive'); }, async origin => {
    await assert.rejects(download('direct:' + origin + '/missing', path.join(dir, 'missing')), /HTTP 404/);
    await assert.rejects(download('direct:' + origin + '/malformed', path.join(dir, 'malformed')));
  });
  assert.deepEqual(await readdir(dir), []);
});
test('Template redirects cannot switch to a UNIX socket or file scheme', async () => {
  const dir = await area();
  await http((request, response) => { response.writeHead(302, { Location: 'file:///synthetic-sentinel' }); response.end(); }, async origin => {
    await assert.rejects(download('direct:' + origin + '/redirect', path.join(dir, 'output')), /HTTP\(S\)/);
  });
  assert.deepEqual(await readdir(dir), []);
});
test('Cross-origin redirects strip synthetic authorization and cookie headers', async () => {
  const dir = await area(); let received = false;
  await http((request, response) => {
    received = true; assert.equal(request.headers.authorization, undefined); assert.equal(request.headers.cookie, undefined);
    response.end(zip('redirected bytes'));
  }, async destination => {
    await http((request, response) => { assert.equal(request.headers.authorization, 'Bearer synthetic-value'); response.writeHead(302, { Location: destination + '/archive' }); response.end(); }, async origin => {
      await download('direct:' + origin + '/redirect', path.join(dir, 'output'), { headers: { authorization: 'Bearer synthetic-value', cookie: 'synthetic-cookie' } });
    });
  });
  assert.equal(received, true); assert.equal(await readFile(path.join(dir, 'output/content.txt'), 'utf8'), 'redirected bytes');
});
test('Redirect cycles and download timeouts stop without leaving staging directories', async () => {
  const dir = await area();
  await http((request, response) => { if (request.url === '/cycle') { response.writeHead(302, { Location: '/cycle' }); response.end(); } }, async origin => {
    await assert.rejects(download('direct:' + origin + '/cycle', path.join(dir, 'cycle')), /redirect limit/);
    await assert.rejects(download('direct:' + origin + '/timeout', path.join(dir, 'timeout'), { timeout: 50 }), /timeout|abort/i);
  });
  assert.deepEqual(await readdir(dir), []);
});
test('A real local Git template preserves branch bytes and removes only its own .git directory', async () => {
  const dir = await area(), repository = path.join(dir, 'source with spaces'), destination = path.join(dir, 'template & [copy]');
  await mkdir(repository);
  const git = args => run('git', args, { cwd: repository, windowsHide: true, timeout: 20000 });
  await git(['init', '--initial-branch=fixture']);
  await git(['config', 'user.name', 'Synthetic Fixture']); await git(['config', 'user.email', 'fixture@example.invalid']);
  await writeFile(path.join(repository, 'content.txt'), 'real Git fixture bytes'); await git(['add', '--', 'content.txt']); await git(['commit', '--message', 'synthetic fixture']);
  await download('direct:' + repository + '#fixture', destination, { clone: true });
  assert.equal(await readFile(path.join(destination, 'content.txt'), 'utf8'), 'real Git fixture bytes');
  assert.equal(await exists(path.join(destination, '.git')), false); assert.equal(await exists(path.join(repository, '.git')), true);
});
test('Git option revisions and external-command transports are rejected before any clone', async () => {
  const dir = await area();
  await assert.rejects(download('direct:ext::synthetic-command#main', path.join(dir, 'ext'), { clone: true }), /transport/);
  await assert.rejects(download('direct:synthetic-repo#--help', path.join(dir, 'flag'), { clone: true }), /revision/);
  await assert.rejects(download('direct:synthetic-repo', path.join(dir, 'checkout'), { clone: true, checkout: '--help' }), /revision/);
  assert.deepEqual(await readdir(dir), []);
});
test('Unsupported options are explicit and callback failures are delivered exactly once', async () => {
  const dir = await area(); let calls = 0;
  await new Promise(resolve => cliRequire('download-git-repo')('direct:http://127.0.0.1/archive', path.join(dir, 'output'), { shell: true }, error => {
    calls++; assert.match(error.message, /Unsupported template option/); resolve();
  }));
  await new Promise(resolve => setImmediate(resolve)); assert.equal(calls, 1); assert.deepEqual(await readdir(dir), []);
});
test('Taro CommonJS latest-version caller uses current metadata and supports explicit tags', async () => {
  let requests = 0;
  await http((request, response) => {
    requests++; assert.equal(request.url, '/synthetic-fixture'); response.setHeader('Content-Type', 'application/json');
    response.end(JSON.stringify({ name: 'synthetic-fixture', 'dist-tags': { latest: '2.0.0', next: '3.0.0' }, versions: { '2.0.0': { version: '2.0.0' }, '3.0.0': { version: '3.0.0' } } }));
  }, async origin => {
    assert.equal(await latestVersion('SYNTHETIC-FIXTURE', { registryUrl: origin + '/' }), '2.0.0');
    assert.equal(await latestVersion('synthetic-fixture', { registryUrl: origin + '/', version: 'next' }), '3.0.0');
  });
  assert.equal(requests, 2);
});
test('Maintained metadata caller rejects a missing registry package', async () => {
  await http((request, response) => { response.writeHead(404); response.end('{}'); }, async origin => {
    await assert.rejects(latestVersion('missing-synthetic-fixture', { registryUrl: origin + '/' }), { name: 'PackageNotFoundError' });
  });
});
