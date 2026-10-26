import Taro from '@tarojs/taro';
import { useSyncExternalStore } from 'react';
export type Preference = 'system' | 'light' | 'dark';
const listeners = new Set<() => void>();
let preference: Preference = 'system', system: 'light' | 'dark' = 'light';
try { const saved: unknown = Taro.getStorageSync('cp.appearance'); if (saved === 'system' || saved === 'light' || saved === 'dark') preference = saved; system = Taro.getSystemInfoSync().theme === 'dark' ? 'dark' : 'light'; } catch { /* A theme preference is optional. */ }
const emit = () => { for (const f of listeners) f(); };
try { Taro.onThemeChange(event => { system = event.theme === 'dark' ? 'dark' : 'light'; emit(); }); } catch { /* Unsupported platforms retain the explicit appearance control. */ }
const subscribe = (f: () => void) => { listeners.add(f); return () => { listeners.delete(f); }; };
export function chooseTheme(value: Preference) { preference = value; try { Taro.setStorageSync('cp.appearance', value); } catch { /* In-memory selection still works. */ } emit(); }
export function useAppearance() { const value = useSyncExternalStore(subscribe, () => preference + ':' + system); const [selected, fallback] = value.split(':'); return { preference: selected as Preference, theme: (selected === 'system' ? fallback : selected) as 'light' | 'dark' }; }
