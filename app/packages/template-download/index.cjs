'use strict';

const path = require('node:path');
const fs = require('node:fs/promises');
const { execFile } = require('node:child_process');
const { promisify } = require('node:util');
const extract = require('@copypaste/archive-extraction');
const gitExec = promisify(execFile);
const archiveLimit = 256 * 1024 * 1024;
const supported = new Set(['clone', 'checkout', 'shallow', 'git', 'headers', 'timeout']);

function text(value, name) {
  if (typeof value !== 'string' || !value || value.length > 4096 || /[\0\r\n]/.test(value)) throw new TypeError('Invalid ' + name);
  return value;
}
function source(repo, clone) {
  text(repo, 'template repository');
  const hash = repo.indexOf('#');
  const ref = hash < 0 ? 'master' : repo.slice(hash + 1);
  text(ref, 'template revision');
  if (ref.startsWith('-') || ref.includes('#')) throw new TypeError('Invalid template revision');
  let input = hash < 0 ? repo : repo.slice(0, hash);
  if (input.startsWith('direct:')) return { url: text(input.slice(7), 'direct repository'), ref };
  let provider = 'github';
  const prefix = /^(github|gitlab|bitbucket):/.exec(input);
  if (prefix) { provider = prefix[1]; input = input.slice(prefix[0].length); }
  const match = /^(?:(.+):)?([^/]+)\/([^#]+)$/.exec(input);
  if (!match) throw new TypeError('Expected provider:owner/repository#revision');
  const host = match[1] || { github: 'github.com', gitlab: 'gitlab.com', bitbucket: 'bitbucket.org' }[provider];
  const owner = match[2], name = match[3];
  if (!/^[\w.-]+$/.test(owner) || !/^[\w./-]+$/.test(name) || name.split('/').some(x => !x || x === '..' || x === '.')) throw new TypeError('Invalid template repository path');
  const base = /^[a-z]+:\/\//i.test(host) ? host.replace(/\/$/, '') : (clone ? 'git@' + host + ':' : 'https://' + host);
  const root = base + (base.endsWith(':') ? '' : '/') + owner + '/' + name;
  const encoded = encodeURIComponent(ref);
  return { ref, url: clone ? root + '.git' : provider === 'github' ? root + '/archive/' + encoded + '.zip' : provider === 'gitlab' ? root + '/repository/archive.zip?ref=' + encoded : root + '/get/' + encoded + '.zip' };
}
function httpURL(input) {
  const url = new URL(input);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) throw new TypeError('Template archives require an HTTP(S) URL without embedded credentials');
  return url;
}
async function archive(url, options, timeout) {
  let target = httpURL(url), headers = new Headers(options.headers || {});
  const signal = AbortSignal.timeout(timeout);
  for (let redirects = 0; redirects <= 8; redirects++) {
    const response = await fetch(target, { headers, redirect: 'manual', signal });
    if ([301, 302, 303, 307, 308].includes(response.status)) {
      await response.body?.cancel();
      const location = response.headers.get('location');
      if (!location || redirects === 8) throw new Error('Template redirect limit exceeded');
      const next = httpURL(new URL(location, target).href);
      if (target.protocol === 'https:' && next.protocol !== 'https:') throw new Error('Template HTTPS downgrade rejected');
      if (next.origin !== target.origin) for (const key of ['authorization', 'cookie', 'proxy-authorization']) headers.delete(key);
      target = next; continue;
    }
    if (!response.ok || !response.body) { await response.body?.cancel(); throw new Error('Template download HTTP ' + response.status); }
    if (Number(response.headers.get('content-length')) > archiveLimit) { await response.body.cancel(); throw new Error('Template archive exceeds 256 MiB'); }
    const chunks = []; let length = 0;
    for await (const chunk of response.body) {
      length += chunk.length;
      if (length > archiveLimit) throw new Error('Template archive exceeds 256 MiB');
      chunks.push(chunk);
    }
    return Buffer.concat(chunks, length);
  }
}
async function run(repo, destination, options) {
  text(destination, 'template destination');
  if (!options || typeof options !== 'object' || Array.isArray(options)) throw new TypeError('Invalid template options');
  for (const key of Object.keys(options)) if (!supported.has(key)) throw new TypeError('Unsupported template option: ' + key);
  if (options.clone !== undefined && typeof options.clone !== 'boolean') throw new TypeError('Invalid clone option');
  const timeout = options.timeout ?? 60000;
  if (!Number.isInteger(timeout) || timeout < 1 || timeout > 120000) throw new TypeError('Template timeout must be 1..120000 ms');
  const info = source(repo, options.clone);
  const finalPath = path.resolve(destination), parent = await fs.realpath(path.dirname(finalPath));
  if (path.join(parent, path.basename(finalPath)) !== finalPath) throw new Error('Template destination must use its canonical parent');
  try { await fs.lstat(finalPath); throw new Error('Template destination already exists'); } catch (error) { if (error.code !== 'ENOENT') throw error; }
  const staging = await fs.mkdtemp(path.join(parent, '.cp-template-'));
  const owned = async () => {
    const actual = await fs.realpath(staging);
    if (path.dirname(actual) !== parent || !path.basename(actual).startsWith('.cp-template-')) throw new Error('Template staging ownership changed');
    return actual;
  };
  let renamed = false;
  try {
    if (options.clone) {
      const url = text(info.url, 'clone repository');
      const protocol = /^[a-z][a-z+.-]*:/i.test(url);
      const knownTransport = /^(?:https?|ssh|git|file):\/\//i.test(url) || /^[a-z]:[\\/]/i.test(url) || /^[^/@:\s]+@[^:\s]+:/.test(url);
      if (url.startsWith('-') || (protocol && !knownTransport)) throw new TypeError('Unsafe Git transport');
      const ref = text(options.checkout ?? info.ref, 'checkout revision');
      if (ref.startsWith('-') || /[\0\r\n]/.test(ref)) throw new TypeError('Invalid checkout revision');
      const git = options.git === undefined ? 'git' : text(options.git, 'Git executable');
      const settings = { timeout, windowsHide: true, maxBuffer: 4 * 1024 * 1024, env: { ...process.env, GIT_TERMINAL_PROMPT: '0' } };
      const args = ['-c', 'protocol.ext.allow=never', 'clone'];
      if (options.shallow ?? ref === 'master') args.push('--depth', '1');
      await gitExec(git, [...args, '--', url, staging], settings);
      const commit = (await gitExec(git, ['rev-parse', '--verify', '--end-of-options', ref + '^{commit}'], { ...settings, cwd: staging })).stdout.trim();
      if (!/^[a-f0-9]{40,64}$/.test(commit)) throw new Error('Git did not resolve a commit');
      await gitExec(git, ['checkout', '--quiet', '--detach', commit], { ...settings, cwd: staging });
      const gitPath = path.join(await owned(), '.git');
      const gitInfo = await fs.lstat(gitPath);
      if (!gitInfo.isDirectory() || gitInfo.isSymbolicLink()) throw new Error('Unexpected cloned Git directory');
      await fs.rm(gitPath, { recursive: true });
    } else {
      const files = await extract(await archive(info.url, options, timeout), staging, { strip: 1 });
      if (!files.some(file => file.type === 'file')) throw new Error('Template archive contains no supported files');
    }
    await owned();
    // Publish only a complete extraction; reject targets observed to exist.
    try { await fs.lstat(finalPath); throw new Error('Template destination appeared during download'); } catch (error) { if (error.code !== 'ENOENT') throw error; }
    await fs.rename(staging, finalPath); renamed = true;
  } finally {
    if (!renamed) await fs.rm(await owned(), { recursive: true });
  }
}

module.exports = function download(repo, destination, options, callback) {
  if (typeof options === 'function') { callback = options; options = {}; }
  if (typeof callback !== 'function') throw new TypeError('Template download requires a callback');
  run(repo, destination, options ?? {}).then(() => callback(), error => callback(error));
};
