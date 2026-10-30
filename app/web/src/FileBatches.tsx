import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { FileText, ArrowUpRight } from 'lucide-react';
import { batchActions, batchBlockers, batchInputMatches, batchStates, checkedFileBatch, fileBatchCreate, fileBatchDecision, type BatchAction, type FileBatchPlan } from '@copypaste/contracts/file-batch';
import { api, ApiError, ensureDownloadSession, type Me, type Page, type Schemas } from './api';
import { Busy, classificationNames, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';
import { TransferFolderPicker } from './ScopeTransfers';
import './file-batches.css';

type File = Schemas['File'];
type Start = { action: BatchAction; files: File[] };
export function BatchFileList({ items, me }: { items: File[]; me: Me }) {
  const [selected, setSelected] = useState<File[]>([]), [start, setStart] = useState<Start>();
  function toggle(f: File) { setSelected(v => v.some(i => i.id === f.id) ? v.filter(i => i.id !== f.id) : v.length < 50 ? [...v, f] : v); }
  return <><div className="panel report-sources"><div className="section-heading"><strong>已选择 {selected.length} / 50 个文件</strong><div className="post-actions"><button className="text-button" onClick={() => setSelected(items.slice(0, 50))}>选择已加载文件</button><button className="text-button" disabled={!selected.length} onClick={() => setSelected([])}>清空选择</button></div></div><p className="muted">切换范围或筛选后重新选择；批量移动限相同项目与分类。先分析固定清单，再由本人确认。</p><div className="post-actions">{(Object.keys(batchActions) as BatchAction[]).map(action => <button className="button secondary" key={action} disabled={!selected.length || selected.some(f => action === 'restore' ? f.state !== 'trashed' || !f.can_restore : f.state !== 'active' || !(action === 'move' ? f.can_move : f.can_trash))} onClick={() => setStart({ action, files: [...selected] })}>{batchActions[action]}</button>)}</div></div>
    <div className="panel file-list">{!items.length && <Empty title="当前范围没有文件" detail="可切换范围或文件状态继续查阅。" />}{items.map(f => <div className="batch-file-row" key={f.id}><label className="batch-select"><input type="checkbox" aria-label={'选择文件 ' + f.name} checked={selected.some(i => i.id === f.id)} disabled={selected.length >= 50 && !selected.some(i => i.id === f.id)} onChange={() => toggle(f)} /></label><Link className="file-row" to={'/files/' + f.id}><FileText size={22} /><div><strong>{f.name}</strong><small>{classificationNames[f.classification]} · {date(f.updated_at)}</small></div><span className="badge neutral">{f.state === 'trashed' ? '回收站' : '版本 ' + f.version}</span><ArrowUpRight size={17} /></Link></div>)}</div>
    {start && <BatchWorkspace me={me} start={start} close={() => setStart(undefined)} completed={() => setSelected([])} />}</>;
}
function useBatchCommand(owner: string, saved: (p: FileBatchPlan) => void) {
  const [busy, setBusy] = useState(false), [blocked, setBlocked] = useState(false), [error, setError] = useState<unknown>();
  const pending = useRef(false), signature = useRef({ body: '', key: '' }), life = useRef(new AbortController());
  useEffect(() => { const controller = new AbortController(); life.current = controller; return () => controller.abort(); }, []);
  async function submit(path: string, make: () => unknown, expected: (p: FileBatchPlan) => boolean) {
    if (pending.current || blocked) return;
    pending.current = true; setBusy(true); setError(undefined); let sent = false;
    try {
      const body = make(), serialized = JSON.stringify({ path, body });
      if (signature.current.body !== serialized) signature.current = { body: serialized, key: crypto.randomUUID() };
      await ensureDownloadSession(life.current.signal); sent = true;
      const p = checkedFileBatch(await api(path, { method: 'POST', body, key: signature.current.key, signal: life.current.signal }), owner);
      if (!expected(p)) throw new Error('批量回执与本次准确操作不一致，请刷新核对。');
      if (!life.current.signal.aborted) { setBlocked(true); saved(p); }
    } catch (cause) {
      if (!life.current.signal.aborted) {
        setError(cause instanceof ApiError ? cause : new ApiError(sent ? 0 : 422, sent ? 'RESULT_UNCONFIRMED' : 'BATCH_INPUT_INVALID', cause instanceof Error ? cause.message : '请刷新核对本次结果。', ''));
        if (sent && (!(cause instanceof ApiError) || cause.status === 0 || cause.status >= 500 || [403, 404, 409, 412].includes(cause.status))) setBlocked(true);
      }
    } finally { pending.current = false; if (!life.current.signal.aborted) setBusy(false); }
  }
  return { submit, busy, blocked, error };
}
function BatchWorkspace({ me, start, initialPlan, close, completed }: { me: Me; start?: Start; initialPlan?: string; close(): void; completed?(): void }) {
  const cache = useQueryClient(), scope = ['file-batch', me.user.id, me.authorization_version];
  const [planID, setPlanID] = useState(initialPlan), [target, setTarget] = useState<Schemas['Folder']>(), [reason, setReason] = useState(''), [action, setAction] = useState<'execute' | 'cancel'>();
  const detail = useQuery({ queryKey: [...scope, planID], enabled: !!planID, gcTime: 0, staleTime: 0, retry: false, queryFn: async ({ signal }) => checkedFileBatch(await api('/file-batch-plans/' + planID, { signal }), me.user.id, planID) });
  function saved(p: FileBatchPlan) { setPlanID(p.id); setAction(undefined); void cache.invalidateQueries(); if (p.state === 'executed') completed?.(); }
  const command = useBatchCommand(me.user.id, saved), locked = command.busy || command.blocked;
  const plan = !detail.error && !detail.isFetching ? detail.data : undefined;
  function create() { if (!start) return; let expected: Schemas['FileBatchCreate']; void command.submit('/file-batch-plans', () => { expected = fileBatchCreate(start.action, start.files, target, reason); return expected; }, p => p.state === 'pending' && batchInputMatches(p.input, expected)); }
  return <Dialog title={start ? batchActions[start.action] : '批量文件计划'} onClose={close} busy={command.busy}><ErrorNotice error={command.error ?? detail.error} />{!planID && start && <><p>固定选择 {start.files.length} 个文件</p>{start.files.map(f => <p className="muted" key={f.id}>{f.name} · 修订 {f.version}</p>)}{start.action === 'move' && <TransferFolderPicker authority={scope} selected={target} choose={setTarget} disabled={locked} />}<Field label="批量整理原因"><textarea aria-label="批量整理原因" maxLength={200} value={reason} disabled={locked} onChange={e => setReason(e.target.value)} /></Field><button className="button" disabled={locked || !reason.trim() || start.action === 'move' && !target} onClick={create}>分析批量清单</button></>}{command.blocked && !planID && <p className="notice neutral">请关闭并刷新列表，再从相关文件的批量记录核对本次结果。</p>}
    {planID && detail.isFetching && <Busy label="正在核对固定批量清单" />}{plan && <><BatchDetails plan={plan} /><div className="form-actions">{plan.state === 'pending' && <><button className="button" disabled={!plan.can_execute} onClick={() => setAction('execute')}>核对并执行整批操作</button><button className="button secondary" onClick={() => setAction('cancel')}>撤销此批量计划</button></>}<button className="text-button" onClick={() => { setAction(undefined); void detail.refetch(); }}>刷新批量计划</button></div>{action && <BatchDecision key={plan.id + ':' + plan.version + ':' + action} plan={plan} action={action} owner={me.user.id} saved={saved} close={() => setAction(undefined)} />}</>}</Dialog>;
}
function BatchDetails({ plan: p }: { plan: FileBatchPlan }) {
  return <section aria-label="固定批量清单"><h3>{batchStates[p.state]} · {batchActions[p.input.action]}</h3><p>{p.input.reason}</p><p>完整 {p.snapshot.items.length} 个文件 · 有效期结束 {date(p.expires_at)}</p>{p.snapshot.target && <p>目标：{[...[...p.snapshot.target.path].reverse().map(v => v.name), p.snapshot.target.folder.name].join(' / ')} · 修订 {p.snapshot.target.folder.version}</p>}<p className="muted">清单编号：{p.id}</p><p className="muted"><code>{p.plan_hash}</code></p>{p.state === 'pending' && !p.can_execute && <p className="notice neutral">存在阻塞、依据已变化或计划到期，请处理原业务后重新选择并拟定清单。</p>}{p.snapshot.items.map((item, i) => <article className="report-context" key={item.file.id}><strong>{item.file.name} · 修订 {item.file.version}</strong><p>{[...[...item.path].reverse().map(v => v.name), item.file.name].join(' / ')}</p><p className="muted">{classificationNames[item.file.classification]} · {item.file.state === 'trashed' ? '回收站' : '当前有效'}{item.retention_held ? ' · 存在保留锁' : ''}</p>{item.blocking_reasons.map(code => <p className="notice neutral" key={code}>{batchBlockers[code]}</p>)}{p.results[i] && <p className="notice neutral">本次回执：修订 {p.results[i].version} · {p.results[i].state === 'trashed' ? '已移入回收站' : '当前有效'} · <Link to={'/files/' + item.file.id}>查阅实际文件</Link></p>}</article>)}{p.completed_at && <p className="notice neutral">{date(p.completed_at)} · {p.decision_reason}</p>}<p className="muted">保留原正式内容版本；任一项冲突则整批不执行。每个相关文件均可重新查阅本次记录。</p></section>;
}
function BatchDecision({ plan, action, owner, saved, close }: { plan: FileBatchPlan; action: 'execute' | 'cancel'; owner: string; saved(p: FileBatchPlan): void; close(): void }) {
  const [reason, setReason] = useState(''), [ack, setAck] = useState(false), [fresh, setFresh] = useState(false);
  const command = useBatchCommand(owner, saved), locked = command.busy || command.blocked, title = action === 'execute' ? '确认并执行整批操作' : '确认撤销批量计划';
  return <section className="panel report-sources" aria-label={title}><h3>{title}</h3><p>{batchActions[plan.input.action]} · 完整 {plan.snapshot.items.length} 个文件</p><ErrorNotice error={command.error} /><Field label="批量计划确认原因"><textarea aria-label="批量计划确认原因" maxLength={200} value={reason} disabled={locked} onChange={e => setReason(e.target.value)} /></Field><label><input type="checkbox" checked={ack} disabled={locked} onChange={e => setAck(e.target.checked)} />我已核对全部文件、目标及本次操作影响</label>{action === 'execute' && <MFAConfirmation onFresh={setFresh} />}<div className="dialog-actions"><button className="button secondary" disabled={command.busy} onClick={close}>返回批量清单</button><button className="button" disabled={locked || !ack || !reason.trim() || action === 'execute' && !fresh} onClick={() => void command.submit('/file-batch-plans/' + plan.id + '/' + action, () => fileBatchDecision(plan, reason, ack, action), p => p.id === plan.id && p.plan_hash === plan.plan_hash && batchInputMatches(p.input, plan.input) && p.state === (action === 'execute' ? 'executed' : 'cancelled') && p.version === plan.version + 1)}>{title}</button></div>{command.blocked && <p>请返回并刷新清单，核对准确结果。</p>}</section>;
}
export function FileBatchHistory({ fileID, me }: { fileID: string; me: Me }) {
  const [open, setOpen] = useState(false), [planID, setPlanID] = useState<string>();
  const plans = useInfiniteQuery({ queryKey: ['file-batch-history', me.user.id, me.authorization_version, fileID], initialPageParam: '', enabled: open, gcTime: 0, retry: false, queryFn: async ({ pageParam, signal }) => { const p = await api<Page<FileBatchPlan>>('/files/' + fileID + '/batch-plans?limit=10' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }); p.items.forEach(v => checkedFileBatch(v, me.user.id, undefined, fileID)); return p; }, getNextPageParam: p => p.next_cursor ?? undefined });
  return <section className="panel report-sources"><div className="section-heading"><h2>本人的批量整理记录</h2><button className="text-button" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起批量记录' : '查阅批量记录'}</button></div>{open && <><ErrorNotice error={plans.error} />{plans.isPending ? <Busy /> : !plans.error && !plans.data?.pages.some(p => p.items.length) ? <Empty title="尚无批量计划" detail="从文件列表多选后分析清单；相关文件均可重新查阅。" /> : !plans.error && plans.data?.pages.flatMap(p => p.items).map(p => <div className="report-context" key={p.id}><span>{batchActions[p.input.action]} · {batchStates[p.state]} · {p.snapshot.items.length}个文件</span><button className="text-button" onClick={() => setPlanID(p.id)}>查阅这份批量清单</button></div>)}{plans.hasNextPage && <button className="text-button" disabled={plans.isFetchingNextPage} onClick={() => void plans.fetchNextPage()}>加载更多批量记录</button>}</>}{planID && <BatchWorkspace key={planID} me={me} initialPlan={planID} close={() => setPlanID(undefined)} />}</section>;
}
