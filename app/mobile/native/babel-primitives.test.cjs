const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { createRequire } = require('node:module');
const babel = require('@babel/core');
process.env.TARO_ENV = 'rn';
const support = createRequire(require.resolve('@tarojs/rn-supporter'));
function transform(source, filename) {
  return babel.transformSync(source, {
    filename, configFile: false, babelrc: false,
    presets: [['taro', { framework: 'react', ts: true }]],
    plugins: [require.resolve('./babel-primitives.cjs'), support.resolve('babel-plugin-transform-react-jsx-to-rn-stylesheet')],
  }).code;
}
test('native platform conversion preserves dynamic classes and changes the actual primitives before the generic RN style plugin', () => {
  const code = transform("import { Text, View } from '@tarojs/components'; export const A = ({tone}) => <View className='cp-panel'><Text className={'cp-state '+tone}>资料</Text></View>", path.resolve(__dirname, '../src/pages/home/example.tsx'));
  assert.match(code, /primitives\.rn/); assert.match(code, /cpClass:\s*['"]cp-panel/);
  assert.match(code, /cpClass:\s*['"]cp-state /); assert.doesNotMatch(code, /components-rn\/dist\/components\/(Text|View)/);
});
test('custom controls retain their classes and external packages keep their own primitives', () => {
  const own = transform("import { Button } from './components'; export const A=()=> <Button className='cp-button cp-primary'>保存</Button>", path.resolve(__dirname, '../src/platform/example.tsx'));
  assert.match(own, /cpClass:\s*['"]cp-button cp-primary/);
  const external = transform("import { Text } from '@tarojs/components'; export const A=()=> <Text>外部组件</Text>", path.resolve(__dirname, '../../node_modules/example/index.tsx'));
  assert.doesNotMatch(external, /primitives\.rn/);
});
