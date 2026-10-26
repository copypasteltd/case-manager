import { useEffect, type PropsWithChildren } from 'react';
import { AppState, NativeModules } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { prepareContentStorage } from './content-sink';
import { observeNativeUploads } from './uploads';

export default function App({ children }: PropsWithChildren) {
  useEffect(() => {
    // Android owns only scratch files; a failed sweep blocks new file reads.
    void prepareContentStorage().catch(() => {});
    void NativeModules.CPReadCache?.protectScreen();
    const stopUploads = observeNativeUploads();
    const subscription = AppState.addEventListener('change', state => {
      if (state !== 'active') void NativeModules.CPReadCache?.protectScreen();
    });
    return () => { subscription.remove(); stopUploads(); };
  }, []);
  return <SafeAreaProvider>{children}</SafeAreaProvider>;
}
