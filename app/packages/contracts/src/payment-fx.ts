import type { components } from './api';
type Schemas = components['schemas'];
type Allocation = Schemas['PaymentAllocation'];
export type DualAllocation = Allocation & Required<Pick<Allocation, 'cost_amount' | 'cost_currency' | 'payment_currency' | 'cost_exponent' | 'payment_exponent' | 'cost_shares' | 'remaining_cost_amount' | 'fx'>>;
const id = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const decimal = /^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/;
const ratePattern = /^(0|[1-9][0-9]{0,13})(\.[0-9]{1,10})?$/;
const exp = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value) && value >= 0 && value <= 6;
const fail = (): never => { throw new Error('双币核销依据无效或已变化，请重新读取准确原分配并核对。'); };
function units(value: unknown): bigint { if (typeof value !== 'string' || !decimal.test(value)) return fail(); const [whole, fraction = ''] = value.split('.'); return BigInt(whole) * 1000000n + BigInt(fraction.padEnd(6, '0')); }
// Taro's legacy exponent transform lowers BigInt powers to Math.pow, which
// rejects BigInt at runtime. These bounded integer products survive all four
// actual build consumers without converting money to a floating-point number.
function unitStep(exponent: number): bigint { let step = 1n; for (let i = exponent; i < 6; i++) step *= 10n; return step; }
const exact = (value: unknown, exponent: number) => exp(exponent) && units(value) % unitStep(exponent) === 0n;
const money = (value: bigint) => (value / 1000000n).toString() + '.' + (value % 1000000n).toString().padStart(6, '0');
function rateUnits(value: unknown) { if (typeof value !== 'string' || !ratePattern.test(value)) return fail(); const [whole, fraction = ''] = value.split('.'); const r = BigInt(whole) * 10000000000n + BigInt(fraction.padEnd(10, '0')); if (r <= 0n) return fail(); return r; }
function roundedRatio(numerator: bigint, denominator: bigint) { if (denominator <= 0n || numerator < 0n) return fail(); return numerator / denominator + (numerator % denominator * 2n >= denominator ? 1n : 0n); }
function text(value: unknown): value is string { return typeof value === 'string' && value.trim().length > 0 && [...value.trim()].length <= 200 && !/[\p{Cc}\p{Cf}]/u.test(value); }
// The same six supported currencies are offered by the existing financial forms.
// Existing allocations use their recorded exponents instead of this form list.
export function settlementExponent(currency: string | null | undefined) { if (currency === 'JPY') return 0; if (currency && ['HKD', 'CNY', 'USD', 'EUR', 'GBP'].includes(currency)) return 2; throw new Error('当前表单不支持此币种精度，请核对实际币种。'); }
export function reviewedRefundFX(original: string, cashCurrency: string, principal: string, cash: string, rate: string, source: string): Schemas['PaymentRefundFX'] {
  const oe = settlementExponent(original), ce = settlementExponent(cashCurrency);
  if (original === cashCurrency || !exact(principal, oe) || units(principal) <= 0n || !exact(cash, ce) || units(cash) <= 0n || !text(source)) throw new Error('请核对异币退款原本金、实收金额、币种精度与凭证来源。');
  if (roundedRatio(units(principal) * rateUnits(rate), 10000000000n * unitStep(ce)) * unitStep(ce) !== units(cash)) throw new Error('原付款本金乘实际汇率须按实收币种精度舍入后等于实收金额。');
  return { original_currency: original, original_amount: principal, rate, source: source.trim() };
}
export function refundFXEqual(left: Schemas['PaymentRefundFX'] | undefined, right: Schemas['PaymentRefundFX'] | undefined) {
  if (!left || !right) return left === right;
  try { return left.original_currency === right.original_currency && units(left.original_amount) === units(right.original_amount) && rateUnits(left.rate) === rateUnits(right.rate) && left.source === right.source.trim(); } catch { return false; }
}
export function reviewedPaymentFX(payment: Schemas['Payment'], cost: Schemas['Cost'], cash: string, input?: Schemas['PaymentFXInput']): Schemas['PaymentFXInput'] | undefined {
  const pe = settlementExponent(payment.currency), ce = settlementExponent(cost.currency);
  if (!exact(cash, pe) || units(cash) <= 0n) throw new Error('请填写准确付款币本金与精度。');
  if (payment.currency === cost.currency) { if (input) throw new Error('同币种核销不接受汇率。'); return; }
  if (!input || input.acknowledged !== true || !text(input.source) || !exact(input.cost_amount, ce) || units(input.cost_amount) <= 0n) throw new Error('请核对费用原币金额、实际汇率、凭证来源并明确确认。');
  const value = roundedRatio(units(input.cost_amount) * rateUnits(input.rate), 10000000000n * unitStep(pe)) * unitStep(pe);
  if (value !== units(cash)) throw new Error('费用原币金额乘实际汇率须按付款币精度舍入后等于本次付款本金。');
  return { ...input, source: input.source.trim() };
}
export function checkedDualAllocation(raw: Allocation): DualAllocation {
  if (!raw || !id.test(raw.id) || !id.test(raw.payment_id) || !id.test(raw.cost_id) || !id.test(raw.cost_revision_id) || !id.test(raw.root_id)
    || !['allocation', 'release', 'refund', 'reversal'].includes(raw.kind) || ![-1, 1].includes(raw.effect)
    || typeof raw.cost_currency !== 'string' || !/^[A-Z]{3}$/.test(raw.cost_currency) || typeof raw.payment_currency !== 'string' || !/^[A-Z]{3}$/.test(raw.payment_currency)
    || !exp(raw.cost_exponent) || !exp(raw.payment_exponent) || !Array.isArray(raw.cost_shares) || !Array.isArray(raw.shares) || raw.shares.length < 1 || raw.shares.length > 100 || raw.cost_shares.length !== raw.shares.length) return fail();
  const v = raw as DualAllocation;
  if (!exact(v.amount, v.payment_exponent) || units(v.amount) <= 0n || !exact(v.cost_amount, v.cost_exponent) || !exact(v.remaining_amount, v.payment_exponent) || !exact(v.remaining_cost_amount, v.cost_exponent)) return fail();
  if (v.kind === 'allocation' && (units(v.cost_amount) <= 0n || v.id !== v.root_id || v.effect !== 1 || v.source_allocation_id !== null || units(v.remaining_amount) > units(v.amount) || units(v.remaining_cost_amount) > units(v.cost_amount))) return fail();
  let cash = 0n, cost = 0n; const scopes = new Set<string | null>();
  v.shares.forEach((s, i) => { const c = v.cost_shares[i]; if (!c || s.project_id !== c.project_id || s.project_id !== null && !id.test(s.project_id) || scopes.has(s.project_id) || !exact(s.amount, v.payment_exponent) || !exact(c.amount, v.cost_exponent) || units(s.amount) + units(c.amount) <= 0n) return fail(); scopes.add(s.project_id); cash += units(s.amount); cost += units(c.amount); });
  if (cash !== units(v.amount) || cost !== units(v.cost_amount)) return fail();
  if (v.cost_currency === v.payment_currency) {
    if (v.fx !== null || v.cost_exponent !== v.payment_exponent || units(v.cost_amount) !== units(v.amount) || units(v.remaining_cost_amount) !== units(v.remaining_amount) || v.shares.some((s, i) => units(s.amount) !== units(v.cost_shares[i].amount))) return fail();
  } else {
    if (!v.fx || !text(v.fx.source) || !/^\d{4}-\d{2}-\d{2}$/.test(v.fx.quoted_on) || new Date(v.fx.quoted_on + 'T00:00:00Z').toISOString().slice(0,10) !== v.fx.quoted_on) return fail();
    const rate = rateUnits(v.fx.rate);
    if (v.kind === 'allocation' && roundedRatio(units(v.cost_amount) * rate, 10000000000n * unitStep(v.payment_exponent)) * unitStep(v.payment_exponent) !== units(v.amount)) return fail();
  }
  if (v.refund_cash) {
    const cash = v.refund_cash, ce = settlementExponent(cash.currency);
    if (!['refund', 'reversal'].includes(v.kind) || cash.original_currency !== v.payment_currency || !Array.isArray(cash.shares) || cash.shares.length !== v.shares.length || !exact(cash.amount, ce) || units(cash.amount) > units(cash.whole_amount)) return fail();
    reviewedRefundFX(cash.original_currency, cash.currency, cash.original_amount, cash.whole_amount, cash.rate, cash.source);
    let total = 0n;
    cash.shares.forEach((s, i) => { if (s.project_id !== v.shares[i].project_id || !exact(s.amount, ce) || units(v.shares[i].amount) === 0n && units(s.amount) !== 0n) return fail(); total += units(s.amount); });
    if (total !== units(cash.amount) || units(v.amount) > units(cash.original_amount)) return fail();
    const minor = units(cash.amount) / unitStep(ce), denominator = units(v.amount);
    const parts = v.shares.map((s, index) => ({ index, value: minor * units(s.amount) / denominator, rest: minor * units(s.amount) % denominator }));
    const extra = minor - parts.reduce((sum, part) => sum + part.value, 0n);
    const ordered = parts.filter((part) => units(v.shares[part.index].amount) > 0n).sort((a, b) => a.rest === b.rest ? a.index - b.index : a.rest > b.rest ? -1 : 1);
    ordered.forEach((part, index) => { if (BigInt(index) < extra) part.value += 1n; });
    if (parts.some(part => units(cash.shares[part.index].amount) !== part.value * unitStep(ce))) return fail();
  }
  return v;
}
export function inversePaymentFX(source: Allocation, cash: string): { cost_amount?: string } {
  // Legacy archived receipts cannot establish a current cross-currency basis.
  const v = checkedDualAllocation(source), amount = units(cash), remaining = units(v.remaining_amount);
  if (v.kind !== 'allocation' || amount <= 0n || amount > remaining || !exact(cash, v.payment_exponent)) return fail();
  if (v.cost_currency === v.payment_currency) return {};
  const step = unitStep(v.cost_exponent);
  return { cost_amount: money(roundedRatio(amount * units(v.remaining_cost_amount), remaining * step) * step) };
}
export function inversePaymentFXLabel(source: Allocation, cash: string) {
  try { const v = checkedDualAllocation(source), result = inversePaymentFX(v, cash); return `本次回转：${cash} ${v.payment_currency} → ${result.cost_amount ?? cash} ${v.cost_currency}；原依据 ${v.fx ? v.fx.rate + ' · ' + v.fx.source : '同币种'}。`; }
  catch { return '请填写有效付款币金额，重新核对原分配当前双币余额。'; }
}
export function allocationFXReceipt(row: Allocation, expected: { cash: string; cost?: string; fx?: Schemas['PaymentFXInput']; source?: Allocation; paymentCurrency?: string | null; costCurrency?: string | null; quotedOn?: string | null; refund?: Schemas['Payment']; actualCash?: string }) {
  try { const v = checkedDualAllocation(row); if (units(v.amount) !== units(expected.cash) || expected.cost !== undefined && units(v.cost_amount) !== units(expected.cost)) return false;
    if (expected.paymentCurrency !== undefined && v.payment_currency !== expected.paymentCurrency || expected.costCurrency !== undefined && v.cost_currency !== expected.costCurrency || expected.quotedOn && v.fx && v.fx.quoted_on !== expected.quotedOn) return false;
    const quote = expected.fx ?? expected.source?.fx;
    if (quote ? !v.fx || rateUnits(v.fx.rate) !== rateUnits(quote.rate) || v.fx.source !== quote.source.trim() : v.fx !== null) return false;
    if (expected.source && (v.source_allocation_id !== expected.source.id || v.root_id !== expected.source.root_id || v.cost_id !== expected.source.cost_id || v.cost_revision_id !== expected.source.cost_revision_id || v.cost_currency !== expected.source.cost_currency || v.payment_currency !== expected.source.payment_currency || v.fx?.quoted_on !== expected.source.fx?.quoted_on)) return false;
    if (expected.refund?.refund_fx) {
      const cash = v.refund_cash, p = expected.refund;
      if (!cash || v.payment_id !== p.id || v.kind !== 'refund' || cash.currency !== p.currency || units(cash.whole_amount) !== units(p.amount) || !refundFXEqual({ original_currency: cash.original_currency, original_amount: cash.original_amount, rate: cash.rate, source: cash.source }, p.refund_fx) || expected.actualCash === undefined || units(cash.amount) !== units(expected.actualCash)) return false;
    } else if (v.refund_cash) return false;
    return true;
  } catch { return false; }
}

