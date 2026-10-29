import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { createRequire } from 'node:module';
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import { createServer, request } from 'node:http';
import { readFileSync, existsSync } from 'node:fs';
import { mkdtemp, mkdir, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(path.join(app, 'package.json'));
const from = name => createRequire(path.join(app, 'node_modules', name, 'package.json'));
const merge = require('deepmerge');
const decode = require('decode-uri-component');
const local = await realpath(path.join(app, '.local'));
const workspace = await mkdtemp(path.join(local, 'dependency-integrity-'));

function info(loader, name) {
  let directory = path.dirname(loader.resolve(name));
  while (directory !== path.dirname(directory)) {
    const file = path.join(directory, 'package.json');
    if (existsSync(file)) {
      const metadata = JSON.parse(readFileSync(file, 'utf8'));
      // uuid's dist/cjs/package.json describes only the module type. Keep
      // walking to the package manifest that actually identifies the code.
      if (metadata.name && metadata.version) return metadata;
    }
    directory = path.dirname(directory);
  }
  throw new Error('Missing installed package metadata: ' + name);
}

after(async () => {
  const target = await realpath(workspace);
  assert.equal(path.dirname(target), local);
  assert.ok(path.basename(target).startsWith('dependency-integrity-'));
  await rm(target, { recursive: true, force: true });
});

test('All seven advisory consumers resolve declared implementations and patched upstream versions', () => {
  assert.equal(info(require, 'deepmerge').name, '@copypaste/deepmerge-safe');
  assert.equal(info(require, 'decode-uri-component').name, '@copypaste/uri-decoder-compat');
  for (const name of ['@react-native-community/cli-config', '@react-native-community/cli-doctor',
    '@react-native-community/cli', '@svgr/plugin-svgo', 'webpack-chain']) {
    assert.equal(info(from(name), 'deepmerge').name, '@copypaste/deepmerge-safe');
  }
  for (const name of ['@tarojs/router', '@tarojs/taro-h5', 'source-map-resolve']) {
    assert.equal(info(from(name), 'decode-uri-component').name, '@copypaste/uri-decoder-compat');
  }
  const rnQuery = createRequire(from('@tarojs/router-rn').resolve('query-string'));
  assert.equal(info(rnQuery, 'decode-uri-component').name, '@copypaste/uri-decoder-compat');
  const uri = createRequire(require.resolve('decode-uri-component'));
  assert.equal(info(uri, '@copypaste/uri-decoder-upstream').version, '0.5.1');
  assert.equal(info(require, '@babel/core').version, '7.29.7');
  assert.equal(info(from('@tarojs/helper'), 'esbuild').version, '0.28.2');
  assert.equal(info(from('@tarojs/webpack5-runner'), 'esbuild').version, '0.28.2');
  assert.equal(info(from('@react-native-community/cli-platform-android'), 'fast-xml-parser').version, '5.7.0');
  assert.equal(info(from('@expo/cli'), 'send').version, '0.19.2');
  for (const name of ['xcode', '@expo/bunyan', '@expo/rudder-sdk-node']) {
    assert.equal(info(from(name), 'uuid').version, '11.1.1');
  }
});

test('Prototype-poisoning targets cannot change the returned object prototype at any merged depth', () => {
  const target = JSON.parse('{"__proto__":{"syntheticInherited":true},"nested":{"__proto__":{"syntheticNested":true}}}');
  const result = merge(target, { nested: {} });
  for (const [object, field] of [[result, 'syntheticInherited'], [result.nested, 'syntheticNested']]) {
    assert.equal(Object.getPrototypeOf(object), Object.prototype);
    assert.equal(object[field], undefined);
    assert.equal(Object.hasOwn(Object.prototype, field), false);
  }
  assert.ok(Object.hasOwn(target, '__proto__'));
  assert.equal(Object.getPrototypeOf(target), Object.prototype);
});

test('Source, null-prototype and constructor payloads remain data without poisoning global prototypes', () => {
  const payload = JSON.parse('{"__proto__":{"syntheticInherited":true},"constructor":{"prototype":{"syntheticInherited":true}}}');
  const nullTarget = Object.create(null);
  Object.defineProperty(nullTarget, '__proto__', { value: { syntheticInherited: true }, enumerable: true });
  for (const output of [merge({}, payload), merge(nullTarget, {}), merge(payload, payload)]) {
    assert.equal(Object.getPrototypeOf(output), Object.prototype);
    assert.equal(output.syntheticInherited, undefined);
    assert.equal(Object.hasOwn(Object.prototype, 'syntheticInherited'), false);
  }
});

test('Deep merge preserves arrays, clone options, symbols, special values and custom callbacks', () => {
  const symbol = Symbol('synthetic');
  const date = new Date('2020-01-01T00:00:00Z');
  const regexp = /synthetic/;
  const left = { a: [{ value: 1 }], nested: { left: true }, date, regexp, [symbol]: { left: true } };
  const right = { a: [{ value: 2 }], nested: { right: true }, [symbol]: { right: true } };
  const output = merge(left, right);
  assert.deepEqual(output.a, [{ value: 1 }, { value: 2 }]);
  assert.notEqual(output.a[0], left.a[0]);
  assert.deepEqual(output.nested, { left: true, right: true });
  assert.deepEqual(output[symbol], { left: true, right: true });
  assert.equal(output.date, date); assert.equal(output.regexp, regexp);
  assert.equal(merge(left, {}, { clone: false }).nested, left.nested);
  assert.deepEqual(merge({ a: [1] }, { a: [2] }, { arrayMerge: (_target, source) => source }), { a: [2] });
  assert.deepEqual(merge({ nested: { x: 1 } }, { nested: { x: 2 } }, {
    customMerge: key => key === 'nested' ? (a, b) => ({ total: a.x + b.x }) : undefined,
  }), { nested: { total: 3 } });
  assert.deepEqual(merge.all([{ a: 1 }, { b: 2 }]), { a: 1, b: 2 });
  assert.throws(() => merge.all({}), /array/);
});

test('Actual React Native CLI config merging retains its array replacement contract', () => {
  const { default: mergeConfig } = from('@react-native-community/cli-config')('./build/merge.js');
  const first = { platforms: { android: { sourceDir: 'one', libraries: ['old'] } }, commands: ['one'] };
  const second = { platforms: { android: { libraries: ['new'] } }, commands: ['two'] };
  const output = mergeConfig(first, second);
  assert.deepEqual(output, { platforms: { android: { sourceDir: 'one', libraries: ['new'] } }, commands: ['two'] });
  assert.deepEqual(first.commands, ['one']);
});

test('Actual webpack-chain merging preserves entries, aliases, extensions and optimization options', () => {
  const Config = require('webpack-chain');
  const config = new Config();
  config.merge({ mode: 'development', entry: { main: ['./synthetic.js'] }, resolve: { alias: { '@synthetic': workspace }, extensions: ['.ts', '.js'] }, optimization: { minimize: false } });
  config.merge({ resolve: { alias: { '@second': app } } });
  const output = config.toConfig();
  assert.deepEqual(output.entry.main, ['./synthetic.js']);
  assert.deepEqual(output.resolve.extensions, ['.ts', '.js']);
  assert.equal(output.resolve.alias['@synthetic'], workspace);
  assert.equal(output.resolve.alias['@second'], app);
  assert.equal(output.optimization.minimize, false);
});

test('The actual SVGR optimization consumer retains SVG viewBox and visible content', () => {
  const optimize = require('@svgr/plugin-svgo');
  const input = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10H0z"/></svg>';
  const output = optimize(input, { svgo: true, dimensions: false, runtimeConfig: false }, { filePath: path.join(workspace, 'synthetic.svg') });
  assert.match(output, /viewBox="0 0 10 10"/);
  assert.match(output, /<path|<rect/);
});

test('URI bridge supports both CJS and ESM defaults and normal Unicode without changing plus handling', async () => {
  const imported = await import(pathToFileURL(require.resolve('decode-uri-component')).href);
  assert.equal(imported.default, decode);
  assert.equal(decode('%E9%9B%AA%20%F0%9F%8C%8D'), '雪 🌍');
  assert.equal(decode('a+b%2Bc'), 'a+b+c');
  assert.throws(() => decode({}), TypeError);
  for (const loader of [from('@react-navigation/core'), from('@tarojs/router')]) {
    const loaded = await import(pathToFileURL(loader.resolve('query-string')).href);
    const parser = loaded.default || loaded;
    const output = parser.parse('name=%E9%9B%AA&plus=a+b&repeated=1&repeated=2');
    assert.equal(output.name, '雪'); assert.equal(output.plus, 'a b');
    assert.deepEqual(output.repeated, ['1', '2']);
  }
});

test('The source-map-resolve CJS consumer decodes an encoded map filename and loads its actual contents', () => {
  const resolve = require('source-map-resolve');
  const filename = 'https://synthetic.invalid/assets/source.js';
  const expected = 'https://synthetic.invalid/assets/map 雪.json';
  let readPath;
  const output = resolve.resolveSourceMapSync('let x = 1;\n//# sourceMappingURL=map%20%E9%9B%AA.json', filename, name => {
    readPath = name;
    return JSON.stringify({ version: 3, file: 'source.js', sources: ['original.ts'], names: [], mappings: '' });
  });
  assert.equal(readPath, expected);
  assert.equal(output.map.version, 3);
  assert.deepEqual(output.map.sources, ['original.ts']);
});

test('Malformed URI input completes in a bounded child and retains valid decoded runs', () => {
  const source = `const decode=require(${JSON.stringify(require.resolve('decode-uri-component'))}); const input='%C0'.repeat(50000)+'%E9%9B%AA'; const output=decode(input); console.log(JSON.stringify({length:output.length,tail:output.slice(-1)}));`;
  const result = spawnSync(process.execPath, ['-e', source], { cwd: app, encoding: 'utf8', timeout: 60000, maxBuffer: 8192, windowsHide: true, env: { ...process.env, NODE_OPTIONS: '' } });
  assert.equal(result.error, undefined); assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout), { length: 150001, tail: '雪' });
});

