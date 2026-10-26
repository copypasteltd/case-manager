import { NativeModules } from 'react-native';
import { Image, Text, View } from '@tarojs/components';
import { useState } from 'react';
import type { ContentArtifact } from '../../platform/content-sink';
import { Action, Notice } from '../../ui';
import { MobileError } from '../../platform/session';

export default function ContentView({ artifact }: { artifact: ContentArtifact }) {
  const [error, setError] = useState<unknown>(), [opening, setOpening] = useState(false);
  if (artifact.kind === 'text') return <Text className='cp-content-text' selectable>{artifact.text || '（此固定版本为空文本。）'}</Text>;
  if (artifact.kind === 'image') return <View><Image className='cp-content-image' src={artifact.url} mode='aspectFit' onError={() => setError(new MobileError('CONTENT_VIEW_FAILED', '图片无法显示，请重试读取固定版本。'))} /><Notice error={error} /></View>;
  const open = async () => {
    setError(undefined); setOpening(true); let release: (() => void) | undefined;
    try { release = artifact.hold?.(); await NativeModules.CPReadCache.open(artifact.url); }
    catch { setError(new MobileError('CONTENT_VIEW_UNAVAILABLE', '本机没有可用的文档查看器，或读取已结束。')); }
    finally { release?.(); setOpening(false); }
  };
  return <View><Text className='cp-paragraph'>固定版本已完成摘要核对。可用本机应用打开；查看器可能另存或缓存内容，请按公司资料权限使用。</Text><Action onClick={() => void open()} loading={opening}>用本机应用打开</Action><Notice error={error} /></View>;
}
