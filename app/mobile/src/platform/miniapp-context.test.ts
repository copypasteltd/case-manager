import { test } from 'node:test';
import assert from 'node:assert/strict';
import { runtimeMiniappID, acquireMiniappIdentity } from './miniapp-context';

test('WeChat reads its actual miniProgram AppID without invoking a Douyin API', () => {
  assert.equal(runtimeMiniappID('wechat', { wechatAccount: () => ({ miniProgram: { appId: 'wx0123456789abcdef' } }), douyinEnvironment: () => { throw new Error('wrong API'); } }), 'wx0123456789abcdef');
});
test('Douyin reads microapp AppID from getEnvInfoSync without a WeChat account API', () => {
  assert.equal(runtimeMiniappID('douyin', { douyinEnvironment: () => ({ microapp: { appId: 'tt-owned-app' } }), wechatAccount: () => { throw new Error('wrong API'); } }), 'tt-owned-app');
});
test('Unavailable or malformed runtime identity fails before any login or configured-ID fallback', () => {
  for (const runtime of [{}, { douyinEnvironment: () => ({ microapp: {} }) }, { douyinEnvironment: () => ({ microapp: { appId: 'bad app' } }) }]) assert.throws(() => runtimeMiniappID('douyin', runtime), { code: 'MINIAPP_RUNTIME_REQUIRED' });
  assert.throws(() => runtimeMiniappID('wechat', { wechatAccount: () => { throw new Error('private diagnostic'); } }), error => error instanceof Error && !error.message.includes('private diagnostic'));
});
test('An unconfigured or cross-platform AppID never requests a platform code', async () => {
  let calls = 0; const login = async () => { calls++; return { code: 'unused' }; };
  await assert.rejects(acquireMiniappIdentity('douyin', 'tt-owned-app', [{ platform: 'wechat', app_id: 'tt-owned-app' }], login), { code: 'MINIAPP_UNAVAILABLE' });
  await assert.rejects(acquireMiniappIdentity('douyin', 'tt-owned-app', [{ platform: 'douyin', app_id: 'tt-other-app' }], login), { code: 'MINIAPP_UNAVAILABLE' }); assert.equal(calls, 0);
});
test('A matching configured runtime returns only the platform, actual AppID and newly obtained code', async () => {
  const result = await acquireMiniappIdentity('douyin', 'tt-owned-app', [{ platform: 'douyin', app_id: 'tt-owned-app' }], async () => ({ code: 'single-use-code' }));
  assert.deepEqual(result, { platform: 'douyin', app_id: 'tt-owned-app', code: 'single-use-code' });
});
test('Missing, malformed and anonymous-only login results cannot become employee proofs', async () => {
  for (const code of [undefined, '', 12, 'invalid code', 'x'.repeat(513)]) await assert.rejects(acquireMiniappIdentity('wechat', 'wx-owned-app', [{ platform: 'wechat', app_id: 'wx-owned-app' }], async () => ({ code })), { code: 'MINIAPP_CODE_REQUIRED' });
});
test('Separate user actions obtain fresh codes instead of reusing a runtime result', async () => {
  let calls = 0; const login = async () => ({ code: 'one-use-' + ++calls });
  const profiles = [{ platform: 'wechat', app_id: 'wx-owned-app' }];
  assert.equal((await acquireMiniappIdentity('wechat', 'wx-owned-app', profiles, login)).code, 'one-use-1');
  assert.equal((await acquireMiniappIdentity('wechat', 'wx-owned-app', profiles, login)).code, 'one-use-2');
});
