import { NativeModules } from 'react-native';

// The native host exposes only bounded proof copying, never clipboard reading.
export async function copyTelegramProof(value: string): Promise<void> {
  await NativeModules.CPClipboard.copyTelegramProof(value);
}
