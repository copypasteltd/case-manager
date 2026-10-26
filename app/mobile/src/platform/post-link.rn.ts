import { Linking } from 'react-native';
import { safeLink } from './post-markdown';
export async function openPostLink(url: string): Promise<void> {
  if (!safeLink(url)) throw new Error('链接地址无效。');
  await Linking.openURL(url);
}
