import { Text, View } from '@tarojs/components';
import { useDidHide } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { Input } from './components';
import { MobileError, Scope, type Schemas } from './session';
import { client } from './taro';
import { CommandKeys, UnresolvedCommand } from './command-keys';
import { utf8ByteLength } from './encoding';
import { Action, Field, Notice } from '../ui';

const keys = new CommandKeys(() => client.operationKey());
client.subscribe(() => { if (!client.authenticated) keys.clear(); });

export function useCommand<T = unknown>(onCommitted: (value: T) => void, binding: string, validate?: (value: T) => boolean) {
  const [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [blocked, setBlocked] = useState(false);
  const pending = useRef(false), alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useDidHide(() => { alive.current = false; });
  async function submit(path: string, body: unknown, method: 'POST' | 'PATCH' = 'POST') {
    if (pending.current || blocked || !alive.current) return;
    pending.current = true; setBusy(true); setError(undefined);
    let key: string | undefined, responseReceived = false;
    try {
      if (path.startsWith('/auth/')) throw new MobileError('INVALID_COMMAND', '验证操作不能作为业务草稿保存。');
      if (utf8ByteLength(JSON.stringify(body)) > 1048576) throw new MobileError('REQUEST_TOO_LARGE', '本次表单超过服务器1MiB请求上限，请减少说明或明细后再保存。', 413);
      const serialized = JSON.stringify({ method, path, body });
      key = keys.reserve(binding, serialized);
      const value = await client.request<T>(path, { method, body, key });
      responseReceived = true;
      if (!value || typeof value !== 'object' || Array.isArray(value) || validate && !validate(value)) throw new MobileError('INVALID_RESPONSE', '操作返回的资料不完整，结果尚未核对。请先刷新业务列表和当前记录。');
      if (alive.current) { setBlocked(true); onCommitted(value); }
      keys.settle(binding, key, 'succeeded', alive.current);
    } catch (cause) {
      const uncertain = responseReceived || cause instanceof MobileError && ['RESULT_UNCONFIRMED', 'INVALID_RESPONSE', 'SESSION_CHANGED'].includes(cause.code);
      if (key) keys.settle(binding, key, uncertain ? 'uncertain' : 'rejected');
      if (alive.current) {
        setError(cause instanceof UnresolvedCommand ? new MobileError('PREVIOUS_RESULT_UNCONFIRMED', cause.message) : responseReceived && !(cause instanceof MobileError) ? new MobileError('RESULT_UNCONFIRMED', '服务器已返回结果，但页面更新未完成。请先刷新资料核对，避免重复创建。') : cause);
        if (cause instanceof UnresolvedCommand || (cause instanceof MobileError && ([403, 404, 409, 412].includes(cause.status) || uncertain))) setBlocked(true);
      }
    } finally { pending.current = false; if (alive.current) setBusy(false); }
  }
  return { submit, busy, error, blocked };
}

export function CommandNotice({ blocked }: { blocked: boolean }) {
  return blocked ? <View className='cp-notice'><Text>输入已保留。请先刷新资料，核对本次结果、当前权限和最新修订后再继续；不要重复创建相同资料。</Text></View> : null;
}
export function DraftNotice({ restored }: { restored: boolean }) {
  return restored ? <View className='cp-notice'><Text>已恢复此会话中尚未提交的文字输入。请再次核对当前资料；文件选择、核实声明和两步验证需要重新操作。</Text></View> : null;
}

export function MFAConfirmation({ onFresh, disabled }: { onFresh(value: boolean): void; disabled: boolean }) {
  const [code, setCode] = useState(''), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [verified, setVerified] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(), alive = useRef(true), callback = useRef(onFresh), scope = useRef<Scope>();
  callback.current = onFresh;
  function clear() {
    alive.current = false; scope.current?.close();
    if (timer.current) clearTimeout(timer.current);
    setCode(''); setVerified(false); callback.current(false);
  }
  useDidHide(clear);
  useEffect(() => { alive.current = true; return clear; }, []);
  async function verify() {
    if (busy || disabled || !alive.current || !/^\d{6}$/.test(code)) return;
    setBusy(true); setError(undefined); const current = new Scope(); scope.current = current;
    try {
      await client.request<Schemas['MFAStepUpResult']>('/auth/mfa/step-up', { method: 'POST', body: { code }, scope: current });
      if (alive.current && !current.closed) {
        setVerified(true); callback.current(true);
        if (timer.current) clearTimeout(timer.current);
        timer.current = setTimeout(() => { setVerified(false); callback.current(false); }, 9 * 60000);
      }
    } catch (cause) { if (alive.current && !current.closed) { setError(cause); setVerified(false); callback.current(false); } }
    finally { if (alive.current && !current.closed) { setCode(''); setBusy(false); } }
  }
  return <View><Notice error={error} />{verified ? <Text className='cp-notice'>已通过本次两步验证，请继续核对并确认操作。</Text> : <><Field label='验证器六位验证码'><Input ariaLabel='验证器六位验证码' className='cp-input' type='number' maxlength={6} value={code} disabled={busy || disabled} onInput={event => setCode(event.detail.value)} /></Field><Action secondary loading={busy} disabled={disabled || !/^\d{6}$/.test(code)} onClick={() => void verify()}>确认本人操作</Action></>}</View>;
}
