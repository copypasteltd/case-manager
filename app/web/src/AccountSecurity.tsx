import { useState } from 'react';
import { MiniappBindings } from './MiniappBindings';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field, date } from './components';

const devices = { web: 'Web 浏览器', android: 'Android', wechat: '微信小程序', douyin: '抖音小程序' };
const states = { active: '有效', expired: '已到期', revoked: '已撤销' };
type Device = Schemas['DeviceSession'];
function signedOut() { window.dispatchEvent(new Event('cp:session-expired')); }

export function AccountVerification() {
  const [verified, setVerified] = useState(false);
  const verify = useMutation({ mutationFn: (code: string) => api('/auth/mfa/step-up', { method: 'POST', body: { code } }), onSuccess: () => setVerified(true) });
  return <div className="panel"><form onSubmit={event => { event.preventDefault(); const form = event.currentTarget; const code = String(new FormData(form).get('code')); form.reset(); verify.mutate(code); }}><ErrorNotice error={verify.error} /><Field label="两步验证确认" hint="敏感操作需要最近10分钟内验证。已登录时可在这里重新确认。"><input name="code" autoComplete="one-time-code" inputMode="numeric" pattern="[0-9]{6}" minLength={6} maxLength={6} required /></Field><button className="button secondary" disabled={verify.isPending}>验证本人身份</button>{verified && <p role="status">已完成两步验证。</p>}</form></div>;
}

export default function AccountSecurity({ me }: { me: Me }) {
  const cache = useQueryClient();
  const [revoke, setRevoke] = useState<Device | null>(null), [change, setChange] = useState(false);
  const sessions = useInfiniteQuery({ queryKey: ['own-sessions', me.user.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Schemas['DeviceSessionListResponse']['data']>('/me/sessions?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: value => value.next_cursor ?? undefined });
  const rows = sessions.data?.pages.flatMap(page => page.items) ?? [];
  async function refresh() { await cache.invalidateQueries({ queryKey: ['own-sessions'] }); }
  return <><div className="page-heading"><div><span className="eyebrow">YOUR ACCOUNT</span><h1>账号安全<span className="accent">.</span></h1><p>查看登录设备、撤销会话和修改密码。</p></div><button className="button primary" onClick={() => setChange(true)}>修改密码</button></div>
    <ErrorNotice error={sessions.error} />
    <div className="section-heading"><h2>我的登录设备</h2><button className="button secondary" disabled={sessions.isFetching} onClick={() => void refresh()}>刷新设备</button></div>
    <p className="muted">同一次登录的凭证刷新会合并到一个设备会话。到期或撤销的历史记录仍可查阅。</p>
    {sessions.isPending ? <Busy /> : !rows.length ? <Empty title="暂无设备记录" detail="刷新列表后核对登录状态。" /> : <div className="panel table-panel"><div className="table-scroll" tabIndex={0} aria-label="我的登录设备"><table><thead><tr><th>设备</th><th>状态</th><th>首次登录 / 最近刷新</th><th>最长会话有效至</th><th>操作</th></tr></thead><tbody>{rows.map(item => <tr key={item.id} data-session-id={item.id}><td><strong>{devices[item.device_type]}</strong>{item.is_current && <small>当前设备</small>}</td><td>{states[item.status]}</td><td>{date(item.started_at)}<small>{date(item.rotated_at)}</small></td><td>{date(item.refresh_expires_at)}</td><td>{item.status === 'active' && <button className="text-button danger-text" onClick={() => setRevoke(item)}>{item.is_current ? '退出当前设备' : '撤销此设备'}</button>}</td></tr>)}</tbody></table></div></div>}
    {sessions.hasNextPage && <button className="button secondary" disabled={sessions.isFetchingNextPage} onClick={() => void sessions.fetchNextPage()}>加载更多设备</button>}
    <MiniappBindings />
    <section className="section-block"><h2>两步验证</h2><p className="muted">已设置验证器的账号可在这里确认本人操作。</p><AccountVerification /></section>
    {revoke && <RevokeDevice item={revoke} onClose={() => setRevoke(null)} onDone={async () => { if (revoke.is_current) signedOut(); else { setRevoke(null); await refresh(); } }} />}
    {change && <ChangePassword onClose={() => setChange(false)} />}
  </>;
}
function RevokeDevice({ item, onClose, onDone }: { item: Device; onClose(): void; onDone(): Promise<void> }) {
  const [key] = useState(() => crypto.randomUUID());
  const revoke = useMutation({ mutationFn: () => api('/me/sessions/' + item.id + '/revoke', { method: 'POST', key, body: { reason: item.is_current ? '本人退出当前设备' : '本人撤销不再使用的设备' } }), onSuccess: onDone });
  return <Dialog title={item.is_current ? '退出当前设备' : '撤销登录设备'} busy={revoke.isPending} onClose={onClose}><p>设备：{devices[item.device_type]}。撤销后，此次登录的访问与刷新凭证会立即失效。</p><ErrorNotice error={revoke.error} /><div className="dialog-actions"><button className="button secondary" disabled={revoke.isPending} onClick={onClose}>取消</button><button className="button danger" disabled={revoke.isPending} onClick={() => revoke.mutate()}>确认撤销</button></div></Dialog>;
}
function ChangePassword({ onClose }: { onClose(): void }) {
  const [key] = useState(() => crypto.randomUUID()), [mismatch, setMismatch] = useState(false);
  const change = useMutation({ mutationFn: (body: Schemas['PasswordChangeInput']) => api('/me/password', { method: 'POST', key, body }), onSuccess: signedOut });
  return <Dialog title="修改密码" onClose={onClose} busy={change.isPending}><form onSubmit={event => { event.preventDefault(); const form = event.currentTarget, body = new FormData(form); if (body.get('password') !== body.get('confirm')) { setMismatch(true); return; } setMismatch(false); form.reset(); change.mutate({ current_password: String(body.get('current_password')), password: String(body.get('password')) }); }}><p className="muted">保存后，所有设备会退出登录。请使用新密码重新登录；两步验证配置继续保留。</p><ErrorNotice error={change.error} />{mismatch && <p role="alert">两次新密码不一致。</p>}<Field label="当前密码"><input name="current_password" type="password" autoComplete="current-password" required maxLength={1024} /></Field><Field label="新密码" hint="至少12个字符。"><input name="password" type="password" autoComplete="new-password" required minLength={12} maxLength={1024} /></Field><Field label="再次输入新密码"><input name="confirm" type="password" autoComplete="new-password" required minLength={12} maxLength={1024} /></Field><div className="dialog-actions"><button type="button" className="button secondary" disabled={change.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={change.isPending}>保存新密码</button></div></form></Dialog>;
}
