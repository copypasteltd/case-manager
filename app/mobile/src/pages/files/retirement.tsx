import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { checkedRetirementPlan, checkedCleanupReceipts, retirementKinds, retirementStates, retirementBlockers, retirementCreate, retirementDecision, type RetirementCandidate, type RetirementPlan, type RetirementItem } from '@copypaste/contracts/object-retirement';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Empty, Fact, Notice, Row, Section, bytes, date } from '../../ui';
import { TextField } from '../business/controls';

export function ObjectRetirement({ file, me }: { file: Schemas['File']; me: Schemas['Me'] }) {
 const [open,setOpen] = useState(false), [revision,setRevision] = useState(0), [selected,setSelected] = useState<string>();
 function refresh(id = selected) { setSelected(id); setRevision(v => v + 1); }
 return <Section title='原件保留与清理'><Text className='cp-muted cp-paragraph'>盘点这个文件的历史正式原件与已提交上传的草稿，经过保留期、观察期和人工批准后逐项处理。</Text><Action secondary onClick={() => setOpen(!open)}>{open ? '收起原件清理' : '盘点历史与草稿原件'}</Action>{open && <><Action secondary onClick={() => refresh()}>刷新原件与清单</Action><Workspace key={[file.id,file.version,me.user.id,me.authorization_version,revision].join(':')} file={file} me={me} selected={selected} refresh={refresh} /></>}</Section>;
}
function asError(e: unknown) { return e instanceof MobileError ? e : new MobileError('RETIREMENT_INVALID',e instanceof Error ? e.message : '请刷新核对原件清单。'); }
function Workspace({ file,me,selected,refresh }: { file:Schemas['File']; me:Schemas['Me']; selected?:string; refresh(id?:string):void }) {
 const candidates = useCollection<RetirementCandidate>('/files/' + file.id + '/object-retirement-candidates?limit=20'), plans = useCollection<RetirementPlan>('/files/' + file.id + '/object-retirement-plans?limit=10');
 const [candidate,setCandidate] = useState<RetirementCandidate>(), [reason,setReason] = useState(''), [id,setID] = useState(selected), [action,setAction] = useState<'approve'|'cancel'|RetirementItem>(), [receipt,setReceipt] = useState<string>(), [localError,setLocalError] = useState<unknown>();
 const detail = useRecord<RetirementPlan>(id ? '/object-retirement-plans/' + id : null);
 const command = useCommand<RetirementPlan>(p => refresh(p.id),[file.id,file.version,me.user.id,me.authorization_version,'retirement-create'].join(':'),p => checkedRetirementPlan(p,file.id).state === 'observing' && p.snapshot.target_id === candidate?.target_id && p.snapshot.kind === candidate?.kind);
 let plan:RetirementPlan|undefined,integrity:unknown;
 try { if (detail.value && !detail.error) plan = checkedRetirementPlan(detail.value,file.id,id); plans.items.forEach(p => checkedRetirementPlan(p,file.id)); } catch (e) { integrity = asError(e); }
 const locked = command.busy || command.blocked;
 return <View><Notice error={candidates.error ?? plans.error ?? detail.error ?? integrity ?? localError ?? command.error} /><CommandNotice blocked={command.blocked} />
 <Section title='固定候选'>{candidates.loading ? <Busy /> : !candidates.error && candidates.items.length ? candidates.items.map(row => <Fact key={row.kind + row.target_id} label={retirementKinds[row.kind] + ' · 内容版本 ' + row.seq}><Text>{bytes(row.size_bytes)} · {row.retain_until ? '保留至 ' + date(row.retain_until) : '保留制度待明确'}</Text><Check checked={candidate?.kind === row.kind && candidate.target_id === row.target_id} disabled={!row.eligible || locked} onChange={value => setCandidate(value ? row : undefined)}>选择{retirementKinds[row.kind]} {row.seq}</Check>{row.blocking_reasons.map(code => <Text key={code} className='cp-muted cp-paragraph'>{retirementBlockers[code] ?? code}</Text>)}</Fact>) : !candidates.error && <Empty>没有可见的固定原件候选。</Empty>}
 {candidates.next && <Action secondary loading={candidates.moreBusy} onClick={() => void candidates.more()}>加载更多原件候选</Action>}<TextField name='retirement-reason' label='原件清理原因' multiline value={reason} onChange={setReason} disabled={locked} /><Action secondary loading={command.busy} disabled={command.blocked || !candidate || !reason.trim() || !!candidates.error} onClick={() => { try { if (!candidate) return; const body = retirementCreate(candidate,reason); setLocalError(undefined); void command.submit('/files/' + file.id + '/object-retirement-plans',body); } catch (e) { setLocalError(asError(e)); } }}>建立原件观察清单</Action></Section>
 <Section title='原件清单记录'>{plans.loading ? <Busy /> : !plans.error && !integrity && plans.items.map(p => <Row key={p.id} title={retirementStates[p.state] + ' · ' + retirementKinds[p.snapshot.kind]} subtitle={p.reason} onClick={() => { setID(p.id); setAction(undefined); setReceipt(undefined); }} />)}{plans.next && <Action secondary loading={plans.moreBusy} onClick={() => void plans.more()}>加载更多原件清单</Action>}</Section>
 {detail.loading && <Busy />}{plan && !integrity && <Section title={retirementStates[plan.state] + ' · ' + plan.items.filter(v => v.state === 'succeeded').length + '/' + plan.items.length + '项已确认'}><Text>{retirementKinds[plan.snapshot.kind]} · 版本{plan.snapshot.seq}</Text><Text className='cp-notice'>批准会不可逆地停用所选原件。停止只停止剩余清理；已有结果和历史元数据继续保留。</Text><Fact label='观察期结束'><Text>{date(plan.ready_after)}</Text></Fact><Fact label='批准有效期至'><Text>{date(plan.expires_at)}</Text></Fact><Fact label='固定清单摘要'><Text className='cp-code' selectable>{plan.snapshot_sha256}</Text></Fact>
 {plan.items.map(item => <Fact key={item.id} label={item.provider.toUpperCase() + ' · ' + retirementStates[item.state]}><Text>第{item.generation}轮 · {item.attempts}次结果{item.last_error ? ' · ' + item.last_error : ''}</Text>{item.confirmed_at && <Text>确认于 {date(item.confirmed_at)}</Text>}<Action secondary onClick={() => setReceipt(item.id)}>查阅原件回执</Action>{plan!.state === 'approved' && ['failed','blocked'].includes(item.state) && <Action secondary onClick={() => setAction(item)}>重试此原件</Action>}</Fact>)}
 {plan.state === 'observing' && <Action onClick={() => setAction('approve')}>核对原件并批准</Action>}{['observing','approved'].includes(plan.state) && <Action secondary onClick={() => setAction('cancel')}>停止剩余原件清理</Action>}<Action secondary onClick={() => refresh(plan!.id)}>刷新原件清单</Action>{action && <Decision key={[plan.id,plan.version,typeof action === 'string' ? action : action.id + action.version].join(':')} plan={plan} action={action} me={me} close={() => setAction(undefined)} saved={p => refresh(p.id)} />}{receipt && <Receipts key={receipt} id={receipt} />}</Section>}
 </View>;
}
function Decision({plan,action,me,close,saved}:{plan:RetirementPlan;action:'approve'|'cancel'|RetirementItem;me:Schemas['Me'];close():void;saved(p:RetirementPlan):void}) {
 const [reason,setReason] = useState(''), [ack,setAck] = useState(false), [fresh,setFresh] = useState(false), [error,setError] = useState<unknown>();
 const item = typeof action === 'string' ? undefined : action, mode = item ? 'retry' : action as 'approve'|'cancel', title = mode === 'approve' ? '批准原件清理' : mode === 'cancel' ? '确认停止剩余清理' : '确认重试此原件';
 const command = useCommand<RetirementPlan>(saved,[plan.id,plan.version,mode,item?.id,item?.version,me.authorization_version].join(':'),p => {
  checkedRetirementPlan(p,plan.snapshot.file_id,plan.id);
  return p.snapshot_sha256 === plan.snapshot_sha256 && (item ? p.items.some(v => v.id === item.id && v.generation === item.generation + 1 && v.state === 'queued') : p.state === (mode === 'approve' ? 'approved' : 'cancelled'));
 });
 function submit() {
  try { if (!ack || !fresh) throw new Error('请核对范围并完成本次身份验证。'); const body = item ? { expected_version:item.version } : retirementDecision(plan,reason,ack,mode as 'approve'|'cancel'); setError(undefined); void command.submit(item ? '/object-retirement-items/' + item.id + '/retry' : '/object-retirement-plans/' + plan.id + '/' + mode,body); } catch (e) { setError(asError(e)); }
 }
 const locked = command.busy || command.blocked;
 return <Section title={title}><Text className='cp-paragraph'>仅处理这份固定清单。批准后保留停用屏障；停止不能恢复已经删除的原件。</Text><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />{!item && <TextField name='retirement-decision-reason' label='原件操作依据' multiline value={reason} onChange={setReason} disabled={locked} />}<Check checked={ack} onChange={setAck} disabled={locked}>我已核对原件范围、摘要及不可逆影响</Check><MFAConfirmation disabled={locked} onFresh={setFresh} /><Action loading={command.busy} disabled={command.blocked || !ack || !fresh || !item && !reason.trim()} onClick={submit}>{title}</Action><Action secondary disabled={command.busy} onClick={close}>返回原件清单</Action></Section>;
}
function Receipts({id}:{id:string}) {
 const rows = useCollection<Schemas['CleanupReceipt']>('/object-retirement-items/' + id + '/receipts?limit=20');let integrity:unknown;
 try { checkedCleanupReceipts(rows.items); } catch (e) { integrity = asError(e); }
 return <Section title='原件逐次回执'><Notice error={rows.error ?? integrity} />{rows.loading ? <Busy /> : !rows.error && !integrity && rows.items.map(v => <Fact key={v.id} label={'第' + v.generation + '轮／尝试' + v.attempt}><Text>{retirementStates[v.outcome]} · {date(v.created_at)}{v.error_code ? ' · ' + v.error_code : ''}</Text></Fact>)}{rows.next && <Action secondary loading={rows.moreBusy} onClick={() => void rows.more()}>加载更多原件回执</Action>}</Section>;
}
