import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { contractBodyAdopted, contractBodyFacts, contractBodyProven, makeContractBody, makeContractBodyAdopt, type ContractBodyInput, type ContractBodyView } from '@copypaste/contracts/contract-body';
import { useRecord } from '../../platform/collection';
import { CommandNotice, useCommand } from '../../platform/command';
import { go, goVersion } from '../../platform/page';
import { MobileError, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Fact, Notice } from '../../ui';
import { ChoiceList, TextField } from '../business/controls';

export function ConversationContractBody({ conversation, file, disabled, value, onChange }: { conversation: Schemas['Conversation']; file: Schemas['AgentFileCreateInput']; disabled: boolean; value?: ContractBodyInput; onChange(value?: ContractBodyInput): void }) {
  const [open, setOpen] = useState(false), [picking, setPicking] = useState(false), [id, setID] = useState(''), [reason, setReason] = useState(''), [error, setError] = useState<unknown>();
  const record = useRecord<Schemas['Contract']>(open && id ? '/contracts/' + id : null), folder = useRecord<Schemas['Folder']>(open ? '/folders/' + file.folder_id : null);
  const current = record.error ? undefined : record.value;
  if (picking) return <ChoiceList<Schemas['Contract']> title='起草正文的合同' path={'/contracts?limit=20' + (conversation.project_id ? '&project_id=' + conversation.project_id : '')} label={contract => contract.title} eligible={contract => contract.classification === conversation.classification && contract.can_edit && contract.lifecycle === 'draft' && contract.project_id === conversation.project_id && ['not_submitted', 'returned', 'withdrawn'].includes(contract.approval_state)} onSelect={contract => { if (!disabled) { setID(contract.id); setPicking(false); setError(undefined); } }} onClose={() => setPicking(false)} />;
  return <View><Text className='cp-row-title'>完整合同正文依据（可选）</Text><Notice error={error ?? record.error ?? folder.error} />{value ? <><Text className='cp-paragraph'>已选准确合同 · 原始修订 {value.expected_version}</Text><Text className='cp-small cp-paragraph'>秘书按所选资料新建完整正文文件，完成后由本人核对采用。</Text><Action secondary disabled={disabled} onClick={() => { onChange(undefined); setOpen(false); }}>移除合同正文依据</Action></> : !open ? <Action secondary disabled={disabled} onClick={() => { setOpen(true); setPicking(true); }}>选择合同正文依据</Action> : <>
    {record.loading && <Busy />}{current && <Text className='cp-paragraph'>{current.title} · {current.currency ?? '币种待明确'} {current.amount ?? '金额待明确'} · 修订 {current.version}</Text>}<Action secondary disabled={disabled} onClick={() => setPicking(true)}>重新选择起草合同</Action><TextField name='contract-body-reason' label='合同正文起草说明' value={reason} max={200} disabled={disabled} onChange={setReason} />
    <Text className='cp-small cp-paragraph'>合同和新文件目录须属于同一项目、使用相同分类。完整正文最多48KiB。</Text><Action secondary disabled={disabled || !current || !folder.value || !!folder.error || record.loading || folder.loading || !reason.trim()} onClick={() => { try { onChange(makeContractBody(current!, folder.value!, file, conversation.project_id, reason, conversation.classification)); setOpen(false); } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请核对合同正文依据。')); } }}>将准确合同依据加入请求</Action>
  </>}</View>;
}

export function ContractBodyReview({ run }: { run: Schemas['Run'] }) {
  const original = run.contract_body!;
  const [saved, setSaved] = useState<ContractBodyView>(), [reviewed, setReviewed] = useState(false), [reason, setReason] = useState(''), [error, setError] = useState<unknown>();
  const value = saved ?? original, proven = contractBodyProven(value), adopted = proven && value.status === 'adopted';
  const command = useCommand<ContractBodyView>(result => { setSaved(result); setReason(''); setReviewed(false); }, 'contract-body-adopt:' + run.id + ':' + original.intent_sha256, result => contractBodyAdopted(result, value));
  return <View className='cp-editor-row'><Text className='cp-row-title'>完整合同正文与人工采用</Text><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />{!proven ? <Text className='cp-paragraph'>合同正文依据或文件回执未通过核对，请重新读取任务。</Text> : <>
    {contractBodyFacts(value).map(([label, text]) => <Fact key={label} label={label}>{text}</Fact>)}{!value.file ? <Text className='cp-paragraph'>已固定合同资料，等待本次任务创建并扫描完整正文文件。</Text> : <>
      <Text className='cp-paragraph'>{value.file.evidence.name} · 固定文件版本 {value.file.evidence.seq} · {value.file.evidence.size_bytes} 字节 · 扫描通过</Text><Text className='cp-code' selectable>{value.file.evidence.sha256}</Text><Text className='cp-row-title'>准确文件的完整正文</Text><Text className='cp-paragraph' selectable data-testid='contract-body-full-text'>{value.file.content}</Text>
      {adopted ? <Text className='cp-paragraph'>完整正文已采用 · 合同修订 {value.result!.version} · 操作 {value.operation_id}</Text> : run.state === 'succeeded' && run.outcome === 'completed' && !run.cancel_requested_at ? <><Check checked={reviewed} disabled={command.busy || command.blocked} onChange={setReviewed}>我已核对合同原始资料与完整正文，确认采用此准确文件版本</Check><TextField name='contract-body-adopt-reason' label='采用完整正文的依据' value={reason} max={200} disabled={command.busy || command.blocked} onChange={setReason} /><Action secondary disabled={command.busy || command.blocked || !reviewed || !reason.trim()} onClick={() => { try { void command.submit('/runs/' + run.id + '/contract-body-adoption', makeContractBodyAdopt(value, reviewed, reason)); } catch (cause) { setError(new MobileError('INPUT_REQUIRED', cause instanceof Error ? cause.message : '请核对合同正文。')); } }}>采用完整合同正文</Action></> : <Text className='cp-paragraph'>本次任务需成功结束后才能采用。已取消或失败的任务不能采用。</Text>}
      <Text className='cp-small cp-paragraph'>采用会保存合同新修订。请另行核对送审、签署及生效。</Text><Action secondary onClick={() => goVersion(value.file!.evidence.file_id, value.file!.evidence.version_id)}>查看准确正文文件版本</Action>
    </>}<Action secondary onClick={() => go('contract', value.input.contract_id)}>查看完整合同资料</Action>
  </>}</View>;
}