// A foreign refund's entered amount is ORIGINAL payment principal. These
// helpers keep original settlement, expense and actual received cash separate.
export function refundPaymentPlan(payment: Schemas['Payment'], source: Allocation, principal: string) {
  const v = checkedDualAllocation(source), inverse = inversePaymentFX(v, principal);
  if (payment.refund_of_id !== v.payment_id || payment.direction !== 'refund' || payment.record_kind !== 'payment') return fail();
  if (payment.refund_fx) {
    if (payment.refund_fx.original_currency !== v.payment_currency) return fail();
    reviewedRefundFX(v.payment_currency, payment.currency!, payment.refund_fx.original_amount, payment.amount!, payment.refund_fx.rate, payment.refund_fx.source);
    return { source_allocation_id: v.id, amount: principal, cost_amount: inverse.cost_amount ?? principal };
  }
  if (payment.currency !== v.payment_currency) return fail();
  return { source_allocation_id: v.id, amount: principal, ...inverse };
}

export function refundCashPreview(payment: Schemas['Payment'], principal: string, previous: { principal: string; cash: string }[] = []) {
  if (!payment.refund_fx) return principal;
  const q = payment.refund_fx, pe = settlementExponent(q.original_currency), ce = settlementExponent(payment.currency);
  reviewedRefundFX(q.original_currency, payment.currency!, q.original_amount, payment.amount!, q.rate, q.source);
  let remaining = units(q.original_amount) - units(payment.state === 'draft' ? '0' : payment.refund_allocated_principal);
  let cash = units(payment.state === 'draft' ? payment.amount : payment.unallocated_amount);
  for (const row of previous) { remaining -= units(row.principal); cash -= units(row.cash); }
  const amount = units(principal);
  if (!exact(principal, pe) || amount <= 0n || amount > remaining || cash < 0n || !exact(money(cash), ce)) return fail();
  const step = unitStep(ce);
  return money(roundedRatio(amount * cash, remaining * step) * step);
}

export function refundPrincipalLabel(payment: Schemas['Payment'], source: Allocation, principal: string, priorPrincipal: string[] = []) {
  try {
    const plan = refundPaymentPlan(payment, source, principal);
    const previous: { principal: string; cash: string }[] = [];
    for (const amount of priorPrincipal) previous.push({ principal: amount, cash: refundCashPreview(payment, amount, previous) });
    return payment.refund_fx ? `原付款本金 ${principal} ${source.payment_currency}；费用回转 ${plan.cost_amount} ${source.cost_currency}；实收现金份额 ${refundCashPreview(payment, principal, previous)} ${payment.currency}。` : inversePaymentFXLabel(source, principal);
  } catch { return '请重新核对原分配、本次原本金、费用原币和该退款的剩余实收余额。'; }
}
