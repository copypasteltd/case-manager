import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { api, type Schemas } from './api';
import { AccountVerification } from './AccountSecurity';
import { Dialog, ErrorNotice, Field, date } from './components';

export default function PasswordResetDialog({ user, onClose }: { user: Schemas['User']; onClose(): void }) {
  const [key] = useState(() => crypto.randomUUID()), [copied, setCopied] = useState(false);
  const issue = useMutation({ mutationFn: (reason: string) => api<Schemas['PasswordResetReceipt']>('/users/' + user.id + '/password-resets', { method: 'POST', key, body: { expected_version: user.version, reason } }) });
  return <Dialog title={'重置密码 · ' + user.display_name} onClose={onClose} busy={issue.isPending}>
    <ErrorNotice error={issue.error} />
    {issue.data ? <>{issue.data.reset_token ? <><p>请通过已核实身份的渠道，将以下凭证交给该成员。请勿发到公司群。</p><pre className="token-value">{issue.data.reset_token}</pre><button className="button secondary" onClick={() => { void navigator.clipboard.writeText(issue.data!.reset_token!).then(() => setCopied(true), () => setCopied(false)); }}>{copied ? '已复制重置凭证' : '复制重置凭证'}</button><p className="muted">成员在登录页选择“忘记密码”，粘贴凭证并设置新密码。</p></> : <p role="status">此凭证已经失效或被使用，请关闭后重新签发。</p>}<p>有效至 {date(issue.data.expires_at)}。</p><button className="button primary" onClick={onClose}>完成</button></> : <><p className="muted">签发前请核实成员身份。新凭证会替代旧凭证；完成重置后，该成员所有设备退出，两步验证继续保留。</p><AccountVerification /><form onSubmit={event => { event.preventDefault(); issue.mutate(String(new FormData(event.currentTarget).get('reason')).trim()); }}><Field label="重置原因"><textarea name="reason" required maxLength={200} rows={3} /></Field><button className="button primary" disabled={issue.isPending}>签发15分钟重置凭证</button></form></>}
  </Dialog>;
}
