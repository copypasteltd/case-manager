import { Text, View } from '@tarojs/components';
import { useEffect, useRef, useState } from 'react';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { useFormDraft } from '../../platform/form';
import { Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Empty, Fact, Notice, Section, date } from '../../ui';
import { TextField } from '../business/controls';

type Plan = Schemas['AgentOrganizationPlan'];
type Entry = Schemas['AgentOrganizationEntry'];
const states = { proposed: '待本人批准', approved: '已批准，等待整理', committed: '资料已整理', revoked: '已撤销' };
const kinds = { 'folder.create': '新建目录', 'folder.move': '调整目录', 'file.move': '调整文件' };
function location(path: Entry[], entry: Entry) { return [...path].reverse().map(e => e.name).concat(entry.name).join(' / '); }

export function ConversationOrganization({ conversation: c, me, onReference, referenceDisabled }: {
  conversation: Schemas['Conversation']; me: Schemas['Me']; onReference?(plan: Plan): void; referenceDisabled?: boolean;
}) {
  const [value, setValue] = useState<Schemas['AgentOrganizationPlanList']>(), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0), [verifiedAt, setVerifiedAt] = useState<string | null>(null);
  const enabled = c.type === 'private' && c.creator_id === me.user.id && me.actions.includes('agent.plan.read');
  useEffect(() => {
    const scope = new Scope(); let flight = false; setValue(undefined); setError(undefined); setLoading(enabled);
    async function read() {
      if (scope.closed || flight || !enabled) return; flight = true;
      try { const [result, current] = await Promise.all([
        client.request<Schemas['AgentOrganizationPlanList']>('/conversations/' + c.id + '/organization-plans', { scope }),
        client.request<Schemas['Me']>('/me', { scope }),
      ]); if (!scope.closed) { setValue(result); setVerifiedAt(current.mfa_verified_at); setError(undefined); } }
      catch (cause) { if (!scope.closed) { setValue(undefined); setError(cause); } }
      finally { flight = false; if (!scope.closed) setLoading(false); }
    }
    void read(); const timer = setInterval(() => void read(), 5000);
    return () => { clearInterval(timer); scope.close(); };
  }, [c.id, c.current_revision_id, me.authorization_version, enabled, revision]);
  if (!enabled) return null;
  return <Section title='资料整理计划'><View className='cp-pad'>
    <Text className='cp-paragraph'>核对原位置、目标位置及名称后，由本人通过两步验证批准。每份最多10项，有效期30分钟，整批成功后才提交。</Text><Notice error={error} />
    {loading ? <Busy /> : value && <>{value.items.map(p => <OrganizationCard key={p.id + ':' + p.version} plan={p} me={me}
      recentMFA={!!verifiedAt && Date.parse(verifiedAt) <= Date.now() + 5000 && Date.now() - Date.parse(verifiedAt) < 9 * 60000}
      onDone={() => setRevision(v => v + 1)} onReference={onReference} referenceDisabled={referenceDisabled} />)}
      {!value.items.length && <Empty>尚无资料整理计划。可以在 Web 选择目录拟定，或请秘书在此私密会话中拟定。</Empty>}
      {value.has_more && <Text>当前显示最近50份整理计划。</Text>}
    </>}
    <Action secondary onClick={() => setRevision(v => v + 1)}>重新核对整理计划</Action>
  </View></Section>;
}

