import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api, type Me, type Page, type Schemas } from './api';
import { Busy, Dialog, ErrorNotice } from './components';
import { money } from './ContractDisplay';
import { paymentTimestamp } from './PaymentDisplay';
import { FixedBillingFiles } from './SubscriptionDisplay';
import { ContractFinanceActionDialog, type ContractFinanceAction } from './ContractFinanceActions';

const reviewNames: Record<Schemas['ContractFinanceNode']['review_state'], string> = { unreviewed: '尚未人工核对', current: '当前依据已核对', stale: '来源已变化，须重新核对', restricted: '核对情况不可查阅' };
const sourceStateNames: Record<string, string> = { confirmed: '已确认发生', recorded: '已登记实际资金', reversed: '原记录已冲销' };
const eventNames: Record<Schemas['ContractFinanceEvent']['kind'], string> = { cost: '归属真实费用', cash: '归属实际收付款', refund: '归属实际退回款', release: '解除部分归属', review: '人工核对节点' };
export function ContractFinance({ contract, me }: { contract: Schemas['Contract']; me: Me }) {
  const financialStage = ['effective', 'completed', 'terminated'].includes(contract.lifecycle);
  const query = useQuery({ queryKey: ['contract-finance', contract.id], queryFn: ({ signal }) => api<Schemas['ContractFinance']>('/contracts/' + contract.id + '/finance', { signal }), enabled: financialStage });
  const [action, setAction] = useState<{ node: Schemas['ContractFinanceNode']; kind: ContractFinanceAction; event?: Schemas['ContractFinanceEvent'] } | null>(null), [history, setHistory] = useState<Schemas['ContractFinanceNode'] | null>(null);
  if (!financialStage) return null;
  const view = query.error ? undefined : query.data;
  return <section className="panel party-detail contract-finance"><div className="section-heading"><h2>合同财务归属</h2><span className="muted">固定计划与实际事实分别核对</span></div><p className="form-note">归属用于解释既有费用和资金。净收付已扣除本节点归属退回款；核对有效性与合同法律阶段分别记录。</p><ErrorNotice error={query.error} />
    {query.isPending ? <Busy label="正在读取合同财务归属" /> : view && (!view.nodes.length ? <p className="muted">此合同没有固定收付款节点。</p> : <div className="contract-finance-nodes">{view.nodes.map(node => <article className="contract-finance-node" key={node.id} aria-label={node.title + '财务归属'}>
      <div className="section-heading"><h3>{node.title}</h3><span className={'badge ' + (node.review_state === 'stale' ? 'warning' : 'neutral')}>{reviewNames[node.review_state]}</span></div>
      <dl className="file-facts"><div><dt>{node.direction === 'payable' ? '计划应付' : '计划应收'}</dt><dd>{money(node.amount, node.currency)}</dd></div><div><dt>已归属真实费用</dt><dd>{node.cost_visibility === 'not_applicable' ? '应收节点不登记支出费用' : node.cost_amount === null ? '当前不可查阅' : money(node.cost_amount, node.currency)}</dd></div><div><dt>已归属净收付</dt><dd>{node.cash_amount === null ? '当前不可查阅' : money(node.cash_amount, node.currency)}</dd></div><div><dt>与计划尚差 / 超过计划</dt><dd>{node.outstanding_amount === null ? '当前不可查阅' : money(node.outstanding_amount, node.currency) + ' / ' + money(node.excess_amount, node.currency)}</dd></div><div><dt>约定条件核实</dt><dd>{node.conditions_satisfied === null ? '尚无可查阅核实结果' : (node.conditions_satisfied ? '已核实满足' : '仍未确认全部满足') + (node.review_state === 'stale' ? '（须重新核对）' : '')}</dd></div></dl>
      {node.has_unattributed_refunds && <p className="notice warning">原付款存在尚未归属的实际退款，请核实它对本节点的影响。</p>}
      <div className="post-actions">{view.can_reconcile && view.history_visibility === 'complete' && <>{node.direction === 'payable' && me.actions.includes('cost.record') && <button className="button secondary" onClick={() => setAction({ node, kind: 'cost' })}>归属真实费用</button>}{me.actions.includes('payment.record') && <><button className="button secondary" onClick={() => setAction({ node, kind: 'cash' })}>归属实际收付款</button><button className="button secondary" onClick={() => setAction({ node, kind: 'refund' })}>归属实际退回款</button></>}{node.can_review && <button className="button primary" onClick={() => setAction({ node, kind: 'review' })}>核对节点财务</button>}</>}{view.history_visibility === 'complete' && <button className="text-button" onClick={() => setHistory(node)}>查看节点归属历史</button>}</div>
    </article>)}</div>)}
    {view && action && <ContractFinanceActionDialog contract={contract} view={view} node={action.node} action={action.kind} event={action.event} onClose={() => setAction(null)} />}
    {view && history && <ContractFinanceHistory contractID={contract.id} node={history} onClose={() => setHistory(null)} onRelease={event => setAction({ node: history, kind: 'release', event })} />}
  </section>;
}