test('Actual Android and iOS CLI XML readers retain manifest and scheme selection', async () => {
  const android = from('@react-native-community/cli-platform-android')('./build/config/getMainActivity.js').default;
  const manifest = path.join(workspace, 'AndroidManifest.xml');
  await writeFile(manifest, '<manifest xmlns:android="http://schemas.android.com/apk/res/android"><application><activity android:name=".Other"/><activity android:name=".MainActivity"><intent-filter><action android:name="android.intent.action.MAIN"/><category android:name="android.intent.category.LAUNCHER"/></intent-filter></activity></application></manifest>');
  assert.equal(android(manifest), '.MainActivity');
  const ios = from('@react-native-community/cli-platform-ios')('./build/tools/getBuildConfigurationFromXcScheme.js').getBuildConfigurationFromXcScheme;
  const folder = path.join(workspace, 'Synthetic.xcodeproj/xcshareddata/xcschemes');
  await mkdir(folder, { recursive: true });
  await writeFile(path.join(folder, 'Synthetic.xcscheme'), '<Scheme><LaunchAction buildConfiguration="Release"/></Scheme>');
  assert.equal(ios('Synthetic', 'Debug', workspace, { schemes: ['Synthetic'] }), 'Release');
});

test('Patched XML builder cannot inject elements through comment and CDATA delimiters', () => {
  const { XMLBuilder, XMLParser, XMLValidator } = require('fast-xml-parser');
  const parser = new XMLParser({ ignoreAttributes: false });
  const normal = new XMLBuilder({ commentPropName: '#comment', cdataPropName: '#cdata' })
    .build({ root: { '#comment': 'ordinary comment', '#cdata': 'text < & >', ordinary: 'value' } });
  assert.equal(XMLValidator.validate(normal), true);
  assert.equal(parser.parse(normal).root.ordinary, 'value');
  for (const [field, value] of [['#comment', '--><injected/> <!--'], ['#cdata', 'before]]><injected/><![CDATA[after']]) {
    const builder = new XMLBuilder({ commentPropName: '#comment', cdataPropName: '#cdata' });
    let output;
    try { output = builder.build({ root: { [field]: value, ordinary: '正常 & value' } }); }
    catch (error) { assert.match(error.message, /CDATA|comment|delimiter|--|\]\]>/i); continue; }
    assert.equal(XMLValidator.validate(output), true);
    const parsed = parser.parse(output);
    assert.equal(parsed.root.injected, undefined);
    assert.equal(parsed.root.ordinary, '正常 & value');
  }
  assert.equal(XMLValidator.validate('<root><unclosed></root>') === true, false);
});

