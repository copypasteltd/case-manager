import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { api } from './api';
import { ErrorNotice, Field } from './components';

export function PasswordRecovery({ onBack }: { onBack(): void }) {
  const [done, setDone] = useState(false), [mismatch, setMismatch] = useState(false);
  const reset = useMutation({ mutationFn: (body: { token: string; password: string }) => api('/auth/password/reset', { method: 'POST', body }), onSuccess: () => setDone(true) });
  if (done) return <><p role="status">密码已重置，所有设备已退出。请使用新密码登录；已设置的两步验证仍然需要完成。</p><button className="button primary full" onClick={onBack}>返回登录</button></>;
  return <><p className="muted">请联系公司管理员核实身份，获取15分钟内有效的单用重置凭证。将凭证粘贴到下方。</p><ErrorNotice error={reset.error} />{mismatch && <p role="alert">两次新密码不一致。</p>}<form onSubmit={event => { event.preventDefault(); const form = event.currentTarget, body = new FormData(form); if (body.get('password') !== body.get('confirm')) { setMismatch(true); return; } setMismatch(false); form.reset(); reset.mutate({ token: String(body.get('token')).trim(), password: String(body.get('password')) }); }}><Field label="密码重置凭证"><textarea name="token" autoComplete="off" required minLength={40} maxLength={100} rows={3} /></Field><Field label="新密码" hint="至少12个字符。"><input name="password" type="password" autoComplete="new-password" required minLength={12} maxLength={1024} /></Field><Field label="再次输入新密码"><input name="confirm" type="password" autoComplete="new-password" required minLength={12} maxLength={1024} /></Field><button className="button primary full" disabled={reset.isPending}>重置密码</button></form><button className="text-button back-login" disabled={reset.isPending} onClick={onBack}>返回登录</button></>;
}
