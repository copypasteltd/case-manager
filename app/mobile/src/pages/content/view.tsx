import { Image, Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useState } from 'react';
import type { ContentArtifact } from '../../platform/content-sink';
import { Action, Notice } from '../../ui';
import { MobileError } from '../../platform/session';

export default function ContentView({ artifact }: { artifact: ContentArtifact }) {
  const [error, setError] = useState<unknown>(), [opening, setOpening] = useState(false);
  if (artifact.kind === 'text') return <Text className='cp-content-text' selectable>{artifact.text || '（此固定版本为空文本。）'}</Text>;
  if (artifact.kind === 'image') return <View><Image className='cp-content-image' src={artifact.url} mode='widthFix' onError={() => setError(new MobileError('CONTENT_VIEW_FAILED', '图片无法显示，请使用 Web 下载原件核对。'))} /><Notice error={error} /></View>;
  const open = async () => {
    setError(undefined); setOpening(true);
    let release: (() => void) | undefined;
    try { release = artifact.hold?.(); await Taro.openDocument({ filePath: artifact.url, fileType: artifact.extension as 'pdf', showMenu: false }); }
    catch { setError(new MobileError('CONTENT_VIEW_UNAVAILABLE', '当前客户端无法打开此文档，请使用 Web 下载原件。')); }
    finally { release?.(); setOpening(false); }
  };
  return <View><Text className='cp-paragraph'>内容已核对完整，继续由本机文档查看器打开。系统查看器可能另存或缓存内容，请按公司资料权限使用。</Text><Text className='cp-muted cp-small'>抖音 Android 的内嵌预览支持有限；无法打开时请使用 Web 下载原件。</Text><Action onClick={() => void open()} loading={opening}>打开固定版本文档</Action><Notice error={error} /></View>;
}
