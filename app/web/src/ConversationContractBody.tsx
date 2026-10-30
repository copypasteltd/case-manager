import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { contractBodyAdopted, contractBodyFacts, contractBodyProven, makeContractBody, makeContractBodyAdopt, type ContractBodyInput, type ContractBodyView } from '@copypaste/contracts/contract-body';
import { api, type Page, type Schemas } from './api';
import { Busy, ErrorNotice, Field } from './components';

export function ConversationContractBody({ conversation, file, disabled, value, onChange }: { conversation: Schemas['Conversation']; file: Schemas['AgentFileCreateInput']; disabled: boolean; value?: ContractBodyInput; onChange(value?: ContractBodyInput): void }) {
  const [open, setOpen] = useState(false), [id, setID] = useState(''), [reason, setReason] = useState(''), [error, setError] = useState<unknown>();
  const options = useInfiniteQuery({ queryKey: ['contract-body-options', conversation.id], enabled: open && !value, initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['Contract']>>('/contracts?limit=20' + (conversation.project_id ? '&project_id=' + conversation.project_id : '') + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined, gcTime: 0 });
  const record = useQuery({ queryKey: ['contract-body-basis', conversation.id, value?.contract_id ?? id], enabled: !!(value?.contract_id ?? id), queryFn: ({ signal }) => api<Schemas['Contract']>('/contracts/' + (value?.contract_id ?? id), { signal }), gcTime: 0 });
  const folder = useQuery({ queryKey: ['contract-body-folder', file.folder_id, file.expected_folder_version], enabled: open, queryFn: ({ signal }) => api<Schemas['Folder']>('/folders/' + file.folder_id, { signal }), gcTime: 0 });
  const current = record.error ? undefined : record.data;
  return <section className='conversation-source-preview'><h3>完整合同正文依据（可选）</h3><ErrorNotice error={error ?? record.error ?? folder.error ?? options.error} />{value ? <><p>{current?.title ?? '已选准确合同'} · 原始修订 {value.expected_version}</p><p className='form-note'>秘书按这份合同资料新建完整正文文件，完成后由本人核对采用。</p><button type='button' className='button secondary' disabled={disabled} onClick={() => { onChange(undefined); setOpen(false); }}>移除合同正文依据</button></> : !open ? <button type='button' className='button secondary' disabled={disabled} onClick={() => setOpen(true)}>选择合同正文依据</button> : <>
    <Field label='起草正文的合同'><select value={id} disabled={disabled} onChange={event => { setID(event.target.value); setError(undefined); }}><option value=''>请选择可编辑合同</option>{options.data?.pages.flatMap(page => page.items).filter(contract => contract.classification === conversation.classification && contract.can_edit && contract.lifecycle === 'draft' && contract.project_id === conversation.project_id && ['not_submitted', 'returned', 'withdrawn'].includes(contract.approval_state)).map(contract => <option value={contract.id} key={contract.id}>{contract.title}</option>)}</select></Field>
    {options.hasNextPage && <button type='button' className='button secondary' disabled={disabled || options.isFetchingNextPage} onClick={() => void options.fetchNextPage()}>加载更多合同</button>}{record.isFetching && id && <Busy label='正在核对合同原始资料' />}
    {current && <p>{current.title} · {current.currency ?? '币种待明确'} {current.amount ?? '金额待明确'} · 修订 {current.version}</p>}
    <Field label='合同正文起草说明'><input value={reason} maxLength={200} disabled={disabled} onChange={event => setReason(event.target.value)} /></Field><p className='form-note'>合同和新文件目录须属于同一项目、使用相同分类。正文最多48KiB；商业条件缺项由本人补充确认。</p>
    <button type='button' className='button secondary' disabled={disabled || !current || !folder.data || !!folder.error || record.isFetching || folder.isFetching || !reason.trim()} onClick={() => { try { onChange(makeContractBody(current!, folder.data!, file, conversation.project_id, reason, conversation.classification)); setOpen(false); } catch (cause) { setError(cause); } }}>将准确合同依据加入请求</button>
  </>}</section>;
}

export function ContractBodyReview({ run }: { run: Schemas['Run'] }) {
  const cache = useQueryClient(), original = run.contract_body!;
  const [saved, setSaved] = useState<ContractBodyView>(), [reviewed, setReviewed] = useState(false), [reason, setReason] = useState(''), [busy, setBusy] = useState(false), [blocked, setBlocked] = useState(false), [error, setError] = useState<unknown>();
  const alive = useRef(true), flight = useRef(false), key = useRef(crypto.randomUUID());
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const value = saved ?? original, proven = contractBodyProven(value), adopted = proven && value.status === 'adopted';
  async function adopt() {
    if (flight.current || blocked) return;
    let body: Schemas['AgentContractBodyAdopt'];
    try { body = makeContractBodyAdopt(value, reviewed, reason); } catch (cause) { setError(cause); return; }
    flight.current = true; setBusy(true); setError(undefined);
    try {
      const result = await api<ContractBodyView>('/runs/' + run.id + '/contract-body-adoption', { method: 'POST', body, key: key.current });
      if (!contractBodyAdopted(result, value)) throw new Error('合同采用回执尚未核对，请重新读取任务与合同修订。');
      if (alive.current) { setSaved(result); setBlocked(true); setReason(''); setReviewed(false); }
      await cache.invalidateQueries({ queryKey: ['agent-run-events', run.id] });
      await cache.invalidateQueries({ queryKey: ['contracts'] });
    } catch (cause) { if (alive.current) { setError(cause); setBlocked(true); } }
    finally { flight.current = false; if (alive.current) setBusy(false); }
  }
  return <section className='conversation-source-preview contract-body-review'><h3>完整合同正文与人工采用</h3><ErrorNotice error={error} />{!proven ? <p>合同正文依据或文件回执未通过核对，请重新读取任务。</p> : <>
    <dl>{contractBodyFacts(value).map(([label, text]) => <div key={label}><dt>{label}</dt><dd>{text}</dd></div>)}</dl>
    {!value.file ? <p>已固定合同资料，等待本次任务创建并扫描完整正文文件。</p> : <><p>{value.file.evidence.name} · 固定文件版本 {value.file.evidence.seq} · {value.file.evidence.size_bytes} 字节 · 扫描通过</p><code className='full-digest'>{value.file.evidence.sha256}</code><h4>准确文件的完整正文</h4><pre className='oa-prose-text' data-testid='contract-body-full-text'>{value.file.content}</pre>
      {adopted ? <p className='notice neutral'>完整正文已采用 · 合同修订 {value.result!.version} · 操作 {value.operation_id}</p> : run.state === 'succeeded' && run.outcome === 'completed' && !run.cancel_requested_at ? <><label className='check-row'><input type='checkbox' checked={reviewed} disabled={busy || blocked} onChange={event => setReviewed(event.target.checked)} />我已核对合同原始资料与完整正文，确认采用此准确文件版本</label><Field label='采用完整正文的依据'><input value={reason} maxLength={200} disabled={busy || blocked} onChange={event => setReason(event.target.value)} /></Field><button type='button' className='button secondary' disabled={busy || blocked || !reviewed || !reason.trim()} onClick={() => void adopt()}>{busy ? '正在保存合同修订' : '采用完整合同正文'}</button>{blocked && <p>请重新读取任务和合同资料，核对实际保存结果。</p>}</> : <p>本次任务需成功结束后才能采用。已取消或失败的任务不能采用。</p>}
      <p className='form-note'>采用会保存合同新修订。请另行核对送审、签署及生效。</p></>}
    <Link to={'/contracts/' + value.input.contract_id}>查看完整合同资料</Link>
  </>}</section>;
}
