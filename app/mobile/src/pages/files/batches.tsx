import { Text, View } from '@tarojs/components';
import { useDidHide } from '@tarojs/taro';
import { useRef, useState } from 'react';
import { batchActions, batchBlockers, batchInputMatches, batchStates, checkedFileBatch, fileBatchCreate, fileBatchDecision, type BatchAction, type FileBatchPlan } from '@copypaste/contracts/file-batch';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { fileWorkspace } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Empty, Fact, Notice, Row, Section, date } from '../../ui';
import { TextField } from '../business/controls';
import { TargetPicker } from './scope-transfers';

type File = Schemas['File'];
type Start = { action: BatchAction; files: File[] };
const error = (e: unknown) => e instanceof MobileError ? e : new MobileError('BATCH_REVIEW_INVALID', e instanceof Error ? e.message : '请刷新核对完整清单。');
export function BatchFileList({ items, me, refresh }: { items: File[]; me: Schemas['Me']; refresh(): void }) {
  const [selected, setSelected] = useState<File[]>([]), [start, setStart] = useState<Start>();
  useDidHide(() => { setSelected([]); setStart(undefined); });
  function toggle(f: File) { setSelected(v => v.some(i => i.id === f.id) ? v.filter(i => i.id !== f.id) : v.length < 50 ? [...v, f] : v); }
  return <View><Fact label={'已选择 ' + selected.length + ' / 50 个文件'}><Text>切换范围或筛选后重新选择。先分析固定清单，再由本人确认；移动限同项目、同分类。</Text><Action secondary disabled={!!start} onClick={() => setSelected(items.slice(0, 50))}>选择已加载文件</Action><Action secondary disabled={!!start || !selected.length} onClick={() => setSelected([])}>清空选择</Action>{(Object.keys(batchActions) as BatchAction[]).map(action => <Action key={action} secondary disabled={!!start || !selected.length || selected.some(f => action === 'restore' ? f.state !== 'trashed' || !f.can_restore : f.state !== 'active' || !(action === 'move' ? f.can_move : f.can_trash))} onClick={() => setStart({ action, files: [...selected] })}>{batchActions[action]}</Action>)}</Fact>
    {items.map(f => <Fact key={f.id} label={f.name}><Check checked={selected.some(i => i.id === f.id)} disabled={!!start || selected.length >= 50 && !selected.some(i => i.id === f.id)} onChange={() => toggle(f)}>{'选择文件 ' + f.name}</Check><Row title={'查阅 ' + f.name} subtitle={'修订 ' + f.version} state={f.state} onClick={() => fileWorkspace({ file_id: f.id }, true)} /></Fact>)}
    {start && <BatchWorkspace me={me} start={start} close={() => { setStart(undefined); setSelected([]); refresh(); }} completed={() => setSelected([])} />}</View>;
}
function BatchWorkspace({ me, start, initialPlan, close, completed }: { me: Schemas['Me']; start?: Start; initialPlan?: string; close(): void; completed?(): void }) {
  const [planID, setPlanID] = useState(initialPlan), [revision, setRevision] = useState(0);
  function saved(p: FileBatchPlan) { setPlanID(p.id); setRevision(v => v + 1); if (p.state === 'executed') completed?.(); }
  return <Section title={start ? batchActions[start.action] : '批量文件计划'}><Action secondary onClick={close}>关闭批量工作区</Action>{!planID && start && <CreateBatch me={me} start={start} saved={saved} />}{planID && <BatchReview key={planID + ':' + revision} me={me} planID={planID} saved={saved} reload={() => setRevision(v => v + 1)} />}</Section>;
}
function CreateBatch({ me, start, saved }: { me: Schemas['Me']; start: Start; saved(p: FileBatchPlan): void }) {
  const [target, setTarget] = useState<Schemas['Folder']>(), [reason, setReason] = useState(''), [localError, setLocalError] = useState<unknown>();
  const expected = useRef<Schemas['FileBatchCreate']>();
  const binding = ['file-batch-create', me.user.id, me.authorization_version, start.action, ...start.files.map(f => f.id + ':' + f.version).sort()].join(':');
  const command = useCommand<FileBatchPlan>(saved, binding, p => { checkedFileBatch(p, me.user.id); return p.state === 'pending' && !!expected.current && batchInputMatches(p.input, expected.current); });
  const locked = command.busy || command.blocked;
  function create() { try { expected.current = fileBatchCreate(start.action, start.files, target, reason); setLocalError(undefined); void command.submit('/file-batch-plans', expected.current); } catch (e) { setLocalError(error(e)); } }
  return <View><Text>固定选择 {start.files.length} 个文件</Text>{start.files.map(f => <Text className='cp-muted cp-paragraph' key={f.id}>{f.name} · 修订 {f.version}</Text>)}<Notice error={localError ?? command.error} /><CommandNotice blocked={command.blocked} />{start.action === 'move' && <TargetPicker selected={target} choose={setTarget} disabled={locked} />}<TextField name='batch-reason' label='批量整理原因' value={reason} onChange={setReason} disabled={locked} multiline /><Action disabled={locked || !reason.trim() || start.action === 'move' && !target} loading={command.busy} onClick={create}>分析批量清单</Action></View>;
}
function BatchReview({ me, planID, saved, reload }: { me: Schemas['Me']; planID: string; saved(p: FileBatchPlan): void; reload(): void }) {
  const record = useRecord<FileBatchPlan>('/file-batch-plans/' + planID), [action, setAction] = useState<'execute' | 'cancel'>();
  let plan: FileBatchPlan | undefined, invalid: unknown;
  try { if (record.value && !record.error && !record.loading) plan = checkedFileBatch(record.value, me.user.id, planID); } catch (e) { invalid = error(e); }
  return <View><Notice error={record.error ?? invalid} />{record.loading ? <Busy /> : plan && <><BatchDetails plan={plan} />{plan.state === 'pending' && <><Action disabled={!plan.can_execute} onClick={() => setAction('execute')}>核对并执行整批操作</Action><Action secondary onClick={() => setAction('cancel')}>撤销此批量计划</Action></>}<Action secondary onClick={reload}>刷新批量计划</Action>{action && <Decision key={plan.id + action} plan={plan} action={action} me={me} saved={saved} close={() => setAction(undefined)} />}</>}</View>;
}
function BatchDetails({ plan: p }: { plan: FileBatchPlan }) {
  return <View><Fact label={batchStates[p.state] + ' · ' + batchActions[p.input.action]}><Text>{p.input.reason}</Text><Text>完整 {p.snapshot.items.length} 个文件 · 有效期结束 {date(p.expires_at)}</Text>{p.snapshot.target && <Text>目标：{[...[...p.snapshot.target.path].reverse().map(v => v.name), p.snapshot.target.folder.name].join(' / ')} · 修订 {p.snapshot.target.folder.version}</Text>}<Text className='cp-code' selectable>{p.id}</Text><Text className='cp-code' selectable>{p.plan_hash}</Text>{p.completed_at && <Text>{date(p.completed_at)} · {p.decision_reason}</Text>}</Fact>{p.state === 'pending' && !p.can_execute && <Text className='cp-notice'>存在阻塞、依据已变化或计划到期，请处理原业务后重新选择并拟定清单。</Text>}{p.snapshot.items.map((item, i) => <Fact key={item.file.id} label={item.file.name + ' · 修订 ' + item.file.version}><Text>{[...[...item.path].reverse().map(v => v.name), item.file.name].join(' / ')}</Text><Text>{item.file.state === 'trashed' ? '回收站' : '当前有效'}{item.retention_held ? ' · 存在保留锁' : ''}</Text>{item.blocking_reasons.map(code => <Text className='cp-notice' key={code}>{batchBlockers[code]}</Text>)}{p.results[i] && <Text className='cp-notice'>本次回执：修订 {p.results[i].version} · {p.results[i].state === 'trashed' ? '已移入回收站' : '当前有效'}</Text>}</Fact>)}<Text className='cp-muted cp-paragraph'>保留原正式内容版本；任一项冲突则整批不执行。每个相关文件均可重新查阅本次记录。</Text></View>;
}
function Decision({ plan, action, me, saved, close }: { plan: FileBatchPlan; action: 'execute' | 'cancel'; me: Schemas['Me']; saved(p: FileBatchPlan): void; close(): void }) {
  const [reason, setReason] = useState(''), [ack, setAck] = useState(false), [fresh, setFresh] = useState(false), [localError, setLocalError] = useState<unknown>();
  const command = useCommand<FileBatchPlan>(saved, ['file-batch-decision', me.user.id, me.authorization_version, plan.id, action].join(':'), p => { checkedFileBatch(p, me.user.id, plan.id); return p.plan_hash === plan.plan_hash && batchInputMatches(p.input, plan.input) && p.state === (action === 'execute' ? 'executed' : 'cancelled') && p.version === plan.version + 1; });
  const locked = command.busy || command.blocked, title = action === 'execute' ? '确认并执行整批操作' : '确认撤销批量计划';
  function submit() { try { const body = fileBatchDecision(plan, reason, ack, action); setLocalError(undefined); void command.submit('/file-batch-plans/' + plan.id + '/' + action, body); } catch (e) { setLocalError(error(e)); } }
  return <Section title={title}><Text>{batchActions[plan.input.action]} · 完整 {plan.snapshot.items.length} 个文件</Text><Notice error={localError ?? command.error} /><CommandNotice blocked={command.blocked} /><TextField name='batch-decision-reason' label='批量计划确认原因' value={reason} onChange={setReason} disabled={locked} multiline /><Check checked={ack} disabled={locked} onChange={setAck}>我已核对全部文件、目标及本次操作影响</Check>{action === 'execute' && <MFAConfirmation onFresh={setFresh} disabled={locked} />}<Action disabled={locked || !ack || !reason.trim() || action === 'execute' && !fresh} loading={command.busy} onClick={submit}>{title}</Action><Action secondary disabled={command.busy} onClick={close}>返回批量清单</Action></Section>;
}
export function FileBatchHistory({ fileID, me }: { fileID: string; me: Schemas['Me'] }) {
  const [open, setOpen] = useState(false), [planID, setPlanID] = useState<string>();
  const plans = useCollection<FileBatchPlan>(open ? '/files/' + fileID + '/batch-plans?limit=10' : null);
  let invalid: unknown; try { plans.items.forEach(p => checkedFileBatch(p, me.user.id, undefined, fileID)); } catch (e) { invalid = error(e); }
  return <Section title='本人的批量整理记录'><Action secondary onClick={() => setOpen(!open)}>{open ? '收起批量记录' : '查阅批量记录'}</Action>{open && <><Notice error={plans.error ?? invalid} />{plans.loading ? <Busy /> : !plans.error && !invalid && (plans.items.length ? plans.items.map(p => <Fact key={p.id} label={batchActions[p.input.action] + ' · ' + batchStates[p.state]}><Text>{p.snapshot.items.length}个文件</Text><Action secondary onClick={() => setPlanID(p.id)}>查阅这份批量清单</Action></Fact>) : <Empty>尚无本人的批量计划。</Empty>)}{!plans.error && !invalid && plans.next && <Action secondary loading={plans.moreBusy} onClick={() => void plans.more()}>加载更多批量记录</Action>}</>}{planID && <BatchWorkspace key={planID} me={me} initialPlan={planID} close={() => setPlanID(undefined)} completed={plans.reload} />}</Section>;
}
