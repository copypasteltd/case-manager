import { Text, View } from '@tarojs/components';
import { MiniappBindings } from './miniapp-bindings';
import Taro, { useDidHide, useDidShow } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { Input } from '../../platform/components';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { MFAConfirmation } from '../../platform/command';
import { Action, Fact, Field, Notice, Section, date } from '../../ui';

type Devices = Schemas['DeviceSessionListResponse']['data'];
const devices = { web: 'Web 浏览器', android: 'Android', wechat: '微信小程序', douyin: '抖音小程序' };
const states = { active: '有效', expired: '已到期', revoked: '已撤销' };
export function AccountSettings({ initial, me, onReload }: { initial: Devices; me: Schemas['Me']; onReload(): Promise<void> }) {
  const [page, setPage] = useState(initial), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false), [changing, setChanging] = useState(false);
  const active = useRef(true), pending = useRef(false), scope = useRef(new Scope()), keys = useRef(new Map<string, string>());
  useEffect(() => () => { active.current = false; scope.current.close(); keys.current.clear(); }, []);
  useDidShow(() => { active.current = true; });
  useDidHide(() => { active.current = false; scope.current.close(); });
  async function more() {
    if (pending.current || !page.next_cursor) return; pending.current = true; setBusy(true); setError(undefined);
    try { const next = await client.request<Devices>('/me/sessions?' + new URLSearchParams({ limit: '20', cursor: page.next_cursor }), { scope: scope.current }); if (active.current) setPage({ items: [...page.items, ...next.items.filter(item => !page.items.some(old => old.id === item.id))], next_cursor: next.next_cursor }); }
    catch (cause) { if (active.current) setError(cause); } finally { pending.current = false; if (active.current) setBusy(false); }
  }
  async function revoke(item: Schemas['DeviceSession']) {
    if (pending.current) return;
    pending.current = true; setError(undefined);
    try {
      const answer = await Taro.showModal({ title: item.is_current ? '退出当前设备' : '撤销登录设备', content: devices[item.device_type] + ' 的访问与刷新凭证将立即失效。', confirmText: '确认撤销', cancelText: '取消' });
      if (!answer.confirm || !active.current) return; setBusy(true);
      const key = keys.current.get(item.id) ?? client.operationKey(); keys.current.set(item.id, key);
      await client.revokeDevice(item.id, item.is_current, key); keys.current.delete(item.id);
      if (active.current && !item.is_current) await onReload();
    } catch (cause) { if (active.current) setError(cause); } finally { pending.current = false; if (active.current) setBusy(false); }
  }
  return <><MiniappBindings /><Section title='账号安全'><View className='cp-pad'><Action secondary disabled={busy} onClick={() => setChanging(value => !value)}>{changing ? '关闭密码表单' : '修改密码'}</Action>{changing && <ChangePassword me={me} />}</View></Section><Section title='我的登录设备'><Notice error={error} />{page.items.map(item => <View key={item.id} className='cp-pad'><Fact label='设备'><Text>{devices[item.device_type]}{item.is_current ? ' · 当前设备' : ''}</Text></Fact><Fact label='状态'><Text>{states[item.status]}</Text></Fact><Fact label='首次登录'><Text>{date(item.started_at)}</Text></Fact><Fact label='最近刷新'><Text>{date(item.rotated_at)}</Text></Fact><Fact label='最长会话有效至'><Text>{date(item.refresh_expires_at)}</Text></Fact>{item.status === 'active' && <Action secondary disabled={busy} onClick={() => void revoke(item)}>{item.is_current ? '退出当前设备' : '撤销此设备'}</Action>}</View>)}{page.next_cursor && <Action secondary loading={busy} onClick={() => void more()}>加载更多设备</Action>}<Action secondary disabled={busy} onClick={() => void onReload()}>刷新设备</Action></Section></>;
}
function ChangePassword({ me }: { me: Schemas['Me'] }) {
  const [current, setCurrent] = useState(''), [password, setPassword] = useState(''), [confirm, setConfirm] = useState('');
  const [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false), [uncertain, setUncertain] = useState(false);
  const active = useRef(true), pending = useRef(false), key = useRef('');
  const clear = () => { setCurrent(''); setPassword(''); setConfirm(''); };
  useDidHide(() => { active.current = false; clear(); });
  useEffect(() => () => { active.current = false; }, []);
  async function submit() {
    if (pending.current || uncertain) return;
    if (password !== confirm) { setError(new MobileError('PASSWORD_MISMATCH', '两次新密码不一致。')); return; }
    pending.current = true; setBusy(true); setError(undefined);
    const old = current, next = password; clear();
    try { if (!key.current) key.current = client.operationKey(); await client.changePassword(old, next, key.current); }
    catch (cause) { if (active.current) { setError(cause); setUncertain(cause instanceof MobileError && ['RESULT_UNCONFIRMED', 'INVALID_RESPONSE', 'SESSION_CHANGED'].includes(cause.code)); } }
    finally { pending.current = false; if (active.current) setBusy(false); }
  }
  const disabled = busy || uncertain;
  return <View><Text className='cp-muted cp-paragraph'>保存后所有设备退出。请用新密码重新登录；两步验证继续保留。</Text><Notice error={error} />{me.mfa_verified_at && <MFAConfirmation disabled={disabled} onFresh={() => {}} />}<Field label='当前密码'><Input ariaLabel='当前密码' password className='cp-input' maxlength={1024} value={current} disabled={disabled} onInput={event => setCurrent(event.detail.value)} /></Field><Field label='新密码' hint='至少12个字符。'><Input ariaLabel='新密码' password className='cp-input' maxlength={1024} value={password} disabled={disabled} onInput={event => setPassword(event.detail.value)} /></Field><Field label='再次输入新密码'><Input ariaLabel='再次输入新密码' password className='cp-input' maxlength={1024} value={confirm} disabled={disabled} onInput={event => setConfirm(event.detail.value)} /></Field><Action loading={busy} disabled={disabled || !current || [...password].length < 12 || !confirm} onClick={() => void submit()}>保存新密码</Action>{uncertain && <Text>操作结果尚未确认，请使用新密码重新登录核对。</Text>}</View>;
}
