import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { fileRecoveryMessage, makeFileRecovery, type FileRecovery } from '@copypaste/contracts/file-recovery';
import { useRecord } from '../../platform/collection';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Fact, Notice } from '../../ui';
import { ChoiceList, SelectField } from '../business/controls';
import { FileUpdateReceipt } from './file-updates';

export function ConversationFileRecovery({ conversation: c, actor, disabled, value, onChange }: { conversation: Schemas['Conversation']; actor: string; disabled: boolean; value?: FileRecovery; onChange(value?: FileRecovery): void }) {
  const [open, setOpen] = useState(false), [picking, setPicking] = useState(false), [source, setSource] = useState(''), [draftID, setDraftID] = useState(''), [reviewed, setReviewed] = useState(false), [error, setError] = useState<unknown>();
  const run = useRecord<Schemas['Run']>(open && source && !value ? '/runs/' + source : null);
  const saved = run.error ? undefined : run.value?.file_update ?? run.value?.file_create;
  if (c.type !== 'private' || c.creator_id !== actor) return null;
  if (picking) return <ChoiceList<Schemas['Message']> title='选择原文件请求' path={'/conversations/' + c.id + '/messages?limit=20'} searchable={false} label={m => m.body.slice(0, 70) + ' · ' + m.run_id.slice(0, 8)} eligible={fileRecoveryMessage} onSelect={m => { if (!disabled) { setSource(m.run_id); setDraftID(''); setReviewed(false); setError(undefined); setPicking(false); } }} onClose={() => setPicking(false)} />;
  return <View><Text className='cp-row-title'>恢复已保存草稿</Text>{value ? <><Fact label='原任务与准确草稿'><Text className='cp-code' selectable>{value.source_run_id} · {value.draft_id}</Text></Fact><Fact label='选定内容摘要'><Text className='cp-code' selectable>{value.content_sha256}</Text></Fact><Text className='cp-small cp-paragraph'>发送后重新核对原目标、基础版本、依赖资料和权限，并重新上传、校验和扫描。</Text><Action secondary disabled={disabled} onClick={() => { onChange(undefined); setOpen(false); setReviewed(false); }}>移除本次草稿恢复</Action></> : !open ? <Action secondary disabled={disabled} onClick={() => { setOpen(true); setPicking(true); }}>选择旧草稿继续处理</Action> : <>
    <Notice error={error ?? run.error} /><Action secondary disabled={disabled} onClick={() => setPicking(true)}>选择原文件请求</Action>{run.loading ? <Busy /> : saved && run.value && <><FileUpdateReceipt value={saved} stopped={['succeeded', 'failed', 'cancelled', 'interrupted'].includes(run.value.state)} /><SelectField label='本次恢复的准确草稿' value={draftID} options={[[ '', '请选择一份草稿' ], ...saved.drafts.map((d, index): [string, string] => [d.id, '草稿 ' + (index + 1) + ' · ' + d.id.slice(0, 8) + ' · ' + d.size_bytes + ' 字节'])]} disabled={disabled} onChange={v => { setDraftID(v); setReviewed(false); setError(undefined); }} /><Check checked={reviewed} disabled={disabled || !draftID} onChange={setReviewed}>我已核对选定草稿的差异、内容摘要及原目标</Check><Action secondary disabled={disabled || !draftID || !reviewed || run.loading} onClick={() => { try { if (run.value!.id !== source) throw new Error('原任务回执与选择不符。'); onChange(makeFileRecovery(run.value!, c, actor, draftID, reviewed)); setOpen(false); } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请重新核对选定草稿。')); } }}>将准确草稿恢复加入请求</Action></>}
    <Text className='cp-small cp-paragraph'>选择本身不发送。请填写继续处理要求后确认发送；原运行仍保持停止，新运行仅恢复选定内容。</Text><Action secondary disabled={disabled} onClick={() => { setOpen(false); setReviewed(false); }}>收起草稿恢复</Action>
  </>}</View>;
}
