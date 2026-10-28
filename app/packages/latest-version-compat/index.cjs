'use strict';

// Keep the existing Promise-returning CommonJS API used by Taro update.
// All metadata handling and HTTP requests use the maintained upstream module.
module.exports = async (name, options) => (await import('@copypaste/latest-version-upstream')).default(name, options);
