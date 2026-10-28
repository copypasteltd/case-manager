import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { checkedCleanupPlan, checkedCleanupReceipts, cleanupBlockers, cleanupCreate, cleanupDecision, cleanupProgress, cleanupStates, type CleanupCandidate, type CleanupPlan, type CleanupReceipt, type CleanupSelection } from '@copypaste/contracts/multipart-cleanup';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Empty, Fact, Notice, Row, Section, bytes, date } from '../../ui';
import { TextField } from '../business/controls';

export function MultipartCleanup({ folder, me }: { folder: Schemas['Folder']; me: Schemas['Me'] }) {
  const [open, setOpen] = useState(false), [revision, setRevision] = useState(0), [planID, setPlanID] = useState<string>();
  function refresh(id = planID) { setPlanID(id); setRevision(value => value + 1); }
  return <Section title='未完成上传清理'><Text className='cp-muted cp-paragraph'>按当前目录盘点过期上传。固定清单经过观察期和人工批准后，后台逐项终止分片并核对回执。</Text><Action secondary onClick={() => setOpen(!open)}>{open ? '收起清理工作区' : '盘点与清理'}</Action>{open && <><Action secondary onClick={() => refresh()}>刷新盘点与执行结果</Action><CleanupWorkspace key={[folder.id, me.user.id, me.authorization_version, revision].join(':')} folder={folder} me={me} initialPlan={planID} refresh={refresh} /></>}</Section>;
}

