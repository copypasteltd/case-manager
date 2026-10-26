import Taro from '@tarojs/taro';
import { safeLink } from './post-markdown';
export async function openPostLink(url: string): Promise<void> {
  if (!safeLink(url)) throw new Error('链接地址无效。');
  await Taro.setClipboardData({ data: url });
  await Taro.showToast({ title: '链接已复制，请在浏览器打开', icon: 'none' });
}
