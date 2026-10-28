import { MobileError } from './session';

type Platform = 'wechat' | 'douyin';
type Runtime = { wechatAccount?: () => { miniProgram?: { appId?: unknown } }; douyinEnvironment?: () => { microapp?: { appId?: unknown } } };
export function runtimeMiniappID(platform: Platform, runtime: Runtime): string {
  let value: unknown;
  try { value = platform === 'wechat' ? runtime.wechatAccount?.()?.miniProgram?.appId : runtime.douyinEnvironment?.()?.microapp?.appId; }
  catch { throw new MobileError('MINIAPP_RUNTIME_REQUIRED', '无法读取当前小程序信息，请更新宿主应用后重试。'); }
  if (typeof value !== 'string' || !/^[A-Za-z0-9_-]{5,128}$/.test(value)) throw new MobileError('MINIAPP_RUNTIME_REQUIRED', '无法确认当前小程序AppID，请更新宿主应用后重试。');
  return value;
}
export async function acquireMiniappIdentity(platform: Platform, app_id: string, profiles: readonly { platform: string; app_id: string }[], login: () => Promise<{ code?: unknown }>) {
  if (!profiles.some(item => item.platform === platform && item.app_id === app_id)) throw new MobileError('MINIAPP_UNAVAILABLE', '此小程序尚未配置平台登录，请使用内部账号登录。');
  const result = await login();
  if (typeof result.code !== 'string' || !result.code || result.code.length > 512 || /[\x00\s]/.test(result.code)) throw new MobileError('MINIAPP_CODE_REQUIRED', '未能取得平台登录凭证，请重新操作。');
  return { platform, app_id, code: result.code };
}