function OrganizationCard({ plan: p, me, recentMFA, onDone, onReference, referenceDisabled, readOnly }: {
  plan: Plan; me: Schemas['Me']; recentMFA: boolean; onDone(): void; onReference?(plan: Plan): void; referenceDisabled?: boolean; readOnly?: boolean;
}) {
  const [fresh, setFresh] = useState(false), intent = useRef<string>();
  const draft = useFormDraft(me, { kind: 'agent-organization-action', id: p.id, version: p.version }, p.state);
  const command = useCommand<Plan>(() => { draft.forget(); onDone(); }, draft.binding,
    v => v.id === p.id && v.plan_hash === p.plan_hash && (intent.current !== 'commit' || (v.state === 'committed' && !!v.operation_id && v.results.length === p.preview.length)));
  const disabled = command.busy || command.blocked;
  function act(action: 'approve' | 'revoke' | 'commit') {
    intent.current = action;
    void command.submit('/organization-plans/' + p.id + '/' + action, { expected_version: p.version, plan_hash: p.plan_hash, reason: draft.fields.reason });
  }
  return <View className='cp-card'><Text className='cp-heading'>{p.preview.length}项资料整理 · {states[p.state]}</Text>
    <Fact label='整理原因'>{p.reason}</Fact>
    {p.preview.map((v, index) => <View key={index} className='cp-pad'>
      <Text className='cp-heading'>第{index + 1}项 · {kinds[v.action.kind]}</Text>
      <Fact label='原位置'>{location(v.path, v.source)} · 版本{v.source.version}</Fact>
      <Fact label='变更后'>{v.action.kind === 'folder.create' ? location(v.path, v.source) + ' / ' + v.action.name : v.target ? location(v.target_path, v.target) + ' / ' + (v.action.name ?? v.source.name) : [...v.path].reverse().map(e => e.name).concat(v.action.name ?? v.source.name).join(' / ')}</Fact>
      <Fact label='资料分类'>{v.source.classification === 'internal' ? '公司内部' : v.source.classification === 'project_restricted' ? '项目受限' : '严格受限'}</Fact>
      {v.target && <Fact label='目标目录版本'>{v.target.version}</Fact>}
    </View>)}
    <Fact label='有效期至'>{date(p.expires_at)}</Fact><Fact label='准确计划摘要'>{p.plan_hash}</Fact>
    {onReference && <Action secondary disabled={referenceDisabled} onClick={() => onReference(p)}>引用这份整理计划</Action>}
    {p.blocked_reason && <Text className='cp-notice'>会话、权限、资料或有效期已变化，请重新拟定并批准。仍可撤销当前计划。</Text>}
    {p.state === 'committed' && <><Fact label='批次回执'>{p.operation_id}</Fact>{p.results.map((r, i) => <Fact key={r.operation_id} label={'第' + (i + 1) + '项回执'}>{r.data.name} · {r.operation_id}</Fact>)}</>}
    {!readOnly && (p.can_approve || p.can_commit || p.can_revoke) && <><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
      <TextField name='organization-reason' label='整理操作说明' value={draft.fields.reason} onChange={reason => draft.update({ reason })} disabled={disabled} />
      {(p.can_approve || p.can_commit) && !recentMFA && <MFAConfirmation onFresh={setFresh} disabled={disabled} />}
      {(p.can_approve || p.can_commit) && recentMFA && <Text className='cp-muted cp-small'>最近已完成两步验证，请核对逐项内容后确认。</Text>}
      {p.can_approve && <Action loading={command.busy} disabled={disabled || (!fresh && !recentMFA) || !draft.fields.reason.trim()} onClick={() => act('approve')}>批准这份整理计划</Action>}
      {p.can_commit && <Action loading={command.busy} disabled={disabled || (!fresh && !recentMFA) || !draft.fields.reason.trim()} onClick={() => act('commit')}>按已批准计划整理资料</Action>}
      {p.can_revoke && <Action secondary loading={command.busy} disabled={disabled || !draft.fields.reason.trim()} onClick={() => act('revoke')}>撤销整理计划</Action>}
    </>}
  </View>;
}

export function ReferencedOrganizationPlan({ reference, conversation: c, me, onClose }: {
  reference: Schemas['AgentSourceSnapshot']; conversation: Schemas['Conversation']; me: Schemas['Me']; onClose(): void;
}) {
  const [value, setValue] = useState<Plan>(), [error, setError] = useState<unknown>(), [revision, setRevision] = useState(0);
  useEffect(() => {
    const scope = new Scope(); setValue(undefined); setError(undefined);
    void client.request<Plan>('/organization-plans/' + reference.id, { scope }).then(p => {
      if (scope.closed) return;
      if (p.conversation_id !== c.id || p.actor_id !== me.user.id || p.plan_hash !== reference.sha256) throw new Error('引用与当前计划不匹配，请重新核对。');
      setValue(p);
    }).catch(cause => { if (!scope.closed) setError(cause); });
    return () => scope.close();
  }, [reference.id, reference.sha256, c.id, me.user.id, me.authorization_version, revision]);
  return <Section title='引用的资料整理计划'><View className='cp-pad'><Notice error={error} />
    {!value && !error && <Busy />}{value && <OrganizationCard plan={value} me={me} recentMFA={false} onDone={() => setRevision(v => v + 1)} readOnly />}
    <Action secondary onClick={() => setRevision(v => v + 1)}>重新读取引用计划</Action><Action secondary onClick={onClose}>关闭引用计划</Action>
  </View></Section>;
}
