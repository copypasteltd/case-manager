import { Text, View } from '@tarojs/components';
import { useRef, useState } from 'react';
import { checkedScopeTransfer, scopeTransferCreate, scopeTransferDecision, transferBlockers, transferStates, type ScopeTransfer, type TransferResource } from '@copypaste/contracts/scope-transfer';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { fileWorkspace } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Empty, Fact, Notice, Section, date } from '../../ui';
import { classifications } from '../../platform/file-input';
import { TextField } from '../business/controls';

type Kind = 'file' | 'folder';
type Props = { kind: Kind; resource: TransferResource; me: Schemas['Me'] };
const error = (cause: unknown) => cause instanceof MobileError ? cause : new MobileError('TRANSFER_INPUT_INVALID', cause instanceof Error ? cause.message : '请刷新并核对准确移动计划。');
const label = (value: string) => classifications[value as keyof typeof classifications] ?? value;

export function ScopeTransfers({ kind, resource, me }: Props) {
  const [open, setOpen] = useState(false), [revision, setRevision] = useState(0), [planID, setPlanID] = useState<string>();
  if (!me.actions.includes('policy.manage') || !me.actions.includes('user.read') || resource.state !== 'active'
    || (kind === 'file' ? !(resource as Schemas['File']).can_move : !(resource as Schemas['Folder']).can_update || (resource as Schemas['Folder']).parent_id === null)) return null;
  function refresh(id = planID) { setPlanID(id); setRevision(v => v + 1); }
  return <Section title='跨项目与分类整理'><Text className='cp-muted cp-paragraph'>先分析完整资料清单和读取权限变化，再由本人明确确认执行。正式内容与历史版本继续保留。</Text><Action secondary onClick={() => setOpen(!open)}>{open ? '收起跨范围工作区' : '拟定跨范围移动'}</Action>{open && <><Action secondary onClick={() => refresh()}>刷新计划与当前依据</Action><TransferWorkspace key={[kind, resource.id, resource.version, me.authorization_version, revision].join(':')} kind={kind} resource={resource} me={me} planID={planID} refresh={refresh} /></>}</Section>;
}

function TransferWorkspace({ kind, resource, me, planID, refresh }: Props & { planID?: string; refresh(id?: string): void }) {
  const plans = useCollection<ScopeTransfer>('/' + kind + 's/' + resource.id + '/scope-transfers?limit=10');
  const detail = useRecord<ScopeTransfer>(planID ? '/file-scope-transfers/' + planID : null);
  const [target, setTarget] = useState<Schemas['Folder']>(), [newName, setNewName] = useState(resource.name), [why, setWhy] = useState(''), [localError, setLocalError] = useState<unknown>(), [action, setAction] = useState<'execute' | 'cancel'>();
  let plan: ScopeTransfer | undefined, integrityError: unknown;
  function check(value: unknown, id?: string) { const p = checkedScopeTransfer(value, kind, resource.id, id); if (p.creator_id !== me.user.id) throw new Error('计划不属于当前员工，请刷新。'); return p; }
  try { if (detail.value && !detail.error && !detail.loading) plan = check(detail.value, planID); plans.items.forEach(p => check(p)); } catch (cause) { integrityError = error(cause); }
  const binding = [me.user.id, me.authorization_version, kind, resource.id, resource.version, 'scope-transfer'].join(':');
  const expected = useRef<Schemas['ScopeTransferCreate']>();
  const command = useCommand<ScopeTransfer>(p => refresh(p.id), binding, p => check(p).state === 'pending' && !!expected.current && JSON.stringify(p.input) === JSON.stringify(expected.current));
  const locked = command.busy || command.blocked, completed = plan?.state === 'executed';
  function create() { try { if (!target) throw new Error('请选择实际目标目录。'); const body = scopeTransferCreate(kind, resource, target, newName, why); expected.current = body; setLocalError(undefined); void command.submit('/file-scope-transfers', body); } catch (cause) { setLocalError(error(cause)); } }
  return <View><Notice error={plans.error ?? detail.error ?? integrityError ?? localError ?? command.error} /><CommandNotice blocked={command.blocked} />
    {!completed && <Section title='选择实际目标'><TargetPicker sourceID={kind === 'folder' ? resource.id : undefined} selected={target} choose={setTarget} disabled={locked} /><TextField name='transfer-name' label='移动后的名称' value={newName} onChange={setNewName} disabled={locked} /><TextField name='transfer-reason' label='跨范围整理原因' value={why} onChange={setWhy} disabled={locked} multiline /><Action secondary disabled={locked || !target || !why.trim()} loading={command.busy} onClick={create}>分析跨范围移动</Action></Section>}
    <Section title='本人的固定计划'>{plans.loading ? <Busy /> : !plans.error && !integrityError && plans.items.length ? plans.items.map(p => <Fact key={p.id} label={transferStates[p.state]}><Text>{p.snapshot.source.name} → {p.snapshot.target.name} · {p.snapshot.items.length}项</Text><Action secondary onClick={() => refresh(p.id)}>查阅此移动计划</Action></Fact>) : !plans.error && !integrityError && <Empty>尚无跨范围计划；分析只建立计划，确认后才移动。</Empty>}{plans.next && <Action secondary loading={plans.moreBusy} onClick={() => void plans.more()}>加载更多移动计划</Action>}</Section>
    {planID && detail.loading && <Busy />}{plan && !integrityError && <Section title='跨范围移动计划详情'><PlanDetails plan={plan} />{plan.state === 'pending' && <><Action disabled={!plan.can_execute} onClick={() => setAction('execute')}>核对并执行跨范围移动</Action><Action secondary onClick={() => setAction('cancel')}>撤销此移动计划</Action></>}{plan.state === 'executed' && <Action onClick={() => fileWorkspace(kind === 'file' ? { file_id: resource.id, folder_id: plan.snapshot.target.id, project_id: plan.snapshot.target.project_id ?? undefined } : { folder_id: resource.id, project_id: plan.snapshot.target.project_id ?? undefined }, true)}>打开移动后的资料</Action>}{action && <Decision key={plan.id + ':' + action} plan={plan} owner={me.user.id} binding={binding} action={action} close={() => setAction(undefined)} saved={p => refresh(p.id)} />}</Section>}
  </View>;
}

