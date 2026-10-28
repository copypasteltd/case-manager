import { MobileError } from './session';

// Android manages employee bindings through the API. It never imports a Taro
// platform login implementation or any associated Expo native dependency.
export function miniappPlatform(): 'wechat' | 'douyin' | null { return null; }
export async function miniappIdentity(): Promise<{ platform: 'wechat' | 'douyin'; app_id: string; code: string }> {
  throw new MobileError('MINIAPP_RUNTIME_REQUIRED', '请在公司的微信或抖音小程序中操作。');
}
