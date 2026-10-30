import { sha256 } from '@noble/hashes/sha2.js';
import { contractBodyBytes } from './contract-body';
import { checkedDualAllocation } from './payment-fx';
import type { components } from './api';

type S = components['schemas'];
export type FeePreview = S['PaymentFeePreview'];
export type FeeBasis = S['PaymentFeeBasis'];
export type FeeLink = S['PaymentFeeLink'];
export type FeeLedger = S['PaymentFeeLedger'];
type Payment = S['Payment'];
const id = (v: string) => /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v);
const hash = (v: string) => /^[a-f0-9]{64}$/.test(v);
const version = (v: number) => Number.isSafeInteger(v) && v >= 0;
const positive = (v: string) => /^\d+(\.\d{1,6})?$/.test(v) && /[1-9]/.test(v);
const canonical = (v: unknown): string => Array.isArray(v) ? '[' + v.map(canonical).join(',') + ']' : v && typeof v === 'object' ? '{' + Object.keys(v).sort().map(k => JSON.stringify(k) + ':' + canonical((v as Record<string, unknown>)[k])).join(',') + '}' : JSON.stringify(v);
function why(v: string) { const s = v.trim(); if (!s || [...s].length > 200 || /[\u0000-\u001f\u007f]/.test(s)) throw new Error('请填写1至200字手续费操作依据。'); return s; }
export function paymentFeeDigest(text: string) { return [...sha256(contractBodyBytes(text))].map(v => v.toString(16).padStart(2, '0')).join(''); }
export function checkedFeeBasis(raw: { basis: string; basis_sha256: string }, parent: Payment, exact = false): FeeBasis {
  if (typeof raw.basis !== 'string' || contractBodyBytes(raw.basis).length > 131072 || !hash(raw.basis_sha256) || paymentFeeDigest(raw.basis) !== raw.basis_sha256) throw new Error('手续费依据摘要不完整，请重新读取。');
  const v: FeeBasis = JSON.parse(raw.basis);
  for (const fact of [v.payment, v.fee_payment, v.cost]) {
    if (!fact || !id(fact.id) || !version(fact.version) || fact.version < 1 || !id(fact.revision_id) || !hash(fact.content_hash) || !fact.title?.trim() || !positive(fact.amount) || fact.project_id !== parent.project_id || fact.classification !== parent.classification) throw new Error('手续费原事实范围或准确版本无效。');
  }
  if (v.payment.id !== parent.id || v.payment.revision_id !== parent.current_revision_id || v.payment.content_hash !== parent.content_hash || v.payment.version > parent.version || exact && v.payment.version !== parent.version) throw new Error('原付款已变化，请刷新资料后重新核对。');
  const a = checkedDualAllocation(v.allocation);
  if (a.kind !== 'allocation' || a.effect !== 1 || a.id !== a.root_id || a.payment_id !== v.fee_payment.id || a.cost_id !== v.cost.id || a.cost_revision_id !== v.cost.revision_id || a.payment_currency !== v.fee_payment.currency || a.cost_currency !== v.cost.currency || !positive(a.amount) || !positive(a.cost_amount)) throw new Error('手续费须引用准确的实际费用与原资金分配。');
  return v;
}
export function checkedFeePreview(raw: FeePreview, parent: Payment): FeePreview {
  if (!raw || raw.payment_id !== parent.id || !version(raw.fee_version)) throw new Error('手续费核对范围无效。');
  const b = checkedFeeBasis(raw, parent, true), a = checkedDualAllocation(b.allocation);
  if (!positive(a.remaining_amount) || !positive(a.remaining_cost_amount)) throw new Error('所选分配已无有效手续费余额。');
  return raw;
}
export function checkedFeeLedger(raw: FeeLedger, parent: Payment): FeeLedger {
  if (!raw || raw.payment_id !== parent.id || !version(raw.fee_version) || typeof raw.can_manage !== 'boolean' || !Array.isArray(raw.items) || raw.items.length > 100 || raw.next_cursor !== null && (typeof raw.next_cursor !== 'string' || !raw.next_cursor)) throw new Error('手续费历史列表不完整。');
  const ids = new Set<string>();
  for (const { link, current, state } of raw.items) {
    if (!link || !id(link.id) || ids.has(link.id) || link.payment_id !== parent.id || !version(link.version) || link.version < 1 || link.version > raw.fee_version || !id(link.actor_id) || !link.reason?.trim() || !Number.isFinite(Date.parse(link.created_at))) throw new Error('手续费历史回执无效。');
    ids.add(link.id);
    const b = checkedFeeBasis(link, parent), a = checkedDualAllocation(current);
    const fixed = (v: S['PaymentAllocation']) => { const { remaining_amount: _cash, remaining_cost_amount: _cost, ...rest } = v; return rest; };
    if (canonical(fixed(a)) !== canonical(fixed(b.allocation))) throw new Error('当前手续费余额引用了其他原分配。');
    const rev = link.revocation;
    if (rev && (!id(rev.id) || !id(rev.actor_id) || !version(rev.version) || rev.version <= link.version || rev.version > raw.fee_version || !rev.reason?.trim() || !Number.isFinite(Date.parse(rev.created_at)))) throw new Error('手续费撤销历史无效。');
    const expected = rev ? 'revoked' : positive(a.remaining_amount) && positive(a.remaining_cost_amount) ? 'active' : 'exhausted';
    if (state !== expected) throw new Error('手续费有效状态与真实余额不一致。');
  }
  return raw;
}
function base(parent: Payment, reason: string) { return { expected_version: parent.version, revision_id: parent.current_revision_id, content_hash: parent.content_hash, reason: why(reason) }; }
export function makePaymentFee(parent: Payment, preview: FeePreview, reason: string): S['PaymentFeeCreate'] {
  checkedFeePreview(preview, parent); const b = checkedFeeBasis(preview, parent, true);
  return { ...base(parent, reason), expected_fee_version: preview.fee_version, fee_payment_id: b.fee_payment.id, allocation_id: b.allocation.id, basis_sha256: preview.basis_sha256, reviewed: true };
}
export function revokePaymentFee(parent: Payment, link: FeeLink, feeVersion: number, reason: string): S['PaymentFeeRevoke'] {
  checkedFeeBasis(link, parent);
  if (link.revocation || !version(feeVersion) || feeVersion < link.version) throw new Error('请先读取尚未撤销的准确关联。');
  return { ...base(parent, reason), expected_fee_version: feeVersion, basis_sha256: link.basis_sha256, reviewed: true };
}
export function checkedFeeReceipt(raw: FeeLink, parent: Payment, expected: S['PaymentFeeCreate'] | S['PaymentFeeRevoke'], actor: string, original?: FeeLink) {
  const b = checkedFeeBasis(raw, parent);
  if (raw.payment_id !== parent.id || raw.basis_sha256 !== expected.basis_sha256 || !id(raw.id)) throw new Error('手续费回执与所核对依据不符，请刷新历史。');
  if ('allocation_id' in expected) {
    if (raw.revocation || raw.version !== expected.expected_fee_version + 1 || raw.actor_id !== actor || raw.reason !== expected.reason || b.allocation.id !== expected.allocation_id || b.fee_payment.id !== expected.fee_payment_id) throw new Error('手续费关联回执尚未核对。');
  } else if (!original || raw.id !== original.id || raw.basis !== original.basis || raw.version !== original.version || !raw.revocation || raw.revocation.version !== expected.expected_fee_version + 1 || raw.revocation.actor_id !== actor || raw.revocation.reason !== expected.reason) throw new Error('手续费撤销回执尚未核对。');
  return raw;
}
