'use strict';

const { readFileSync } = require('node:fs');
const { minify } = require('@copypaste/html-minifier-upstream');

(async () => {
  const { input, options } = JSON.parse(readFileSync(0, 'utf8'));
  const output = await minify(input, options);
  process.stdout.write(JSON.stringify({ output }));
})().catch(() => {
  // Avoid copying source templates or filesystem paths into build errors.
  process.stderr.write('Template minification failed.\n');
  process.exitCode = 1;
});
