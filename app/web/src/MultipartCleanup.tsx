import { useEffect, useRef, useState } from 'react';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { checkedCleanupPlan, checkedCleanupReceipts, cleanupBlockers, cleanupCreate, cleanupDecision, cleanupProgress, cleanupStates, type CleanupCandidate, type CleanupPlan, type CleanupReceipt, type CleanupSelection } from '@copypaste/contracts/multipart-cleanup';
import { api, ApiError, ensureDownloadSession, type Me, type Page, type Schemas } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

export function MultipartCleanup({ folder, me }: { folder: Schemas['Folder']; me: Me }) {
  const [open, setOpen] = useState(false), [revision, setRevision] = useState(0);
  return <section className="panel report-sources"><div className="section-heading"><div><span className="eyebrow">STORAGE / ABANDONED UPLOADS</span><h2>未完成上传清理</h2></div><button className="button secondary" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起清理工作区' : '盘点与清理'}</button></div><p className="muted">按当前目录核对已过期的未完成上传。固定候选清单经过观察期和人工批准后，由后台逐项终止分片并记录查询回执。</p>{open && <><button className="text-button" onClick={() => setRevision(value => value + 1)}>刷新盘点与执行结果</button><CleanupWorkspace key={[folder.id, me.user.id, me.authorization_version, revision].join(':')} folder={folder} authority={[me.user.id, me.authorization_version]} /></>}</section>;
}

function useCleanupCommand(folderID: string, committed: (plan: CleanupPlan) => void) {
  const [busy, setBusy] = useState(false), [blocked, setBlocked] = useState(false), [error, setError] = useState<unknown>();
  const pending = useRef(false), signature = useRef({ value: '', key: '' }), lifetime = useRef(new AbortController());
  useEffect(() => { const controller = new AbortController(); lifetime.current = controller; return () => controller.abort(); }, []);
  async function submit(path: string, input: () => unknown, expected?: (plan: CleanupPlan) => boolean) {
    if (pending.current || blocked) return;
    pending.current = true; setBusy(true); setError(undefined); let sent = false;
    try {
      const body = input(), value = JSON.stringify({ path, body });
      if (signature.current.value !== value) signature.current = { value, key: crypto.randomUUID() };
      await ensureDownloadSession(lifetime.current.signal); sent = true;
      const plan = checkedCleanupPlan(await api<CleanupPlan>(path, { method: 'POST', body, key: signature.current.key, signal: lifetime.current.signal }), folderID);
      if (expected && !expected(plan)) throw new Error('操作回执与本次固定清单不一致，请刷新核对。');
      if (!lifetime.current.signal.aborted) { setBlocked(true); committed(plan); }
    } catch (cause) {
      if (!lifetime.current.signal.aborted) {
        setError(cause instanceof ApiError ? cause : new ApiError(sent ? 0 : 422, sent ? 'RESULT_UNCONFIRMED' : 'CLEANUP_FORM_INVALID', cause instanceof Error ? cause.message : '请刷新并核对操作结果。', ''));
        if (sent && (!(cause instanceof ApiError) || cause.status === 0 || cause.status >= 500 || [403, 404, 409, 412].includes(cause.status))) setBlocked(true);
      }
    } finally { pending.current = false; if (!lifetime.current.signal.aborted) setBusy(false); }
  }
  return { submit, busy, blocked, error };
}

