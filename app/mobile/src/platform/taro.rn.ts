import { NativeModules } from 'react-native';
import { MobileClient } from './session';
import { createNativeWire, type NativeHTTP } from './native-wire';
export const client = new MobileClient(createNativeWire(NativeModules.CPHTTP as NativeHTTP), 'android');
client.subscribe(() => { if (!client.authenticated) NativeModules.CPUpload?.clearScope(); });
