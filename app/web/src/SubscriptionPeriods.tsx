import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft } from 'lucide-react';
import { api, ApiError, type Me, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field } from './components';
import { FileVersionPicker, useCommandKey } from './BusinessSelectors';
import { MFAConfirmation } from './MFAConfirmation';
import { CostFacts, type Cost } from './CostDisplay';
import { money } from './ContractDisplay';
import { paymentTimestamp } from './PaymentDisplay';
import { BillingBadge, BillingSignals, FixedBillingFiles, billingBase, refreshSubscriptions, type BillingPeriod, type Subscription } from './SubscriptionDisplay';

type Action = Schemas['SubscriptionPeriodDecision']['action'] | 'link' | 'reforecast';
const names: Record<Action, string> = { confirm: '确认本次核对', waive: '登记免付', void: '作废预测', reopen: '重新开放核对', link: '关联真实费用', reforecast: '按当前配置重估' };
function normalizedAmount(value: string) { const [integer, fraction = ''] = value.split('.'); return (integer.replace(/^0+(?=\d)/, '') || '0') + '.' + fraction.replace(/0+$/, ''); }

export default function SubscriptionPeriodDetail({ me }: { me: Me }) {
  const { id } = useParams();
  const query = useQuery({ queryKey: ['subscription-period', id], enabled: me.actions.includes('subscription.read'), queryFn: ({ signal }) => api<BillingPeriod>('/subscription-periods/' + id, { signal }) });
  const [action, setAction] = useState<Action | null>(null), [history, setHistory] = useState(false), [events, setEvents] = useState(false);
  if (!me.actions.includes('subscription.read')) return <Empty title="没有账期业务权限" detail="请联系业务负责人核对授权。" />;
  if (query.error) return <ErrorNotice error={query.error} />;
  if (!query.data) return <Busy />;
  const p = query.data;
  return <><Link className="back-link" to={'/subscriptions/' + p.subscription_id}><ArrowLeft size={16} />{p.product}</Link><div className="page-heading"><div><h1>{p.start_date} 账期</h1><p>{p.product} · 至 {p.end_date} 前 · {p.timezone}</p></div><BillingBadge value={p} /></div><BillingSignals values={p.anomalies} />
    <div className="post-actions billing-period-actions">{p.can_manage && <>{me.actions.includes('cost.record') && <button className="button secondary" onClick={() => setAction('link')}>关联真实费用</button>}{['forecast', 'confirmed'].includes(p.recorded_state) && p.cost_ids.length > 0 && <button className="button primary" onClick={() => setAction('confirm')}>核对实际扣款</button>}{p.recorded_state === 'forecast' && !p.cost_ids.length && <><button className="button secondary" onClick={() => setAction('reforecast')}>按当前配置重估</button><button className="button secondary" onClick={() => setAction('waive')}>登记免付</button><button className="button secondary" onClick={() => setAction('void')}>作废预测</button></>}{p.recorded_state !== 'forecast' && <button className="button secondary" onClick={() => setAction('reopen')}>重新开放核对</button>}</>}<button className="text-button" onClick={() => setHistory(true)}>预测修订历史</button><button className="text-button" onClick={() => setEvents(true)}>账期操作历史</button></div>
    <div className="cost-report-grid"><section className="panel cost-report-card"><h2>原账期预测</h2><p className="cost-net">{money(p.amount, p.currency)}</p><p>固定价格下的预计收费，保留独立口径。</p></section><section className="panel cost-report-card"><h2>真实发生费用</h2><p className="cost-net">{p.actual_amount === null ? '尚未登记' : money(p.actual_amount, p.currency)}</p><p>{p.review_current ? '本次费用集合已人工核对。' : '尚无当前有效的人工核对。'}费用冲销后须重新核对；此金额不代表实际付款。</p></section></div>
    <section className="panel party-detail"><h2>账期依据</h2><dl className="file-facts"><div><dt>开支类别</dt><dd>{p.category}</dd></div><div><dt>收费日</dt><dd>{p.start_date} · {p.timezone}</dd></div><div><dt>原核对理由</dt><dd>{p.review_reason ?? '尚未登记'}</dd></div></dl><details><summary>固定版本信息</summary><dl className="file-facts"><div><dt>账期版本</dt><dd>{p.version}</dd></div><div><dt>固定配置修订</dt><dd className="cost-source-key">{p.configuration_revision_id}</dd></div><div><dt>固定价格记录</dt><dd className="cost-source-key">{p.price_id}</dd></div></dl></details></section>
    <section className="panel party-detail"><h2>关联规范费用</h2>{p.cost_ids.length ? p.cost_ids.map(costID => <LinkedCost key={costID} id={costID} />) : <p className="muted">尚无实际费用关联。账期到达不会自动登记扣款。</p>}</section>
    {action && <PeriodAction period={p} action={action} onClose={() => setAction(null)} />}{history && <PeriodHistory period={p} onClose={() => setHistory(false)} />}{events && <PeriodEvents period={p} onClose={() => setEvents(false)} />}
  </>;
}
function LinkedCost({ id }: { id: string }) {
  const query = useQuery({ queryKey: ['cost', id], queryFn: ({ signal }) => api<Cost>('/costs/' + id, { signal }) });
  return <article className="cost-business-link"><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取真实费用" /> : !query.error && query.data && <><Link className="text-button" to={'/costs/' + id}>{query.data.title}</Link><p>{money(query.data.amount, query.data.currency)} · {query.data.state === 'reversed' ? '原费用已冲销' : '当前已确认费用'}</p></>}</article>;
}