test('UUID consumers retain valid generation and the patched namespace-buffer bounds', () => {
  const uuid = require('uuid');
  const xcode = require('xcode');
  const project = xcode.project(path.join(workspace, 'synthetic.pbxproj'));
  project.hash = { project: { objects: {} } };
  assert.match(project.generateUuid(), /^[A-F0-9]{24}$/);
  for (const loader of [from('@expo/bunyan'), from('@expo/rudder-sdk-node')]) {
    const current = loader('uuid');
    assert.equal(current.validate(current.v4()), true);
  }
  for (const name of ['v3', 'v5']) {
    const generated = uuid[name]('synthetic.example', uuid[name].DNS);
    const buffer = Buffer.alloc(16);
    assert.equal(uuid[name]('synthetic.example', uuid[name].DNS, buffer), buffer);
    assert.equal(buffer.toString('hex'), generated.replaceAll('-', ''));
    assert.throws(() => uuid[name]('synthetic.example', uuid[name].DNS, Buffer.alloc(15)), RangeError);
    assert.throws(() => uuid[name]('synthetic.example', uuid[name].DNS, Buffer.alloc(16), 1), RangeError);
  }
  assert.throws(() => uuid.v6({}, Buffer.alloc(15)), RangeError);
});

test('Babel reads legitimate source maps but refuses an input comment outside the package root', async () => {
  const babel = require('@babel/core');
  const project = path.join(workspace, 'babel-project');
  const source = path.join(project, 'src', 'source.js');
  const outside = path.join(workspace, 'outside.map');
  const inside = source + '.map';
  await mkdir(path.dirname(source), { recursive: true });
  await writeFile(path.join(project, 'package.json'), '{"name":"synthetic-map-package"}');
  const map = marker => JSON.stringify({ version: 3, sources: ['original.ts'], sourcesContent: [marker], names: [], mappings: 'AAAA' });
  await writeFile(outside, map('OUTSIDE_SYNTHETIC_SOURCE_MAP'));
  await writeFile(inside, map('INSIDE_SYNTHETIC_SOURCE_MAP'));
  const options = { filename: source, cwd: project, root: project, configFile: false, babelrc: false, sourceMaps: true };
  const original = fs.readFileSync;
  const reads = [];
  fs.readFileSync = function (name, ...args) {
    if (typeof name === 'string') reads.push(path.resolve(name));
    return original.call(this, name, ...args);
  };
  try {
    const rejected = babel.transformSync('const value = 1;\n//# sourceMappingURL=../../outside.map', options);
    assert.ok(!reads.includes(outside));
    assert.ok(!JSON.stringify(rejected.map).includes('OUTSIDE_SYNTHETIC_SOURCE_MAP'));
    const accepted = babel.transformSync('const value = 1;\n//# sourceMappingURL=source.js.map', options);
    assert.ok(reads.includes(inside));
    assert.ok(JSON.stringify(accepted.map).includes('INSIDE_SYNTHETIC_SOURCE_MAP'));
  } finally { fs.readFileSync = original; }
});

