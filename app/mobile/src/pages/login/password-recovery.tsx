import { Text, View } from '@tarojs/components';
import { useDidHide } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { Input } from '../../platform/components';
import { client } from '../../platform/taro';
import { MobileError } from '../../platform/session';
import { Action, Field, Notice, Page } from '../../ui';

export function PasswordRecovery({ onBack }: { onBack(): void }) {
  const [token, setToken] = useState(''), [password, setPassword] = useState(''), [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false), [done, setDone] = useState(false), [error, setError] = useState<unknown>();
  const active = useRef(true), pending = useRef(false);
  const clear = () => { setToken(''); setPassword(''); setConfirm(''); };
  useDidHide(clear); useEffect(() => () => { active.current = false; }, []);
  async function submit() {
    if (pending.current) return;
    if (password !== confirm) { setError(new MobileError('PASSWORD_MISMATCH', '两次新密码不一致。')); return; }
    pending.current = true; setBusy(true); setError(undefined);
    const secret = token.trim(), next = password; clear();
    try { await client.resetPassword(secret, next); if (active.current) setDone(true); }
    catch (cause) { if (active.current) setError(cause); } finally { pending.current = false; if (active.current) setBusy(false); }
  }
  return <Page title='重置登录密码' eyebrow='RECOVER YOUR ACCOUNT'><Notice error={error} />{done ? <Text>密码已重置，所有设备已退出。请使用新密码登录；两步验证继续保留。</Text> : <View><Text className='cp-muted cp-paragraph'>请联系公司管理员核实身份，取得15分钟内有效的单用重置凭证。</Text><Field label='密码重置凭证'><Input ariaLabel='密码重置凭证' password className='cp-input' value={token} maxlength={100} disabled={busy} onInput={event => setToken(event.detail.value)} /></Field><Field label='新密码' hint='至少12个字符。'><Input ariaLabel='新密码' password className='cp-input' value={password} maxlength={1024} disabled={busy} onInput={event => setPassword(event.detail.value)} /></Field><Field label='再次输入新密码'><Input ariaLabel='再次输入新密码' password className='cp-input' value={confirm} maxlength={1024} disabled={busy} onInput={event => setConfirm(event.detail.value)} /></Field><Action loading={busy} disabled={token.trim().length < 40 || [...password].length < 12 || !confirm} onClick={() => void submit()}>重置密码</Action></View>}<Action secondary disabled={busy} onClick={onBack}>返回登录</Action></Page>;
}