function ContractFinanceHistory({ contractID, node, onClose, onRelease }: { contractID: string; node: Schemas['ContractFinanceNode']; onClose: () => void; onRelease: (event: Schemas['ContractFinanceEvent']) => void }) {
  const query = useInfiniteQuery({ queryKey: ['contract-finance-events', contractID, node.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['ContractFinanceEvent']>>('/contracts/' + contractID + '/finance/events?' + new URLSearchParams({ milestone_id: node.id, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  return <Dialog title="节点归属与核对历史" onClose={onClose}><p><strong>{node.title}</strong> · 历史保留操作时依据，当前有效金额随来源冲销及解除重新计算。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && query.data?.pages.flatMap(p => p.items).map(event => <article className="cost-business-link" key={event.id}>
    <div className="section-heading"><h3>{eventNames[event.kind]} · 记录 {event.seq}</h3>{event.amount !== null && <strong>{money(event.amount, node.currency)}</strong>}</div><p>{event.reason}</p><small>{event.actor_name} · {paymentTimestamp(event.created_at)}</small>
    {event.cost_id && <Link className="text-button" to={'/costs/' + event.cost_id} onClick={onClose}>核对来源费用</Link>}{event.payment_id && <Link className="text-button" to={'/payments/' + event.payment_id} onClick={onClose}>核对来源资金</Link>}
    {event.return_cash && <ReturnCash value={event.return_cash} />}
    {event.refund_cash && <p className="notice">整笔退款实收 {money(event.refund_cash.amount, event.refund_cash.currency)}；原本金 {money(event.refund_cash.refund_fx.original_amount, event.refund_cash.refund_fx.original_currency)}。本条归属／解除只使用 {node.currency} 本金。汇率依据：{event.refund_cash.refund_fx.source}</p>}
    {event.remaining_amount !== null && <p>尚未解除 {money(event.remaining_amount, node.currency)} · 当前有效 {money(event.effective_amount, node.currency)}</p>}
    {event.can_release && <button className="button secondary" onClick={() => onRelease(event)}>解除部分归属</button>}
    {event.review_snapshot && <><p>{event.conditions_satisfied ? '当时已确认条件满足。' : '当时尚未确认条件全部满足。'}{event.acknowledge_difference ? '已登记差额核对。' : ''}{event.acknowledge_unattributed_refunds ? '已核实未归属退款影响。' : ''}</p><details><summary>查阅当时固定的财务依据</summary>{event.review_snapshot.sources.length ? event.review_snapshot.sources.map(source => <div className="billing-cost-snapshot" key={source.event_id}><strong>{eventNames[source.kind]} · {money(source.effective_amount, source.attribution_currency ?? source.currency)}</strong><span>当时状态 {sourceStateNames[source.state] ?? '当前无法解释的状态'} · 尚未解除 {money(source.remaining_amount, source.attribution_currency ?? source.currency)}</span>{source.return_cash && <ReturnCash value={source.return_cash} />}{source.refund_cash && <span>整笔实收 {money(source.refund_cash.amount, source.refund_cash.currency)} · 归属按 {source.attribution_currency} 本金</span>}<small className="cost-source-key">固定来源修订 {source.current_revision_id}</small></div>) : <p>当时没有来源归属。</p>}</details><FixedBillingFiles ids={event.attachments.map(file => file.version_id)} files={event.attachments} /></>}
  </article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早节点归属</button>}</Dialog>;
}

function ReturnCash({ value: v }: { value: Schemas['ContractReturnCash'] }) {
  return <p className="notice">本条实际付出归属／解除 {money(v.amount, v.currency)}；尚占用 {v.remaining_amount === null ? '见原归属' : money(v.remaining_amount, v.currency)}。整笔资金 {money(v.whole_amount, v.currency)} · {v.paid_on}。原归属合同本金 {money(v.original_amount, v.original_currency)}；每1本金兑换 {v.rate} {v.currency} · {v.source}</p>;
}
