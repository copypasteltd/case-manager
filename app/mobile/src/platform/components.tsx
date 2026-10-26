import { Button as NativeButton, Checkbox as NativeCheckbox, Input as NativeInput, Picker as NativePicker, Textarea as NativeTextarea } from '@tarojs/components';
import type { ComponentProps } from 'react';

// Mini-program accessibility names use camel-case props. Taro's H5 React
// primitives forward arbitrary props, so use actual HTML accessibility attrs
// for that local review target and preserve native props on shipping targets.
export function semantic(role: string) { return process.env.TARO_ENV === 'h5' ? { role } : { ariaRole: role }; }
function labelled(label?: string) { return process.env.TARO_ENV === 'h5' ? { 'aria-label': label } : { ariaLabel: label }; }
// Taro's JSX compiler can normalise ariaLabel even on our own components.
// Accept either spelling before adapting the actual platform primitive.
type Labelled<P> = P & { 'aria-label'?: string };
export function Button({ ariaLabel, 'aria-label': compiledLabel, ...props }: Labelled<ComponentProps<typeof NativeButton>>) {
  const web = process.env.TARO_ENV === 'h5' ? {
    role: 'button', 'aria-disabled': Boolean(props.disabled), tabIndex: props.disabled ? -1 : 0,
    onKeyDown: (event: { key: string; preventDefault(): void; currentTarget: { click(): void } }) => { if (!props.disabled && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); event.currentTarget.click(); } },
  } : { ariaRole: 'button' };
  // H5 custom elements can reflect disabled="false". Styling [disabled]
  // therefore also fades enabled controls; derive the class from the boolean.
  const className = [props.className, props.disabled ? 'cp-disabled' : ''].filter(Boolean).join(' ');
  return <NativeButton {...props} className={className} {...labelled(ariaLabel ?? compiledLabel)} {...web} />;
}
export function Input({ ariaLabel, 'aria-label': compiledLabel, ...props }: Labelled<ComponentProps<typeof NativeInput>>) {
  ariaLabel ??= compiledLabel;
  const nativeProps = process.env.TARO_ENV === 'h5' ? { ...props.nativeProps, 'aria-label': ariaLabel } : props.nativeProps;
  return <NativeInput {...props} nativeProps={nativeProps} {...labelled(ariaLabel)} />;
}
export function Checkbox({ ariaLabel, 'aria-label': compiledLabel, ...props }: Labelled<ComponentProps<typeof NativeCheckbox>>) {
  ariaLabel ??= compiledLabel;
  const nativeProps = process.env.TARO_ENV === 'h5' ? { ...props.nativeProps, 'aria-label': ariaLabel } : props.nativeProps;
  return <NativeCheckbox {...props} nativeProps={nativeProps} {...labelled(ariaLabel)} />;
}
export function Textarea({ ariaLabel, 'aria-label': compiledLabel, ...props }: Labelled<ComponentProps<typeof NativeTextarea>>) {
  ariaLabel ??= compiledLabel;
  const nativeProps = process.env.TARO_ENV === 'h5' ? { ...props.nativeProps, 'aria-label': ariaLabel } : props.nativeProps;
  return <NativeTextarea {...props} nativeProps={nativeProps} {...labelled(ariaLabel)} />;
}
export function Picker({ ariaLabel, 'aria-label': compiledLabel, ...props }: Labelled<ComponentProps<typeof NativePicker>>) { return <NativePicker {...props} {...labelled(ariaLabel ?? compiledLabel)} />; }
