import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { mkdtemp, mkdir, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { request } from 'node:http';

const app = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(path.join(app, 'package.json'));
const taroRequire = createRequire(require.resolve('@tarojs/webpack5-runner/package.json'));
const simulateRequire = createRequire(require.resolve('miniprogram-simulate/package.json'));
const minifier = taroRequire('html-minifier');
const Server = taroRequire('webpack-dev-server');
const webpack = require('webpack');
const local = await realpath(path.join(app, '.local'));
const workspace = await mkdtemp(path.join(local, 'dependency-toolchain-'));

after(async () => {
  const target = await realpath(workspace);
  assert.equal(path.dirname(target), local);
  assert.ok(path.basename(target).startsWith('dependency-toolchain-'));
  await rm(target, { recursive: true, force: true });
});

test('Installed Taro consumers resolve the named adapters and maintained upstream implementations', async () => {
  for (const [name, expected] of [['html-minifier', '@copypaste/template-minifier'], ['webpack-dev-server', '@copypaste/dev-server-compat']]) {
    const pkg = taroRequire(name + '/package.json');
    assert.equal(pkg.name, expected); assert.equal(pkg.version, '0.1.0');
  }
  const adapterRequire = createRequire(taroRequire.resolve('webpack-dev-server'));
  const upstream = adapterRequire('@copypaste/dev-server-upstream/package.json');
  assert.equal(upstream.name, 'webpack-dev-server'); assert.equal(upstream.version, '6.1.0');
  const templateRequire = createRequire(taroRequire.resolve('html-minifier'));
  const html = templateRequire('@copypaste/html-minifier-upstream/package.json');
  assert.equal(html.name, 'html-minifier-terser'); assert.equal(html.version, '7.2.0');
  assert.equal(simulateRequire('postcss/package.json').version, '8.5.32');
  const mobileRequire = createRequire(path.join(app, 'mobile/package.json'));
  assert.equal(mobileRequire.resolve('@tarojs/plugin-framework-react'), require.resolve('@tarojs/plugin-framework-react'));
  const frameworkRequire = createRequire(require.resolve('@tarojs/plugin-framework-react'));
  assert.equal(frameworkRequire('@pmmmwh/react-refresh-webpack-plugin/package.json').version, '0.6.3');
  assert.equal(typeof frameworkRequire('@pmmmwh/react-refresh-webpack-plugin'), 'function');
  assert.ok(taroRequire.resolve('webpack-dev-server/client/socket').endsWith(path.join('client', 'socket.js')));
  assert.equal(mobileRequire('vite/package.json').version, '8.3.5');
  const reactPlugin = path.resolve(path.dirname(mobileRequire.resolve('@vitejs/plugin-react')), '..', 'package.json');
  assert.equal(JSON.parse(await readFile(reactPlugin, 'utf8')).version, '6.1.2');
});

test('Actual synchronous MiniPlugin generation preserves Chinese text, directives, bindings and closing slashes', () => {
  const { default: MiniPlugin } = taroRequire('./dist/plugins/MiniPlugin.js');
  const input = '<view wx:if="{{ready}}" data-title="你好">  <text>{{ label }}  A   B</text> <image src="{{url}}" /> </view>';
  const compilation = { assets: {} };
  const context = {
    options: { combination: { config: { minifyXML: { collapseWhitespace: true } } } },
    getTemplatePath: value => value + '.wxml', getComponentName: () => 'synthetic-page',
  };
  MiniPlugin.prototype.generateTemplateFile.call(context, compilation, { webpack }, 'synthetic.tsx', () => input);
  const output = compilation.assets['synthetic-page.wxml'].source();
  assert.equal(typeof output, 'string'); assert.ok(!output.includes('[object Promise]'));
  assert.match(output, /wx:if="\{\{ready\}\}"/); assert.match(output, /data-title="你好"/);
  assert.match(output, /\{\{ label \}\} A B/); assert.match(output, /<image src="\{\{url\}\}"\s*\/>/);
});

test('Both WeChat and Douyin templates retain conditionals, loops and literal preformatted whitespace', () => {
  for (const prefix of ['wx', 'tt']) {
    const input = `<view ${prefix}:for="{{items}}" ${prefix}:key="id">  <text>{{item.title}}</text> </view><pre> A   B\n C </pre>`;
    const output = minifier.minify(input, { collapseWhitespace: true, keepClosingSlash: true });
    assert.equal(typeof output, 'string'); assert.ok(output.includes(`${prefix}:for="{{items}}"`));
    assert.ok(output.includes(`${prefix}:key="id"`)); assert.ok(output.includes('<pre> A   B\n C </pre>'));
  }
});

test('Template adapter rejects unrepresentable options and excessive inputs before starting a child', () => {
  for (const input of [null, Buffer.from('x'), {}]) assert.throws(() => minifier.minify(input), TypeError);
  for (const options of [null, [], Object.create(null), { collapseWhitespace: 1 }, { ignoreCustomFragments: [/x/] }, { minifyJS: true }]) {
    assert.throws(() => minifier.minify('<view/>', options), TypeError);
  }
  const options = {}; Object.defineProperty(options, 'collapseWhitespace', { get() { throw new Error('Getter must not run.'); } });
  assert.throws(() => minifier.minify('<view/>', options), /Unsupported template/);
  assert.throws(() => minifier.minify('x'.repeat(8 * 1024 * 1024 + 1)), RangeError);
});

test('Malformed and former custom-ignore patterns complete within the bounded worker', () => {
  const payload = '<!--!'.repeat(1500) + ' '.repeat(5000) + '<view title="{{value}}"> x </view>';
  let result;
  try { result = minifier.minify(payload, { collapseWhitespace: true, keepClosingSlash: true }); }
  catch (error) { assert.notEqual(error.code, 'ETIMEDOUT'); assert.equal(error.message, 'Template minification failed.'); return; }
  assert.equal(typeof result, 'string'); assert.ok(result.length > 0);
});

test('Actual miniprogram-simulate WXSS compile works with PostCSS 8 and unchanged namespace/rpx behavior', () => {
  const wxss = simulateRequire('./src/wxss.js');
  const output = wxss.compile('.card .item { width: 10rpx; color: red; } @media (min-width: 1px) { .active { display: block; } }', { prefix: 'synthetic' });
  assert.ok(output.includes('.synthetic--card .synthetic--item'));
  assert.ok(output.includes('width:10rpx')); assert.ok(output.includes('color:red'));
  assert.ok(output.includes('@media')); assert.ok(output.includes('.synthetic--active'));
  assert.throws(() => wxss.compile('.broken { color: red;'), /Unclosed block/);
});

test('Legacy server TLS/proxy options translate without mutating inputs or silently losing callbacks', () => {
  const fn = () => {};
  const input = { host: '127.0.0.1', https: false, proxy: [{ context: ['/api'], target: 'http://127.0.0.1:12345', onProxyReq: fn }] };
  const result = Server.normalizeOptions(input);
  assert.equal(result.server, 'http'); assert.ok(!Object.hasOwn(result, 'https'));
  assert.deepEqual(result.proxy[0].pathFilter, ['/api']); assert.equal(result.proxy[0].on.proxyReq, fn);
  assert.ok(!Object.hasOwn(result.proxy[0], 'context')); assert.equal(input.proxy[0].onProxyReq, fn);
  assert.equal(input.https, false);
  assert.deepEqual(Server.normalizeOptions({ https: { key: 'synthetic', cert: 'synthetic' } }).server, { type: 'https', options: { key: 'synthetic', cert: 'synthetic' } });
});

test('Local server rejects unrestricted hosts and legacy behaviors that require an explicit migration', () => {
  for (const input of [{ host: '0.0.0.0' }, { host: 'local-ip' }, { allowedHosts: 'all' }, { allowedHosts: ['.example.com'] },
    { allowedHosts: [] }, { https: true, server: 'http' }, { proxy: [{ context: '/api', bypass() {} }] },
    { proxy: { '/api': 'http://127.0.0.1' } }, { onBeforeSetupMiddleware() {} }, { proxy: [{ context: '/a', pathFilter: '/b' }] }]) {
    assert.throws(() => Server.normalizeOptions(input));
  }
});

function http(port, route, headers = {}) {
  return new Promise((resolve, reject) => {
    const req = request({ host: '127.0.0.1', port, path: route, headers }, response => {
      const chunks = []; response.on('data', data => chunks.push(data));
      response.on('end', () => resolve({ status: response.statusCode, headers: response.headers, text: Buffer.concat(chunks).toString('utf8') }));
    });
    req.setTimeout(10000, () => req.destroy(new Error('Synthetic HTTP request timed out.')));
    req.on('error', reject); req.end();
  });
}

test('Modern server compiles and serves webpack assets, rejects hostile Host/traversal and closes its resources', { timeout: 300000 }, async () => {
  const directory = path.join(workspace, 'server'); await mkdir(path.join(directory, 'public'), { recursive: true });
  await writeFile(path.join(directory, 'entry.js'), 'import { client } from "webpack-dev-server/client/socket"; console.log("synthetic-toolchain-asset", client);');
  await writeFile(path.join(directory, 'public/index.html'), '<!doctype html><title>Synthetic toolchain page</title>');
  await writeFile(path.join(directory, 'private.txt'), 'SYNTHETIC_PRIVATE_CANARY_DO_NOT_SERVE');
  const compiler = webpack({ mode: 'development', target: 'web', devtool: false, entry: path.join(directory, 'entry.js'),
    output: { path: path.join(directory, 'output'), publicPath: '/assets', filename: 'main.js' },
    infrastructureLogging: { level: 'error' }, stats: 'errors-only' });
  const compiled = new Promise((resolve, reject) => {
    compiler.hooks.done.tap('SyntheticToolchain', stats => stats.hasErrors() ? reject(new Error(stats.toString('errors-only'))) : resolve());
    compiler.hooks.failed.tap('SyntheticToolchain', reject);
  });
  const server = new Server({ host: '127.0.0.1', port: 0, https: false, allowedHosts: ['localhost', '127.0.0.1'],
    hot: false, client: false, static: [{ directory: path.join(directory, 'public'), publicPath: '/' }],
    devMiddleware: { publicPath: '/assets', stats: false }, historyApiFallback: { index: '/index.html' },
    setupMiddlewares(middlewares, current) {
      current.app.get('/synthetic-api', (_request, response) => response.json({ synthetic: true })); return middlewares;
    } }, compiler);
  let port, listener;
  try {
    await server.start(); listener = server.server; port = listener.address().port; await compiled;
    const asset = await http(port, '/assets/main.js'); assert.equal(asset.status, 200); assert.match(asset.text, /synthetic-toolchain-asset/);
    const page = await http(port, '/'); assert.equal(page.status, 200); assert.match(page.text, /Synthetic toolchain page/);
    const api = await http(port, '/synthetic-api'); assert.equal(api.status, 200); assert.deepEqual(JSON.parse(api.text), { synthetic: true });
    for (const host of ['untrusted.invalid', 'untrusted.invalid:80', '[]']) {
      const denied = await http(port, '/assets/main.js', { host }); assert.ok([400, 403].includes(denied.status));
      assert.ok(!denied.text.includes('synthetic-toolchain-asset'));
    }
    const crossOrigin = await http(port, '/assets/main.js', { origin: 'http://untrusted.invalid' });
    assert.ok([400, 403].includes(crossOrigin.status) ||
      !['*', 'http://untrusted.invalid'].includes(crossOrigin.headers['access-control-allow-origin']));
    for (const route of ['/assets../private.txt', '/assets/%2e%2e/private.txt', '/assets../%2e%2e/private.txt']) {
      const denied = await http(port, route); assert.ok(!denied.text.includes('SYNTHETIC_PRIVATE_CANARY_DO_NOT_SERVE'));
    }
  } finally {
    await server.stop(); await new Promise((resolve, reject) => compiler.close(error => error ? reject(error) : resolve()));
  }
  assert.ok(!listener.listening);
  if (port) await assert.rejects(http(port, '/'), /ECONNREFUSED|socket hang up/);
  assert.equal(await readFile(path.join(directory, 'private.txt'), 'utf8'), 'SYNTHETIC_PRIVATE_CANARY_DO_NOT_SERVE');
});
