module.exports = {
  presets: [['taro', { framework: 'react', ts: true, compiler: 'webpack5' }]],
  plugins: process.env.TARO_ENV === 'rn' ? [require.resolve('./native/babel-primitives.cjs')] : [],
};
