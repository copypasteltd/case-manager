import type { UserConfigExport } from '@tarojs/cli';
import path from 'node:path';

const input = process.env.CP_MOBILE_API_ORIGIN;
if (process.env.TARO_ENV === 'h5' && process.env.CP_MOBILE_BUILD_PROFILE !== 'local') throw new Error('The H5 target is a local review harness only.');
if (!input) throw new Error('CP_MOBILE_API_ORIGIN is required; no production endpoint is inferred.');
const origin = new URL(input);
const local = process.env.CP_MOBILE_BUILD_PROFILE === 'local' && ['127.0.0.1', 'localhost'].includes(origin.hostname);
if ((origin.protocol !== 'https:' && !(local && origin.protocol === 'http:')) || origin.username || origin.password || origin.search || origin.hash || origin.pathname !== '/') {
  throw new Error('The mobile API requires an explicit HTTPS origin; local builds may use loopback HTTP.');
}
const config: UserConfigExport = {
  projectName: 'copypaste-internal', date: '2026-10-23', designWidth: 750,
  deviceRatio: { 640: 2.34 / 2, 750: 1, 828: 1.81 / 2 },
  sourceRoot: 'src', outputRoot: 'dist/' + process.env.TARO_ENV,
  framework: 'react', compiler: { type: 'webpack5', prebundle: { enable: false } },
  // Hoisted platform dependencies must resolve this workspace's framework
  // runtime rather than searching only their own parent node_modules tree.
  alias: { '@tarojs/plugin-framework-react': path.dirname(require.resolve('@tarojs/plugin-framework-react/package.json')) },
  defineConstants: { __CP_MOBILE_API_ORIGIN__: JSON.stringify(origin.origin) },
  cache: { enable: false },
  // The shared mutation contract contains runtime TypeScript as well as types.
  // Taro otherwise transpiles only sourceRoot and its own framework packages.
  mini: { compile: { include: [path.resolve(__dirname, '../../packages/contracts/src')] }, postcss: { pxtransform: { enable: true }, cssModules: { enable: false } } },
  h5: { compile: { include: [path.resolve(__dirname, '../../packages/contracts/src')] }, publicPath: '/', router: { mode: 'hash' }, postcss: { pxtransform: { enable: true } } },
  rn: {
    appName: 'CopyPasteWorkspace',
    output: {
      android: path.resolve(__dirname, '../android/app/src/main/assets/index.android.bundle'),
      androidAssetsDest: path.resolve(__dirname, '../android/app/src/main/res'),
      androidSourcemapOutput: path.resolve(__dirname, '../../.local/android/index.android.map'),
    },
  },
};
export default config;
