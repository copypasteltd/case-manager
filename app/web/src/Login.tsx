import { useState, type FormEvent } from 'react';
import { useMutation } from '@tanstack/react-query';
import { ArrowRight, ArrowLeft, ShieldCheck, Check, Copy } from 'lucide-react';
import { api, type Schemas } from './api';
import { ErrorNotice, Field } from './components';
import { PasswordRecovery } from './PasswordRecovery';

export default function Login({ onSignedIn }: { onSignedIn: () => void }) {
  const [resetting, setResetting] = useState(false);
  const [challenge, setChallenge] = useState('');
  const [step, setStep] = useState<'login' | 'mfa' | 'mfa_enroll' | 'recovery'>('login');
  const [secret, setSecret] = useState('');
  const [uri, setUri] = useState('');
  const [codes, setCodes] = useState<string[]>([]);
  const [copied, setCopied] = useState(false);
  const [saved, setSaved] = useState(false);
  const [recovering, setRecovering] = useState(false);
  const [activating, setActivating] = useState(false);
  const [activated, setActivated] = useState(false);
  const activation = useMutation({
    mutationFn: (form: FormData) => api('/auth/activate', { method: 'POST', body: { token: String(form.get('token')).trim(), password: form.get('password') } }),
    onSuccess: () => { setActivating(false); setActivated(true); },
  });
  const mutation = useMutation({
    mutationFn: async (form: FormData) => {
      if (step === 'login') {
        const result = await api<Schemas['LoginResult']>('/auth/login', { method: 'POST', body: { login: form.get('login'), password: form.get('password'), device_type: 'web' } });
        if (result.step === 'mfa_enroll' && result.challenge) {
          const enrollment = await api<Schemas['LoginResult']>('/auth/mfa/enroll', { method: 'POST', body: { challenge: result.challenge } });
          setSecret(enrollment.mfa_secret ?? ''); setUri(enrollment.mfa_uri ?? '');
        }
        return result;
      }
      return api<Schemas['LoginResult']>(recovering ? '/auth/mfa/recover' : '/auth/mfa/verify', { method: 'POST', body: { challenge, code: form.get('code') } });
    },
    onSuccess: result => {
      if (result.step === 'authenticated') {
        if (result.recovery_codes?.length) { setCodes(result.recovery_codes); setStep('recovery'); } else onSignedIn();
      } else {
        setChallenge(result.challenge ?? challenge); setStep(result.step);
      }
    },
  });
  function submit(event: FormEvent<HTMLFormElement>) { event.preventDefault(); mutation.mutate(new FormData(event.currentTarget)); }
  async function copy(value: string) { try { await navigator.clipboard.writeText(value); setCopied(true); } catch { setCopied(false); } }
  if (resetting) return <main className="login-layout"><section className="login-content"><div className="login-form"><span className="eyebrow">RECOVER YOUR ACCOUNT</span><h2>重置登录密码</h2><PasswordRecovery onBack={() => setResetting(false)} /></div></section></main>;
  return <main className="login-layout">
    <section className="login-content"><div className="login-form">
      <div className="login-brand"><img src="/brand/logo-light.png" className="logo-light" alt="" /><img src="/brand/logo-dark.png" className="logo-dark" alt="" /><span className="brand-name">copypaste<span className="accent">.</span></span></div>
      <span className="eyebrow">WELCOME TO COPYPASTE</span><h2>{activating ? '接受公司邀请' : step === 'login' ? '回到工作空间' : step === 'mfa_enroll' ? '设置两步验证' : step === 'recovery' ? '保存恢复码' : '确认是你本人'}</h2>
      <p className="muted">{step === 'login' ? '使用公司的内部账号登录，继续今天的工作。' : step === 'mfa_enroll' ? '在验证器中添加账号，再输入生成的六位验证码。' : step === 'recovery' ? '恢复码仅显示一次，请保存至安全的位置。每个恢复码只能使用一次。' : '请输入验证器当前显示的六位验证码。'}</p>
      <ErrorNotice error={activating ? activation.error : mutation.error} />
      {activated && !activating && <div className="notice neutral" role="status">账号已激活，请使用管理员分配的内部账号和刚设置的密码登录。</div>}
      {activating ? <form onSubmit={event => { event.preventDefault(); activation.mutate(new FormData(event.currentTarget)); }}><Field label="邀请码"><textarea name="token" autoComplete="off" required rows={3} minLength={40} maxLength={100} autoFocus /></Field><Field label="设置密码" hint="至少12个字符，建议使用密码管理器生成的独立密码。"><input name="password" type="password" autoComplete="new-password" minLength={12} maxLength={1024} required /></Field><button className="button primary full" disabled={activation.isPending}>{activation.isPending ? '正在激活…' : '激活内部账号'}<ArrowRight size={16} /></button></form> : step === 'recovery' ? <><div className="recovery-codes">{codes.map(code => <code key={code}>{code}</code>)}</div><button className="button secondary full" onClick={() => void copy(codes.join('\n'))}>{copied ? <Check size={16} /> : <Copy size={16} />}复制恢复码</button><label className="check-field"><input type="checkbox" checked={saved} onChange={event => setSaved(event.target.checked)} />我已安全保存这些恢复码</label><button className="button primary full" disabled={!saved} onClick={() => { setCodes([]); onSignedIn(); }}>进入工作空间 <ArrowRight size={18} /></button></> : <form onSubmit={submit}>
        {step === 'login' ? <><Field label="内部账号"><input name="login" autoComplete="username" required autoFocus maxLength={200} placeholder="你的公司账号" /></Field><Field label="密码"><input name="password" type="password" autoComplete="current-password" required maxLength={1024} placeholder="输入密码" /></Field></> : <>
          {step === 'mfa_enroll' && <div className="mfa-setup"><ShieldCheck size={22} /><span>验证器设置密钥</span><code>{secret}</code><button type="button" className="text-button" onClick={() => void copy(secret)}>{copied ? '已复制' : '复制密钥'}</button><a href={uri}>在此设备的验证器中添加</a></div>}
          {recovering ? <Field label="一次性恢复码"><input key="recovery" name="code" autoComplete="off" pattern="[A-Za-z0-9_-]{20}" minLength={20} maxLength={20} required autoFocus placeholder="输入已保存的恢复码" /></Field> : <Field label="六位验证码"><input key="totp" name="code" className="otp-input" autoComplete="one-time-code" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} minLength={6} required autoFocus placeholder="000000" /></Field>}
        </>}
        <button className="button primary full" type="submit" disabled={mutation.isPending}>{mutation.isPending ? '正在验证…' : step === 'login' ? '登录工作空间' : '验证并继续'}<ArrowRight size={18} /></button>
      </form>}
      {step === 'login' && <button className="text-button back-login" disabled={mutation.isPending || activation.isPending} onClick={() => { setActivating(value => !value); mutation.reset(); activation.reset(); }}>{activating ? '返回登录' : '接受邀请'}</button>}
      {step === 'mfa' && <button className="text-button" onClick={() => { setRecovering(value => !value); mutation.reset(); }}>{recovering ? '使用验证器验证码' : '无法使用验证器？使用恢复码'}</button>}
      {step !== 'login' && step !== 'recovery' && <button className="text-button back-login" onClick={() => { setStep('login'); setRecovering(false); setChallenge(''); setSecret(''); mutation.reset(); }}><ArrowLeft size={16} />返回登录</button>}
      {step === 'login' && !activating && <button className="text-button back-login" disabled={mutation.isPending} onClick={() => setResetting(true)}>忘记密码</button>}
      <p className="login-note"><ShieldCheck size={15} />仅限获授权的公司成员使用</p>
    </div><footer>一起创造，更好的下一步。<span>CopyPaste / 香港</span></footer></section>
  </main>;
}
