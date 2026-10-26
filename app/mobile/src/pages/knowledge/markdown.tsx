import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { markdown } from '../../platform/post-markdown';
import { openPostLink } from '../../platform/post-link';
import { Notice } from '../../ui';

export function Markdown({ body }: { body: string }) {
  const [error, setError] = useState<unknown>();
  return <View className='cp-markdown'><Notice error={error} />{markdown(body).map((block, i) => <View className={'cp-md-block cp-md-' + block.kind} key={i}>
    {block.kind === 'item' && <Text>• </Text>}{block.text.map((part, j) => <Text key={j} selectable={part.kind !== 'link'} className={'cp-md-' + part.kind}
      onClick={part.url ? () => { void openPostLink(part.url!).catch(setError); } : undefined}>{part.text}{part.url ? ' ↗' : ''}</Text>)}
  </View>)}</View>;
}
