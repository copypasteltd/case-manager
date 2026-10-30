import { useEffect, useRef, useState } from 'react';
import { Text, View } from '@tarojs/components';
import { checkedFeeBasis, checkedFeeLedger, checkedFeePreview, checkedFeeReceipt, makePaymentFee, revokePaymentFee, type FeeBasis, type FeeLedger, type FeeLink, type FeePreview } from '@copypaste/contracts/payment-fees';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { go } from '../../platform/page';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Fact, Notice, Section } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';

const feeNotice = '手续费关联引用已经登记的费用、实际资金和准确原分配。请先核对费用及资金凭证；保存或撤销关联只整理依据，金额仍由原费用与现金流水计算。';
function useFeeResource<T>(path: string | null, parent: Scope, revision: number, validate: (v: T) => T) {
  const [value, setValue] = useState<T>(), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(Boolean(path)), check = useRef(validate); check.current = validate;
  useEffect(() => { const scope = new Scope(), unbind = parent.own(() => scope.close()); setValue(undefined); setError(undefined); setLoading(Boolean(path)); if (path) void client.request<T>(path, { scope }).then(v => { if (!scope.closed) setValue(check.current(v)); }).catch(e => { if (!scope.closed) setError(e); }).finally(() => { if (!scope.closed) setLoading(false); }); return () => { scope.close(); unbind(); }; }, [path, parent, revision]);
  return { value, error, loading };
}
export function PaymentFees({ payment, me, scope, refresh }: { payment: Schemas['Payment']; me: Schemas['Me']; scope: Scope; refresh(): void }) {
  const [open, setOpen] = useState(false);
  if (payment.record_kind !== 'payment' || payment.direction !== 'outgoing' || payment.state === 'draft') return null;
  return <Section title='付款手续费依据'><Text className='cp-muted cp-paragraph'>{feeNotice}</Text><Action secondary onClick={() => setOpen(!open)}>{open ? '收起手续费依据' : '查阅手续费依据'}</Action>{open && <FeeBody key={payment.id + ':' + payment.version} payment={payment} me={me} scope={scope} refresh={refresh} />}</Section>;
}
function FeeBody({ payment, me, scope, refresh }: { payment: Schemas['Payment']; me: Schemas['Me']; scope: Scope; refresh(): void }) {
  const [cursors, setCursors] = useState(['']), [edit, setEdit] = useState<true | FeeLink>(), path = '/payments/' + payment.id + '/fees';
  const history = useFeeResource<FeeLedger>(path + '?limit=20' + (cursors[cursors.length - 1] ? '&cursor=' + encodeURIComponent(cursors[cursors.length - 1]!) : ''), scope, 0, v => checkedFeeLedger(v, payment)), value = history.value;
  return <View><Notice error={history.error} />{history.loading ? <Busy /> : value && <>
    <Text className='cp-paragraph'>关联记录版本 {value.fee_version} · 本页 {value.items.length} 条历史</Text>
    {value.can_manage && payment.state === 'recorded' && <Action secondary onClick={() => setEdit(true)}>关联手续费依据</Action>}
    {value.items.map(item => <Section key={item.link.id} title={checkedFeeBasis(item.link, payment).cost.title}><Text className='cp-paragraph'>{item.state === 'active' ? '有效关联' : item.state === 'exhausted' ? '原分配已无有效余额' : '关联已撤销'}</Text><FeeFacts basis={checkedFeeBasis(item.link, payment)} /><Fact label='当前原分配余额'><Text>{item.current.payment_currency} {item.current.remaining_amount} · 费用原币 {item.current.cost_currency} {item.current.remaining_cost_amount}</Text></Fact><Fact label='关联依据'><Text>{item.link.reason} · {item.link.created_at}</Text></Fact>{item.link.revocation && <Fact label='撤销依据'><Text>{item.link.revocation.reason} · {item.link.revocation.created_at}</Text></Fact>}{value.can_manage && !item.link.revocation && <Action secondary onClick={() => setEdit(item.link)}>撤销此手续费关联</Action>}</Section>)}
    {!value.items.length && <Text className='cp-muted cp-paragraph'>尚无手续费关联记录；这不代表此付款没有手续费。</Text>}
    {cursors.length > 1 && <Action secondary onClick={() => { setEdit(undefined); setCursors(v => v.slice(0, -1)); }}>上一页手续费历史</Action>}{value.next_cursor && <Action secondary onClick={() => { setEdit(undefined); setCursors(v => [...v, value.next_cursor!]); }}>下一页手续费历史</Action>}
    {edit && <FeeEditor key={edit === true ? 'new:' + value.fee_version : edit.id} payment={payment} original={edit === true ? undefined : edit} version={value.fee_version} me={me} parent={scope} onClose={() => setEdit(undefined)} onSaved={refresh} />}
  </>}<Action secondary onClick={refresh}>重新读取付款与手续费历史</Action></View>;
}
function FeeFacts({ basis: b }: { basis: FeeBasis }) {
  return <><Fact label='原付款'><Text>{b.payment.title} · {b.payment.currency} {b.payment.amount}</Text></Fact><Fact label='手续费费用'><Text>{b.cost.title} · {b.cost.currency} {b.cost.amount}</Text><Action secondary onClick={() => go('cost', b.cost.id)}>查阅手续费费用及凭证</Action></Fact><Fact label='手续费实际资金'><Text>{b.fee_payment.title} · {b.fee_payment.currency} {b.fee_payment.amount}</Text><Action secondary onClick={() => go('payment', b.fee_payment.id)}>查阅手续费资金及凭证</Action></Fact><Fact label='整条原分配'><Text>{b.allocation.payment_currency} {b.allocation.amount} · 费用原币 {b.allocation.cost_currency} {b.allocation.cost_amount}</Text></Fact><Fact label='核对时有效余额'><Text>{b.allocation.payment_currency} {b.allocation.remaining_amount} · {b.allocation.cost_currency} {b.allocation.remaining_cost_amount}</Text></Fact>{b.allocation.fx && <Fact label='实际结算汇率'><Text>每1 {b.allocation.cost_currency} 兑换 {b.allocation.fx.rate} {b.allocation.payment_currency} · {b.allocation.fx.source}</Text></Fact>}{[b.payment, b.fee_payment, b.cost].map((v, i) => <Fact key={i} label={v.title + '准确版本'}><Text>版本 {v.version} · {v.revision_id}</Text><Text className='cp-code' selectable>{v.content_hash}</Text></Fact>)}<Text className='cp-code cp-small' selectable>原分配 {b.allocation.id}</Text></>;
}
function FeeEditor({ payment, original, version, me, parent, onClose, onSaved }: { payment: Schemas['Payment']; original?: FeeLink; version: number; me: Schemas['Me']; parent: Scope; onClose(): void; onSaved(): void }) {
  const [fee, setFee] = useState<Schemas['Payment']>(), [allocation, setAllocation] = useState<Schemas['PaymentAllocation']>(), [picking, setPicking] = useState<'payment' | 'allocation'>(), [reason, setReason] = useState(''), [reviewed, setReviewed] = useState(false), [fresh, setFresh] = useState(false), [localError, setLocalError] = useState<unknown>();
  const path = '/payments/' + payment.id + '/fees';
  const preview = useFeeResource<FeePreview>(!original && fee && allocation ? path + '/basis?' + new URLSearchParams({ fee_payment_id: fee.id, allocation_id: allocation.id }) : null, parent, 0, v => { checkedFeePreview(v, payment); const b = checkedFeeBasis(v, payment, true); if (b.fee_payment.id !== fee?.id || b.allocation.id !== allocation?.id || v.fee_version !== version) throw new Error('手续费关联已变化，请重新读取历史。'); return v; });
  const expected = useRef<Schemas['PaymentFeeCreate'] | Schemas['PaymentFeeRevoke']>();
  const command = useCommand<FeeLink>(onSaved, 'payment-fees:' + payment.id + ':' + payment.version + ':' + version, v => { if (!expected.current) return false; checkedFeeReceipt(v, payment, expected.current, me.user.id, original); return true; });
  const locked = command.busy || command.blocked, value = original ?? (!preview.error ? preview.value : undefined);
  function changed() { setReviewed(false); setLocalError(undefined); }
  function submit() {
    try {
      if (!reviewed || !fresh || !value) throw new Error('请核对完整依据、明确确认并完成两步验证。');
      const body = original ? revokePaymentFee(payment, original, version, reason) : makePaymentFee(payment, preview.value!, reason);
      expected.current = body; void command.submit(path + (original ? '/' + original.id + '/revoke' : ''), body);
    } catch (e) { setLocalError(new MobileError('PAYMENT_FEE_INPUT_INVALID', e instanceof Error ? e.message : '请核对手续费资料。', 422)); }
  }
  return <Section title={original ? '撤销手续费关联' : '关联手续费依据'}><View className='cp-pad'><Text className='cp-paragraph'>{feeNotice}</Text><Notice error={localError ?? command.error ?? preview.error} /><CommandNotice blocked={command.blocked} />
    {!original && <><Action secondary disabled={locked} onClick={() => setPicking('payment')}>选择手续费实际资金</Action><Action secondary disabled={locked} onClick={() => { setFee(payment); setAllocation(undefined); changed(); }}>使用本笔付款中的分配</Action>{fee && <><Text className='cp-paragraph'>所选资金：{fee.title}</Text><Action secondary disabled={locked} onClick={() => setPicking('allocation')}>选择手续费原分配</Action></>}</>}
    {preview.loading && <Busy />}{value && <FeeFacts basis={checkedFeeBasis(value, payment)} />}
    <TextField name='payment-fee-reason' label='手续费操作依据' value={reason} onChange={v => { setReason(v); changed(); }} disabled={locked} multiline />
    <SelectField label='确认手续费依据' value={reviewed ? 'yes' : ''} options={[[ '', '尚未确认' ], [ 'yes', original ? '已核对准确关联，确认撤销并保留历史' : '已核对原费用、实际资金、整条原分配及凭证，确认为手续费依据' ]]} disabled={locked} onChange={v => setReviewed(v === 'yes')} />
    <MFAConfirmation disabled={locked} onFresh={setFresh} /><Action loading={command.busy} disabled={locked || !value || !reason.trim() || !reviewed || !fresh} onClick={submit}>{original ? '确认撤销手续费关联' : '确认关联手续费依据'}</Action><Action secondary disabled={command.busy} onClick={onClose}>关闭手续费操作</Action>
    {picking === 'payment' && <ChoiceList<Schemas['Payment']> title='选择手续费实际资金' path={'/payments?limit=20&state=recorded&direction=outgoing' + (payment.project_id ? '&project_id=' + payment.project_id : '&company_only=true')} label={v => v.title + ' · ' + v.currency + ' ' + v.amount} eligible={v => v.record_kind === 'payment' && v.classification === payment.classification} onClose={() => setPicking(undefined)} onSelect={v => { setFee(v); setAllocation(undefined); changed(); setPicking(undefined); }} />}
    {picking === 'allocation' && fee && <ChoiceList<Schemas['PaymentAllocation']> title='选择手续费原分配' path={'/payments/' + fee.id + '/allocations?limit=20'} searchable={false} label={v => '费用 ' + v.cost_id + ' · ' + v.remaining_amount + ' ' + v.payment_currency + ' · ' + v.remaining_cost_amount + ' ' + v.cost_currency} eligible={v => v.kind === 'allocation' && /[1-9]/.test(v.remaining_amount) && /[1-9]/.test(v.remaining_cost_amount ?? '')} onClose={() => setPicking(undefined)} onSelect={v => { setAllocation(v); changed(); setPicking(undefined); }} />}
  </View></Section>;
}