test('Actual Taro esbuild loader retains TypeScript, alias and define configuration', async () => {
  const directory = path.join(workspace, 'esbuild'); await mkdir(directory);
  await writeFile(path.join(directory, 'dependency.ts'), 'export const value: number = 42;');
  const filename = path.join(directory, 'config.ts');
  await writeFile(filename, 'import { value } from "@synthetic"; export default { value, enabled: __SYNTHETIC__ };');
  const loader = from('@tarojs/helper')('./dist/esbuild/index.js');
  const output = loader.requireWithEsbuild(filename, {
    cwd: directory,
    customConfig: { alias: { '@synthetic': path.join(directory, 'dependency.ts') }, define: { __SYNTHETIC__: 'true' } },
    customSwcConfig: { module: { type: 'commonjs' } },
  });
  assert.deepEqual(output.default, { value: 42, enabled: true });
  const built = loader.esbuild.transformSync('const label: string = "雪"; console.log(label);', { loader: 'ts', minify: true, target: 'es2018' });
  assert.match(built.code, /console\.log/);
  assert.ok(!built.code.includes(': string'));
});

function http(port, route) {
  return new Promise((resolve, reject) => {
    const req = request({ hostname: '127.0.0.1', port, path: route }, response => {
      const chunks = []; response.on('data', value => chunks.push(value));
      response.on('end', () => resolve({ status: response.statusCode, headers: response.headers, text: Buffer.concat(chunks).toString('utf8') }));
    });
    req.setTimeout(60000, () => req.destroy(new Error('Synthetic static request timed out.')));
    req.on('error', reject); req.end();
  });
}