function CleanupWorkspace({ folder, me, initialPlan, refresh }: { folder: Schemas['Folder']; me: Schemas['Me']; initialPlan?: string; refresh(id?: string): void }) {
  const candidates = useCollection<CleanupCandidate>('/folders/' + folder.id + '/upload-cleanup-candidates?limit=20');
  const plans = useCollection<CleanupPlan>('/folders/' + folder.id + '/upload-cleanup-plans?limit=10');
  const [selection, setSelection] = useState<CleanupSelection[]>([]), [reason, setReason] = useState(''), [planID, setPlanID] = useState(initialPlan), [form, setForm] = useState<'approve' | 'cancel' | Schemas['CleanupItem']>(), [receipt, setReceipt] = useState<string>(), [localError, setLocalError] = useState<unknown>();
  const detail = useRecord<CleanupPlan>(planID ? '/upload-cleanup-plans/' + planID : null);
  const command = useCommand<CleanupPlan>(value => refresh(value.id), [folder.id, me.user.id, me.authorization_version, 'multipart-cleanup-create'].join(':'), value => checkedCleanupPlan(value, folder.id).state === 'observing');
  let plan: CleanupPlan | undefined, integrityError: unknown;
  try { if (detail.value && !detail.error) plan = checkedCleanupPlan(detail.value, folder.id, planID); plans.items.forEach(value => checkedCleanupPlan(value, folder.id)); } catch (cause) { integrityError = asError(cause); }
  const locked = command.busy || command.blocked, progress = plan ? cleanupProgress(plan) : undefined;
  function create() {
    try { const body = cleanupCreate(candidates.items, selection, reason); setLocalError(undefined); void command.submit('/folders/' + folder.id + '/upload-cleanup-plans', body); } catch (cause) { setLocalError(asError(cause)); }
  }
  return <View><Notice error={candidates.error ?? plans.error ?? detail.error ?? integrityError ?? localError ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Section title='候选上传'><Text className='cp-muted cp-paragraph'>选中 {selection.length} 项，最多50项。仅终止准确的未完成上传分片；正式内容及历史版本继续保留。</Text>
      {candidates.loading ? <Busy /> : !candidates.error && candidates.items.length ? candidates.items.map(row => <Fact key={row.upload_id} label={row.name}><Text>{bytes(row.size_bytes)} · 修订 {row.version} · 过期于 {date(row.expires_at)}</Text><Check checked={selection.some(v => v.upload_id === row.upload_id)} disabled={locked || !row.eligible || selection.length >= 50 && !selection.some(v => v.upload_id === row.upload_id)} onChange={checked => setSelection(current => checked ? [...current, { upload_id: row.upload_id, expected_version: row.version }] : current.filter(v => v.upload_id !== row.upload_id))}>选择清理 {row.name}</Check>{row.blocking_reasons.map(code => <Text className='cp-muted cp-paragraph' key={code}>{cleanupBlockers[code] ?? code}</Text>)}</Fact>) : !candidates.error && <Empty>此目录暂无可见的未完成上传。</Empty>}
      {candidates.next && <Action secondary loading={candidates.moreBusy} onClick={() => void candidates.more()}>加载更多候选上传</Action>}
      <TextField name='cleanup-reason' label='清理原因' multiline value={reason} onChange={setReason} disabled={locked} />
      <Action secondary loading={command.busy} disabled={command.blocked || !selection.length || !reason.trim() || !!candidates.error} onClick={create}>建立固定清单并进入观察期</Action>
    </Section>
    <Section title='清理清单'>{plans.loading ? <Busy /> : !plans.error && !integrityError && plans.items.length ? plans.items.map(row => <Row key={row.id} title={cleanupStates[row.state] + ' · ' + row.items.length + ' 项'} subtitle={row.reason} onClick={() => { setPlanID(row.id); setReceipt(undefined); setForm(undefined); }} />) : !plans.error && !integrityError && <Empty>尚未建立清理清单。</Empty>}{plans.next && <Action secondary loading={plans.moreBusy} onClick={() => void plans.more()}>加载更多清理清单</Action>}</Section>
    {detail.loading && <Busy />}{plan && !integrityError && <Section title={cleanupStates[plan.state] + ' · ' + progress?.confirmed + '/' + progress?.total + ' 项确认不存在'}><Text className='cp-paragraph'>{plan.reason}</Text><Fact label='观察期结束'><Text>{date(plan.ready_after)}</Text></Fact><Fact label='批准有效期结束'><Text>{date(plan.expires_at)}</Text></Fact><Fact label='固定清单摘要'><Text className='cp-code' selectable>{plan.snapshot_sha256}</Text></Fact>{plan.approved_at && <Fact label='人工批准'><Text>{date(plan.approved_at)} · {plan.approval_reason}</Text></Fact>}{plan.cancelled_at && <Fact label='撤销记录'><Text>{date(plan.cancelled_at)} · {plan.cancellation_reason}</Text></Fact>}
      {plan.items.map(item => <Fact key={item.id} label={plan!.snapshot.items.find(v => v.upload_id === item.upload_id)?.name ?? item.upload_id}><Text>{cleanupStates[item.state]} · 轮次 {item.generation} · 已记录结果 {item.attempts} 次</Text>{item.last_error && <Text>{item.last_error}</Text>}{item.confirmed_at && <Text>确认时间 {date(item.confirmed_at)}</Text>}<Action secondary onClick={() => setReceipt(item.id)}>查阅逐次回执</Action>{plan!.state === 'approved' && item.state === 'failed' && <Action secondary onClick={() => setForm(item)}>明确重试此项</Action>}</Fact>)}
      {plan.state === 'observing' && <Action onClick={() => setForm('approve')}>核对并批准清理</Action>}{plan.state !== 'cancelled' && <Action secondary onClick={() => setForm('cancel')}>撤销尚未执行项</Action>}
      <Action secondary onClick={() => refresh(plan!.id)}>刷新此清单</Action>
      {form && <CleanupForm key={plan.id + ':' + plan.version + ':' + (typeof form === 'string' ? form : form.id + ':' + form.version)} plan={plan} mode={form} me={me} close={() => setForm(undefined)} saved={value => refresh(value.id)} />}
      {receipt && <Receipts key={receipt} id={receipt} />}
    </Section>}
  </View>;
}

function asError(cause: unknown) { return cause instanceof MobileError ? cause : new MobileError('CLEANUP_FORM_INVALID', cause instanceof Error ? cause.message : '请刷新并核对清理资料。'); }

function CleanupForm({ plan, mode, me, close, saved }: { plan: CleanupPlan; mode: 'approve' | 'cancel' | Schemas['CleanupItem']; me: Schemas['Me']; close(): void; saved(plan: CleanupPlan): void }) {
  const [reason, setReason] = useState(''), [ack, setAck] = useState(false), [fresh, setFresh] = useState(false), [localError, setLocalError] = useState<unknown>();
  const retry = typeof mode !== 'string', item = retry ? mode : undefined, action = retry ? 'retry' : mode;
  const title = action === 'approve' ? '批准固定清单' : action === 'cancel' ? '撤销尚未执行项' : '明确重试此项';
  const command = useCommand<CleanupPlan>(saved, [plan.id, plan.version, action, item?.id, item?.version, me.authorization_version].join(':'), value => {
    checkedCleanupPlan(value, plan.snapshot.folder_id, plan.id);
    return value.snapshot_sha256 === plan.snapshot_sha256 && (item ? value.items.some(v => v.id === item.id && v.generation === item.generation + 1 && v.state === 'queued') : value.state === (action === 'approve' ? 'approved' : 'cancelled'));
  });
  function submit() {
    try {
      if (!ack || !fresh) throw new Error('请核对固定清单并完成本次身份验证。');
      const path = item ? '/upload-cleanup-items/' + item.id + '/retry' : '/upload-cleanup-plans/' + plan.id + '/' + action;
      const body = item ? { expected_version: item.version } : cleanupDecision(plan, reason, ack, mode as 'approve' | 'cancel');
      setLocalError(undefined); void command.submit(path, body);
    } catch (cause) { setLocalError(asError(cause)); }
  }
  const locked = command.busy || command.blocked;
  return <Section title={title}><Text className='cp-paragraph'>{item ? plan.snapshot.items.find(v => v.upload_id === item.upload_id)?.name : '固定 ' + plan.items.length + ' 项'} · 观察期结束 {date(plan.ready_after)}</Text><Text className='cp-muted cp-paragraph'>批准及来源必须继续有效；撤销停止剩余工作，过往回执继续保留。</Text><Text className='cp-code' selectable>{plan.snapshot_sha256}</Text><Notice error={localError ?? command.error} /><CommandNotice blocked={command.blocked} />{!retry && <TextField name='cleanup-decision-reason' label='操作原因' multiline value={reason} onChange={setReason} disabled={locked} />}<Check checked={ack} disabled={locked} onChange={setAck}>我已核对固定候选、摘要和操作影响</Check><MFAConfirmation disabled={locked} onFresh={setFresh} /><Action loading={command.busy} disabled={command.blocked || !fresh || !ack || !retry && !reason.trim()} onClick={submit}>{title}</Action><Action secondary disabled={command.busy} onClick={close}>返回清单</Action></Section>;
}

function Receipts({ id }: { id: string }) {
  const receipts = useCollection<CleanupReceipt>('/upload-cleanup-items/' + id + '/receipts?limit=20');
  let error: unknown; try { checkedCleanupReceipts(receipts.items); } catch (cause) { error = asError(cause); }
  return <Section title='逐次执行回执'><Notice error={receipts.error ?? error} />{receipts.loading ? <Busy /> : !receipts.error && !error && receipts.items.length ? receipts.items.map(row => <Fact key={row.id} label={'第 ' + row.generation + ' 轮 / 尝试 ' + row.attempt}><Text>{cleanupStates[row.outcome]} · {date(row.created_at)}{row.error_code ? ' · ' + row.error_code : ''}</Text></Fact>) : !receipts.error && !error && <Empty>尚无 Worker 执行回执。</Empty>}{receipts.next && <Action secondary loading={receipts.moreBusy} onClick={() => void receipts.more()}>加载更多回执</Action>}</Section>;
}
