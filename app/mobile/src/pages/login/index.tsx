import { Checkbox, Text, View } from '@tarojs/components';
import { Button, Input } from '../../platform/components';
import Taro, { useDidHide } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { client } from '../../platform/taro';
import type { Schemas } from '../../platform/session';
import { Action, Field, Notice, Page } from '../../ui';
import { PasswordRecovery } from './password-recovery';

export default function LoginPage() {
  const [resetting, setResetting] = useState(false);
  const [step, setStep] = useState<'login' | 'mfa' | 'mfa_enroll' | 'recovery' | 'activate'>('login');
  const [login, setLogin] = useState(''), [password, setPassword] = useState(''), [code, setCode] = useState(''), [token, setToken] = useState('');
  const [challenge, setChallenge] = useState(''), [secret, setSecret] = useState(''), [codes, setCodes] = useState<string[]>([]);
  const [recovery, setRecovery] = useState(false), [saved, setSaved] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [notice, setNotice] = useState('');
  const alive = useRef(true), finished = useRef(false);
  useDidHide(() => { setPassword(''); setCode(''); });
  useEffect(() => () => { alive.current = false; if (!finished.current) client.clear(); }, []);
  async function enter() { finished.current = true; setCodes([]); setSecret(''); setChallenge(''); try { await Taro.reLaunch({ url: '/pages/home/index' }); } catch { finished.current = false; setNotice('页面暂时未能打开，请再次进入。'); } }
  async function submit() {
    if (busy) return; setBusy(true); setError(undefined); setNotice('');
    try {
      if (step === 'activate') { await client.activate(token.trim(), password); if (alive.current) { setStep('login'); setToken(''); setPassword(''); setNotice('账号已激活，请使用分配给你的内部账号登录。'); } return; }
      let result: Schemas['LoginResult'];
      if (step === 'login') {
        result = await client.login(login.trim(), password); setPassword('');
        if (result.step === 'mfa_enroll' && result.challenge) { const enrollment = await client.enroll(result.challenge); if (alive.current) setSecret(enrollment.mfa_secret ?? ''); }
      } else result = await client.verify(challenge, code.trim(), recovery);
      if (!alive.current) return; setCode('');
      if (result.step === 'authenticated') { if (result.recovery_codes?.length) { setCodes(result.recovery_codes); setSaved(false); setStep('recovery'); } else await enter(); }
      else { setChallenge(result.challenge ?? ''); setStep(result.step); }
    } catch (cause) { if (alive.current) setError(cause); }
    finally { if (alive.current) setBusy(false); }
  }
  function back() { client.clear(); setStep('login'); setChallenge(''); setCode(''); setSecret(''); setRecovery(false); setError(undefined); }
  const valid = step === 'login' ? login.trim() && password : step === 'activate' ? token.trim().length >= 40 && password.length >= 12 : recovery ? /^[A-Za-z0-9_-]{20}$/.test(code.trim()) : /^\d{6}$/.test(code);
  if (resetting) return <PasswordRecovery onBack={() => setResetting(false)} />;
  return <Page title={step === 'login' ? '回到工作空间' : step === 'activate' ? '接受公司邀请' : step === 'mfa_enroll' ? '设置两步验证' : step === 'recovery' ? '保存恢复码' : '确认是你本人'} eyebrow='WELCOME TO COPYPASTE'>
    <Text className='cp-muted cp-paragraph'>{step === 'login' ? '使用公司的内部账号登录，继续今天的工作。' : step === 'mfa_enroll' ? '在验证器中添加下方设置密钥，再输入生成的六位验证码。' : step === 'recovery' ? '恢复码仅显示一次，请保存至安全位置。每个码只能使用一次。' : step === 'activate' ? '使用管理员提供的邀请码，设置你的独立密码。' : '输入验证器验证码，或使用已保存的一次性恢复码。'}</Text>
    <Notice error={error} />{notice && <View className='cp-notice'><Text>{notice}</Text></View>}
    {step === 'recovery' ? <><View className='cp-code'>{codes.map(value => <Text key={value} className='cp-paragraph' selectable>{value}</Text>)}</View><View className='cp-check' onClick={() => setSaved(value => !value)}><Checkbox ariaLabel='我已安全保存这些恢复码' value='saved' checked={saved} /><Text>我已安全保存这些恢复码</Text></View><Action onClick={() => void enter()} disabled={!saved}>进入工作空间 ↗</Action></> : <>
      {step === 'login' ? <><Field label='内部账号'><Input ariaLabel='内部账号' className='cp-input' value={login} maxlength={200} disabled={busy} placeholder='你的公司账号' placeholderClass='cp-placeholder' onInput={event => setLogin(event.detail.value)} /></Field><Field label='密码'><Input ariaLabel='密码' className='cp-input' password value={password} maxlength={1024} disabled={busy} placeholder='输入密码' placeholderClass='cp-placeholder' onInput={event => setPassword(event.detail.value)} onConfirm={() => valid && void submit()} /></Field></> : step === 'activate' ? <><Field label='邀请码'><Input ariaLabel='邀请码' className='cp-input' password value={token} maxlength={100} disabled={busy} onInput={event => setToken(event.detail.value)} /></Field><Field label='设置密码' hint='至少12个字符，建议使用密码管理器生成独立密码。'><Input ariaLabel='密码' className='cp-input' password value={password} maxlength={1024} disabled={busy} onInput={event => setPassword(event.detail.value)} /></Field></> : <>
        {step === 'mfa_enroll' && <View className='cp-code'><Text selectable>{secret}</Text></View>}
        <Field label={recovery ? '一次性恢复码' : '六位验证码'}><Input ariaLabel={recovery ? '一次性恢复码' : '六位验证码'} key={recovery ? 'recovery' : 'totp'} className='cp-input' value={code} type={recovery ? 'text' : 'number'} maxlength={recovery ? 20 : 6} disabled={busy} onInput={event => setCode(event.detail.value)} onConfirm={() => valid && void submit()} /></Field>
      </>}
      <Action onClick={() => void submit()} loading={busy} disabled={!valid}>{step === 'login' ? '登录工作空间' : step === 'activate' ? '激活内部账号' : '验证并继续'} ↗</Action>
      {step === 'mfa' && <Button className='cp-link' disabled={busy} onClick={() => { setRecovery(v => !v); setCode(''); setError(undefined); }}>{recovery ? '使用验证器验证码' : '使用一次性恢复码'}</Button>}
      {step === 'login' ? <Button className='cp-link' disabled={busy} onClick={() => { setPassword(''); setStep('activate'); setError(undefined); }}>接受公司邀请</Button> : <Button className='cp-link' disabled={busy} onClick={back}>返回登录</Button>}
    </>}
    {step === 'login' && <Action secondary disabled={busy} onClick={() => { setPassword(''); setResetting(true); }}>忘记密码</Action>}
    <Text className='cp-muted cp-small cp-paragraph'>仅限获授权的公司成员使用</Text>
  </Page>;
}
