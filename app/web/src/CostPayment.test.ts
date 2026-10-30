import { describe, expect, it } from 'vitest';
import { checkedCostPaymentBasis, copyCostPaymentText, costPaymentBody, costPaymentClaimLines, costPaymentPath, costPaymentReceipt, costPaymentRoute, costPaymentSeed, type CostPaymentBasis } from '@copypaste/contracts/cost-payment';
import { ApiError, type Schemas } from './api';
import { costPaymentOutcomeUncertain } from './cost-payment-outcome';
const costID = '713cfcdf-79a5-4a4e-b9df-e8ac234cffca', revision = '813cfcdf-79a5-4a4e-b9df-e8ac234cffca', partyID = '913cfcdf-79a5-4a4e-b9df-e8ac234cffca', paymentID = 'a13cfcdf-79a5-4a4e-b9df-e8ac234cffca', allocationID = 'b13cfcdf-79a5-4a4e-b9df-e8ac234cffca', actor = 'c13cfcdf-79a5-4a4e-b9df-e8ac234cffca';
const basis = (): CostPaymentBasis => ({ cost: { id: costID, version: 3, current_revision_id: revision, content_hash: 'a'.repeat(64), state: 'confirmed', entry_kind: 'expense', reversal_id: null, reverses_id: null, title: 'Actual expense', currency: 'HKD', amount: '100', project_id: null, classification: 'internal', category: 'service', counterparty: { party_id: partyID, revision_id: revision }, allocations: [{ project_id: null, amount: '100' }], links: [] } as unknown as Schemas['Cost'], as_of: '2026-10-30T00:00:00Z' });
const text = () => ({ ...costPaymentSeed(basis(), 'manual:actual-payment-once'), date: '2025-02-10T09:30:00', amount: '60', allocation: '40', method: ' Bank transfer ', external: ' external-1 ', reason: 'Human checked actual cash and expense' });
const body = () => costPaymentBody(basis(), text(), { party_id: partyID, revision_id: revision }, [revision], null, '', true, true);
const receipt = () => ({ payment: { ...body(), id: paymentID, version: 3, state: 'recorded', record_kind: 'payment', project_id: null, classification: 'internal', creator_id: actor, current_revision_id: revision, content_hash: 'b'.repeat(64), reversal_id: null, reverses_id: null, paid_on: '2025-02-10', effective_allocated_amount: '40', refunded_amount: '0', contract_return_reserved_amount: '0', unallocated_amount: '20' }, allocation: { id: allocationID, payment_id: paymentID, cost_id: costID, cost_revision_id: revision, kind: 'allocation', effect: 1, actor_id: actor, reason: body().reason, claim_id: null, claim_revision_id: null, claim_line_id: null, remaining_amount: '40', remaining_cost_amount: '40', amount: '40', cost_amount: '40', cost_currency: 'HKD', payment_currency: 'HKD', cost_exponent: 2, payment_exponent: 2, shares: [{ project_id: null, amount: '40' }], cost_shares: [{ project_id: null, amount: '40' }], fx: null, root_id: allocationID, source_allocation_id: null } });
describe('Expense to actual cash settlement', () => {
  it('blocks edited repeat submissions after missing unreadable server or post-response failures', () => {
    for (const [status, code] of [[0, 'RESULT_UNCONFIRMED'], [201, 'INVALID_RESPONSE'], [502, 'REQUEST_FAILED']] as const) expect(costPaymentOutcomeUncertain(new ApiError(status, code, 'Unknown result', ''), false)).toBe(true);
    expect(costPaymentOutcomeUncertain(new Error('Receipt or navigation failure'), true)).toBe(true);
    expect(costPaymentOutcomeUncertain(new Error('Input needs correction'), false)).toBe(false);
    expect(costPaymentOutcomeUncertain(new ApiError(422, 'VALIDATION_FAILED', 'Rejected', ''), false)).toBe(false);
  });
  it('prefills descriptive context while leaving actual date cash amount and allocation blank', () => {
    const v = costPaymentSeed(basis(), 'manual:once'); for (const k of ['date', 'amount', 'allocation', 'method', 'costAmount', 'rate']) expect(v[k as keyof typeof v]).toBe(''); expect(v.currency).toBe('HKD');
    for (const patch of [{ state: 'draft' }, { entry_kind: 'reversal' }, { version: 0 }, { amount: '0' }, { allocations: [] }, { counterparty: null }]) expect(() => checkedCostPaymentBasis({ ...basis(), cost: { ...basis().cost, ...patch } }, costID)).toThrow();
  });
  it('keeps total cash and assigned cash independent with precise strings and fixed references', () => {
    const v = body(); expect(v.amount).toBe('60'); expect(v.allocation_amount).toBe('40'); expect(v.method_label).toBe('Bank transfer'); expect(v.external_ref).toBe('external-1'); expect(v.paid_at).toBe('2025-02-10T01:30:00.000Z'); expect(v.cost.content_hash).toBe('a'.repeat(64)); expect('project_id' in v || 'classification' in v).toBe(false);
    for (const patch of [{ amount: '0' }, { amount: '39.99' }, { amount: '60.001' }, { allocation: '100.01' }, { date: '2025-02-29T09:30:00' }, { method: '' }]) expect(() => costPaymentBody(basis(), { ...text(), ...patch }, v.counterparty, [revision], null, '', true, true)).toThrow();
    for (const [actual, reviewed] of [[false, true], [true, false]]) expect(() => costPaymentBody(basis(), text(), v.counterparty, [revision], null, '', actual, reviewed)).toThrow();
  });
  it('requires exact reviewed cross currency arithmetic and keeps unassigned cash separate', () => {
    const v = costPaymentBody(basis(), { ...text(), currency: 'USD', amount: '13', allocation: '6.50', costAmount: '50', rate: '0.13', quoteSource: 'Actual bank conversion' }, body().counterparty, [revision], null, '', true, true); expect(v.fx?.cost_amount).toBe('50'); expect(v.fx?.acknowledged).toBe(true);
    expect(() => costPaymentBody(basis(), { ...text(), costAmount: '40' }, body().counterparty, [revision], null, '', true, true)).toThrow();
    expect(() => costPaymentBody(basis(), { ...text(), currency: 'USD', costAmount: '100', rate: '0.13', quoteSource: 'Bank' }, body().counterparty, [revision], null, '', true, true)).toThrow();
  });
  it('binds an approved employee advance to the linked exact source and supplier', () => {
    const b = basis(); b.cost.links = [{ source_type: 'claim', source_id: paymentID, source_revision_id: revision } as never];
    const claim = { id: paymentID, version: 7, content_hash: 'c'.repeat(64), kind: 'claim', approval_state: 'approved', deleted_at: null, currency: 'HKD', current_revision_id: revision, approved_revision_id: revision, creator_id: actor, lines: [{ id: allocationID, funding: 'employee_advance', declared_amount: '100', supplier: { party_id: partyID, revision_id: revision } }] } as unknown as Schemas['Spend'];
    expect(costPaymentClaimLines(b, claim)).toHaveLength(1); const v = costPaymentBody(b, { ...text(), payee: 'employee' }, null, [revision], claim, allocationID, true, true); expect(v.employee_id).toBe(actor); expect(v.claim?.version).toBe(7); expect(v.claim?.line_id).toBe(allocationID);
    expect(costPaymentClaimLines(b, { ...claim, current_revision_id: costID })).toEqual([]); expect(() => costPaymentBody(b, { ...text(), payee: 'employee' }, null, [revision], claim, costID, true, true)).toThrow();
  });
  it('checks the full original compound receipt and refuses changed money scope payee or dual shares', () => {
    expect(costPaymentReceipt(receipt(), basis(), body(), actor)).toBe(true);
    for (const patch of [{ version: 1 }, { state: 'draft' }, { amount: '40' }, { unallocated_amount: '0' }, { employee_id: actor }, { project_id: partyID }, { evidence_version_ids: [revision, revision] }]) expect(costPaymentReceipt({ ...receipt(), payment: { ...receipt().payment, ...patch } }, basis(), body(), actor)).toBe(false);
    for (const patch of [{ cost_id: partyID }, { claim_line_id: allocationID }, { cost_amount: '39' }, { remaining_amount: '39' }, { cost_shares: [{ project_id: partyID, amount: '40' }] }]) expect(costPaymentReceipt({ ...receipt(), allocation: { ...receipt().allocation, ...patch } }, basis(), body(), actor)).toBe(false);
  });
  it('copies only text and uses a fixed cost route with no scope or refund overrides', () => {
    const v = copyCostPaymentText({ ...text(), files: [revision], claim_id: actor, fresh: true }); expect(v).toEqual(text());
    expect(costPaymentPath(costID)).toBe('/costs/' + costID + '/payment-basis'); expect(costPaymentRoute({ kind: 'cost', action: 'recordPayment', id: costID })).toBe(costID);
    for (const params of [{ kind: 'payment' }, { refund: paymentID }, { project_id: partyID }, { id: costID + '?amount=1' }]) expect(() => costPaymentRoute({ kind: 'cost', action: 'recordPayment', id: costID, ...params })).toThrow();
  });
});
