import { Children, createContext, useContext, useEffect, useRef, type ComponentProps } from 'react';
import { Image as RNImage, Pressable, ScrollView, Text as RNText, View as RNView, StyleSheet, type TextStyle, type AccessibilityRole } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import type * as TaroComponents from '@tarojs/components';
import tokens from '@copypaste/design/tokens.json';
import { useAppearance } from './theme';
import { nativeClassStyle, textStyle } from './native-styles';

export const NativeTextContext = createContext<TextStyle>({});
type NativeProps<P> = P & { cpClass?: string; accessibilityRole?: AccessibilityRole; accessibilityLiveRegion?: 'none' | 'polite' | 'assertive' };
function useStyle(className?: string, explicit?: unknown) {
  const { theme } = useAppearance();
  const inherited = useContext(NativeTextContext);
  const own = StyleSheet.flatten([nativeClassStyle(theme, className), typeof explicit === 'object' ? explicit : {}]) ?? {};
  return { theme, own, text: { color: tokens.themes[theme]['fg.default'], fontSize: 15, lineHeight: 24, ...inherited, ...textStyle(own) } as TextStyle };
}
export function Text({ className, cpClass, style, children, onClick, selectable, ...rest }: NativeProps<ComponentProps<typeof TaroComponents.Text>>) {
  const value = useStyle(cpClass ?? className, style);
  return <RNText accessibilityLabel={rest.ariaLabel} accessibilityRole={rest.accessibilityRole} accessibilityLiveRegion={rest.accessibilityLiveRegion} selectable={selectable} style={[value.text, value.own]} onPress={onClick ? event => onClick(event as never) : undefined}><NativeTextContext.Provider value={value.text}>{children}</NativeTextContext.Provider></RNText>;
}
export function View({ className, cpClass, style, children, onClick, ...rest }: NativeProps<ComponentProps<typeof TaroComponents.View>>) {
  className = cpClass ?? className;
  const value = useStyle(className, style);
  const content = <NativeTextContext.Provider value={value.text}>{Children.map(children, child => typeof child === 'string' || typeof child === 'number' ? <Text>{child}</Text> : child)}</NativeTextContext.Provider>;
  if ((cpClass ?? className)?.split(/\s+/).includes('cp-page')) return <SafeAreaView style={{ flex: 1, backgroundColor: tokens.themes[value.theme]['bg.canvas'] }} edges={['bottom', 'left', 'right']}><ScrollView keyboardShouldPersistTaps='handled' contentContainerStyle={value.own}>{content}</ScrollView></SafeAreaView>;
  if (onClick) return <Pressable testID={rest.id} accessibilityLabel={rest.ariaLabel} accessibilityRole={rest.accessibilityRole} accessibilityLiveRegion={rest.accessibilityLiveRegion} onPress={event => onClick(event as never)} style={value.own}>{content}</Pressable>;
  return <RNView testID={rest.id} accessibilityLabel={rest.ariaLabel} accessibilityRole={rest.accessibilityRole} accessibilityLiveRegion={rest.accessibilityLiveRegion} style={value.own}>{content}</RNView>;
}
export function Image({ className, cpClass, style, src, mode, ariaHidden, ariaLabel, onError }: NativeProps<ComponentProps<typeof TaroComponents.Image>>) {
  const value = useStyle(cpClass ?? className, style);
  const source = typeof src === 'number' ? src : { uri: src };
  return <RNImage source={source} resizeMode={mode === 'aspectFill' ? 'cover' : 'contain'} accessible={!ariaHidden} accessibilityLabel={ariaLabel} style={value.own} onError={event => onError?.(event as never)} />;
}
type Group = { register(value: string, checked: boolean, disabled: boolean): () => void; toggle(value?: string): void };
const GroupContext = createContext<Group | undefined>(undefined);
export function CheckboxGroup({ children, onChange }: ComponentProps<typeof TaroComponents.CheckboxGroup>) {
  const checked = useRef(new Set<string>()), values = useRef(new Set<string>()), disabled = useRef(new Set<string>());
  const group: Group = {
    register(value, selected, locked) { values.current.add(value); if (selected) checked.current.add(value); else checked.current.delete(value); if (locked) disabled.current.add(value); else disabled.current.delete(value); return () => { values.current.delete(value); checked.current.delete(value); disabled.current.delete(value); }; },
    toggle(value = values.current.values().next().value) { if (!value || disabled.current.has(value)) return; const next = new Set(checked.current); if (next.has(value)) next.delete(value); else next.add(value); onChange?.({ detail: { value: [...next] } } as never); },
  };
  return <GroupContext.Provider value={group}>{children}</GroupContext.Provider>;
}
export function Label({ className, cpClass, children }: NativeProps<ComponentProps<typeof TaroComponents.Label>>) {
  const group = useContext(GroupContext), value = useStyle(cpClass ?? className);
  return <Pressable onPress={group ? () => group.toggle() : undefined} style={value.own}>{children}</Pressable>;
}
export function Checkbox({ checked, disabled, value = '', ariaLabel, onClick }: ComponentProps<typeof TaroComponents.Checkbox>) {
  const group = useContext(GroupContext), { theme } = useAppearance();
  useEffect(() => group?.register(value, Boolean(checked), Boolean(disabled)), [group, value, checked, disabled]);
  const visual = <RNText style={{ color: tokens.themes[theme]['accent.fg'], fontSize: 22 }}>{checked ? '☑' : '☐'}</RNText>;
  if (!group && !onClick) return <RNView pointerEvents='none' accessible accessibilityRole='checkbox' accessibilityLabel={ariaLabel} accessibilityState={{ checked, disabled }}>{visual}</RNView>;
  return <Pressable disabled={disabled} accessibilityRole='checkbox' accessibilityLabel={ariaLabel} accessibilityState={{ checked, disabled }} hitSlop={8} onPress={event => { if (group) group.toggle(value); else onClick?.(event as never); }}>{visual}</Pressable>;
}
