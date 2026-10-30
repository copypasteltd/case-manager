import { test } from 'node:test';
import assert from 'node:assert/strict';
import { copyCostPaymentText, costPaymentBody, costPaymentRoute, costPaymentSeed, type CostPaymentBasis } from '../../../packages/contracts/src/cost-payment';
import { MemoryDrafts } from './drafts';
const costID = '713cfcdf-79a5-4a4e-b9df-e8ac234cffca', revision = '813cfcdf-79a5-4a4e-b9df-e8ac234cffca';
const basis = { cost: { id: costID, version: 3, current_revision_id: revision, content_hash: 'a'.repeat(64), state: 'confirmed', entry_kind: 'expense', reversal_id: null, reverses_id: null, title: 'Actual expense', currency: 'HKD', amount: '100', category: 'service', project_id: null, classification: 'internal', counterparty: { party_id: costID, revision_id: revision }, allocations: [{ project_id: null, amount: '100' }], links: [] }, as_of: '2026-10-30T00:00:00Z' } as unknown as CostPaymentBasis;
test('mobile cash draft retains bounded text only and is bound to exact expense and current authority', () => {
  const drafts = new MemoryDrafts(() => ({ authenticated: true, epoch: 1 })), auth = { epoch: 1, userID: 'human', authorizationVersion: 2 }, target = { kind: 'cost-payment' as const, id: costID, version: 3, revision, hash: basis.cost.content_hash }, handle = drafts.open(auth, target)!;
  const text = { ...costPaymentSeed(basis, 'manual:once'), amount: '60', allocation: '40', date: '2025-02-10T09:30:00', method: 'Bank', reason: 'Reviewed' };
  assert.equal(handle.write({ reason: '', action: 'recordPayment', costPayment: { ...text, file_version_ids: [revision], claim_id: costID, fresh: true }, actual_paid: true } as never), true);
  const restored = handle.read()!; assert.deepEqual(restored.costPayment, text); assert.equal('actual_paid' in restored, false); assert.equal('fresh' in restored.costPayment!, false); assert.equal('claim_id' in restored.costPayment!, false);
  assert.throws(() => costPaymentBody(basis, restored.costPayment!, null, [], null, '', true, true));
  for (const patch of [{ version: 4 }, { revision: costID }, { hash: 'b'.repeat(64) }]) assert.equal(drafts.open(auth, { ...target, ...patch })!.read(), undefined);
  drafts.open({ ...auth, authorizationVersion: 3 }, target); assert.equal(handle.read(), undefined);
});
test('mobile routing and text bounds cannot introduce cash facts or external scope', () => {
  assert.equal(costPaymentRoute({ kind: 'cost', action: 'recordPayment', id: costID }), costID); assert.throws(() => costPaymentRoute({ kind: 'cost', action: 'recordPayment', id: costID, project_id: revision }));
  assert.equal(copyCostPaymentText({ ...costPaymentSeed(basis, 'manual:once'), reason: 'x'.repeat(201) }), undefined);
  assert.equal(costPaymentSeed(basis, 'manual:once').amount, ''); assert.equal(costPaymentSeed(basis, 'manual:once').allocation, '');
});
