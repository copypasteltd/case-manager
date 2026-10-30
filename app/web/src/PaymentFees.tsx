import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { checkedFeeBasis, checkedFeeLedger, checkedFeePreview, checkedFeeReceipt, makePaymentFee, revokePaymentFee, type FeeBasis, type FeeLedger, type FeeLink, type FeePreview } from '@copypaste/contracts/payment-fees';
import { api, ApiError, type Me, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field } from './components';
import { useCommandKey } from './BusinessSelectors';
import { MFAConfirmation } from './MFAConfirmation';
import type { Payment } from './PaymentDisplay';

export const feeNotice = '手续费关联引用已经登记的费用、实际资金和准确原分配。请先核对费用及资金凭证；保存或撤销关联只整理依据，金额仍由原费用与现金流水计算。';
export function PaymentFees({ payment, me }: { payment: Payment; me: Me }) {
  const [open, setOpen] = useState(false);
  if (payment.record_kind !== 'payment' || payment.direction !== 'outgoing' || payment.state === 'draft') return null;
  return <section className="panel party-detail"><div className="section-heading"><h2>付款手续费依据</h2><button className="button secondary" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '收起手续费依据' : '查阅手续费依据'}</button></div><p className="muted">{feeNotice}</p>{open && <FeeBody key={[payment.id, payment.version, me.authorization_version].join(':')} payment={payment} me={me} />}</section>;
}
function FeeBody({ payment, me }: { payment: Payment; me: Me }) {
  const client = useQueryClient();
  const [revision, setRevision] = useState(0), [edit, setEdit] = useState<true | FeeLink>(), path = '/payments/' + payment.id + '/fees';
  const history = useInfiniteQuery({ queryKey: ['payment-fees', payment.id, payment.version, revision], initialPageParam: '', queryFn: async ({ pageParam, signal }) => checkedFeeLedger(await api<FeeLedger>(path + '?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), payment), getNextPageParam: v => v.next_cursor ?? undefined, refetchOnWindowFocus: false });
  const pages = history.error ? undefined : history.data?.pages, overview = pages?.[0];
  const refresh = () => { setEdit(undefined); setRevision(v => v + 1); void client.invalidateQueries({ queryKey: ['payment', payment.id] }); };
  return <><ErrorNotice error={history.error} />{history.isPending ? <Busy /> : overview && <>
    <p>关联记录版本 {overview.fee_version} · 本次已载入 {pages!.reduce((sum, p) => sum + p.items.length, 0)} 条历史</p>
    {overview.can_manage && payment.state === 'recorded' && <button className="button secondary" onClick={() => setEdit(true)}>关联手续费依据</button>}
    {pages!.flatMap(v => v.items).map(item => <div className="report-context" key={item.link.id}><h3>{checkedFeeBasis(item.link, payment).cost.title} · {item.state === 'active' ? '有效关联' : item.state === 'exhausted' ? '原分配已无有效余额' : '关联已撤销'}</h3><FeeFacts basis={checkedFeeBasis(item.link, payment)} /><p>当前原分配余额：{item.current.payment_currency} {item.current.remaining_amount} · 费用原币 {item.current.cost_currency} {item.current.remaining_cost_amount}</p><p>关联依据：{item.link.reason} · {item.link.created_at}</p>{item.link.revocation && <p>撤销依据：{item.link.revocation.reason} · {item.link.revocation.created_at}</p>}{overview.can_manage && !item.link.revocation && <button className="text-button" onClick={() => setEdit(item.link)}>撤销此手续费关联</button>}</div>)}
    {!pages!.some(v => v.items.length) && <p className="muted">尚无手续费关联记录；这不代表此付款没有手续费。</p>}
    {history.hasNextPage && <button className="button secondary" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>加载更多手续费历史</button>}
    {edit && <FeeEditor key={edit === true ? 'new:' + overview.fee_version : edit.id} payment={payment} version={overview.fee_version} original={edit === true ? undefined : edit} actor={me.user.id} onClose={() => setEdit(undefined)} onSaved={refresh} />}
  </>}<button className="text-button" onClick={refresh}>重新读取付款与手续费历史</button></>;
}
function FeeFacts({ basis: b }: { basis: FeeBasis }) {
  return <><p>原付款：{b.payment.title} · {b.payment.currency} {b.payment.amount}</p><p>手续费费用：<Link to={'/costs/' + b.cost.id}>{b.cost.title}</Link> · {b.cost.currency} {b.cost.amount}</p><p>手续费实际资金：<Link to={'/payments/' + b.fee_payment.id}>{b.fee_payment.title}</Link> · {b.fee_payment.currency} {b.fee_payment.amount}</p><p>原分配：{b.allocation.payment_currency} {b.allocation.amount} · 费用原币 {b.allocation.cost_currency} {b.allocation.cost_amount}</p><p>核对时有效余额：{b.allocation.payment_currency} {b.allocation.remaining_amount} · {b.allocation.cost_currency} {b.allocation.remaining_cost_amount}</p>{b.allocation.fx && <p>实际结算汇率：每1 {b.allocation.cost_currency} 兑换 {b.allocation.fx.rate} {b.allocation.payment_currency} · {b.allocation.fx.source}</p>}<details><summary>准确版本与原分配</summary>{[b.payment, b.fee_payment, b.cost].map((v, i) => <p className="record-code" key={i}>{v.title} · 版本 {v.version} · {v.revision_id}<br />{v.content_hash}</p>)}<p className="record-code">原分配 {b.allocation.id}</p></details></>;
}
type FeeCommand = { path: string; body: Schemas['PaymentFeeCreate'] | Schemas['PaymentFeeRevoke']; original?: FeeLink };
function FeeEditor({ payment, version, original, actor, onClose, onSaved }: { payment: Payment; version: number; original?: FeeLink; actor: string; onClose(): void; onSaved(): void }) {
  const [fee, setFee] = useState<Payment>(), [allocation, setAllocation] = useState<Schemas['PaymentAllocation']>(), [picking, setPicking] = useState<'payment' | 'allocation'>(), [reason, setReason] = useState(''), [reviewed, setReviewed] = useState(false), [fresh, setFresh] = useState(false), [blocked, setBlocked] = useState(false), [uncertain, setUncertain] = useState(false), [localError, setLocalError] = useState<unknown>();
  const key = useCommandKey(), lifetime = useRef(new AbortController()), pending = useRef<FeeCommand>();
  useEffect(() => { const c = new AbortController(); lifetime.current = c; return () => c.abort(); }, []);
  const path = '/payments/' + payment.id + '/fees';
  const preview = useQuery({ queryKey: ['payment-fee-basis', payment.id, payment.version, fee?.id, allocation?.id], enabled: Boolean(!original && fee && allocation), queryFn: async ({ signal }) => { const v = checkedFeePreview(await api<FeePreview>(path + '/basis?' + new URLSearchParams({ fee_payment_id: fee!.id, allocation_id: allocation!.id }), { signal }), payment); const b = checkedFeeBasis(v, payment, true); if (b.fee_payment.id !== fee!.id || b.allocation.id !== allocation!.id || v.fee_version !== version) throw new Error('手续费关联已变化，请重新读取历史。'); return v; }, refetchOnWindowFocus: false });
  const mutation = useMutation({ mutationFn: async (value: FeeCommand) => checkedFeeReceipt(await api<FeeLink>(value.path, { method: 'POST', body: value.body, key: key(value), signal: lifetime.current.signal }), payment, value.body, actor, value.original), onSuccess: onSaved, onError: error => { if (error instanceof ApiError && [401, 403, 404, 409, 412].includes(error.status)) setBlocked(true); else if (!(error instanceof ApiError) || !error.status || error.status >= 500) setUncertain(true); } });
  const locked = mutation.isPending || blocked || uncertain, value = original ?? (!preview.error ? preview.data : undefined);
  function changed() { setReviewed(false); setLocalError(undefined); }
  function submit() {
    try {
      if (!fresh || !reviewed || !value) throw new Error('请核对完整依据、明确确认并完成两步验证。');
      const command: FeeCommand = original ? { path: path + '/' + original.id + '/revoke', body: revokePaymentFee(payment, original, version, reason), original } : { path, body: makePaymentFee(payment, preview.data!, reason) };
      pending.current = command; mutation.mutate(command);
    } catch (cause) { setLocalError(new ApiError(422, 'PAYMENT_FEE_INPUT_INVALID', cause instanceof Error ? cause.message : '请核对手续费资料。', '')); }
  }
  return <Dialog title={original ? '撤销手续费关联' : '关联手续费依据'} onClose={() => { if (!mutation.isPending) onClose(); }}><ErrorNotice error={localError ?? mutation.error ?? preview.error} /><fieldset disabled={locked}>
    <p>{feeNotice}</p>{!original && <><button className="button secondary" onClick={() => setPicking('payment')}>选择手续费实际资金</button><button className="text-button" onClick={() => { setFee(payment); setAllocation(undefined); changed(); }}>使用本笔付款中的分配</button>{fee && <><p>所选资金：{fee.title}</p><button className="button secondary" onClick={() => setPicking('allocation')}>选择手续费原分配</button></>}</>}
    {preview.isFetching && <Busy label="核对准确手续费依据" />}{value && <FeeFacts basis={checkedFeeBasis(value, payment)} />}
    <Field label="手续费操作依据"><textarea aria-label="手续费操作依据" rows={3} maxLength={200} value={reason} onChange={e => { setReason(e.target.value); changed(); }} /></Field>
    <MFAConfirmation onFresh={setFresh} /><label className="check-field"><input type="checkbox" aria-label="确认手续费依据" checked={reviewed} onChange={e => setReviewed(e.target.checked)} />我已核对原费用、实际资金、整条原分配与凭证，确认{original ? '撤销此关联并保留历史' : '将此原分配作为本次付款的手续费依据'}</label>
    <button className="button primary" disabled={locked || !value || !reason.trim() || !reviewed || !fresh} onClick={submit}>{original ? '确认撤销手续费关联' : '确认关联手续费依据'}</button>
  </fieldset>{blocked && <p className="notice warning">资料或权限已变化，请关闭表单并重新读取当前付款和手续费历史。</p>}{uncertain && <><p className="notice warning">结果尚未核对，输入已锁定。请先重新读取历史，或原样重试同一请求。</p><button className="button secondary" disabled={mutation.isPending} onClick={() => pending.current && mutation.mutate(pending.current)}>原样核对手续费请求</button></>}
    {picking === 'payment' && <FeePicker<Payment> title="选择手续费实际资金" path={'/payments?limit=20&state=recorded&direction=outgoing' + (payment.project_id ? '&project_id=' + payment.project_id : '&company_only=true')} label={v => v.title + ' · ' + v.currency + ' ' + v.amount} eligible={v => v.record_kind === 'payment' && v.classification === payment.classification} onClose={() => setPicking(undefined)} onSelect={v => { setFee(v); setAllocation(undefined); changed(); setPicking(undefined); }} />}
    {picking === 'allocation' && fee && <FeePicker<Schemas['PaymentAllocation']> searchable={false} title="选择手续费原分配" path={'/payments/' + fee.id + '/allocations?limit=20'} label={v => '费用 ' + v.cost_id + ' · ' + v.remaining_amount + ' ' + v.payment_currency + ' · ' + v.remaining_cost_amount + ' ' + v.cost_currency} eligible={v => v.kind === 'allocation' && /[1-9]/.test(v.remaining_amount) && /[1-9]/.test(v.remaining_cost_amount ?? '')} onClose={() => setPicking(undefined)} onSelect={v => { setAllocation(v); changed(); setPicking(undefined); }} />}
  </Dialog>;
}
function FeePicker<T extends { id: string }>({ title, path, label, eligible, onSelect, onClose, searchable = true }: { title: string; path: string; label(v: T): string; eligible(v: T): boolean; onSelect(v: T): void; onClose(): void; searchable?: boolean }) {
  const [search, setSearch] = useState(''), [query, setQuery] = useState('');
  const result = useInfiniteQuery({ queryKey: ['fee-picker', path, query], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<T>>(path + (query ? '&q=' + encodeURIComponent(query) : '') + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: v => v.next_cursor ?? undefined });
  const rows = result.error ? [] : result.data?.pages.flatMap(v => v.items) ?? [];
  return <Dialog title={title} onClose={onClose}>{searchable && <><Field label="查找手续费资料"><input aria-label="查找手续费资料" type="search" value={search} maxLength={200} onChange={e => setSearch(e.target.value)} /></Field><button className="button secondary" onClick={() => setQuery(search.trim())}>查找手续费资料</button></>}<ErrorNotice error={result.error} />{result.isPending ? <Busy /> : rows.length ? rows.map(v => <button className="reference-option" key={v.id} disabled={!eligible(v)} onClick={() => onSelect(v)}>{label(v)}{!eligible(v) && ' · 当前不可选'}</button>) : !result.error && <Empty title="当前页没有可选依据" detail="可查找更多记录，或先登记真实费用、资金及对应分配。" />}{result.hasNextPage && <button className="button secondary" disabled={result.isFetchingNextPage} onClick={() => void result.fetchNextPage()}>加载更多手续费候选</button>}</Dialog>;
}
