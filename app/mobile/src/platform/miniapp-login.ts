import Taro from '@tarojs/taro';
import { client } from './taro';
import { MobileError } from './session';
import { runtimeMiniappID, acquireMiniappIdentity } from './miniapp-context';

declare const tt: { getEnvInfoSync?: () => { microapp?: { appId?: unknown } } };

export function miniappPlatform(): 'wechat' | 'douyin' | null {
  if (process.env.TARO_ENV === 'weapp' && Taro.getEnv() === 'WEAPP') return 'wechat';
  if (process.env.TARO_ENV === 'tt' && Taro.getEnv() === 'TT') return 'douyin';
  return null;
}
// H5 previews, Android and mismatched/unconfigured app IDs cannot produce a
// platform proof. Codes remain local variables and never enter persistent state.
export async function miniappIdentity() {
  const platform = miniappPlatform();
  if (!platform) throw new MobileError('MINIAPP_RUNTIME_REQUIRED', '请在公司的微信或抖音小程序中操作。');
  const app_id = runtimeMiniappID(platform, {
    wechatAccount: () => Taro.getAccountInfoSync(),
    douyinEnvironment: typeof tt !== 'undefined' && typeof tt.getEnvInfoSync === 'function' ? () => tt.getEnvInfoSync!() : undefined,
  });
  const profiles = await client.miniappProfiles();
  return acquireMiniappIdentity(platform, app_id, profiles.items, () => Taro.login({ timeout: 10000 }));
}
