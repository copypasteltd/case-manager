import { useContext, useEffect, useRef, useState, type ComponentProps } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, Text as RNText, TextInput, View as RNView, type TextStyle } from 'react-native';
import type * as TaroComponents from '@tarojs/components';
import tokens from '@copypaste/design/tokens.json';
import { NativeTextContext, Checkbox as NativeCheckbox } from './primitives.rn';
import { nativeClassStyle, textStyle } from './native-styles';
import { useAppearance } from './theme';

type Labelled<P> = P & { 'aria-label'?: string; cpClass?: string };
export function semantic(role: 'heading' | 'status' | 'group' | 'progressbar') { return { accessibilityRole: role === 'heading' ? 'header' : role === 'progressbar' ? 'progressbar' : 'none', accessibilityLiveRegion: role === 'status' ? 'polite' : undefined } as const; }
export function Button({ className, cpClass, style, children, disabled, loading, onClick, ariaLabel, 'aria-label': label }: Labelled<ComponentProps<typeof TaroComponents.Button>>) {
  className = cpClass ?? className;
  const { theme } = useAppearance(), inherited = useContext(NativeTextContext);
  const own = Object.assign({}, nativeClassStyle(theme, className), typeof style === 'object' ? style : {}) as ReturnType<typeof nativeClassStyle>;
  const text = { color: tokens.themes[theme]['fg.default'], ...inherited, ...textStyle(own) };
  return <NativeTextContext.Provider value={text}><Pressable accessibilityRole='button' accessibilityLabel={ariaLabel ?? label} accessibilityState={{ disabled: Boolean(disabled || loading), busy: Boolean(loading) }} disabled={disabled || loading} onPress={event => onClick?.(event as never)} style={({ pressed }) => [own, { opacity: disabled ? .55 : pressed ? .7 : 1 }]}>{loading && <ActivityIndicator color={text.color} />}{typeof children === 'string' || typeof children === 'number' ? <RNText style={text}>{children}</RNText> : children}</Pressable></NativeTextContext.Provider>;
}
type InputProps = Labelled<ComponentProps<typeof TaroComponents.Input>>;
function NativeInput({ className, cpClass, style, value, disabled, focus, password, type, placeholder, maxlength, onInput, onFocus, onBlur, ariaLabel, 'aria-label': label, nativeProps, multiline = false }: InputProps & { multiline?: boolean }) {
  className = cpClass ?? className;
  const { theme } = useAppearance();
  const ref = useRef<TextInput>(null);
  useEffect(() => { if (focus) ref.current?.focus(); }, [focus]);
  const accessible = nativeProps && typeof nativeProps === 'object' && 'accessibilityLabel' in nativeProps ? String(nativeProps.accessibilityLabel) : ariaLabel ?? label;
  const event = (text: string) => ({ detail: { value: text }, target: { value: text }, currentTarget: { value: text }, type: 'input', timeStamp: Date.now() });
  return <TextInput ref={ref} accessibilityLabel={accessible} accessibilityState={{ disabled }} autoFocus={focus} autoCapitalize='none' autoCorrect={false} importantForAutofill='noExcludeDescendants' textContentType='none' value={value == null ? '' : String(value)} editable={!disabled} secureTextEntry={password} keyboardType={type === 'number' ? 'number-pad' : type === 'digit' ? 'decimal-pad' : 'default'} multiline={multiline} placeholder={placeholder} placeholderTextColor={tokens.themes[theme]['fg.placeholder']} maxLength={maxlength && maxlength > 0 ? maxlength : undefined} style={[{ color: tokens.themes[theme]['fg.default'] }, nativeClassStyle(theme, className), typeof style === 'object' ? style as TextStyle : {}]} onChangeText={text => onInput?.(event(text) as never)} onFocus={() => onFocus?.(event(value ?? '') as never)} onBlur={() => onBlur?.(event(value ?? '') as never)} />;
}
export function Input(props: InputProps) { return <NativeInput {...props} />; }
export function Textarea(props: Labelled<ComponentProps<typeof TaroComponents.Textarea>>) { return <NativeInput {...props as InputProps} multiline />; }
export function Checkbox({ ariaLabel, 'aria-label': label, ...props }: Labelled<ComponentProps<typeof TaroComponents.Checkbox>>) { return <NativeCheckbox {...props} ariaLabel={ariaLabel ?? label} />; }
export function Picker(props: Labelled<ComponentProps<typeof TaroComponents.Picker>>) {
  const { children, disabled, value, onChange, ariaLabel, 'aria-label': label } = props;
  const range = 'range' in props ? props.range : undefined;
  const [open, setOpen] = useState(false), { theme } = useAppearance();
  const options = Array.isArray(range) ? range.map(item => String(item)) : [];
  return <><Pressable disabled={disabled} accessibilityRole='button' accessibilityLabel={ariaLabel ?? label} accessibilityState={{ disabled, expanded: open }} onPress={() => setOpen(true)}>{children}</Pressable><Modal visible={open} transparent animationType='slide' onRequestClose={() => setOpen(false)}><RNView style={{ flex: 1, justifyContent: 'flex-end', backgroundColor: '#0008' }}><RNView accessibilityViewIsModal style={{ maxHeight: '75%', backgroundColor: tokens.themes[theme]['bg.surface'], padding: 16 }}><RNText accessibilityRole='header' style={{ color: tokens.themes[theme]['fg.default'], fontSize: 18 }}>{ariaLabel ?? label ?? '请选择'}</RNText><ScrollView>{options.map((option, index) => <Pressable key={index} accessibilityRole='radio' accessibilityState={{ selected: Number(value) === index }} onPress={() => { onChange?.({ detail: { value: index } } as never); setOpen(false); }} style={{ paddingVertical: 16 }}><RNText style={{ color: tokens.themes[theme]['fg.default'], fontSize: 16 }}>{Number(value) === index ? '● ' : '○ '}{option}</RNText></Pressable>)}</ScrollView><Button className='cp-button cp-secondary' onClick={() => setOpen(false)}>取消</Button></RNView></RNView></Modal></>;
}
