import { useCommand, MFAConfirmation, CommandNotice } from '../../platform/command';
import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { fileRecoveryMessage, makeFileRecovery, type FileRecovery } from '@copypaste/contracts/file-recovery';
import { useRecord } from '../../platform/collection';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Fact, Notice } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';
import { FileUpdateReceipt } from './file-updates';

export function ConversationFileRecovery({ conversation: c, actor, disabled, value, onChange }: { conversation: Schemas['Conversation']; actor: string; disabled: boolean; value?: FileRecovery; onChange(value?: FileRecovery): void }) {
  const [open, setOpen] = useState(false), [picking, setPicking] = useState(false), [source, setSource] = useState(''), [draftID, setDraftID] = useState(''), [reviewed, setReviewed] = useState(false), [error, setError] = useState<unknown>();
  const run = useRecord<Schemas['Run']>(open && source && !value ? '/runs/' + source : null);
  const saved = run.error ? undefined : run.value?.file_update ?? run.value?.file_create;
  if (c.type !== 'private' || c.creator_id !== actor) return null;
  if (picking) return <ChoiceList<Schemas['Message']> title='选择原文件请求' path={'/conversations/' + c.id + '/messages?limit=20'} searchable={false} label={m => m.body.slice(0, 70) + ' · ' + m.run_id.slice(0, 8)} eligible={fileRecoveryMessage} onSelect={m => { if (!disabled) { setSource(m.run_id); setDraftID(''); setReviewed(false); setError(undefined); setPicking(false); } }} onClose={() => setPicking(false)} />;
  return <View><Text className='cp-row-title'>恢复已保存草稿</Text>{value ? <><Fact label='原任务与准确草稿'><Text className='cp-code' selectable>{value.source_run_id} · {value.draft_id}</Text></Fact><Fact label='选定内容摘要'><Text className='cp-code' selectable>{value.content_sha256}</Text></Fact><Text className='cp-small cp-paragraph'>发送后重新核对原目标、基础版本、依赖资料和权限，并重新上传、校验和扫描。</Text><Action secondary disabled={disabled} onClick={() => { onChange(undefined); setOpen(false); setReviewed(false); }}>移除本次草稿恢复</Action></> : !open ? <Action secondary disabled={disabled} onClick={() => { setOpen(true); setPicking(true); }}>选择旧草稿继续处理</Action> : <>
    <Notice error={error ?? run.error} /><Action secondary disabled={disabled} onClick={() => setPicking(true)}>选择原文件请求</Action>{run.loading ? <Busy /> : saved && run.value && <><FileUpdateReceipt value={saved} stopped={['succeeded', 'failed', 'cancelled', 'interrupted'].includes(run.value.state)} /><SelectField label='本次恢复的准确草稿' value={draftID} options={[[ '', '请选择一份草稿' ], ...saved.drafts.map((d, index): [string, string] => [d.id, '草稿 ' + (index + 1) + ' · ' + d.id.slice(0, 8) + ' · ' + d.size_bytes + ' 字节'])]} disabled={disabled} onChange={v => { setDraftID(v); setReviewed(false); setError(undefined); }} /><Check checked={reviewed} disabled={disabled || !draftID} onChange={setReviewed}>我已核对选定草稿的差异、内容摘要及原目标</Check><Action secondary disabled={disabled || !draftID || !reviewed || run.loading} onClick={() => { try { if (run.value!.id !== source) throw new Error('原任务回执与选择不符。'); onChange(makeFileRecovery(run.value!, c, actor, draftID, reviewed)); setOpen(false); } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请重新核对选定草稿。')); } }}>将准确草稿恢复加入请求</Action></>}
    {run.value&&draftID&&<AbandonRecovery run={run.value} draftID={draftID} disabled={disabled} onSaved={()=>{setDraftID('');setReviewed(false);run.reload();}}/>}<Text className='cp-small cp-paragraph'>选择本身不发送。请填写继续处理要求后确认发送；原运行仍保持停止，新运行仅恢复选定内容。</Text><Action secondary disabled={disabled} onClick={() => { setOpen(false); setReviewed(false); }}>收起草稿恢复</Action>
  </>}</View>;
}

function AbandonRecovery({run,draftID,disabled,onSaved}:{run:Schemas['Run'];draftID:string;disabled:boolean;onSaved():void}) {
 const [open,setOpen]=useState(false),[reason,setReason]=useState(''),[reviewed,setReviewed]=useState(false),[fresh,setFresh]=useState(false);
 const draft=(run.file_update??run.file_create)?.drafts.find(v=>v.id===draftID),command=useCommand(onSaved,'abandon-recovery:'+draftID+':'+run.version);
 if(!draft||!['succeeded','failed','cancelled'].includes(run.state)||draft.state==='committed')return null;
 if(draft.recovery_abandoned)return <Text className='cp-notice cp-paragraph'>此准确草稿已永久放弃恢复，历史记录保留。清理仍需独立计划、观察期和工作进程。</Text>;
 return <View><Action secondary onClick={()=>setOpen(v=>!v)} disabled={disabled||command.busy}>放弃此草稿的后续恢复</Action>{open&&<><Text className='cp-notice cp-paragraph'>永久恢复屏障不能重新启用；此步骤不删除文件，清理仍需独立计划及至少24小时观察。</Text><TextField disabled={command.busy || command.blocked} name='abandon-reason' label='放弃原因' value={reason} onChange={setReason} max={200}/><Action secondary onClick={()=>setReviewed(v=>!v)}>{reviewed?'已确认':'确认'}：准确草稿及永久放弃影响</Action><MFAConfirmation onFresh={setFresh} disabled={disabled||command.busy||command.blocked}/><Notice error={command.error}/><CommandNotice blocked={command.blocked}/><Action loading={command.busy} disabled={disabled||command.blocked||!fresh||!reviewed||!reason.trim()} onClick={()=>void command.submit('/runs/'+run.id+'/file-drafts/'+draftID+'/abandon-recovery',{expected_version:run.version,content_sha256:draft.content_sha256,reviewed:true,reason:reason.trim()})}>确认永久放弃恢复</Action></>}</View>;
}
