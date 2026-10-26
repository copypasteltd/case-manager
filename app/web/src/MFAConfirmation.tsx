import { useEffect, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ensureDownloadSession, type Me, type Schemas } from './api';
import { ErrorNotice, Field } from './components';

// Kept inside a business dialog: refreshing proof never submits its decision.
export function MFAConfirmation({ onFresh }: { onFresh: (value: boolean) => void }) {
  const cache = useQueryClient();
  const me = useQuery({ queryKey: ['current-security'], queryFn: ({ signal }) => api<Me>('/me', { signal }), staleTime: 0, refetchInterval: 30_000 });
  const [code, setCode] = useState(''), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [now, setNow] = useState(Date.now());
  const verified = Date.parse(me.data?.mfa_verified_at ?? '');
  const fresh = Number.isFinite(verified) && verified <= now + 5000 && now - verified < 9 * 60_000;
  useEffect(() => { const timer = setInterval(() => setNow(Date.now()), 15_000); return () => clearInterval(timer); }, []);
  useEffect(() => { onFresh(fresh); }, [fresh, onFresh]);
  async function verify() {
    setBusy(true); setError(undefined);
    try {
      await ensureDownloadSession();
      await api<Schemas['MFAStepUpResult']>('/auth/mfa/step-up', { method: 'POST', body: { code } });
      setCode(''); setNow(Date.now()); await cache.invalidateQueries({ queryKey: ['current-security'] });
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  return <section className="mfa-confirmation" aria-label="敏感操作身份验证"><ErrorNotice error={error ?? me.error} />{fresh ? <p className="muted">最近已完成两步验证。请继续核对内容，再确认本次操作。</p> : <><p className="muted">此操作要求最近十分钟内完成两步验证。验证后会保留当前填写内容，仍需手动确认操作。</p><Field label="验证器动态码"><input type="text" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} value={code} onChange={event => setCode(event.target.value.replace(/\D/g, ''))} disabled={busy} /></Field><button className="button secondary" type="button" disabled={busy || code.length !== 6} onClick={() => void verify()}>{busy ? '正在验证' : '验证当前身份'}</button><button className="text-button" type="button" disabled={busy} onClick={() => void me.refetch()}>重新读取验证状态</button></>}</section>;
}
