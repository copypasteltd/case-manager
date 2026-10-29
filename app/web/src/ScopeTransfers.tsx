import { useEffect, useRef, useState } from 'react';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { checkedScopeTransfer, scopeTransferCreate, scopeTransferDecision, transferBlockers, transferStates, type ScopeTransfer, type TransferResource } from '@copypaste/contracts/scope-transfer';
import { api, ApiError, ensureDownloadSession, type Me, type Page, type Schemas } from './api';
import { Busy, classificationNames, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

type Kind = 'file' | 'folder';
type Props = { kind: Kind; resource: TransferResource; me: Me };
const collection = (kind: Kind, id: string) => '/' + kind + 's/' + id + '/scope-transfers';

export function ScopeTransfers({ kind, resource, me }: Props) {
  const [open, setOpen] = useState(false), [revision, setRevision] = useState(0), [planID, setPlanID] = useState<string>();
  if (!me.actions.includes('policy.manage') || !me.actions.includes('user.read') || resource.state !== 'active'
    || (kind === 'file' ? !(resource as Schemas['File']).can_move : !(resource as Schemas['Folder']).can_update || (resource as Schemas['Folder']).parent_id === null)) return null;
  return <section className="panel report-sources"><div className="section-heading"><div><span className="eyebrow">FILES / REVIEWED TRANSFER</span><h2>跨项目与分类整理</h2></div><button className="button secondary" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起跨范围工作区' : '拟定跨范围移动'}</button></div><p className="muted">先选择实际目标目录，逐项核对文件、目录及读取权限变化。固定计划经本人确认后执行，正式内容与历史版本继续保留。</p>{open && <><button className="text-button" onClick={() => setRevision(v => v + 1)}>刷新计划与当前依据</button><TransferWorkspace key={[kind, resource.id, resource.version, me.authorization_version, revision].join(':')} kind={kind} resource={resource} me={me} initialPlan={planID} selected={setPlanID} /></>}</section>;
}

function useTransferCommand(kind: Kind, resourceID: string, owner: string, saved: (p: ScopeTransfer) => void) {
  const [busy, setBusy] = useState(false), [blocked, setBlocked] = useState(false), [error, setError] = useState<unknown>();
  const pending = useRef(false), signature = useRef({ body: '', key: '' }), lifetime = useRef(new AbortController());
  useEffect(() => { const controller = new AbortController(); lifetime.current = controller; return () => controller.abort(); }, []);
  async function submit(path: string, input: () => unknown, expected: (p: ScopeTransfer) => boolean) {
    if (pending.current || blocked) return;
    pending.current = true; setBusy(true); setError(undefined); let sent = false;
    try {
      const body = input(), serialized = JSON.stringify({ path, body });
      if (signature.current.body !== serialized) signature.current = { body: serialized, key: crypto.randomUUID() };
      await ensureDownloadSession(lifetime.current.signal); sent = true;
      const p = checkedScopeTransfer(await api<ScopeTransfer>(path, { method: 'POST', body, key: signature.current.key, signal: lifetime.current.signal }), kind, resourceID);
      if (p.creator_id !== owner || !expected(p)) throw new Error('操作回执与本次准确计划不一致，请刷新核对。');
      if (!lifetime.current.signal.aborted) { setBlocked(true); saved(p); }
    } catch (cause) {
      if (!lifetime.current.signal.aborted) {
        setError(cause instanceof ApiError ? cause : new ApiError(sent ? 0 : 422, sent ? 'RESULT_UNCONFIRMED' : 'TRANSFER_FORM_INVALID', cause instanceof Error ? cause.message : '请刷新核对本次结果。', ''));
        if (sent && (!(cause instanceof ApiError) || cause.status === 0 || cause.status >= 500 || [403, 404, 409, 412].includes(cause.status))) setBlocked(true);
      }
    } finally { pending.current = false; if (!lifetime.current.signal.aborted) setBusy(false); }
  }
  return { submit, busy, blocked, error };
}

function TransferWorkspace({ kind, resource, me, initialPlan, selected }: Props & { initialPlan?: string; selected(id: string): void }) {
  const cache = useQueryClient(), scope = ['file-scope-transfer', me.user.id, me.authorization_version, kind, resource.id];
  const [planID, setPlanID] = useState(initialPlan), [target, setTarget] = useState<Schemas['Folder']>(), [newName, setNewName] = useState(resource.name), [why, setWhy] = useState(''), [action, setAction] = useState<'execute' | 'cancel'>();
  function check(value: unknown, id?: string) { const p = checkedScopeTransfer(value, kind, resource.id, id); if (p.creator_id !== me.user.id) throw new Error('计划不属于当前员工，请刷新。'); return p; }
  const plans = useInfiniteQuery({ queryKey: [...scope, 'history'], initialPageParam: '', gcTime: 0, retry: false, queryFn: async ({ pageParam, signal }) => { const p = await api<Page<ScopeTransfer>>(collection(kind, resource.id) + '?limit=10' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }); p.items.forEach(v => check(v)); return p; }, getNextPageParam: v => v.next_cursor ?? undefined });
  const detail = useQuery({ queryKey: [...scope, 'detail', planID], enabled: !!planID, staleTime: 0, gcTime: 0, retry: false, queryFn: async ({ signal }) => check(await api<ScopeTransfer>('/file-scope-transfers/' + planID, { signal }), planID) });
  function choose(id: string) { selected(id); setPlanID(id); setAction(undefined); }
  function saved(p: ScopeTransfer) { choose(p.id); void cache.invalidateQueries({ queryKey: scope }); if (p.state === 'executed') void cache.invalidateQueries(); }
  const command = useTransferCommand(kind, resource.id, me.user.id, saved);
  const plan = !detail.error && !detail.isFetching ? detail.data : undefined;
  function create() {
    if (!target) return;
    let expected: Schemas['ScopeTransferCreate'];
    void command.submit('/file-scope-transfers', () => { expected = scopeTransferCreate(kind, resource, target, newName, why); return expected; }, p => p.state === 'pending' && JSON.stringify(p.input) === JSON.stringify(expected));
  }
  return <><ErrorNotice error={plans.error ?? detail.error ?? command.error} /><h3>选择实际目标</h3><TransferFolderPicker authority={scope} sourceID={kind === 'folder' ? resource.id : undefined} selected={target} choose={setTarget} disabled={command.busy || command.blocked} /><Field label="移动后的名称"><input aria-label="移动后的名称" maxLength={200} value={newName} disabled={command.busy || command.blocked} onChange={e => setNewName(e.target.value)} /></Field><Field label="跨范围整理原因"><textarea aria-label="跨范围整理原因" maxLength={200} value={why} disabled={command.busy || command.blocked} onChange={e => setWhy(e.target.value)} /></Field><button className="button secondary" disabled={!target || !why.trim() || command.busy || command.blocked} onClick={create}>分析跨范围移动</button>{command.blocked && <p className="notice neutral">请查阅本次计划，或刷新计划与当前依据后继续。</p>}
    <h3>本人的固定计划</h3>{plans.isPending ? <Busy /> : !plans.error && !plans.data?.pages.some(v => v.items.length) ? <Empty title="尚无跨范围计划" detail="分析只建立计划，核对并明确确认后才移动。" /> : !plans.error && plans.data?.pages.flatMap(v => v.items).map(p => <div className="report-context" key={p.id}><span>{transferStates[p.state]} · {p.snapshot.source.name} → {p.snapshot.target.name} · {p.snapshot.items.length}项</span><button className="text-button" onClick={() => choose(p.id)}>查阅此移动计划</button></div>)}{plans.hasNextPage && <button className="text-button" disabled={plans.isFetchingNextPage} onClick={() => void plans.fetchNextPage()}>加载更多移动计划</button>}
    {planID && detail.isFetching && <Busy label="正在重新核对移动计划" />}{plan && <section aria-label="跨范围移动计划详情"><TransferDetails plan={plan} /><div className="form-actions">{plan.state === 'pending' && <><button className="button" disabled={!plan.can_execute} onClick={() => setAction('execute')}>核对并执行跨范围移动</button><button className="button secondary" onClick={() => setAction('cancel')}>撤销此移动计划</button></>}<button className="text-button" onClick={() => { setAction(undefined); void detail.refetch(); }}>重新核对这份计划</button></div>{action && <TransferDecision key={plan.id + ':' + plan.version + ':' + action} plan={plan} action={action} owner={me.user.id} saved={saved} close={() => setAction(undefined)} />}</section>}
  </>;
}

