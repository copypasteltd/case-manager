import Taro from '@tarojs/taro';
import { MobileClient, type Wire } from './session';

const wire: Wire = input => {
  const task = Taro.request({
    url: __CP_MOBILE_API_ORIGIN__ + '/v1' + input.path,
    method: input.method, timeout: 20000,
    header: { Accept: 'application/json', 'Content-Type': 'application/json', ...input.headers },
    data: input.body as Record<string, unknown> | undefined,
    dataType: 'json',
  });
  return { promise: task.then(result => ({ status: result.statusCode, data: result.data })), abort: () => task.abort() };
};
export const client = new MobileClient(wire, process.env.TARO_ENV === 'tt' ? 'douyin' : 'wechat');
