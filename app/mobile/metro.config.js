const path = require('node:path');
const fs = require('node:fs');
const { getDefaultConfig, mergeConfig } = require('@react-native/metro-config');
const { getMetroConfig } = require('@tarojs/rn-supporter');

module.exports = (async () => {
  const taro = await getMetroConfig();
  const entry = require.resolve('@tarojs/rn-supporter/entry-file.js');
  return mergeConfig(getDefaultConfig(__dirname), taro, {
  maxWorkers: 2,
  watchFolders: [path.resolve(__dirname, '../node_modules'), path.resolve(__dirname, '../packages/contracts'), path.resolve(__dirname, '../packages/design')],
  resolver: {
    unstable_enablePackageExports: true,
    // The Taro entry is hoisted outside this workspace. Resolve its generated
    // relative app/page imports from our real entry, including .rn variants.
    resolveRequest(context, name, platform) {
      const origin = context.originModulePath === entry ? { ...context, originModulePath: path.join(__dirname, 'index.js') } : context;
      if (name.startsWith('.') || path.isAbsolute(name)) {
        const target = path.resolve(path.dirname(origin.originModulePath), name);
        const relative = path.relative(path.join(__dirname, 'src'), target);
        if (!relative.startsWith('..') && !path.isAbsolute(relative)) {
          const extension = path.extname(target);
          const variants = ['.ts', '.tsx', '.js', '.jsx'].includes(extension)
            ? [target.slice(0, -extension.length) + '.rn' + extension]
            : ['.rn.ts', '.rn.tsx', '.rn.js', '.rn.jsx'].map(suffix => target + suffix);
          const native = variants.find(file => fs.existsSync(file));
          if (native) return { type: 'sourceFile', filePath: native };
        }
      }
      return taro.resolver.resolveRequest(origin, name, platform);
    },
    nodeModulesPaths: [path.join(__dirname, 'node_modules'), path.resolve(__dirname, '../node_modules')],
    extraNodeModules: { react: path.dirname(require.resolve('react/package.json')) },
  },
  });
})();
