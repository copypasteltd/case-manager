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
export function allocationFXReceipt(row: Allocation, expected: { cash: string; cost?: string; fx?: Schemas['PaymentFXInput']; source?: Allocation; paymentCurrency?: string | null; costCurrency?: string | null; quotedOn?: string | null }) {
  try { const v = checkedDualAllocation(row); if (units(v.amount) !== units(expected.cash) || expected.cost !== undefined && units(v.cost_amount) !== units(expected.cost)) return false;
    if (expected.paymentCurrency !== undefined && v.payment_currency !== expected.paymentCurrency || expected.costCurrency !== undefined && v.cost_currency !== expected.costCurrency || expected.quotedOn && v.fx && v.fx.quoted_on !== expected.quotedOn) return false;
    const quote = expected.fx ?? expected.source?.fx;
    if (quote ? !v.fx || rateUnits(v.fx.rate) !== rateUnits(quote.rate) || v.fx.source !== quote.source.trim() : v.fx !== null) return false;
    if (expected.source && (v.cost_currency !== expected.source.cost_currency || v.payment_currency !== expected.source.payment_currency || v.fx?.quoted_on !== expected.source.fx?.quoted_on)) return false;
    return true;
  } catch { return false; }
}
