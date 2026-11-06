import { Text, View } from '@tarojs/components';
import Taro, { useDidHide } from '@tarojs/taro';
import { useEffect, useState } from 'react';
import { checkedReissuedInvitation, reissueInvitationInput, type Invitation } from '@copypaste/contracts/invitations';
import { client } from '../../platform/taro';
import { usePage } from '../../platform/page';
import { Input, Textarea } from '../../platform/components';
import { MFAConfirmation, useCommand } from '../../platform/command';
import { MobileError, type Schemas, type Scope } from '../../platform/session';
import { Action, Busy, Fact, Field, Notice, Page, Section, date } from '../../ui';

type Users = Schemas['UserListResponse']['data'];
type User = Schemas['User'];
const states = { invited: '待激活', active: '有效', disabled: '已停用' };

export default function MembersPage() {
  const page = usePage(async (scope, me) => {
    if (!me.actions.includes('user.read')) throw new MobileError('FORBIDDEN', '当前账号无法管理成员。');
    return client.request<Users>('/users?limit=20', { scope });
  });
  return <Page title='成员与邀请' eyebrow='MEMBERS / INVITATIONS'><Notice error={page.error} />{page.loading ? <Busy /> : page.result && page.scope &&
    <MembersWorkspace key={page.result.me.user.id + ':' + page.result.me.authorization_version} initial={page.result.value} me={page.result.me} scope={page.scope} />}
    <Action secondary onClick={() => void Taro.navigateBack().catch(() => Taro.redirectTo({ url: '/pages/settings/index' }))}>返回我的工作空间</Action></Page>;
}

function MembersWorkspace({ initial, me, scope }: { initial: Users; me: Schemas['Me']; scope: Scope }) {
  const [users, setUsers] = useState(initial), [busy, setBusy] = useState(false), [error, setError] = useState<unknown>();
  const [mode, setMode] = useState<'invite' | 'read' | null>(null), [selected, setSelected] = useState<User>(), [receipt, setReceipt] = useState<Invitation>(), [epoch, setEpoch] = useState(0);
  const permitted = me.actions.includes('user.invite');
  useDidHide(() => { setReceipt(undefined); setMode(null); setSelected(undefined); });
  async function reload() { const value = await client.request<Users>('/users?limit=20', { scope }); if (!scope.closed) setUsers(value); }
  async function more() {
    if (busy || !users.next_cursor) return;
    setBusy(true); setError(undefined);
    try { const next = await client.request<Users>('/users?' + new URLSearchParams({ limit: '20', cursor: users.next_cursor }), { scope }); if (!scope.closed) setUsers({ items: [...users.items, ...next.items.filter(row => !users.items.some(old => old.id === row.id))], next_cursor: next.next_cursor }); }
    catch (cause) { if (!scope.closed) setError(cause); } finally { if (!scope.closed) setBusy(false); }
  }
  function committed(value: Invitation) {
    setReceipt(value); setMode(null); setEpoch(value => value + 1);
    void reload().catch(cause => { if (!scope.closed) setError(cause); });
  }
  return <><Notice error={error} /><Section title='内部成员'>
    <Text className='cp-muted cp-small'>邀请不会授予业务角色。未激活的邀请可重新签发，旧邀请码立即失效。</Text>
    {users.items.map(user => <View className='cp-card' key={user.id}><Fact label='成员'><Text>{user.display_name}</Text></Fact><Fact label='状态'><Text>{states[user.status]}</Text></Fact>
      {permitted && user.status === 'invited' && <Action secondary onClick={() => { setReceipt(undefined); setSelected(user); setMode('read'); setEpoch(value => value + 1); }}>查看邀请 · {user.display_name}</Action>}</View>)}
    {!users.items.length && <Text>暂无成员。</Text>}{users.next_cursor && <Action secondary loading={busy} onClick={() => void more()}>加载更多成员</Action>}
    {permitted && <Action onClick={() => { setReceipt(undefined); setSelected(undefined); setMode('invite'); setEpoch(value => value + 1); }}>邀请新成员</Action>}</Section>
    {mode && <InvitationAction key={epoch} mode={mode} user={selected} me={me} scope={scope} onReceipt={committed} onClose={() => setMode(null)} />}
    {receipt && <InvitationReceiptPanel key={receipt.invitation_id} receipt={receipt} verifiedAt={me.mfa_verified_at} onReceipt={committed} onClose={() => setReceipt(undefined)} />}
  </>;
}

