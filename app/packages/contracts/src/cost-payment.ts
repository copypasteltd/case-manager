import type { components } from './api';
import { allocationFXReceipt, reviewedPaymentFX, settlementExponent } from './payment-fx';

type S = components['schemas'];
export type CostPaymentBasis = S['CostPaymentBasis'];
const id = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
const hash = /^[a-f0-9]{64}$/;
const key = /^[a-z][a-z0-9_.-]{1,31}:[A-Za-z0-9._:/-]{1,166}$/;
const fail = (message = '付款及核销依据无效，请重新查阅准确费用。'): never => { throw new Error(message); };
function units(value: unknown): bigint {
  if (typeof value !== 'string' || !/^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/.test(value)) return fail();
  const [whole, decimal = ''] = value.split('.'); return BigInt(whole) * 1000000n + BigInt(decimal.padEnd(6, '0'));
}
const validText = (v: unknown, max: number, required = false): v is string => typeof v === 'string' && [...v].length <= max && (!required || !!v.trim()) && !/[\p{Cc}\p{Cf}]/u.test(v.replace(/[\r\n\t]/g, ''));
const limits = { title: 200, date: 19, currency: 3, amount: 30, category: 50, source: 199, allocation: 30, costAmount: 30, rate: 25, quoteSource: 200, method: 100, external: 200, notes: 20000, reason: 200, payee: 8 } as const;
export type CostPaymentText = { [K in keyof typeof limits]: string };
export function copyCostPaymentText(raw: unknown): CostPaymentText | undefined {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return;
  const value = raw as Record<string, unknown>, out: Record<string, string> = {};
  for (const [name, max] of Object.entries(limits)) { const text = value[name]; if (!validText(text, max)) return; out[name] = text; }
  if (!['supplier', 'employee'].includes(out.payee)) return;
  return out as CostPaymentText;
}
export function costPaymentCharacters(v: CostPaymentText) { return Object.values(v).reduce((n, text) => n + text.length, 0); }
export function costPaymentPath(value: unknown) { if (typeof value !== 'string' || !id.test(value)) return fail(); return '/costs/' + value + '/payment-basis'; }
export function costPaymentRoute(params: { kind?: unknown; action?: unknown; id?: unknown; [name: string]: unknown }) {
  if (params.kind !== 'cost' || params.action !== 'recordPayment' || ['refund', 'project_id', 'source_type', 'source_id'].some(k => params[k] !== undefined)) return fail();
  costPaymentPath(params.id); return params.id as string;
}
export function checkedCostPaymentBasis(raw: unknown, costID: string): CostPaymentBasis {
  costPaymentPath(costID);
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return fail();
  const b = raw as CostPaymentBasis, c = b.cost;
  if (!c || c.id !== costID || !Number.isSafeInteger(c.version) || c.version < 1 || !id.test(c.current_revision_id) || !hash.test(c.content_hash)
    || c.state !== 'confirmed' || c.entry_kind !== 'expense' || c.reversal_id !== null || c.reverses_id !== null
    || !validText(c.title, 200, true) || !c.currency || !/^[A-Z]{3}$/.test(c.currency) || units(c.amount) <= 0n
    || !(c.project_id === null || id.test(c.project_id)) || !['internal', 'project_restricted', 'confidential'].includes(c.classification)
    || c.classification === 'project_restricted' && !c.project_id || !c.counterparty || !id.test(c.counterparty.party_id) || !id.test(c.counterparty.revision_id)
    || !Array.isArray(c.allocations) || c.allocations.length < 1 || c.allocations.length > 100 || !Array.isArray(c.links)
    || !Number.isFinite(Date.parse(b.as_of))) return fail();
  const scopes = new Set<string | null>(); let total = 0n;
  for (const line of c.allocations) { if (!(line.project_id === null || id.test(line.project_id)) || scopes.has(line.project_id) || units(line.amount) <= 0n) return fail(); scopes.add(line.project_id); total += units(line.amount); }
  if (total !== units(c.amount)) return fail();
  return b;
}
export function costPaymentSeed(b: CostPaymentBasis, source: string): CostPaymentText {
  checkedCostPaymentBasis(b, b.cost.id);
  return { title: [...('实际付款 · ' + b.cost.title)].slice(0, 200).join(''), date: '', currency: b.cost.currency!, amount: '', category: b.cost.category, source,
    allocation: '', costAmount: '', rate: '', quoteSource: '', method: '', external: '', notes: '', reason: '', payee: 'supplier' };
}
export function costPaymentClaimLines(b: CostPaymentBasis, claim: S['Spend']) {
  const c = b.cost;
  if (claim.kind !== 'claim' || claim.approval_state !== 'approved' || claim.deleted_at !== null || claim.currency !== c.currency
    || claim.current_revision_id !== claim.approved_revision_id || !id.test(claim.creator_id) || !Number.isSafeInteger(claim.version) || claim.version < 1 || !hash.test(claim.content_hash)
    || !c.links.some(v => v.source_type === 'claim' && v.source_id === claim.id && v.source_revision_id === claim.approved_revision_id)) return [];
  return claim.lines.filter(v => v.funding === 'employee_advance' && v.supplier?.party_id === c.counterparty!.party_id && id.test(v.id) && units(v.declared_amount) > 0n);
}
export function costPaymentBody(b: CostPaymentBasis, text: CostPaymentText, party: S['SpendSupplier'] | null, files: string[], claim: S['Spend'] | null, lineID: string, actual: boolean, reviewed: boolean): S['CostPaymentRecord'] {
  const c = checkedCostPaymentBasis(b, b.cost.id).cost;
  if (!copyCostPaymentText(text) || !actual || !reviewed) return fail('请明确核对实际已经付款，以及本笔费用、金额和收款依据。');
  if (!validText(text.title, 200, true) || !validText(text.category, 50, true) || !validText(text.reason, 200, true) || !key.test(text.source.trim()) || text.source.startsWith('reversal:')) return fail('请填写标题、类别、永久资金来源标识及登记原因。');
  const time = new Date(text.date + '+08:00');
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(text.date) || !Number.isFinite(time.getTime()) || new Date(time.getTime() + 8 * 3600000).toISOString().slice(0, 19) !== text.date) return fail('请填写真实付款的香港时间，精确到秒。');
  if (!/^[A-Z]{3}$/.test(text.currency) || units(text.amount) <= 0n || units(text.allocation) <= 0n || units(text.allocation) > units(text.amount)) return fail('请分别填写实际付款总额与本笔核销付款额，核销额不得超过总额。');
  let quantum = 1n; for (let i = settlementExponent(text.currency); i < 6; i++) quantum *= 10n;
  if (units(text.amount) % quantum !== 0n) return fail('实际付款总额须符合所选币种精度。');
  if (!validText(text.method, 100, true)) return fail('请填写实际付款方式。');
  if (!files.length || files.length > 20 || files.some(v => !id.test(v)) || new Set(files).size !== files.length) return fail('请选择至少一份准确付款凭证版本。');
  let fixedClaim: S['CostPaymentClaim'] | null = null, employee: string | null = null, fixedParty: S['SpendSupplier'] | null = null;
  if (text.payee === 'employee') {
    if (!claim || !costPaymentClaimLines(b, claim).some(v => v.id === lineID)) return fail('请选择费用已关联的准确批准员工垫付报销及明细。');
    employee = claim.creator_id;
    fixedClaim = { id: claim.id, version: claim.version, revision_id: claim.approved_revision_id!, content_hash: claim.content_hash, line_id: lineID };
  } else {
    if (claim || lineID || !party || party.party_id !== c.counterparty!.party_id || !id.test(party.revision_id)) return fail('直接付款须明确选择费用同一供应方的准确修订。');
    fixedParty = { party_id: party.party_id, revision_id: party.revision_id };
  }
  const fx = reviewedPaymentFX({ currency: text.currency } as S['Payment'], c, text.allocation, text.currency !== c.currency ? { cost_amount: text.costAmount, rate: text.rate, source: text.quoteSource, acknowledged: true } : undefined);
  if (text.currency === c.currency && (text.costAmount || text.rate || text.quoteSource)) return fail('同币核销不接受额外异币依据。');
  if (units(fx?.cost_amount ?? text.allocation) > units(c.amount)) return fail('本次费用原币核销额不得超过原费用金额。');
  return { title: text.title.trim(), direction: 'outgoing', paid_at: time.toISOString(), currency: text.currency, amount: text.amount, category: text.category.trim(),
    counterparty: fixedParty, employee_id: employee, refund_of_id: null, method_label: text.method.trim(), external_ref: text.external.trim(), evidence_version_ids: [...files], notes: text.notes,
    cost: { id: c.id, version: c.version, revision_id: c.current_revision_id, content_hash: c.content_hash }, claim: fixedClaim, allocation_amount: text.allocation,
    ...(fx ? { fx } : {}), source_key: text.source.trim(), reason: text.reason.trim(), actual_paid: true, settlement_reviewed: true };
}
export function costPaymentReceipt(raw: unknown, b: CostPaymentBasis, body: S['CostPaymentRecord'], actor: string): raw is S['CostPaymentReceipt'] {
  try {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return false;
    const { payment: p, allocation: a } = raw as S['CostPaymentReceipt'];
    if (!p || !a || !id.test(p.id) || p.version !== 3 || p.record_kind !== 'payment' || p.state !== 'recorded' || p.direction !== 'outgoing'
      || p.source_key !== body.source_key || p.project_id !== b.cost.project_id || p.classification !== b.cost.classification || p.creator_id !== actor
      || !id.test(p.current_revision_id) || !hash.test(p.content_hash) || p.reversal_id !== null || p.reverses_id !== null || p.refund_of_id !== null || p.refund_fx !== undefined
      || p.title !== body.title || p.currency !== body.currency || units(p.amount) !== units(body.amount) || Date.parse(p.paid_at!) !== Date.parse(body.paid_at!)
      || p.category !== body.category || p.method_label !== body.method_label || p.external_ref !== body.external_ref || p.notes !== body.notes
      || p.employee_id !== body.employee_id || (p.counterparty?.party_id ?? null) !== (body.counterparty?.party_id ?? null) || (p.counterparty?.revision_id ?? null) !== (body.counterparty?.revision_id ?? null)
      || !Array.isArray(p.evidence_version_ids) || new Set(p.evidence_version_ids).size !== p.evidence_version_ids.length || p.evidence_version_ids.length !== body.evidence_version_ids.length || !p.evidence_version_ids.every(v => body.evidence_version_ids.includes(v))
      || units(p.effective_allocated_amount) !== units(body.allocation_amount) || units(p.refunded_amount) !== 0n || units(p.contract_return_reserved_amount) !== 0n || units(p.unallocated_amount) !== units(body.amount) - units(body.allocation_amount)
      || a.payment_id !== p.id || a.cost_id !== body.cost.id || a.cost_revision_id !== body.cost.revision_id || a.kind !== 'allocation' || a.actor_id !== actor || a.reason !== body.reason
      || a.claim_id !== (body.claim?.id ?? null) || a.claim_revision_id !== (body.claim?.revision_id ?? null) || a.claim_line_id !== (body.claim?.line_id ?? null)
      || units(a.remaining_amount) !== units(a.amount) || units(a.remaining_cost_amount) !== units(a.cost_amount)
      || !allocationFXReceipt(a, { cash: body.allocation_amount, cost: body.fx?.cost_amount ?? body.allocation_amount, fx: body.fx, paymentCurrency: body.currency!, costCurrency: b.cost.currency!, quotedOn: p.paid_on! })) return false;
    return a.cost_shares!.every(v => b.cost.allocations.some(s => s.project_id === v.project_id && units(v.amount) <= units(s.amount)));
  } catch { return false; }
}
