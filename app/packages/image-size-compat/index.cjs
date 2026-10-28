'use strict';

const fs = require('node:fs');
const parser = require('@copypaste/image-size-parser');
let filesystemDisabled = false;
const headerLimit = 512 * 1024;

function readHeader(file) {
  const fd = fs.openSync(file, 'r');
  try {
    const data = Buffer.alloc(Math.min(fs.fstatSync(fd).size, headerLimit));
    return data.subarray(0, fs.readSync(fd, data, 0, data.length, 0));
  } finally { fs.closeSync(fd); }
}
async function readHeaderAsync(file) {
  const handle = await fs.promises.open(file, 'r');
  try {
    const data = Buffer.alloc(Math.min((await handle.stat()).size, headerLimit));
    const { bytesRead } = await handle.read(data, 0, data.length, 0);
    return data.subarray(0, bytesRead);
  } finally { await handle.close(); }
}

// Metro 0.80 calls the module directly with a Buffer. Upstream 2.x exports
// imageSize instead; preserve that call without retaining a vulnerable parser.
function sizeOf(input, callback) {
  const bytes = Buffer.isBuffer(input) || input instanceof Uint8Array;
  const read = () => {
    if (bytes) return parser.imageSize(input);
    if (filesystemDisabled) throw new TypeError('Filesystem image reading is disabled');
    if (typeof input !== 'string' || !input || input.includes('\0')) throw new TypeError('Expected image bytes or a file path');
    return parser.imageSize(readHeader(input));
  };
  if (typeof callback !== 'function') return read();
  if (bytes || filesystemDisabled || typeof input !== 'string' || !input || input.includes('\0')) {
    queueMicrotask(() => { let result; try { result = read(); } catch (error) { callback(error); return; } callback(null, result); });
    return;
  }
  readHeaderAsync(input).then(data => parser.imageSize(data)).then(result => callback(null, result), error => callback(error));
}
sizeOf.imageSize = sizeOf;
sizeOf.default = sizeOf;
sizeOf.types = parser.types;
sizeOf.disableTypes = parser.disableTypes;
sizeOf.disableFS = (disabled = true) => { filesystemDisabled = Boolean(disabled); };
module.exports = sizeOf;