function InvitationAction({ mode, user, me, scope, onReceipt, onClose }: { mode: 'invite' | 'read'; user?: User; me: Schemas['Me']; scope: Scope; onReceipt(value: Invitation): void; onClose(): void }) {
  const [login, setLogin] = useState(''), [name, setName] = useState(''), [fresh, setFresh] = useState(false);
  const [error, setError] = useState<unknown>(), [reading, setReading] = useState(false);
  const command = useCommand<Invitation>(onReceipt, 'member-invitation:' + mode + ':' + (user?.id ?? 'new'));
  async function read() {
    if (reading || !fresh || !user) return;
    setReading(true); setError(undefined);
    try { const value = await client.request<Invitation>('/users/' + user.id + '/invitation', { scope }); if (!scope.closed) onReceipt(value); }
    catch (cause) { if (!scope.closed) setError(cause); } finally { if (!scope.closed) setReading(false); }
  }
  return <Section title={mode === 'read' ? '查看成员邀请' : '邀请新成员'}><Notice error={command.error ?? error} />
    {mode === 'invite' ? <><Field label='内部账号'><Input ariaLabel='内部账号' value={login} maxlength={200} onInput={event => setLogin(event.detail.value)} /></Field><Field label='显示名称'><Input ariaLabel='显示名称' value={name} maxlength={200} onInput={event => setName(event.detail.value)} /></Field></> : <Text>{user?.display_name}</Text>}
    <InvitationVerification verifiedAt={me.mfa_verified_at} onFresh={setFresh} disabled={command.busy || reading} />
    <Action disabled={!fresh || command.blocked || (mode === 'invite' && (!login.trim() || !name.trim()))} loading={command.busy || reading} onClick={() => mode === 'read' ? void read() : void command.submit('/users', { login_key: login.trim(), display_name: name.trim() })}>{mode === 'read' ? '核对并查看当前邀请' : '确认创建邀请'}</Action>
    <Action secondary disabled={command.busy || reading} onClick={onClose}>取消</Action></Section>;
}

function InvitationReceiptPanel({ receipt, verifiedAt, onReceipt, onClose }: { receipt: Invitation; verifiedAt?: string | null; onReceipt(value: Invitation): void; onClose(): void }) {
  const [revealed, setRevealed] = useState(false), [renewing, setRenewing] = useState(false), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false), [localError, setLocalError] = useState<unknown>();
  const command = useCommand<Invitation>(onReceipt, 'member-invitation-reissue:' + receipt.invitation_id, value => { checkedReissuedInvitation(receipt, value); return true; });
  useDidHide(() => { setRevealed(false); setRenewing(false); setFresh(false); });
  function reissue() { try { void command.submit('/users/' + receipt.user.id + '/invitation-reissues', reissueInvitationInput(receipt, reason)); } catch (cause) { setLocalError(cause); } }
  return <Section title={'邀请 · ' + receipt.user.display_name}><Notice error={command.error ?? localError} /><Fact label='有效至'><Text>{date(receipt.expires_at)}</Text></Fact>
    {receipt.replaces_id && <Text className='cp-notice'>已重新签发，原邀请码已失效。</Text>}
    {receipt.activation_token ? <><Text className='cp-muted cp-small'>通过可信渠道交付邀请码。对方在登录页选择「接受邀请」，设置自己的密码。</Text>{revealed ? <Text selectable>{receipt.activation_token}</Text> : <Action secondary onClick={() => setRevealed(true)}>显示邀请码</Action>}</> : <Text>此邀请已过期、失效或被使用。</Text>}
    {receipt.user.status === 'invited' && <><Action secondary onClick={() => setRenewing(value => !value)}>重新签发邀请</Action>{renewing && <>
      <Text className='cp-muted cp-small'>确认后旧邀请码立即失效，同一成员保留，新邀请有效24小时。</Text>
      <Field label='重发原因'><Textarea ariaLabel='重发原因' value={reason} maxlength={200} onInput={event => setReason(event.detail.value)} /></Field>
      <InvitationVerification verifiedAt={verifiedAt} onFresh={setFresh} disabled={command.busy} /><Action loading={command.busy} disabled={!fresh || !reason.trim() || command.blocked} onClick={reissue}>确认重发邀请</Action>
    </>}</>}
    <Action secondary disabled={command.busy} onClick={onClose}>关闭邀请</Action></Section>;
}

function InvitationVerification({ verifiedAt, onFresh, disabled }: { verifiedAt?: string | null; onFresh(value: boolean): void; disabled: boolean }) {
  const verified = Date.parse(verifiedAt ?? '');
  const recent = Number.isFinite(verified) && verified <= Date.now()+5000 && Date.now()-verified < 9*60000;
  const [prompt, setPrompt] = useState(!recent);
  useEffect(() => {
    if (prompt) return;
    onFresh(recent);
    const timer = setTimeout(() => { onFresh(false); setPrompt(true); }, Math.max(1, verified+9*60000-Date.now()));
    return () => clearTimeout(timer);
  }, [prompt, verified, recent, onFresh]);
  return prompt ? <MFAConfirmation onFresh={onFresh} disabled={disabled} /> : <Text className='cp-muted cp-small'>当前会话已通过最近两步验证。</Text>;
}
