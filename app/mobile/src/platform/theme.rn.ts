import AsyncStorage from '@react-native-async-storage/async-storage';
import { Appearance } from 'react-native';
import { useSyncExternalStore } from 'react';
export type Preference = 'system' | 'light' | 'dark';
const listeners = new Set<() => void>();
let preference: Preference = 'system', revision = 0;
const emit = () => { revision++; for (const listener of listeners) listener(); };
const initialRevision = revision;
void AsyncStorage.getItem('cp.appearance').then(saved => { if (revision === initialRevision && (saved === 'system' || saved === 'light' || saved === 'dark')) { preference = saved; emit(); } }).catch(() => {});
Appearance.addChangeListener(emit);
export function chooseTheme(value: Preference) { preference = value; emit(); void AsyncStorage.setItem('cp.appearance', value).catch(() => {}); }
export function useAppearance() {
  useSyncExternalStore(listener => { listeners.add(listener); return () => { listeners.delete(listener); }; }, () => revision);
  return { preference, theme: (preference === 'system' ? Appearance.getColorScheme() === 'dark' ? 'dark' : 'light' : preference) as 'light' | 'dark' };
}
