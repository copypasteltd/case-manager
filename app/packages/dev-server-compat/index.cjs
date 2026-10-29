'use strict';

const [major, minor] = process.versions.node.split('.').map(Number);
if (major < 22 || (major === 22 && minor < 15)) throw new Error('The project development server requires Node.js >=22.15.0.');
const loaded = require('@copypaste/dev-server-upstream');
const ModernServer = loaded.default || loaded;
const loopback = new Set(['127.0.0.1', 'localhost', '::1']);

function normalizeOptions(input = {}) {
  if (!input || Object.getPrototypeOf(input) !== Object.prototype) throw new TypeError('Development server options must be a plain object.');
  const options = { ...input };
  options.host ??= '127.0.0.1';
  if (!loopback.has(options.host)) throw new Error('The local development server requires a loopback host.');
  if (options.allowedHosts === 'all') throw new Error('Unrestricted development-server hosts are not supported.');
  options.allowedHosts ??= ['localhost', '127.0.0.1', '[::1]'];
  if (options.allowedHosts !== 'auto' && (!Array.isArray(options.allowedHosts) || options.allowedHosts.length === 0 ||
      options.allowedHosts.some(host => !['localhost', '127.0.0.1', '[::1]', '::1'].includes(host)))) {
    throw new Error('The local development server only accepts explicit loopback hosts.');
  }
  if (Object.hasOwn(options, 'https')) {
    const legacy = options.https;
    delete options.https;
    if (legacy === false) options.server ??= 'http';
    else if (legacy === true || (legacy && typeof legacy === 'object' && !Array.isArray(legacy))) {
      if (options.server !== undefined) throw new Error('Conflicting development-server TLS options.');
      options.server = { type: 'https', ...(legacy === true ? {} : { options: legacy }) };
    } else throw new TypeError('Unsupported legacy HTTPS option.');
  }
  for (const name of ['onBeforeSetupMiddleware', 'onAfterSetupMiddleware']) {
    if (options[name] !== undefined) throw new Error(name + ' must be migrated to setupMiddlewares.');
  }
  if (options.proxy !== undefined) {
    if (!Array.isArray(options.proxy)) throw new TypeError('Proxy configuration must be an array.');
    options.proxy = options.proxy.map(value => {
      if (typeof value === 'function') throw new TypeError('Dynamic legacy proxy configuration is unsupported.');
      if (!value || typeof value !== 'object' || value.bypass !== undefined) throw new Error('Proxy bypass must be migrated explicitly.');
      const proxy = { ...value };
      if (Object.hasOwn(proxy, 'context')) {
        if (Object.hasOwn(proxy, 'pathFilter')) throw new Error('Conflicting proxy path filters.');
        proxy.pathFilter = proxy.context; delete proxy.context;
      }
      for (const [oldName, newName] of [['onProxyReq', 'proxyReq'], ['onProxyRes', 'proxyRes'], ['onError', 'error'], ['onProxyReqWs', 'proxyReqWs'], ['onOpen', 'open'], ['onClose', 'close']]) {
        if (Object.hasOwn(proxy, oldName)) {
          if (proxy.on?.[newName] !== undefined) throw new Error('Conflicting proxy event handlers.');
          proxy.on = { ...proxy.on, [newName]: proxy[oldName] }; delete proxy[oldName];
        }
      }
      return proxy;
    });
  }
  return options;
}

class CompatibleServer extends ModernServer {
  constructor(options, compiler) { super(normalizeOptions(options), compiler); }
}
CompatibleServer.normalizeOptions = normalizeOptions;
module.exports = CompatibleServer;
