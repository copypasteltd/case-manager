import Taro from '@tarojs/taro';

// An explicit user action; OS clipboard lifetime is independent of page memory.
export async function copyTelegramProof(value: string): Promise<void> {
  await Taro.setClipboardData({ data: value });
}