export function TargetPicker({ sourceID, selected, choose, disabled }: { sourceID?: string; selected?: Schemas['Folder']; choose(v: Schemas['Folder']): void; disabled: boolean }) {
  const [path, setPath] = useState<Schemas['Folder'][]>([]), current = path.at(-1);
  return <View><Text className='cp-muted cp-paragraph'>{['所有可访问根目录', ...path.map(v => v.name)].join(' / ')}</Text>{path.length > 0 && <Action secondary disabled={disabled} onClick={() => setPath(v => v.slice(0, -1))}>返回目标上一级</Action>}{current?.can_create && <Action secondary disabled={disabled || current.id === sourceID} onClick={() => choose(current)}>选择当前目标目录</Action>}<TargetRows key={current?.id ?? 'root'} parent={current?.id} sourceID={sourceID} enter={folder => setPath(v => [...v, folder])} choose={choose} disabled={disabled} canEnter={path.length < 31} />{selected && <Text className='cp-notice'>已选目标：{selected.name} · {label(selected.classification)} · 修订 {selected.version}</Text>}</View>;
}
function TargetRows({ parent, sourceID, enter, choose, disabled, canEnter }: { parent?: string; sourceID?: string; enter(v: Schemas['Folder']): void; choose(v: Schemas['Folder']): void; disabled: boolean; canEnter: boolean }) {
  const folders = useCollection<Schemas['Folder']>('/folders?limit=20' + (parent ? '&parent_id=' + parent : ''));
  return <View><Notice error={folders.error} />{folders.loading ? <Busy /> : !folders.error && folders.items.filter(v => v.id !== sourceID).map(folder => <Fact key={folder.id} label={folder.name}><TargetLabel folder={folder} /><Action secondary disabled={disabled || !canEnter} onClick={() => enter(folder)}>进入目标目录</Action><Action secondary disabled={disabled || !folder.can_create} onClick={() => choose(folder)}>选择此目标</Action></Fact>)}{folders.next && <Action secondary disabled={disabled} loading={folders.moreBusy} onClick={() => void folders.more()}>加载更多目标目录</Action>}</View>;
}
function TargetLabel({ folder }: { folder: Schemas['Folder'] }) {
  const project = useRecord<Schemas['Project']>(folder.parent_id === null && folder.project_id ? '/projects/' + folder.project_id : null);
  return <><Notice error={project.error} /><Text>{folder.parent_id === null ? folder.project_id ? (project.value?.name ?? '正在读取项目') + ' / ' : '公司 / ' : ''}{folder.name} · {label(folder.classification)}</Text></>;
}
function PlanDetails({ plan: p }: { plan: ScopeTransfer }) {
  return <View><Fact label={transferStates[p.state]}><Text>{[...[...p.snapshot.source_path].reverse().map(v => v.name), p.snapshot.source.name].join(' / ')} → {[...[...p.snapshot.target_path].reverse().map(v => v.name), p.snapshot.target.name, p.snapshot.new_name].join(' / ')}</Text><Text>{p.input.reason}</Text><Text>完整 {p.snapshot.items.length}项 · 有效期结束 {date(p.expires_at)}</Text><Text className='cp-code' selectable>{p.plan_hash}</Text>{p.completed_at && <Text>{date(p.completed_at)} · {p.decision_reason}</Text>}</Fact>{p.state === 'pending' && !p.can_execute && <Text className='cp-notice'>当前依据已改变、已到期或存在阻塞。请处理原业务后重新拟定。</Text>}<Text className='cp-muted cp-paragraph'>数量为员工 × 渠道的读取策略组合，分别计算普通访问与 AI 场景；不是人数，也不代表 TG 群或模型实际获准访问。</Text>{p.snapshot.items.map(item => <Fact key={item.id} label={(item.kind === 'file' ? '文件' : '目录') + ' · ' + item.name + ' · 修订 ' + item.version}><Text>{label(item.classification)} → {label(item.new_classification)} · {item.state === 'trashed' ? '保持回收站状态' : '当前有效'}</Text>{item.kind === 'file' && <Text>保留 {item.content_versions} 个正式内容版本</Text>}<Text>普通读取策略组合 {item.human_before} → {item.human_after} · AI读取策略组合 {item.ai_before} → {item.ai_after}</Text>{item.blocking_reasons.map(code => <Text className='cp-notice' key={code}>{transferBlockers[code]}</Text>)}{item.visibility_changes.length ? item.visibility_changes.map(v => <Text className='cp-muted cp-paragraph' key={v.user_id + v.channel + v.ai}>{v.display_name} · {v.channel} · {v.ai ? 'AI场景' : '普通访问'}：{v.before ? '允许' : '不允许'} → {v.after ? '允许' : '不允许'}</Text>) : <Text className='cp-muted'>读取策略无变化。</Text>}</Fact>)}</View>;
}
function Decision({ plan, action, owner, binding, close, saved }: { plan: ScopeTransfer; action: 'execute' | 'cancel'; owner: string; binding: string; close(): void; saved(p: ScopeTransfer): void }) {
  const [why, setWhy] = useState(''), [ack, setAck] = useState(false), [fresh, setFresh] = useState(false), [localError, setLocalError] = useState<unknown>();
  const command = useCommand<ScopeTransfer>(saved, binding + ':' + plan.id + ':' + action, p => checkedScopeTransfer(p, plan.input.kind, plan.input.resource_id, plan.id).creator_id === owner && p.plan_hash === plan.plan_hash && p.version === plan.version + 1 && p.state === (action === 'execute' ? 'executed' : 'cancelled'));
  const locked = command.busy || command.blocked, title = action === 'execute' ? '确认并执行跨范围移动' : '确认撤销此移动计划';
  function submit() { try { setLocalError(undefined); void command.submit('/file-scope-transfers/' + plan.id + '/' + action, scopeTransferDecision(plan, why, ack, action)); } catch (cause) { setLocalError(error(cause)); } }
  return <Section title={title}><Text className='cp-paragraph'>{plan.snapshot.source.name} → {plan.snapshot.target.name} / {plan.snapshot.new_name} · 完整 {plan.snapshot.items.length}项</Text><Notice error={localError ?? command.error} /><CommandNotice blocked={command.blocked} /><TextField name='transfer-decision-reason' label='移动计划确认原因' value={why} onChange={setWhy} disabled={locked} multiline /><Check checked={ack} disabled={locked} onChange={setAck}>我已核对全部清单、读取权限变化及本次操作影响</Check>{action === 'execute' && <MFAConfirmation onFresh={setFresh} disabled={locked} />}<Action disabled={locked || !ack || !why.trim() || action === 'execute' && !fresh} loading={command.busy} onClick={submit}>{title}</Action><Action secondary disabled={command.busy} onClick={close}>返回移动计划</Action></Section>;
}
