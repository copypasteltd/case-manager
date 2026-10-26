const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const babel = require('@babel/core');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const tokens = require('../packages/design/src/tokens.json');
const platform = path.resolve(__dirname, '../mobile/src/platform');
const host = name => ({ children }) => React.createElement('div', { 'data-native': name }, children);
let styles;
function load(filename) {
  const code = babel.transformSync(fs.readFileSync(filename, 'utf8'), {
    filename, configFile: false, babelrc: false,
    presets: [[require.resolve('@babel/preset-typescript'), { allExtensions: true, isTSX: true }], [require.resolve('@babel/preset-react'), { runtime: 'automatic' }]],
    plugins: [require.resolve('@babel/plugin-transform-modules-commonjs')],
  }).code;
  const module = { exports: {} };
  function dependency(name) {
    if (name === 'react-native') return {
      View: host('View'), Text: host('Text'), Image: host('Image'), Pressable: host('Pressable'), ScrollView: host('ScrollView'),
      StyleSheet: { flatten: value => Object.assign({}, ...[value].flat(Infinity).filter(item => item && typeof item === 'object')) },
    };
    if (name === 'react-native-safe-area-context') return { SafeAreaView: host('SafeAreaView') };
    if (name === '@copypaste/design/tokens.json') return tokens;
    if (name === './theme') return { useAppearance: () => ({ theme: 'light' }) };
    if (name === './native-styles') return styles;
    return require(name);
  }
  new Function('require', 'module', 'exports', code)(dependency, module, module.exports);
  return module.exports;
}
styles = load(path.join(platform, 'native-styles.ts'));
const primitives = load(path.join(platform, 'primitives.rn.tsx'));

test('the actual native page accepts the transformed cpClass and supplies scroll and safe-area containers', () => {
  const html = renderToStaticMarkup(React.createElement(primitives.View, { cpClass: 'cp-page cp-theme-light' }, React.createElement(primitives.Text, null, '上传队列')));
  assert.match(html, /data-native="SafeAreaView"/); assert.match(html, /data-native="ScrollView"/); assert.match(html, /上传队列/);
});
test('ordinary native panels retain their view and safely wrap plain text', () => {
  const html = renderToStaticMarkup(React.createElement(primitives.View, { cpClass: 'cp-panel cp-pad' }, '已核对的分片'));
  assert.match(html, /data-native="View"/); assert.match(html, /data-native="Text"/); assert.doesNotMatch(html, /data-native="ScrollView"/);
});