test('Actual Expo static middleware serves files and send escapes directory redirects without outside reads', { timeout: 300000 }, async () => {
  const directory = path.join(workspace, 'expo');
  await mkdir(path.join(directory, 'public', 'directory'), { recursive: true });
  await writeFile(path.join(directory, 'public', 'file 雪.txt'), 'synthetic static content');
  await writeFile(path.join(directory, 'outside.txt'), 'SYNTHETIC_OUTSIDE_CANARY');
  const { ServeStaticMiddleware } = from('@expo/cli')('./build/src/start/server/middleware/ServeStaticMiddleware.js');
  const handler = new ServeStaticMiddleware(directory).getHandler();
  const server = createServer((req, res) => handler(req, res, error => {
    res.statusCode = error?.statusCode || 404; res.end('Synthetic fallthrough');
  }));
  let port;
  try {
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    port = server.address().port;
    const ordinary = await http(port, '/file%20%E9%9B%AA.txt');
    assert.equal(ordinary.status, 200); assert.equal(ordinary.text, 'synthetic static content');
    const redirect = await http(port, '/directory');
    assert.ok([301, 302, 307, 308].includes(redirect.status));
    assert.equal(redirect.headers.location, '/directory/');
    // Windows cannot create filenames containing angle brackets. Exercise the
    // real redirect formatter directly, alongside the actual HTTP file route.
    const headers = {}; let body;
    const stream = from('@expo/cli')('send')({ headers: {} }, '/<synthetic>');
    stream.res = { setHeader(name, value) { headers[name] = value; }, end(value) { body = value; } };
    stream.redirect('/<synthetic>');
    assert.equal(stream.res.statusCode, 301);
    assert.ok(!body.includes('<synthetic>'));
    assert.ok(!headers.Location.includes('<'));
    const traversal = await http(port, '/%2e%2e/outside.txt');
    assert.ok(!traversal.text.includes('SYNTHETIC_OUTSIDE_CANARY'));
  } finally {
    server.closeAllConnections();
    await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  }
  assert.equal(server.listening, false);
  await assert.rejects(http(port, '/'), /ECONNREFUSED|socket hang up/);
});
