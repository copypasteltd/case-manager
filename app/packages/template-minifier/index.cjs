'use strict';

const path = require('node:path');
const { spawnSync } = require('node:child_process');

const INPUT_LIMIT = 8 * 1024 * 1024;
const OUTPUT_LIMIT = 16 * 1024 * 1024;
const TIMEOUT_MS = 15000;
// Taro's MiniPlugin synchronously uses these two boolean options. Do not
// silently serialize callbacks, RegExp values or unsupported legacy options.
const OPTIONS = new Set(['collapseWhitespace', 'keepClosingSlash']);

function minify(input, options = {}) {
  if (typeof input !== 'string') throw new TypeError('Template input must be a string.');
  if (Buffer.byteLength(input, 'utf8') > INPUT_LIMIT) throw new RangeError('Template input exceeds 8 MiB.');
  if (!options || Object.getPrototypeOf(options) !== Object.prototype) throw new TypeError('Template options must be a plain object.');
  for (const key of Reflect.ownKeys(options)) {
    const descriptor = Object.getOwnPropertyDescriptor(options, key);
    if (!OPTIONS.has(key) || !descriptor || !('value' in descriptor) || typeof descriptor.value !== 'boolean') {
      throw new TypeError('Unsupported template minifier option: ' + String(key));
    }
  }
  const result = spawnSync(process.execPath, [path.join(__dirname, 'worker.cjs')], {
    input: JSON.stringify({ input, options }), encoding: 'utf8', shell: false,
    timeout: TIMEOUT_MS, maxBuffer: OUTPUT_LIMIT, windowsHide: true,
    env: { ...process.env, NODE_OPTIONS: '' },
  });
  if (result.error) {
    const error = new Error(result.error.code === 'ETIMEDOUT' ? 'Template minification timed out.' : 'Template minification worker failed.');
    error.code = result.error.code;
    throw error;
  }
  if (result.status !== 0) throw new Error('Template minification failed.');
  let output;
  try { output = JSON.parse(result.stdout); } catch { throw new Error('Invalid template minification result.'); }
  if (!output || typeof output.output !== 'string') throw new Error('Invalid template minification result.');
  return output.output;
}

module.exports = { minify };
