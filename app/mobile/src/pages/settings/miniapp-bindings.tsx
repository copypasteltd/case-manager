import { Text, View } from '@tarojs/components';
import Taro, { useDidHide, useDidShow } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { Input } from '../../platform/components';
import { MFAConfirmation } from '../../platform/command';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { miniappIdentity, miniappPlatform } from '../../platform/miniapp-login';
import { Action, Fact, Field, Notice, Section, date } from '../../ui';

type Page = Schemas['MiniappBindingListResponse']['data'];
const names = { wechat: '微信小程序', douyin: '抖音小程序' };
export function MiniappBindings() {
  const [page, setPage] = useState<Page>(), [password, setPassword] = useState(''), [error, setError] = useState<unknown>(), [notice, setNotice] = useState(''), [busy, setBusy] = useState(false), [uncertain, setUncertain] = useState(false), [verify, setVerify] = useState(false);
  const alive = useRef(true), pending = useRef(false), scope = useRef(new Scope()), revision = useRef(0);
  const clear = () => { alive.current = false; scope.current.close(); setPassword(''); };
  useEffect(() => { alive.current = true; void reload(); return () => { alive.current = false; scope.current.close(); }; }, []);
  useDidHide(clear);
  useDidShow(() => { alive.current = true; setBusy(false); scope.current.close(); scope.current = new Scope(); void reload(); });
  async function reload(more = false) {
    const ticket = ++revision.current;
    try {
      const next = await client.request<Page>('/me/miniapp-bindings?' + new URLSearchParams({ limit: '20', ...(more && page?.next_cursor ? { cursor: page.next_cursor } : {}) }), { scope: scope.current });
      if (alive.current && ticket === revision.current) { setPage(old => more && old ? { ...next, items: [...old.items, ...next.items.filter(item => !old.items.some(prior => prior.id === item.id))] } : next); setUncertain(false); }
    } catch (cause) { if (alive.current && ticket === revision.current) setError(cause); }
  }
  async function bind() {
    if (pending.current || uncertain) return; pending.current = true; setBusy(true); setError(undefined); setNotice('');
    let sent = false;
    try {
      const identity = await miniappIdentity(); if (!alive.current) return;
      const current_password = password; setPassword('');
      const proof = await client.request<Schemas['MiniappProof']>('/me/miniapp-binding-proofs', { method: 'POST', body: { ...identity, current_password }, scope: scope.current });
      if (!alive.current) return;
      const answer = await Taro.showModal({ title: '确认绑定本人平台身份', content: names[proof.platform] + ' · ' + proof.app_id + '。绑定至当前内部账号，今后平台登录仍须完成适用的两步验证。请确认当前平台账号由你本人使用。', confirmText: '确认绑定', cancelText: '取消' });
      if (!answer.confirm || !alive.current) return;
      if (Date.parse(proof.expires_at) <= Date.now()) throw new MobileError('MINIAPP_PROOF_INVALID', '绑定确认已过期，请重新操作。');
      sent = true;
      await client.request('/me/miniapp-bindings', { method: 'POST', key: client.operationKey(), body: { proof_id: proof.id, confirmed: true, reason: '本人确认平台身份并绑定当前内部账号' }, scope: scope.current });
      if (alive.current) { setNotice('平台身份已绑定。'); await reload(); }
    } catch (cause) { if (alive.current) { setError(cause); if (sent) setUncertain(true); } }
    finally { pending.current = false; if (alive.current) setBusy(false); }
  }
  async function revoke(item: Schemas['MiniappBinding']) {
    if (pending.current || uncertain) return; pending.current = true; setError(undefined); setNotice(''); let sent = false;
    try {
      const answer = await Taro.showModal({ title: '解除平台身份绑定', content: names[item.platform] + ' 的绑定登录、刷新凭证及待完成验证将立即失效。' + (item.is_current_session_source ? '本次登录也会退出。' : '绑定历史会保留。'), confirmText: '确认解绑', cancelText: '取消' });
      if (!answer.confirm || !alive.current) return; setBusy(true); sent = true;
      await client.request('/me/miniapp-bindings/' + item.id + '/revoke', { method: 'POST', key: client.operationKey(), body: { expected_version: item.version, reason: '本人解除不再使用的平台身份绑定' }, scope: scope.current });
      if (item.is_current_session_source) { client.clear(); await Taro.reLaunch({ url: '/pages/login/index' }); }
      else if (alive.current) { setNotice('平台身份已解绑。'); await reload(); }
    } catch (cause) { if (alive.current) { setError(cause); if (sent) setUncertain(true); } }
    finally { pending.current = false; if (alive.current) setBusy(false); }
  }
  return <Section title='我的小程序绑定'><View className='cp-pad'><Text className='cp-muted cp-paragraph'>首次绑定请使用内部账号登录对应小程序，并确认当前密码。已设置验证器的账号须先完成最近10分钟内的两步验证。</Text><Action secondary disabled={busy} onClick={() => setVerify(value => !value)}>{verify ? '收起两步验证' : '已设置验证器：确认本人操作'}</Action>{verify && <MFAConfirmation disabled={busy} onFresh={() => {}} />}<Notice error={error} />{notice && <Text className='cp-notice'>{notice}</Text>}{uncertain && <Text className='cp-notice'>操作结果尚未核对。请刷新绑定列表确认，勿重复提交。</Text>}{miniappPlatform() && <><Field label='绑定时确认当前密码'><Input ariaLabel='绑定时确认当前密码' className='cp-input' password value={password} maxlength={1024} disabled={busy || uncertain} onInput={event => setPassword(event.detail.value)} /></Field><Action loading={busy} disabled={!password || uncertain} onClick={() => void bind()}>绑定当前小程序身份</Action></>}{page?.items.map(item => <View key={item.id} className='cp-pad'><Fact label='平台'><Text>{names[item.platform]}{item.is_current_session_source ? ' · 本次登录来源' : ''}</Text></Fact><Fact label='AppID'><Text selectable>{item.app_id}</Text></Fact><Fact label='绑定时间'><Text>{date(item.created_at)}</Text></Fact><Fact label='状态'><Text>{item.revoked_at ? '已解绑 · ' + date(item.revoked_at) : '已绑定'}</Text></Fact>{!item.revoked_at && <Action secondary disabled={busy || uncertain} onClick={() => void revoke(item)}>解除绑定</Action>}</View>)}{page && !page.items.length && <Text className='cp-muted'>暂无平台身份绑定。</Text>}{page?.next_cursor && <Action secondary disabled={busy} onClick={() => void reload(true)}>加载更多绑定</Action>}<Action secondary disabled={busy} onClick={() => void reload()}>刷新绑定</Action></View></Section>;
}
