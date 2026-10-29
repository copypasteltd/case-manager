import { useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { allocationFXReceipt, inversePaymentFX, inversePaymentFXLabel, reviewedPaymentFX } from '@copypaste/contracts/payment-fx';
import { api, ApiError, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field } from './components';
import { useCommandKey } from './BusinessSelectors';
import { CostFacts, type Cost } from './CostDisplay';
import { money } from './ContractDisplay';
import { ApprovedClaimPicker } from './PaymentEditor';
import { fromHongKongInput, paymentBase, PaymentFacts, refreshPayments, type Payment } from './PaymentDisplay';
import { MFAConfirmation } from './MFAConfirmation';

type Allocation = Schemas['PaymentAllocation'];
export type PaymentActionKind = 'record' | 'reverse' | 'allocate' | 'release' | 'refundAllocate';
export function PaymentAction({ payment, action, allocation, onClose }: { payment: Payment; action: PaymentActionKind; allocation?: Allocation; onClose: () => void }) {
  const [fixed] = useState(payment), [amount, setAmount] = useState(''), [reason, setReason] = useState(''), [effective, setEffective] = useState(''), [fresh, setFresh] = useState(false);
  const [cost, setCost] = useState<Cost | null>(null), [claim, setClaim] = useState<Schemas['Spend'] | null>(null), [line, setLine] = useState(''), [picker, setPicker] = useState<'cost' | 'claim' | null>(null);
  const [plans, setPlans] = useState<Record<string, string>>({}), [refundSource, setRefundSource] = useState('');
  const [costAmount, setCostAmount] = useState(''), [rate, setRate] = useState(''), [quoteSource, setQuoteSource] = useState(''), [reviewed, setReviewed] = useState(false);
  const cache = useQueryClient(), key = useCommandKey(), isRefund = fixed.direction === 'refund', cross = !!cost && cost.currency !== fixed.currency;
  const titles: Record<PaymentActionKind, string> = { record: '登记实际资金', reverse: '全额冲销资金', allocate: '分配费用结算', release: '释放费用分配', refundAllocate: '关联退款分配' };
  const original = useQuery({ queryKey: ['payment', fixed.refund_of_id], enabled: !!fixed.refund_of_id && (action === 'record' || action === 'refundAllocate'), queryFn: ({ signal }) => api<Payment>('/payments/' + fixed.refund_of_id, { signal }) });
  const sources = useInfiniteQuery({ queryKey: ['payment-allocations', fixed.refund_of_id, 'refund-picker'], enabled: !!fixed.refund_of_id && (action === 'record' || action === 'refundAllocate'), initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Allocation>>('/payments/' + fixed.refund_of_id + '/allocations?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const sourceItems = sources.error ? [] : sources.data?.pages.flatMap(page => page.items).filter(item => item.kind === 'allocation' && !/^0(\.0+)?$/.test(item.remaining_amount)) ?? [];
  const inverse = action === 'release' ? allocation : sourceItems.find(v => v.id === refundSource);
  const crossInverse = action === 'record' ? sourceItems.some(v => plans[v.id] && v.fx) : !!inverse?.fx;
  function resetReview() { setReviewed(false); }
  const mutation = useMutation({ mutationFn: async () => {
    const base = paymentBase(fixed, reason);
    if ((action === 'allocate' && cross || crossInverse) && !reviewed) throw new Error('请明确核对本次双币金额和汇率依据。');
    const fx = action === 'allocate' && cost ? reviewedPaymentFX(fixed, cost, amount, cross ? { cost_amount: costAmount, rate, source: quoteSource, acknowledged: true } : undefined) : undefined;
    const refundPlans = Object.entries(plans).filter(([, value]) => value !== '').map(([source_allocation_id, value]) => {
      const source = sourceItems.find(v => v.id === source_allocation_id); if (!source) throw new Error('请重新读取所选原分配。');
      return { source_allocation_id, amount: value, ...inversePaymentFX(source, value) };
    });
    const inverseAmount = inverse && ['release', 'refundAllocate'].includes(action) ? inversePaymentFX(inverse, amount) : {};
    const body = action === 'record' ? { ...base, expected_source_version: isRefund ? original.data?.version ?? null : null, refund_allocations: refundPlans }
      : action === 'reverse' ? { ...base, effective_at: fromHongKongInput(effective) }
      : action === 'allocate' && cost ? { ...base, amount, cost_id: cost.id, cost_version: cost.version, cost_revision_id: cost.current_revision_id, cost_content_hash: cost.content_hash, claim_id: claim?.id ?? null, claim_revision_id: claim?.approved_revision_id ?? null, claim_line_id: line || null, ...(fx ? { fx } : {}) }
      : action === 'refundAllocate' ? { ...base, source_allocation_id: refundSource, amount, ...inverseAmount } : { ...base, amount, ...inverseAmount };
    const route = action === 'record' ? 'record' : action === 'reverse' ? 'reverse' : action === 'allocate' ? 'allocations' : action === 'refundAllocate' ? 'refund-allocations' : 'allocations/' + allocation?.id + '/release';
    const result = await api<Payment>('/payments/' + fixed.id + '/' + route, { method: 'POST', body, key: key(body) });
    if (action === 'allocate' || inverse || action === 'record' && refundPlans.length) {
      const rows = (await api<Page<Allocation>>('/payments/' + fixed.id + '/allocations?limit=100')).items;
      const matches = (row: Allocation) => row.payment_id === fixed.id && row.reason === reason;
      if (action === 'allocate' && !rows.some(row => matches(row) && row.kind === 'allocation' && row.cost_id === cost?.id && row.cost_revision_id === cost.current_revision_id && allocationFXReceipt(row, { cash: amount, cost: fx?.cost_amount ?? amount, fx, paymentCurrency: fixed.currency, costCurrency: cost.currency, quotedOn: fixed.paid_on }))) throw new Error('核销回执与已核对双币依据不一致，请重新查阅流水。');
      if (inverse && !rows.some(row => matches(row) && row.source_allocation_id === inverse.id && row.kind === (action === 'release' ? 'release' : 'refund') && allocationFXReceipt(row, { cash: amount, cost: inverseAmount.cost_amount ?? amount, source: inverse }))) throw new Error('回转回执与已核对双币依据不一致，请重新查阅流水。');
      if (action === 'record' && !refundPlans.every(plan => rows.some(row => matches(row) && row.kind === 'refund' && row.source_allocation_id === plan.source_allocation_id && allocationFXReceipt(row, { cash: plan.amount, cost: plan.cost_amount ?? plan.amount, source: sourceItems.find(v => v.id === plan.source_allocation_id) })))) throw new Error('退款分配回执需要重新核对。');
    }
    return result;
  }, onSuccess: async () => { await refreshPayments(cache); onClose(); } });
  const blocked = mutation.error instanceof ApiError && [409, 412].includes(mutation.error.status);
  const disabled = !fresh || (action === 'allocate' && (!cost || (!!fixed.employee_id && (!claim || !line)))) || (action === 'record' && isRefund && (!original.data || !!original.error)) || (action === 'refundAllocate' && !refundSource) || ((action === 'allocate' && cross || crossInverse) && !reviewed);
  return <Dialog title={titles[action]} onClose={onClose}>
    <p className="notice neutral">{action === 'record' ? '登记已经发生的真实资金，不发起银行转账。退款币种保持与原付款一致。' : action === 'reverse' ? '新增全额反向资金与双币分配记录，保留原始日期和依据。实际收到退款请单独登记。' : action === 'release' ? '保留实际付款，追加反向分配。按原分配当前双币余额回转，最后一笔处理完原币尾差。' : '付款币核对资金余额，费用原币核对费用及报销余额；跨币种须人工固定实际结算汇率与凭证来源。'}</p>
    <ErrorNotice error={mutation.error ?? original.error ?? sources.error} />{blocked && <p className="notice warning">输入已保留。请关闭并核对最新资金、费用与双币结算余额。</p>}
    <form onSubmit={e => { e.preventDefault(); mutation.mutate(); }}><fieldset className="form-fieldset" disabled={mutation.isPending || blocked}>
      {action === 'record' && <PaymentFacts value={fixed} />}
      {action === 'reverse' && <Field label="冲销生效时间（香港）"><input required type="datetime-local" step="1" value={effective} onChange={e => setEffective(e.target.value)} /></Field>}
      {action === 'allocate' && <section className="form-section"><h3>准确费用依据</h3>
        {fixed.employee_id && <><p>{claim?.title ?? '先选择该收款员工的已批准报销'}</p><button type="button" className="button secondary" onClick={() => setPicker('claim')}>选择员工垫付依据</button>{claim && <Field label="批准垫付明细"><select required value={line} onChange={e => { setLine(e.target.value); setCost(null); resetReview(); }}><option value="">选择具体垫付明细</option>{claim.lines.filter(l => l.funding === 'employee_advance').map(l => <option key={l.id} value={l.id}>{l.description} · {l.declared_amount} {claim.currency}</option>)}</select></Field>}</>}
        {cost ? <><h3>{cost.title}</h3><CostFacts value={cost} /></> : <p className="muted">尚未选择费用。</p>}
        <button type="button" className="button secondary" disabled={!!fixed.employee_id && (!claim || !line)} onClick={() => setPicker('cost')}>选择结算费用</button>
      </section>}
      {(action === 'record' && isRefund || action === 'refundAllocate') && <section className="form-section"><h3>原付款与费用分配</h3>
        {original.isPending ? <Busy /> : original.data && <p>{original.data.title} · 原币 {money(original.data.amount, original.data.currency)} · 已退款 {money(original.data.refunded_amount, original.data.currency)}</p>}
        <p className="form-note">已分配部分须引用原正向分配，同时核对付款币与费用原币回转金额。</p>
        {sources.isPending ? <Busy /> : !sources.error && !sourceItems.length ? <p>当前页没有剩余可退分配，可继续加载历史分配。</p> : sourceItems.map((source, index) => <div key={source.id} className="payment-plan-item"><strong>原分配 · {money(source.amount, fixed.currency)}</strong><p>当前可退 {money(source.remaining_amount, fixed.currency)} · 费用原币 {source.remaining_cost_amount ?? '需重新读取'} {source.cost_currency ?? ''}</p>
          {action === 'record' ? <><Field label={'本次退款分配 ' + (index + 1)}><input inputMode="decimal" placeholder="不关联时留空" value={plans[source.id] ?? ''} onChange={e => { setPlans(p => ({ ...p, [source.id]: e.target.value })); resetReview(); }} /></Field>{plans[source.id] && <p className="form-note">{inversePaymentFXLabel(source, plans[source.id])}</p>}</> : <label className="check-field"><input type="radio" name="refund-allocation" checked={refundSource === source.id} onChange={() => { setRefundSource(source.id); resetReview(); }} />引用此原分配</label>}
        </div>)}
        {sources.hasNextPage && <button type="button" className="button secondary" disabled={sources.isFetchingNextPage} onClick={() => void sources.fetchNextPage()}>加载更多原分配</button>}
      </section>}
      {['allocate', 'release', 'refundAllocate'].includes(action) && <Field label={'本次付款币本金（' + fixed.currency + '）'}><input required inputMode="decimal" value={amount} onChange={e => { setAmount(e.target.value); resetReview(); }} /></Field>}
      {action === 'allocate' && cross && <section className="form-section"><h3>实际跨币结算依据</h3>
        <p>费用原币金额 × 汇率，按 {fixed.currency} 精度舍入后须等于本次本金。手续费须另有真实费用与资金依据。</p>
        <Field label={'费用原币核销金额（' + cost?.currency + '）'}><input required inputMode="decimal" value={costAmount} onChange={e => { setCostAmount(e.target.value); resetReview(); }} /></Field>
        <Field label={'实际汇率（每1 ' + cost?.currency + '兑换 ' + fixed.currency + '）'}><input required inputMode="decimal" value={rate} onChange={e => { setRate(e.target.value); resetReview(); }} /></Field>
        <Field label="结算汇率凭证来源"><input required maxLength={200} value={quoteSource} onChange={e => { setQuoteSource(e.target.value); resetReview(); }} /></Field>
        <p className="form-note">固定实际付款日 {fixed.paid_on} 的结算依据，请对照已固定资金凭证。</p>
      </section>}
      {inverse && amount && <p className="notice neutral">{inversePaymentFXLabel(inverse, amount)}</p>}
      {(action === 'allocate' && cross || crossInverse) && <label className="check-field"><input type="checkbox" checked={reviewed} onChange={e => setReviewed(e.target.checked)} />我已核对两种币金额、汇率方向、凭证来源与本次回转金额</label>}
      <Field label="操作依据"><textarea required rows={3} maxLength={200} value={reason} onChange={e => setReason(e.target.value)} /></Field>
      <MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button primary" disabled={disabled}>{mutation.isPending ? '正在登记' : titles[action]}</button></div>
    </fieldset></form>
    {picker === 'claim' && <ApprovedClaimPicker employee={fixed.employee_id ?? undefined} onClose={() => setPicker(null)} onSelect={c => { setClaim(c); setLine(''); setCost(null); resetReview(); setPicker(null); }} />}
    {picker === 'cost' && <SettlementCostPicker payment={fixed} claim={claim} supplier={claim?.lines.find(l => l.id === line)?.supplier?.party_id ?? null} onClose={() => setPicker(null)} onSelect={c => { setCost(c); setCostAmount(''); setRate(''); setQuoteSource(''); resetReview(); setPicker(null); }} />}
  </Dialog>;
}

function SettlementCostPicker({ payment, claim, supplier, onSelect, onClose }: { payment: Payment; claim: Schemas['Spend'] | null; supplier: string | null; onSelect: (c: Cost) => void; onClose: () => void }) {
  const [search, setSearch] = useState(''), [q, setQ] = useState('');
  const query = useInfiniteQuery({ queryKey: ['costs', 'settlement-picker', payment.id, claim?.id, q], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Cost>>('/costs?' + new URLSearchParams({ state: 'confirmed', ...(claim?.currency ? { currency: claim.currency } : {}), q, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(p => p.items).filter(c => c.entry_kind === 'expense' && c.counterparty?.party_id === (payment.employee_id ? supplier : payment.counterparty?.party_id) && (!claim || c.currency === claim.currency && c.links.some(l => l.source_type === 'claim' && l.source_id === claim.id && l.source_revision_id === claim.approved_revision_id))) ?? [];
  return <Dialog title="选择准确结算费用" onClose={onClose}><form className="reference-search" onSubmit={e => { e.preventDefault(); setQ(search.trim()); }}><Field label="查找费用标题"><input type="search" maxLength={200} value={search} onChange={e => setSearch(e.target.value)} /></Field><button className="button secondary">查找</button></form><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && !items.length ? <Empty title="当前页没有符合条件的费用" detail="须为相同供应方的已确认原费用。员工报销还须关联准确批准依据，费用币种保持与报销一致。" /> : items.map(c => <button key={c.id} type="button" className="reference-option" onClick={() => onSelect(c)}><strong>{c.title}</strong><span>{money(c.amount, c.currency)} · {c.settlement_visibility === 'complete' ? '未结 ' + money(c.outstanding_amount, c.currency) : '结算金额受权限限制'}</span></button>)}{query.hasNextPage && <button type="button" className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多可见费用</button>}</Dialog>;
}