function TransferFolderPicker({ authority, sourceID, selected, choose, disabled }: { authority: unknown[]; sourceID?: string; selected?: Schemas['Folder']; choose(v: Schemas['Folder']): void; disabled: boolean }) {
  const [path, setPath] = useState<Schemas['Folder'][]>([]), current = path.at(-1);
  const folders = useInfiniteQuery({ queryKey: [...authority, 'targets', current?.id ?? 'root'], initialPageParam: '', gcTime: 0, retry: false, queryFn: ({ pageParam, signal }) => api<Page<Schemas['Folder']>>('/folders?limit=20' + (current ? '&parent_id=' + current.id : '') + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: p => p.next_cursor ?? undefined });
  return <section aria-label="跨范围目标目录"><p className="muted">{['所有可访问根目录', ...path.map(v => v.name)].join(' / ')}</p>{path.length > 0 && <button className="text-button" disabled={disabled} onClick={() => setPath(v => v.slice(0, -1))}>返回目标上一级</button>}{current?.can_create && <button className="button secondary" disabled={disabled || current.id === sourceID} onClick={() => choose(current)}>选择当前目标目录</button>}<ErrorNotice error={folders.error} />{folders.isFetching && !folders.isFetchingNextPage ? <Busy /> : !folders.error && folders.data?.pages.flatMap(p => p.items).filter(v => v.id !== sourceID).map(folder => <div className="report-context" key={folder.id}><FolderLabel folder={folder} /><button className="text-button" disabled={disabled || path.length >= 31} onClick={() => setPath(v => [...v, folder])}>进入目标目录</button><button className="text-button" aria-label={'选择目标 ' + folder.name} disabled={disabled || !folder.can_create} onClick={() => choose(folder)}>选择此目标</button></div>)}{folders.hasNextPage && <button className="text-button" disabled={disabled || folders.isFetchingNextPage} onClick={() => void folders.fetchNextPage()}>加载更多目标目录</button>}{selected && <p className="notice neutral">已选目标：{selected.name} · {classificationNames[selected.classification]} · 修订 {selected.version}</p>}</section>;
}
function FolderLabel({ folder }: { folder: Schemas['Folder'] }) {
  const project = useQuery({ queryKey: ['project', folder.project_id], enabled: !!folder.project_id && folder.parent_id === null, queryFn: ({ signal }) => api<Schemas['Project']>('/projects/' + folder.project_id, { signal }), retry: false, gcTime: 0 });
  return <span>{folder.parent_id === null ? folder.project_id ? project.error ? '项目名称读取失败 · ' : (project.data?.name ?? '正在读取项目') + ' / ' : '公司 / ' : ''}{folder.name} · {classificationNames[folder.classification]}</span>;
}

function TransferDetails({ plan: p }: { plan: ScopeTransfer }) {
  return <><h3>{transferStates[p.state]}</h3><p>{[...[...p.snapshot.source_path].reverse().map(v => v.name), p.snapshot.source.name].join(' / ')} → {[...[...p.snapshot.target_path].reverse().map(v => v.name), p.snapshot.target.name, p.snapshot.new_name].join(' / ')}</p><p>{p.input.reason}</p><dl className="file-facts"><div><dt>有效期结束</dt><dd>{date(p.expires_at)}</dd></div><div><dt>本次完整清单</dt><dd>{p.snapshot.items.length}项</dd></div><div className="wide"><dt>固定计划摘要</dt><dd><code>{p.plan_hash}</code></dd></div></dl>{p.completed_at && <p className="notice neutral">{date(p.completed_at)} · {p.decision_reason}</p>}{p.state === 'pending' && !p.can_execute && <p className="notice neutral">当前依据已改变、已到期或存在阻塞。请处理原业务后重新拟定。</p>}<p className="muted">以下数量为员工 × 渠道的读取策略组合，分别计算普通访问与 AI 场景；不是人数，也不代表 TG 群或模型实际获准访问。</p>{p.snapshot.items.map(item => <article className="report-context" key={item.id}><strong>{item.kind === 'file' ? '文件' : '目录'} · {item.name} · 修订 {item.version}</strong><p>{classificationNames[item.classification]} → {classificationNames[item.new_classification]} · {item.state === 'trashed' ? '保持回收站状态' : '当前有效'}{item.kind === 'file' ? ' · 保留 ' + item.content_versions + ' 个正式内容版本' : ''}</p><p>普通读取策略组合 {item.human_before} → {item.human_after} · AI读取策略组合 {item.ai_before} → {item.ai_after}</p>{item.blocking_reasons.map(code => <p className="notice neutral" key={code}>{transferBlockers[code]}</p>)}{item.visibility_changes.length ? item.visibility_changes.map(v => <p className="muted" key={v.user_id + v.channel + v.ai}>{v.display_name} · {v.channel} · {v.ai ? 'AI场景' : '普通访问'}：{v.before ? '允许' : '不允许'} → {v.after ? '允许' : '不允许'}</p>) : <p className="muted">读取策略无变化。</p>}</article>)}</>;
}
function TransferDecision({ plan, action, owner, close, saved }: { plan: ScopeTransfer; action: 'execute' | 'cancel'; owner: string; close(): void; saved(p: ScopeTransfer): void }) {
  const [why, setWhy] = useState(''), [ack, setAck] = useState(false), [fresh, setFresh] = useState(false);
  const command = useTransferCommand(plan.input.kind, plan.input.resource_id, owner, saved), locked = command.busy || command.blocked;
  const title = action === 'execute' ? '确认并执行跨范围移动' : '确认撤销此移动计划';
  return <Dialog title={title} onClose={close} busy={command.busy}><p>{plan.snapshot.source.name} → {plan.snapshot.target.name} / {plan.snapshot.new_name} · 完整 {plan.snapshot.items.length}项</p><p className="muted">{plan.plan_hash}</p><ErrorNotice error={command.error} /><Field label="移动计划确认原因"><textarea aria-label="移动计划确认原因" maxLength={200} value={why} disabled={locked} onChange={e => setWhy(e.target.value)} /></Field><label><input type="checkbox" checked={ack} disabled={locked} onChange={e => setAck(e.target.checked)} />我已核对全部清单、读取权限变化及本次操作影响</label>{action === 'execute' && <MFAConfirmation onFresh={setFresh} />}<div className="dialog-actions"><button className="button secondary" onClick={close} disabled={command.busy}>返回移动计划</button><button className="button" disabled={locked || !ack || !why.trim() || action === 'execute' && !fresh} onClick={() => void command.submit('/file-scope-transfers/' + plan.id + '/' + action, () => scopeTransferDecision(plan, why, ack, action), p => p.id === plan.id && p.plan_hash === plan.plan_hash && p.version === plan.version + 1 && p.state === (action === 'execute' ? 'executed' : 'cancelled'))}>{title}</button></div>{command.blocked && <p>请返回并刷新计划，核对准确执行结果。</p>}</Dialog>;
}
