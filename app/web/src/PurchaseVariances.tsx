import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft } from 'lucide-react';
import { api, ApiError, type Me, type Page, type Schemas } from './api';
import { Busy, classificationNames, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { ApprovalBadge, money } from './ContractDisplay';
import { useCommandKey } from './BusinessSelectors';
import { MFAConfirmation } from './MFAConfirmation';
import { SpendEditor, refreshSpend } from './SpendEditor';
import { PrepareRequestDialog } from './Requests';
import { PurchaseVarianceFacts } from './PurchaseVarianceFacts';

type Purchase = Schemas['Spend'];
type Variance = Schemas['PurchaseVariance'];
type Plan = Schemas['PurchaseExecutionPlan'];
const conflict = (error: unknown) => error instanceof ApiError && ['VERSION_CONFLICT', 'CONCURRENT_MODIFICATION', 'PURCHASE_VARIANCE_BASIS_CHANGED', 'AUTHORIZATION_CHANGED'].includes(error.code);

export function PurchaseVariances({ purchase, me }: { purchase: Purchase; me: Me }) {
  const [creating, setCreating] = useState(false), approved = purchase.approval_state === 'approved';
  const plan = useQuery({ queryKey: ['spend', 'purchase', purchase.id, 'execution-plan', purchase.version], enabled: approved, queryFn: ({ signal }) => api<Plan>('/purchases/' + purchase.id + '/execution-plan', { signal }) });
  const list = useInfiniteQuery({ queryKey: ['purchase-variance', 'list', purchase.id, purchase.version], enabled: approved && me.actions.includes('purchase_variance.read'), initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Variance>>('/purchases/' + purchase.id + '/variances?' + new URLSearchParams({ limit: '10', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  if (!approved) return null;
  const current = plan.error ? undefined : plan.data, items = list.error ? [] : list.data?.pages.flatMap(page => page.items) ?? [];
  return <section className="panel party-detail" aria-label="执行基线与差异重审"><div className="section-heading"><div><h2>执行基线与差异重审</h2><p className="muted">原批准、差异审批与明确启用分别留存。</p></div>{current && <span className="badge neutral">{current.variance_id ? '差异基线已启用' : '沿用原批准'}</span>}</div><ErrorNotice error={plan.error} />{plan.isPending ? <Busy /> : current && <dl className="file-facts"><div><dt>原商业批准金额</dt><dd>{money(purchase.amount, purchase.currency)}</dd></div><div><dt>当前执行总额</dt><dd>{money(current.amount, current.content.currency)}</dd></div><div><dt>执行依据</dt><dd>{current.variance_id ? <Link className="text-button" to={'/purchase-variances/' + current.variance_id}>查阅已启用差异提案 ↗</Link> : '原采购商业批准'}</dd></div><div><dt>启用时间</dt><dd>{current.activated_at ? date(current.activated_at) : '随原批准确立'}</dd></div></dl>}
    <p className="form-note">数量、单价、税额、折扣或供应商需要调整时，按独立制度重审完整提案。新基线约束后续订单；旧订单保留登记时的单价与依据。</p>{purchase.can_create_variance && <button className="button secondary" disabled={!current || plan.isFetching} onClick={() => setCreating(true)}>起草采购差异提案</button>}
    {me.actions.includes('purchase_variance.read') && <><ErrorNotice error={list.error} />{list.isPending ? <Busy /> : !list.error && (!items.length ? <Empty title="尚无可查阅的差异提案" detail="需要调整时，从当前执行依据起草提案。" /> : items.map(item => <article className="party-revision" key={item.id}><div className="section-heading"><div><span className="record-code">{item.document_no}</span><h3><Link className="text-button" to={'/purchase-variances/' + item.id}>{item.title}</Link></h3></div><ApprovalBadge value={item.approval_state} /></div><p>{money(item.amount, item.currency)} · {current?.variance_id === item.id ? '当前执行基线' : item.lifecycle === 'effective' ? '历史已启用基线' : item.approval_state === 'approved' ? '获批，尚未启用' : '保留原执行依据'}</p><p className="muted">固定采购版本 {item.basis.purchase_version} · {date(item.updated_at)}</p></article>))}{list.hasNextPage && <button className="button secondary" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>加载更早差异提案</button>}</>}
    {creating && current && <SpendEditor me={me} kind="purchase" variance={{ purchase, plan: current }} onClose={() => setCreating(false)} />}
  </section>;
}

export function PurchaseVarianceDetail({ me }: { me: Me }) {
  const { id = '' } = useParams(), [action, setAction] = useState<'edit' | 'prepare' | 'activate' | 'history' | null>(null);
  const query = useQuery({ queryKey: ['purchase-variance', 'detail', id], queryFn: ({ signal }) => api<Variance>('/purchase-variances/' + id, { signal }) });
  const value = query.error ? undefined : query.data;
  const parent = useQuery({ queryKey: ['spend', 'purchase', value?.basis.purchase_id], enabled: !!value, queryFn: ({ signal }) => api<Purchase>('/purchases/' + value!.basis.purchase_id, { signal }) });
  const plan = useQuery({ queryKey: ['spend', 'purchase', value?.basis.purchase_id, 'execution-plan', parent.data?.version], enabled: !!value && !!parent.data && !parent.error, queryFn: ({ signal }) => api<Plan>('/purchases/' + value!.basis.purchase_id + '/execution-plan', { signal }) });
  if (!value) return query.isPending ? <Busy /> : <ErrorNotice error={query.error} />;
  const current = plan.error ? undefined : plan.data, purchase = parent.error ? undefined : parent.data;
  return <><Link className="back-link" to={'/purchases/' + value.basis.purchase_id}><ArrowLeft size={16} />原采购与履行记录</Link><div className="page-heading"><div><div className="eyebrow">{value.document_no} / PURCHASE VARIANCE</div><h1>{value.title}</h1><p>{classificationNames[value.classification]} · 更新于 {date(value.updated_at)}</p></div><ApprovalBadge value={value.approval_state} /></div>
    {value.approval_state === 'approved' && value.lifecycle !== 'effective' && <p className="notice neutral">差异提案已获批，尚未启用。订单仍按当前执行基线登记。</p>}{value.lifecycle === 'effective' && <p className="notice success">{current ? current.variance_id === value.id ? '此提案是当前已启用的执行基线。' : '此提案曾启用，后续基线已另行记录。' : '此提案已有启用记录，当前执行基线正在核对。'}</p>}
    <ErrorNotice error={parent.error ?? plan.error} />{purchase && purchase.version !== value.basis.purchase_version && value.lifecycle !== 'effective' && <p className="notice warning">采购履行已变化。此提案不能按旧依据启用；请核对当前记录后明确修订并重新审批。</p>}
    <div className="post-actions">{value.can_edit && <button className="button primary" disabled={!purchase || !current || plan.isFetching} onClick={() => setAction('edit')}>核对当前依据并编辑提案</button>}{value.can_prepare_request && <button className="button secondary" onClick={() => setAction('prepare')}>准备差异送审申请</button>}{value.request_id && <Link className="button secondary" to={'/requests/' + value.request_id}>查看差异审批申请</Link>}{value.can_activate && <button className="button primary" onClick={() => setAction('activate')}>核对并启用执行基线</button>}{value.can_read_history && <button className="button secondary" onClick={() => setAction('history')}>查看差异商业修订</button>}</div>
    <section className="panel party-detail"><PurchaseVarianceFacts value={value} /></section>
    {action === 'edit' && purchase && current && <SpendEditor me={me} kind="purchase" value={value} variance={{ purchase, plan: current }} onClose={() => setAction(null)} />}{action === 'prepare' && <PrepareRequestDialog business={value} kind="purchase_variance" onClose={() => setAction(null)} />}{action === 'activate' && <ActivationDialog value={value} onClose={() => setAction(null)} />}{action === 'history' && <VarianceHistory value={value} onClose={() => setAction(null)} />}
  </>;
}

function ActivationDialog({ value, onClose }: { value: Variance; onClose: () => void }) {
  const [base] = useState(value), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false), [confirmed, setConfirmed] = useState(false), cache = useQueryClient(), key = useCommandKey();
  const failure = useRef<HTMLDivElement>(null);
  const mutation = useMutation({ mutationFn: () => { const body: Schemas['PurchaseVarianceActivation'] = { expected_version: base.version, revision_id: base.current_revision_id, content_hash: base.content_hash, expected_purchase_version: base.basis.purchase_version, reason }; return api<Variance>('/purchase-variances/' + base.id + '/activate', { method: 'POST', body, key: key(body) }); }, onSuccess: async () => { await refreshSpend(cache); onClose(); } });
  useEffect(() => { if (mutation.error) failure.current?.focus(); }, [mutation.error]);
  return <Dialog title="启用采购执行基线" onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><h3>{base.title}</h3><p className="form-note">将准确获批的完整提案用于后续订单与费用额度。现有订单、验收、费用和付款各自保留依据。</p><dl className="file-facts"><div><dt>原执行总额</dt><dd>{money(base.basis.amount, base.basis.content.currency)}</dd></div><div><dt>拟启用总额</dt><dd>{money(base.amount, base.currency)}</dd></div><div><dt>固定采购版本</dt><dd>{base.basis.purchase_version}</dd></div></dl><PurchaseVarianceFacts value={base} /><div ref={failure} tabIndex={-1} aria-label="启用执行结果"><ErrorNotice error={mutation.error} />{conflict(mutation.error) && <p className="notice neutral">采购或授权已变化，未启用此提案。填写内容仍保留，请关闭后核对当前依据。</p>}</div><Field label="本次启用依据"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><label className="purchase-line-choice"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />已核对完整提案、现有承诺与履行依据，确认用于后续执行</label><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={!fresh || !confirmed || conflict(mutation.error) || mutation.isPending}>确认启用此执行基线</button></div></form></Dialog>;
}

function VarianceHistory({ value, onClose }: { value: Variance; onClose: () => void }) {
  const query = useInfiniteQuery({ queryKey: ['purchase-variance', 'history', value.id, value.version], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['PurchaseVarianceRevision']>>('/purchase-variances/' + value.id + '/revisions?' + new URLSearchParams({ limit: '5', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  return <Dialog title="差异商业修订" onClose={onClose}><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && query.data?.pages.flatMap(page => page.items).map(item => <article className="party-revision" key={item.id}><h3>商业修订 {item.seq} · {item.title}</h3><p className="muted">{date(item.created_at)} · {item.reason}</p><PurchaseVarianceFacts value={item} /></article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早商业修订</button>}</Dialog>;
}