function PeriodAction({ period, action, onClose }: { period: BillingPeriod; action: Action; onClose: () => void }) {
  const [fixed] = useState(period), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false), [ack, setAck] = useState(false), [invoice, setInvoice] = useState(false);
  const [files, setFiles] = useState<Schemas['BusinessFileReference'][]>([]), [picker, setPicker] = useState<'cost' | 'file' | null>(null), [cost, setCost] = useState<Cost | null>(null);
  const subscription = useQuery({ queryKey: ['subscription', fixed.subscription_id], queryFn: ({ signal }) => api<Subscription>('/subscriptions/' + fixed.subscription_id, { signal }) });
  const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({ mutationFn: () => {
    let path = '/subscription-periods/' + fixed.id, body: object;
    const base = billingBase(fixed, reason);
    if (action === 'link') { if (!cost) throw new Error('请先选择已确认的准确费用。'); path += '/costs'; body = { ...base, cost_id: cost.id, cost_version: cost.version, cost_revision_id: cost.current_revision_id, cost_content_hash: cost.content_hash } satisfies Schemas['SubscriptionPeriodLink']; }
    else if (action === 'reforecast') { if (!subscription.data) throw new Error('请先载入当前订阅配置。'); path += '/reforecast'; body = { ...base, configuration_revision_id: subscription.data.current_revision_id, configuration_hash: subscription.data.content_hash } satisfies Schemas['SubscriptionPeriodReforecast']; }
    else { path += '/review'; body = { ...base, action, expected_actual_fingerprint: action === 'confirm' ? fixed.actual_fingerprint : null, acknowledge_difference: action === 'confirm' && ack, invoice_missing: action === 'confirm' && invoice, evidence_version_ids: action === 'confirm' ? files.map(f => f.version_id) : [] } satisfies Schemas['SubscriptionPeriodDecision']; }
    return api<BillingPeriod>(path, { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await refreshSubscriptions(cache); onClose(); } });
  const conflict = mutation.error instanceof ApiError && [409, 412].includes(mutation.error.status);
  const difference = fixed.actual_amount !== null && normalizedAmount(fixed.amount) !== normalizedAmount(fixed.actual_amount);
  const disabled = mutation.isPending || conflict || !fresh || !subscription.data || !!subscription.error || action === 'link' && !cost || action === 'confirm' && (!files.length || fixed.actual_amount === null || difference && !ack);
  return <Dialog title={names[action]} onClose={onClose} busy={mutation.isPending}><p><strong>{fixed.product} · {fixed.start_date}</strong></p><p>原预测 {money(fixed.amount, fixed.currency)}；{fixed.actual_amount === null ? '尚无实际费用' : '真实发生 ' + money(fixed.actual_amount, fixed.currency)}。</p><ErrorNotice error={mutation.error ?? subscription.error} />{conflict && <p className="notice warning">费用集合、版本或业务条件已变化。输入已保留，请核对最新事实后重新打开。</p>}
    <form onSubmit={e => { e.preventDefault(); mutation.mutate(); }}><fieldset className="form-fieldset" disabled={mutation.isPending || conflict}>
      {action === 'reforecast' && <p className="notice warning">将按当前订阅配置生成新的预测修订，保留原修订。仅允许日期合格、从未关联实际费用的预测；最终资格由系统核对。</p>}
      {action === 'link' && <section className="form-section"><h3>本次关联费用</h3>{cost ? <><strong>{cost.title}</strong><CostFacts value={cost} /></> : <p>尚未选择费用。</p>}<button type="button" className="button secondary" onClick={() => setPicker('cost')}>选择准确费用</button><p className="form-note">固定当前费用版本。不同供应方、币种、项目范围或已有其他订阅账期归属将被拒绝。</p></section>}
      {action === 'confirm' && <><p className="notice neutral">固定本次看到的费用集合；后续冲销将使本次核对失效。此操作不登记银行付款。</p>{difference && <label className="check-field"><input type="checkbox" checked={ack} onChange={e => setAck(e.target.checked)} />我已核对预计与实际不一致的原因，并在依据中说明。</label>}<label className="check-field"><input type="checkbox" checked={invoice} onChange={e => setInvoice(e.target.checked)} />发票仍待补，保留异常提示。</label><div className="reference-options">{files.map(file => <div className="reference-option" key={file.version_id}><strong>{file.name}</strong><small>固定版本 {file.seq}</small><button type="button" className="text-button" onClick={() => setFiles(files.filter(f => f.version_id !== file.version_id))}>移除此核对凭证</button></div>)}</div><button type="button" className="button secondary" disabled={files.length >= 20} onClick={() => setPicker('file')}>选择固定核对凭证</button></>}
      {(action === 'waive' || action === 'void') && <p className="notice warning">仅处理没有历史费用关联的预测，保留账期及原预测。不能用此操作消除真实费用。</p>}{action === 'reopen' && <p className="notice warning">重新开放核对，保留旧核对及全部费用历史。</p>}
      <Field label="操作依据"><textarea required rows={3} maxLength={200} value={reason} onChange={e => setReason(e.target.value)} /></Field><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button secondary" type="button" onClick={onClose}>取消</button><button className="button primary" disabled={disabled}>{mutation.isPending ? '正在登记' : names[action]}</button></div>
    </fieldset></form>
    {picker === 'cost' && <BillingCostPicker period={fixed} onClose={() => setPicker(null)} onSelect={value => { setCost(value); setPicker(null); }} />}{picker === 'file' && subscription.data && <FileVersionPicker project={subscription.data.project_id} classification={subscription.data.classification} title="选择准确扣款核对凭证" onClose={() => setPicker(null)} onSelect={file => { setFiles(old => old.some(f => f.version_id === file.version_id) ? old : [...old, file]); setPicker(null); }} />}
  </Dialog>;
}
function BillingCostPicker({ period, onClose, onSelect }: { period: BillingPeriod; onClose: () => void; onSelect: (value: Cost) => void }) {
  const [search, setSearch] = useState(''), [q, setQ] = useState('');
  const query = useInfiniteQuery({ queryKey: ['costs', 'subscription-picker', period.id, q], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Cost>>('/costs?' + new URLSearchParams({ state: 'confirmed', currency: period.currency, q, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(p => p.items).filter(c => c.entry_kind === 'expense' && c.counterparty?.party_id === period.supplier.party_id && !c.links.some(l => l.source_type === 'subscription_period' && l.source_id !== period.id)) ?? [];
  return <Dialog title="选择准确订阅费用" onClose={onClose}><form className="reference-search" onSubmit={e => { e.preventDefault(); setQ(search.trim()); }}><Field label="查找费用标题"><input type="search" maxLength={200} value={search} onChange={e => setSearch(e.target.value)} /></Field><button className="button secondary">查找</button></form><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && !items.length ? <Empty title="当前页没有符合条件的费用" detail="须为相同供应方与原币的已确认原费用，且没有其他订阅账期归属。可继续加载或核对费用台账。" /> : items.map(c => <button key={c.id} type="button" className="reference-option" onClick={() => onSelect(c)}><strong>{c.title}</strong><span>{money(c.amount, c.currency)} · 发生日 {c.occurred_on ?? '待补'}</span></button>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多可见费用</button>}</Dialog>;
}

function PeriodHistory({ period, onClose }: { period: BillingPeriod; onClose: () => void }) {
  const query = useInfiniteQuery({ queryKey: ['subscription-period-history', period.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['SubscriptionPeriodRevision']>>('/subscription-periods/' + period.id + '/revisions?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  return <Dialog title="账期预测修订" onClose={onClose}><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && query.data?.pages.flatMap(p => p.items).map(r => <article className="cost-business-link" key={r.id}><strong>预测修订 {r.seq} · {money(r.amount, r.currency)}</strong><p>{r.reason}</p><small>{r.timezone} · {r.category} · {paymentTimestamp(r.created_at)}</small></article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早预测修订</button>}</Dialog>;
}
const eventNames: Record<Schemas['SubscriptionPeriodEvent']['kind'], string> = { generated: '生成持久预测', unexpected: '登记停订后异常账期', reforecast: '显式重估预测', cost_linked: '关联真实费用', confirmed: '人工核对费用集合', waived: '登记免付', void: '作废预测', reopened: '重新开放核对' };
function PeriodEvents({ period, onClose }: { period: BillingPeriod; onClose: () => void }) {
  const query = useInfiniteQuery({ queryKey: ['subscription-period-events', period.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['SubscriptionPeriodEvent']>>('/subscription-periods/' + period.id + '/events?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  return <Dialog title="账期操作历史" onClose={onClose}><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && query.data?.pages.flatMap(p => p.items).map(e => <article className="cost-business-link" key={e.id}><strong>{eventNames[e.kind]}</strong><p>{e.reason}</p><small>{e.job_id ? '后台持久任务' : '人工操作'} · 版本 {e.version} · {paymentTimestamp(e.created_at)}</small>{e.kind === 'confirmed' && <><p>{e.acknowledge_difference ? '已人工确认金额差异。' : '未登记差异确认。'}{e.invoice_missing ? '发票仍待补。' : ''}</p><h3>当时固定的费用依据</h3>{e.cost_snapshot.map(c => <div className="billing-cost-snapshot" key={c.cost_id}><Link className="text-button" to={'/costs/' + c.cost_id} onClick={onClose}>查阅对应费用</Link><span>{money(c.amount, c.currency)} · {c.state === 'reversed' ? '当时已冲销' : '当时已确认'}</span><small className="cost-source-key">固定修订 {c.revision_id}</small></div>)}<FixedBillingFiles ids={e.evidence_version_ids} files={e.attachments} /></>}</article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早账期操作</button>}</Dialog>;
}
