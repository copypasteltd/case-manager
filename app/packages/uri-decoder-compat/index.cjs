'use strict';

// Keep the legacy synchronous CJS function contract with the patched ESM parser.
const upstream = require('@copypaste/uri-decoder-upstream');
module.exports = upstream.default || upstream;
