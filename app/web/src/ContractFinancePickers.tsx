import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { api, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field } from './components';
import { money } from './ContractDisplay';
import { paymentTimestamp, type Payment } from './PaymentDisplay';
import type { Cost } from './CostDisplay';

export type FinanceSource = { type: 'cost'; value: Cost } | { type: 'cash'; value: Payment };
export const isZeroAmount = (value: string | null) => value !== null && /^0(?:\.0+)?$/.test(value);
export function ContractFinanceSourcePicker({ contract, node, kind, original, onSelect, onClose }: {
  contract: Schemas['Contract']; node: Schemas['ContractFinanceNode']; kind: 'cost' | 'cash' | 'refund';
  original: Schemas['ContractFinanceEvent'] | null; onSelect: (source: FinanceSource) => void; onClose: () => void;
}) {
  const [search, setSearch] = useState(''), [q, setQ] = useState('');
  const direction = kind === 'refund' ? node.direction === 'payable' ? 'refund' : 'outgoing' : node.direction === 'payable' ? 'outgoing' : 'incoming';
  const query = useInfiniteQuery({ queryKey: ['contract-finance-source-picker', contract.id, kind, original?.id, q], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Cost | Payment>>((kind === 'cost' ? '/costs?' : '/payments?') + new URLSearchParams({ state: kind === 'cost' ? 'confirmed' : 'recorded', currency: node.currency, q, limit: '20', ...(kind !== 'cost' ? { direction } : {}), ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(p => p.items).filter(value => {
    if ('entry_kind' in value) return value.entry_kind === 'expense' && value.links.some(link => link.source_type === 'contract' && link.source_id === contract.id && link.source_revision_id === contract.active_revision_id);
    return value.record_kind === 'payment' && !value.employee_id && value.project_id === contract.project_id && value.counterparty?.party_id === contract.counterparty_id && (kind !== 'refund' || node.direction !== 'payable' || value.refund_of_id === original?.payment_id);
  }) ?? [];
  return <Dialog title="选择准确合同财务来源" onClose={onClose}>
    <p className="form-note">{kind === 'cost' ? '选择已确认、且已关联本合同生效修订的费用。可先在费用详情登记合同依据。' : kind === 'refund' ? '选择已经发生、与原资金相符的退回记录。' : '选择相同项目、往来方及币种的已登记实际资金。'}仅列出有权查阅的资料。</p>
    <form className="reference-search" onSubmit={e => { e.preventDefault(); setQ(search.trim()); }}><Field label="查找财务来源标题"><input type="search" maxLength={200} value={search} onChange={e => setSearch(e.target.value)} /></Field><button className="button secondary">查找</button></form>
    <ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && !items.length ? <Empty title="当前页没有符合条件的来源" detail="可继续加载或先在费用、收付款台账核对真实记录。" /> : <div className="reference-options">{items.map(value => <button className="reference-option" key={value.id} type="button" onClick={() => onSelect('entry_kind' in value ? { type: 'cost', value } : { type: 'cash', value })}><strong>{value.title}</strong><span>{money(value.amount, value.currency)}</span><small>{'occurred_on' in value ? '费用发生日 ' + value.occurred_on : value.paid_at ? '实际资金 ' + paymentTimestamp(value.paid_at) : ''}</small></button>)}</div>}
    {query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多财务来源</button>}
  </Dialog>;
}

export function ContractCashRootPicker({ contractID, node, onSelect, onClose }: { contractID: string; node: Schemas['ContractFinanceNode']; onSelect: (event: Schemas['ContractFinanceEvent']) => void; onClose: () => void }) {
  const query = useInfiniteQuery({ queryKey: ['contract-finance-events', contractID, node.id, 'cash-roots'], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Schemas['ContractFinanceEvent']>>('/contracts/' + contractID + '/finance/events?' + new URLSearchParams({ milestone_id: node.id, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(p => p.items).filter(event => event.kind === 'cash' && !isZeroAmount(event.remaining_amount) && !isZeroAmount(event.effective_amount)) ?? [];
  return <Dialog title="选择原正向资金归属" onClose={onClose}><p>固定本节点的原正向收付款归属，再选择实际退回资金。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && !items.length ? <p className="muted">当前页没有有效正向资金归属。</p> : items.map(event => <button className="reference-option" type="button" key={event.id} onClick={() => onSelect(event)}><strong>归属记录 {event.seq} · {money(event.remaining_amount, node.currency)}</strong><span>{event.reason}</span><small>{paymentTimestamp(event.created_at)} · {event.actor_name}</small></button>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早资金归属</button>}</Dialog>;
}
