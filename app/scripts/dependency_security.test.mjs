import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { mkdir, mkdtemp, readFile, realpath, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { gzipSync } from 'node:zlib';
import { after, test } from 'node:test';
import { Readable } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import { promisify } from 'node:util';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(path.join(app, 'package.json'));
const extract = require('decompress');
const AdmZip = require('adm-zip');
const download = promisify(require('download-git-repo'));
const local = await realpath(path.join(app, '.local'));
const workspace = await mkdtemp(path.join(local, 'dependency-security-'));
after(async () => {
  const target = await realpath(workspace);
  assert.equal(path.dirname(target), local);
  assert.ok(path.basename(target).startsWith('dependency-security-'));
  await rm(target, { recursive: true, force: true });
});

let sequence = 0;
async function area() {
  const base = path.join(workspace, String(++sequence));
  const output = path.join(base, 'out'), outside = path.join(base, 'out-old');
  await mkdir(output, { recursive: true }); await mkdir(outside);
  return { base, output, outside };
}
const exists = file => stat(file).then(() => true, error => { if (error.code === 'ENOENT') return false; throw error; });
const entry = (name, text = 'synthetic payload') => ({ path: name, data: Buffer.from(text), mode: 0o100644, mtime: new Date('2025-01-01Z'), type: 'file' });
function tar(files) {
  const blocks = [];
  for (const file of files) {
    const header = Buffer.alloc(512), data = file.data ?? Buffer.alloc(0);
    const number = (offset, length, value) => header.write(value.toString(8).padStart(length - 1, '0') + '\0', offset, length, 'ascii');
    header.write(file.path, 0, 100, 'utf8'); number(100, 8, file.mode ?? 0o644);
    number(108, 8, 0); number(116, 8, 0); number(124, 12, data.length); number(136, 12, 1735689600);
    header.fill(32, 148, 156); header.write(file.flag ?? '0', 156); header.write(file.linkname ?? '', 157, 100);
    header.write('ustar\0', 257); header.write('00', 263);
    const checksum = header.reduce((a, b) => a + b, 0);
    header.write(checksum.toString(8).padStart(6, '0') + '\0 ', 148, 8);
    blocks.push(header, data, Buffer.alloc((512 - data.length % 512) % 512));
  }
  return Buffer.concat([...blocks, Buffer.alloc(1024)]);
}
async function mustReject(run) {
  await assert.rejects(run, /outside|escape|link|relative|traversal/i);
}

test('CommonJS adapter remains callable and the default tar/targz readers preserve actual bytes and strip semantics', async () => {
  assert.equal(typeof extract, 'function');
  const content = '合成香港资料\nexact bytes\n', source = tar([entry('package/report.txt', content)]);
  for (const bytes of [source, gzipSync(source)]) {
    const { output } = await area();
    const promise = extract(bytes, output, { strip: 1 }); assert.equal(typeof promise.then, 'function');
    const result = await promise;
    assert.equal(result.length, 1); assert.equal(result[0].path, 'report.txt');
    assert.equal(await readFile(path.join(output, 'report.txt'), 'utf8'), content);
  }
  const memory = await extract(source, { strip: 1 });
  assert.equal(memory[0].path, 'report.txt'); assert.equal(memory[0].data.toString(), content);
});

test('Taro CommonJS template-download boundary extracts a real loopback ZIP through the maintained adapter', async () => {
  const zip = new AdmZip(); zip.addFile('package/report.txt', Buffer.from('actual CJS download fixture'));
  const bytes = zip.toBuffer(), { base } = await area(), output = path.join(base, 'template');
  const server = createServer((request, response) => {
    assert.equal(request.url, '/fixture.zip'); response.writeHead(200, { 'Content-Type': 'application/zip', 'Content-Length': bytes.length }); response.end(bytes);
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  try {
    await download('direct:http://127.0.0.1:' + server.address().port + '/fixture.zip', output, { timeout: 5000 });
    assert.equal(await readFile(path.join(output, 'report.txt'), 'utf8'), 'actual CJS download fixture');
  } finally { await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve())); }
  assert.equal(server.listening, false);
});

test('A sibling whose name starts with the output path cannot receive a traversing archive entry', async () => {
  const { output, outside } = await area();
  await mustReject(() => extract(Buffer.from('synthetic plugin fixture'), output, { plugins: [async () => [entry('../out-old/escaped.txt')]] }));
  assert.equal(await exists(path.join(outside, 'escaped.txt')), false);
});

test('An archive hardlink cannot expose the contents of an outside owned sentinel', async () => {
  const { output, outside } = await area(), sentinel = path.join(outside, 'sentinel.txt');
  await writeFile(sentinel, 'synthetic outside sentinel');
  await mustReject(() => extract(Buffer.from('synthetic plugin fixture'), output, { plugins: [async () => [{ ...entry('exposed.txt'), type: 'link', linkname: sentinel }]] }));
  assert.equal(await exists(path.join(output, 'exposed.txt')), false);
  assert.equal(await readFile(sentinel, 'utf8'), 'synthetic outside sentinel');
});

test('Symlink chains in the archive cannot redirect a later write outside extraction', async () => {
  const { output, outside } = await area();
  const files = [{ ...entry('a'), type: 'symlink', linkname: '../out-old' },
    { ...entry('b'), type: 'symlink', linkname: 'a/sub' }, entry('b/escaped.txt')];
  await mustReject(() => extract(Buffer.from('synthetic plugin fixture'), output, { plugins: [async () => files] }));
  assert.equal(await exists(path.join(outside, 'sub/escaped.txt')), false);
});

test('A pre-existing filesystem junction to an output-prefix sibling cannot redirect extraction', async () => {
  const { output, outside } = await area();
  await symlink(outside, path.join(output, 'bridge'), process.platform === 'win32' ? 'junction' : 'dir');
  await mustReject(() => extract(Buffer.from('synthetic plugin fixture'), output, { plugins: [async () => [entry('bridge/escaped.txt')]] }));
  assert.equal(await exists(path.join(outside, 'escaped.txt')), false);
});

test('Caller mapping cannot introduce an unchecked escape after otherwise valid archive parsing', async () => {
  const { output, outside } = await area();
  await mustReject(() => extract(tar([entry('safe.txt')]), output, { map: file => ({ ...file, path: '../out-old/escaped.txt' }) }));
  assert.equal(await exists(path.join(outside, 'escaped.txt')), false);
});

test('Swiper rejects the published prototype-pollution input even if Array.indexOf is modified', async () => {
  const { default: Swiper } = await import('swiper');
  const original = Array.prototype.indexOf;
  const marker = 'cp_dependency_probe';
  assert.equal(Object.prototype[marker], undefined);
  try {
    Array.prototype.indexOf = () => -1;
    Swiper.extendDefaults(JSON.parse('{"__proto__":{"cp_dependency_probe":"unsafe"}}'));
    assert.equal(Object.prototype[marker], undefined); assert.equal(({})[marker], undefined);
  } finally { Array.prototype.indexOf = original; delete Object.prototype[marker]; }
});

test('Updated JavaScript serialization preserves data while escaping executable HTML text', () => {
  const serialize = require('serialize-javascript');
  const source = { value: '</script><script>synthetic</script>', nested: [1, null, true], separator: '\u2028' };
  const serialized = serialize(source, { isJSON: true });
  assert.equal(serialized.includes('</script>'), false); assert.deepEqual(JSON.parse(serialized), source);
});

test('Updated XML parser and serializer preserve normal manifest data', () => {
  const plistRequire = createRequire(require.resolve('@expo/plist'));
  const { DOMParser, XMLSerializer } = plistRequire('@xmldom/xmldom');
  const doc = new DOMParser().parseFromString('<manifest><name>CopyPaste &amp; HK</name></manifest>', 'application/xml');
  assert.equal(doc.getElementsByTagName('name')[0].textContent, 'CopyPaste & HK');
  assert.match(new XMLSerializer().serializeToString(doc), /CopyPaste &amp; HK/);
});


test('Updated node-tar extracts normal bounded TAR/GZIP bytes with strip semantics', async () => {
  const nodeTar = require('tar'), content = 'synthetic current tar fixture';
  const { output } = await area();
  await pipeline(Readable.from([gzipSync(tar([entry('package/normal.txt', content)]))]), nodeTar.x({ cwd: output, strip: 1 }));
  assert.equal(await readFile(path.join(output, 'normal.txt'), 'utf8'), content);
});

test('Updated node-tar skips archive parent traversal and external hardlinks', async () => {
  const nodeTar = require('tar'), { output, outside } = await area(), warnings = [];
  const sentinel = path.join(outside, 'sentinel.txt'); await writeFile(sentinel, 'synthetic protected bytes');
  const archive = tar([entry('../out-old/escaped.txt'), { ...entry('exposed.txt'), data: Buffer.alloc(0), flag: '1', linkname: '../out-old/sentinel.txt' }, entry('normal.txt')]);
  await pipeline(Readable.from([archive]), nodeTar.x({ cwd: output, onwarn: (code, message) => warnings.push({ code, message }) }));
  assert.equal(await exists(path.join(outside, 'escaped.txt')), false);
  assert.equal(await exists(path.join(output, 'exposed.txt')), false);
  assert.equal(await readFile(sentinel, 'utf8'), 'synthetic protected bytes');
  assert.equal(await readFile(path.join(output, 'normal.txt'), 'utf8'), 'synthetic payload');
  assert.equal(warnings.length, 2); assert.ok(warnings.every(warning => warning.code === 'TAR_ENTRY_ERROR'));
});
