import type { PropsWithChildren } from 'react';
import Taro, { useLaunch } from '@tarojs/taro';
import { prepareContentStorage } from './content-sink';
import '../tokens.css';
import '../app.css';

export default function AppRoot({ children }: PropsWithChildren) {
  useLaunch(() => { void prepareContentStorage().catch(() => { void Taro.showToast({ title: '临时资料清理失败，请重启客户端', icon: 'none' }).catch(() => {}); }); });
  return children;
}