function CleanupWorkspace({ folder, authority }: { folder: Schemas['Folder']; authority: unknown[] }) {
  const cache = useQueryClient(), scope = ['multipart-cleanup', ...authority, folder.id];
  const [selected, setSelected] = useState<CleanupSelection[]>([]), [reason, setReason] = useState(''), [planID, setPlanID] = useState<string>();
  const [decision, setDecision] = useState<'approve' | 'cancel'>(), [retry, setRetry] = useState<Schemas['CleanupItem']>(), [receiptItem, setReceiptItem] = useState<string>();
  const candidates = useInfiniteQuery({ queryKey: [...scope, 'candidates'], initialPageParam: '', staleTime: 0, queryFn: ({ pageParam, signal }) => api<Page<CleanupCandidate>>('/folders/' + folder.id + '/upload-cleanup-candidates?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: v => v.next_cursor ?? undefined });
  const plans = useInfiniteQuery({ queryKey: [...scope, 'plans'], initialPageParam: '', staleTime: 0, queryFn: async ({ pageParam, signal }) => { const value = await api<Page<CleanupPlan>>('/folders/' + folder.id + '/upload-cleanup-plans?limit=10' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }); value.items.forEach(row => checkedCleanupPlan(row, folder.id)); return value; }, getNextPageParam: v => v.next_cursor ?? undefined });
  const detail = useQuery({ queryKey: [...scope, 'plan', planID], enabled: !!planID, staleTime: 0, queryFn: async ({ signal }) => checkedCleanupPlan(await api<CleanupPlan>('/upload-cleanup-plans/' + planID, { signal }), folder.id, planID) });
  function saved(plan: CleanupPlan) { setSelected([]); setReason(''); setPlanID(plan.id); setDecision(undefined); setRetry(undefined); setReceiptItem(undefined); void cache.invalidateQueries({ queryKey: scope }); }
  const create = useCleanupCommand(folder.id, saved), rows = candidates.error ? [] : candidates.data?.pages.flatMap(v => v.items) ?? [];
  const plan = detail.error ? undefined : detail.data, progress = plan ? cleanupProgress(plan) : undefined;
  return <><ErrorNotice error={candidates.error ?? plans.error ?? detail.error ?? create.error} />
    <h3>候选上传</h3><p className="muted">最多选择50项；只处理固定 UploadId 的未完成分片。文件正式内容与历史版本继续按保留制度管理。选中 {selected.length} 项。</p>
    {candidates.isPending ? <Busy label="正在盘点未完成上传" /> : !rows.length && !candidates.error ? <Empty title="此目录暂无可见的未完成上传" detail="新上传或仍在处理的任务会按当前状态显示。" /> : rows.map(row => <article className="report-context" key={row.upload_id}><label><input type="checkbox" aria-label={'选择清理 ' + row.name} disabled={!row.eligible || create.busy || create.blocked || selected.length >= 50 && !selected.some(v => v.upload_id === row.upload_id)} checked={selected.some(v => v.upload_id === row.upload_id)} onChange={event => setSelected(current => event.target.checked ? [...current, { upload_id: row.upload_id, expected_version: row.version }] : current.filter(v => v.upload_id !== row.upload_id))} /> {row.name}</label><p className="muted">{BigInt(row.size_bytes).toLocaleString()} 字节 · 上传修订 {row.version} · 过期于 {date(row.expires_at)}</p>{row.blocking_reasons.map(code => <p className="notice neutral" key={code}>{cleanupBlockers[code] ?? code}</p>)}</article>)}
    {candidates.hasNextPage && <button className="text-button" disabled={candidates.isFetchingNextPage} onClick={() => void candidates.fetchNextPage()}>加载更多候选上传</button>}
    <Field label="清理原因"><textarea aria-label="清理原因" rows={2} maxLength={200} value={reason} disabled={create.busy || create.blocked} onChange={event => setReason(event.target.value)} /></Field><button className="button secondary" disabled={create.busy || create.blocked || !selected.length || !reason.trim() || !!candidates.error} onClick={() => void create.submit('/folders/' + folder.id + '/upload-cleanup-plans', () => cleanupCreate(rows, selected, reason), value => value.state === 'observing')}>建立固定清单并进入观察期</button>{create.blocked && <p className="notice neutral">本次结果需要核对。请使用上方刷新，查阅已有清单后再操作。</p>}
    <h3>清理清单</h3>{plans.isPending ? <Busy label="正在读取清理清单" /> : !plans.error && !plans.data?.pages.some(v => v.items.length) ? <Empty title="尚未建立清理清单" detail="明确选择候选后建立清单，等待观察期结束。" /> : !plans.error && plans.data?.pages.flatMap(v => v.items).map(row => <div className="report-context" key={row.id}><span>{cleanupStates[row.state]} · {row.items.length} 项 · {row.reason}</span><button className="text-button" onClick={() => { setPlanID(row.id); setReceiptItem(undefined); }}>查阅固定清单</button></div>)}
    {plans.hasNextPage && <button className="text-button" disabled={plans.isFetchingNextPage} onClick={() => void plans.fetchNextPage()}>加载更多清理清单</button>}
    {planID && detail.isPending && <Busy label="正在核对固定清单" />}{plan && <section aria-label="清理清单详情"><h3>{cleanupStates[plan.state]} · {progress?.confirmed}/{progress?.total} 项确认不存在</h3><p>{plan.reason}</p><dl className="file-facts"><div><dt>观察期结束</dt><dd>{date(plan.ready_after)}</dd></div><div><dt>批准有效期结束</dt><dd>{date(plan.expires_at)}</dd></div><div className="wide"><dt>固定清单摘要</dt><dd><code>{plan.snapshot_sha256}</code></dd></div></dl>{plan.approved_at && <p className="muted">批准于 {date(plan.approved_at)} · {plan.approval_reason}</p>}{plan.cancelled_at && <p className="notice neutral">撤销于 {date(plan.cancelled_at)} · {plan.cancellation_reason}</p>}
      {plan.items.map(item => <article className="report-context" key={item.id}><strong>{plan.snapshot.items.find(v => v.upload_id === item.upload_id)?.name}</strong><p>{cleanupStates[item.state]} · 执行轮次 {item.generation} · 已记录结果 {item.attempts} 次{item.last_error ? ' · ' + item.last_error : ''}</p>{item.confirmed_at && <p>确认时间 {date(item.confirmed_at)}</p>}<button className="text-button" onClick={() => setReceiptItem(item.id)}>查阅逐次回执</button>{plan.state === 'approved' && item.state === 'failed' && <button className="text-button" onClick={() => setRetry(item)}>明确重试此项</button>}</article>)}
      <div className="form-actions">{plan.state === 'observing' && <button className="button" onClick={() => setDecision('approve')}>核对并批准清理</button>}{plan.state !== 'cancelled' && <button className="button secondary" onClick={() => setDecision('cancel')}>撤销尚未执行项</button>}<button className="text-button" onClick={() => { setReceiptItem(undefined); void detail.refetch(); }}>刷新此清单</button></div>
      {receiptItem && <CleanupReceipts key={receiptItem} id={receiptItem} authority={scope} />}
      {decision && <CleanupDecisionDialog key={plan.id + ':' + plan.version + ':' + decision} plan={plan} action={decision} close={() => setDecision(undefined)} saved={saved} />}
      {retry && <CleanupRetryDialog key={retry.id + ':' + retry.version} plan={plan} item={retry} close={() => setRetry(undefined)} saved={saved} />}
    </section>}
  </>;
}

function CleanupDecisionDialog({ plan, action, close, saved }: { plan: CleanupPlan; action: 'approve' | 'cancel'; close(): void; saved(plan: CleanupPlan): void }) {
  const [reason, setReason] = useState(''), [ack, setAck] = useState(false), [fresh, setFresh] = useState(false);
  const command = useCleanupCommand(plan.snapshot.folder_id, saved), title = action === 'approve' ? '批准固定清单' : '撤销尚未执行项';
  return <Dialog title={title} onClose={close} busy={command.busy}><p>固定 {plan.items.length} 项 · {plan.snapshot_sha256}</p><p className="muted">观察期结束：{date(plan.ready_after)}。批准有效期：{date(plan.expires_at)}。撤销会停止剩余工作，已确认的回执继续保留。</p><ErrorNotice error={command.error} /><Field label="操作原因"><textarea aria-label="清理操作原因" maxLength={200} value={reason} disabled={command.busy || command.blocked} onChange={e => setReason(e.target.value)} /></Field><label><input type="checkbox" checked={ack} disabled={command.busy || command.blocked} onChange={e => setAck(e.target.checked)} />我已核对固定候选、摘要和操作影响</label><MFAConfirmation onFresh={setFresh} />{command.blocked && <p className="notice neutral">请关闭此窗口并刷新清单，核对当前修订、权限与操作结果。</p>}<div className="dialog-actions"><button className="button secondary" onClick={close} disabled={command.busy}>返回清单</button><button className="button" disabled={command.busy || command.blocked || !fresh || !ack || !reason.trim()} onClick={() => void command.submit('/upload-cleanup-plans/' + plan.id + '/' + action, () => cleanupDecision(plan, reason, ack, action), value => value.id === plan.id && value.snapshot_sha256 === plan.snapshot_sha256 && value.state === (action === 'approve' ? 'approved' : 'cancelled'))}>{title}</button></div></Dialog>;
}

function CleanupRetryDialog({ plan, item, close, saved }: { plan: CleanupPlan; item: Schemas['CleanupItem']; close(): void; saved(plan: CleanupPlan): void }) {
  const [fresh, setFresh] = useState(false), [ack, setAck] = useState(false), command = useCleanupCommand(plan.snapshot.folder_id, saved);
  return <Dialog title="明确重试此项" onClose={close} busy={command.busy}><p>只重试 {plan.snapshot.items.find(v => v.upload_id === item.upload_id)?.name}；原批准与来源仍须有效，过往回执继续保留。</p><ErrorNotice error={command.error} /><label><input type="checkbox" checked={ack} disabled={command.busy || command.blocked} onChange={e => setAck(e.target.checked)} />我确认重新执行此项</label><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button secondary" onClick={close} disabled={command.busy}>返回清单</button><button className="button" disabled={!fresh || !ack || command.busy || command.blocked} onClick={() => void command.submit('/upload-cleanup-items/' + item.id + '/retry', () => ({ expected_version: item.version }), value => value.id === plan.id && value.items.some(v => v.id === item.id && v.generation === item.generation + 1 && v.state === 'queued'))}>确认重试此项</button></div>{command.blocked && <p>请关闭并刷新此清单核对结果。</p>}</Dialog>;
}

function CleanupReceipts({ id, authority }: { id: string; authority: unknown[] }) {
  const receipts = useInfiniteQuery({ queryKey: [...authority, 'receipts', id], initialPageParam: '', queryFn: async ({ pageParam, signal }) => { const value = await api<Page<CleanupReceipt>>('/upload-cleanup-items/' + id + '/receipts?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }); checkedCleanupReceipts(value.items); return value; }, getNextPageParam: v => v.next_cursor ?? undefined });
  return <section aria-label="逐次执行回执"><h3>逐次执行回执</h3><ErrorNotice error={receipts.error} />{receipts.isPending ? <Busy /> : !receipts.error && receipts.data?.pages.flatMap(v => v.items).map(row => <p key={row.id}>第 {row.generation} 轮 / 尝试 {row.attempt} · {cleanupStates[row.outcome]} · {date(row.created_at)}{row.error_code ? ' · ' + row.error_code : ''}</p>)}{!receipts.isPending && !receipts.error && !receipts.data?.pages.some(v => v.items.length) && <p className="muted">尚无 Worker 执行回执。</p>}{receipts.hasNextPage && <button className="text-button" disabled={receipts.isFetchingNextPage} onClick={() => void receipts.fetchNextPage()}>加载更多回执</button>}</section>;
}
