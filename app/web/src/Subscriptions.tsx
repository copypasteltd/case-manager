import { useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { ArrowLeft, ArrowUpRight, Plus } from 'lucide-react';
import { api, type Me, type Page, type Schemas } from './api';
import { Busy, classificationNames, Dialog, Empty, ErrorNotice, Field } from './components';
import { money } from './ContractDisplay';
import { useCostProjects } from './CostDisplay';
import { paymentTimestamp } from './PaymentDisplay';
import { BillingBadge, BillingSignals, SubscriptionBadge, SubscriptionFacts, periodStates, subscriptionStates, type Subscription, type BillingPeriod } from './SubscriptionDisplay';
import { SubscriptionEditor } from './SubscriptionEditor';
import { ProjectBusinessReturn, ProjectBusinessScopeNotice, useProjectBusinessScope } from './ProjectBusinessScope';
import { SubscriptionAction, subscriptionActionNames, type SubscriptionActionKind } from './SubscriptionActions';

export function SubscriptionsPage({ me }: { me: Me }) {
  const [search, setSearch] = useState(''), [q, setQ] = useState(''), [state, setState] = useState('');
  const business = useProjectBusinessScope('subscriptions', me), [params, setParams] = useSearchParams();
  const scope = business.selection.projectID ?? (business.selection.companyOnly ? 'company' : '');
  function setScope(value: string) { const next = new URLSearchParams(params); next.delete('project_id'); next.delete('company_only'); next.delete('create'); if (value === 'company') next.set('company_only', 'true'); else if (value) next.set('project_id', value); setParams(next); }
  const projects = useCostProjects();
  const query = useInfiniteQuery({ queryKey: ['subscriptions', q, state, scope], enabled: me.actions.includes('subscription.read') && business.ready, initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Subscription>>('/subscriptions?' + new URLSearchParams({ q, state, limit: '20', ...business.filters, ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(p => p.items) ?? [];
  if (!business.ready) return <ProjectBusinessScopeNotice scope={business} />;
  if (!me.actions.includes('subscription.read')) return <Empty title="没有订阅业务权限" detail="请联系业务负责人核对授权。" />;
  return <><div className="page-heading"><div><h1>订阅与续费</h1><p>核对服务、计费日历、价格变化及供应方真实状态。</p></div>{business.canCreate && <button className="button primary" onClick={business.open}><Plus size={18} />新建订阅</button>}</div>
    <ProjectBusinessScopeNotice scope={business} /><div className="post-actions"><Link className="text-button" to="/subscription-periods">查阅账期与扣款<ArrowUpRight size={16} /></Link>{me.actions.includes('report.read') && <Link className="text-button" to="/reports/forecast">月度预测<ArrowUpRight size={16} /></Link>}</div>
    <form className="party-filters" onSubmit={e => { e.preventDefault(); setQ(search.trim()); }}><Field label="产品名称"><input type="search" maxLength={200} value={search} onChange={e => setSearch(e.target.value)} /></Field><Field label="内部状态"><select value={state} onChange={e => setState(e.target.value)}><option value="">全部状态</option>{Object.entries(subscriptionStates).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></Field><Field label="订阅范围"><select value={scope} onChange={e => setScope(e.target.value)}><option value="">全部可见订阅</option><option value="company">仅公司范围</option>{business.project && !projects.data?.pages.some(page => page.items.some(p => p.id === business.project!.id)) && <option value={business.project.id}>{business.project.name}</option>}{projects.data?.pages.flatMap(p => p.items).map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select></Field><button className="button secondary">查找订阅</button>{projects.hasNextPage && <button type="button" className="text-button" disabled={projects.isFetchingNextPage} onClick={() => void projects.fetchNextPage()}>加载更多项目</button>}</form>
    <ErrorNotice error={query.error ?? projects.error} />{query.isPending ? <Busy /> : !query.error && !items.length ? <Empty title={q || state || scope ? '当前筛选没有可见订阅' : '尚无可见订阅'} detail="可调整筛选；有权限时可先登记订阅草稿，未知信息留待核实。" /> : <div className="contract-list">{items.map(s => <Link key={s.id} className="contract-row" to={'/subscriptions/' + s.id}><div><strong>{s.product}</strong><small>{s.supplier_evidence?.name ?? '供应方待核实'} · {s.owner_display_name ?? '负责人待补'}</small><small>{s.next_charge_date ? `下次收费日 ${s.next_charge_date} · ${s.timezone}` : '无已知下次收费日'}</small><BillingSignals values={s.signals} /></div><SubscriptionBadge value={s} /><ArrowUpRight size={17} /></Link>)}</div>}
    {query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多订阅</button>}{business.draft && <SubscriptionEditor initialProject={business.draft.project} onClose={business.close} />}
  </>;
}

export function SubscriptionDetail({ me }: { me: Me }) {
  const { id } = useParams();
  const query = useQuery({ queryKey: ['subscription', id], enabled: me.actions.includes('subscription.read'), queryFn: ({ signal }) => api<Subscription>('/subscriptions/' + id, { signal }) });
  const [edit, setEdit] = useState(false), [history, setHistory] = useState(false), [events, setEvents] = useState(false), [action, setAction] = useState<SubscriptionActionKind | null>(null);
  if (!me.actions.includes('subscription.read')) return <Empty title="没有订阅业务权限" detail="请联系业务负责人核对授权。" />;
  if (query.error) return <ErrorNotice error={query.error} />;
  if (!query.data) return <Busy />;
  const s = query.data;
  const actions: SubscriptionActionKind[] = s.state === 'archived' ? ['restore'] : s.state === 'draft' ? ['activate', 'cancel'] : s.state === 'active' ? ['price', 'generate', 'pause', 'cancel', 'verify_vendor'] : s.state === 'paused' ? ['price', 'resume', 'cancel', 'verify_vendor', 'unexpected'] : ['generate', 'verify_vendor', 'unexpected', 'archive'];
  return <><Link className="back-link" to="/subscriptions"><ArrowLeft size={16} />订阅与续费</Link><ProjectBusinessReturn projectID={s.project_id} kind="subscriptions" /><div className="page-heading"><div><h1>{s.product}</h1><p>{classificationNames[s.classification]} · {s.project_id ? '项目订阅' : '公司订阅'} · 当前版本 {s.version}</p></div><SubscriptionBadge value={s} /></div>
    <BillingSignals values={s.signals} /><div className="post-actions">{s.can_edit && <button className="button secondary" onClick={() => setEdit(true)}>修订订阅资料</button>}{s.can_manage && actions.map(kind => <button className="button secondary" key={kind} onClick={() => setAction(kind)}>{subscriptionActionNames[kind]}</button>)}<button className="text-button" onClick={() => setHistory(true)}>配置历史</button><button className="text-button" onClick={() => setEvents(true)}>操作历史</button></div>
    <section className="panel party-detail"><h2>订阅资料</h2><dl className="file-facts"><div><dt>下次已知收费日</dt><dd>{s.next_charge_date ?? '没有已知下次收费日'}</dd></div><div><dt>内部取消生效日</dt><dd>{s.cancel_effective_on ?? '尚未登记取消'}</dd></div><div><dt>最近暂停日</dt><dd>{s.paused_on ?? '当前未暂停'}</dd></div></dl><SubscriptionFacts value={s} /></section>
    <BillingPeriodList subscriptionID={s.id} /><SubscriptionReminders subscription={s} />
    {edit && <SubscriptionEditor subscription={s} onClose={() => setEdit(false)} />}{action && <SubscriptionAction subscription={s} action={action} onClose={() => setAction(null)} />}{history && <SubscriptionHistory subscription={s} onClose={() => setHistory(false)} />}{events && <SubscriptionEvents subscription={s} onClose={() => setEvents(false)} />}
  </>;
}

export function BillingPeriodsPage({ me }: { me: Me }) {
  if (!me.actions.includes('subscription.read')) return <Empty title="没有账期业务权限" detail="请联系业务负责人核对授权。" />;
  return <><div className="page-heading"><div><h1>账期与扣款核对</h1><p>保留原始预测，关联真实费用并核对差异。</p></div><Link className="button secondary" to="/subscriptions">管理订阅<ArrowUpRight size={17} /></Link></div><BillingPeriodList /></>;
}
export function BillingPeriodList({ subscriptionID }: { subscriptionID?: string }) {
  const [params] = useSearchParams(), [state, setState] = useState(params.get('state') ?? ''), [from, setFrom] = useState(params.get('from') ?? ''), [to, setTo] = useState(params.get('to') ?? ''), [applied, setApplied] = useState({ from, to });
  const fixed = Object.fromEntries(['currency', 'category', 'project_id', 'company_only', 'supplier_id', 'owner_id', 'forecast_only'].flatMap(key => params.has(key) ? [[key, params.get(key)!]] : []));
  const query = useInfiniteQuery({ queryKey: ['subscription-periods', subscriptionID, state, applied, fixed], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<BillingPeriod>>((subscriptionID ? '/subscriptions/' + subscriptionID + '/periods?' : '/subscription-periods?') + new URLSearchParams({ ...fixed, state, limit: '20', ...(applied.from ? { from: applied.from } : {}), ...(applied.to ? { to: applied.to } : {}), ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(p => p.items) ?? [];
  return <section className="panel party-detail"><h2>持久账期</h2><p className="form-note">日期按各订阅的计费时区。预计、真实发生与付款分别保留。</p><form className="party-filters" onSubmit={e => { e.preventDefault(); setApplied({ from, to }); }}><Field label="收费日从"><input type="date" value={from} onChange={e => setFrom(e.target.value)} /></Field><Field label="收费日至（不含）"><input type="date" value={to} onChange={e => setTo(e.target.value)} /></Field><Field label="核对状态"><select value={state} onChange={e => setState(e.target.value)}><option value="">全部状态</option>{Object.entries(periodStates).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></Field><button className="button secondary">查询账期</button></form><ErrorNotice error={query.error} />
    {query.isPending ? <Busy /> : !query.error && !items.length ? <p className="muted">当前筛选没有已生成账期。请核对日期范围、已启用配置及价格覆盖。</p> : <div className="contract-list">{items.map(p => <Link key={p.id} className="contract-row billing-period-row" to={'/subscription-periods/' + p.id}><div><strong>{p.product} · {p.start_date}</strong><small>至 {p.end_date} 前 · {p.timezone}</small><small>预计 {money(p.amount, p.currency)} · {p.actual_amount === null ? '尚无实际费用' : '真实发生 ' + money(p.actual_amount, p.currency)}</small><BillingSignals values={p.anomalies} /></div><BillingBadge value={p} /><ArrowUpRight size={17} /></Link>)}</div>}
    {query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多账期</button>}
  </section>;
}

function SubscriptionHistory({ subscription, onClose }: { subscription: Subscription; onClose: () => void }) {
  const [selected, setSelected] = useState<string | null>(null);
  const query = useInfiniteQuery({ queryKey: ['subscription-history', subscription.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['SubscriptionRevision']>>('/subscriptions/' + subscription.id + '/revisions?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const detail = useQuery({ queryKey: ['subscription-history', subscription.id, selected], enabled: !!selected, queryFn: ({ signal }) => api<Schemas['SubscriptionRevision']>('/subscriptions/' + subscription.id + '/revisions/' + selected, { signal }) });
  return <Dialog title="订阅配置历史" onClose={onClose}><ErrorNotice error={query.error ?? detail.error} />{query.isPending ? <Busy /> : !query.error && query.data?.pages.flatMap(p => p.items).map(r => <button className="reference-option" key={r.id} onClick={() => setSelected(r.id)}><strong>配置修订 {r.seq} · {r.product}</strong><span>{r.reason} · {paymentTimestamp(r.created_at)}</span></button>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早配置</button>}{selected && (detail.isPending ? <Busy /> : !detail.error && detail.data && <section className="cost-history-detail"><h3>固定历史配置 {detail.data.seq}</h3><SubscriptionFacts value={detail.data} /></section>)}</Dialog>;
}
const eventNames: Record<Schemas['SubscriptionEvent']['kind'], string> = { draft_created: '登记订阅草稿', config_revised: '修订或核实配置', price_changed: '登记新价格', activated: '启用内部管理', paused: '暂停后续预测', resumed: '恢复后续预测', cancelled: '登记内部取消', archived: '归档订阅', restored: '恢复历史核对', periods_generated: '生成持久账期', period_changed: '账期核对或费用关联' };
function SubscriptionEvents({ subscription, onClose }: { subscription: Subscription; onClose: () => void }) {
  const query = useInfiniteQuery({ queryKey: ['subscription-events', subscription.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['SubscriptionEvent']>>('/subscriptions/' + subscription.id + '/events?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  return <Dialog title="订阅操作历史" onClose={onClose}><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && query.data?.pages.flatMap(p => p.items).map(e => <article className="cost-business-link" key={e.id}><strong>{eventNames[e.kind]}</strong><p>{e.reason}</p><small>{e.job_id ? '后台持久任务' : '人工操作'} · 版本 {e.version} · {paymentTimestamp(e.created_at)}{e.effective_on ? ' · 生效日 ' + e.effective_on : ''}</small>{e.period_id && <Link className="text-button" to={'/subscription-periods/' + e.period_id} onClick={onClose}>查阅对应账期</Link>}</article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早操作</button>}</Dialog>;
}
function SubscriptionReminders({ subscription }: { subscription: Subscription }) {
  const query = useInfiniteQuery({ queryKey: ['subscription-reminders', subscription.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['SubscriptionReminder']>>('/subscriptions/' + subscription.id + '/reminders?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(p => p.items) ?? [];
  return <section className="panel party-detail"><h2>提醒与投递记录</h2><p className="form-note">这里只记录进入内部提醒链路的事实，投递状态取自站内通知记录，送达并不代表已读。补排的过期提醒保留原计划日期；不会据此登记付款。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && !items.length ? <p className="muted">尚无持久提醒记录。请核对提醒配置及后台调度。</p> : items.map(n => <article key={n.id} className="cost-business-link"><div className="section-heading"><Link className="text-button" to={'/subscription-periods/' + n.period_id}>{n.period_start_date} 账期</Link><span className="badge neutral">{{ queued: '已排队', delivered: '已送达站内通知', suppressed: '已按当前职责或权限抑制', retry_wait: '等待重试', failed: '投递失败' }[n.delivery_state]}</span></div><p>{n.days_before === 0 ? '计费当日' : `提前 ${n.days_before} 日`} · 原提醒日 {n.remind_on} · {n.timezone}</p><small>责任人 {n.recipient_display_name}（当前显示名） · 排队于 {paymentTimestamp(n.created_at)}</small></article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早提醒</button>}</section>;
}
