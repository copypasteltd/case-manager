'use strict';

// Existing download callers expect a callable CommonJS export returning a
// Promise. Keep that contract while the maintained extractor owns parsing,
// canonical path/link checks and filesystem writes. Never fall back to the
// unmaintained implementation when loading or extraction fails.
module.exports = (...args) => import('@xhmikosr/decompress')
  .then(({ default: extract }) => extract(...args));
