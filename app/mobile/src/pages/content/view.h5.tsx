import { Image, Text, View } from '@tarojs/components';
import { useState } from 'react';
import type { ContentArtifact } from '../../platform/content-sink';
import { MobileError } from '../../platform/session';
import { Notice } from '../../ui';

// Local H5 review only. No remote URL, HTML injection or new browser tab.
export default function ContentView({ artifact }: { artifact: ContentArtifact }) {
  const [error, setError] = useState<unknown>();
  if (artifact.kind === 'text') return <Text className='cp-content-text' selectable>{artifact.text || '（此固定版本为空文本。）'}</Text>;
  if (artifact.kind === 'image') return <View><Image className='cp-content-image' src={artifact.url} mode='widthFix' onError={() => setError(new MobileError('CONTENT_VIEW_FAILED', '图片无法显示，请使用 Web 下载原件核对。'))} /><Notice error={error} /></View>;
  return <Text className='cp-paragraph'>完整内容已核对。本页当前不渲染此文档格式，请使用 Web 下载原件，或在受支持的本机文档查看器中打开。</Text>;
}
