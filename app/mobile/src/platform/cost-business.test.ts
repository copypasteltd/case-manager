import { test } from 'node:test';
import assert from 'node:assert/strict';
import { businessCostBody, businessCostSeed, businessCostReceipt, checkedBusinessCostBasis, type BusinessCostBasis } from '../../../packages/contracts/src/cost-business';
import { MemoryDrafts } from './drafts';
import { costContent, financialEvidence, financialInput } from './financial-input';
const sourceID = '713cfcdf-79a5-4a4e-b9df-e8ac234cffca', revision = '813cfcdf-79a5-4a4e-b9df-e8ac234cffca';
const basis = (): BusinessCostBasis => ({ source: { type: 'claim', id: sourceID, version: 7, revision_id: revision, content_hash: 'a'.repeat(64) }, project_id: null, classification: 'internal', currency: 'HKD', approved_amount: '50', title: 'Approved claim', as_of: '2026-10-30T00:00:00Z' });
test('restored mobile financial text never supplies fixed evidence, MFA or actual-fact consent', () => {
  const b = basis(), drafts = new MemoryDrafts(() => ({ authenticated: true, epoch: 1 })), auth = { epoch: 1, userID: 'human', authorizationVersion: 2 };
  const target = { kind: 'financial-business-record' as const, id: 'claim:' + sourceID, version: b.source.version, revision, hash: b.source.content_hash }, handle = drafts.open(auth, target)!;
  const input = { ...financialInput('cost', undefined, 'manual:once'), title: 'Actual expense', date: '2025-02-10', currency: 'HKD', amount: '10', category: 'service', reason: 'Reviewed', allocations: [{ project: '', amount: '10' }] };
  assert.equal(handle.write({ reason: '', action: 'save', financial: input, actual_incurred: true, source_reviewed: true, fresh: true, evidence_version_ids: [revision] } as never), true);
  const restored = handle.read()!; assert.equal(restored.financial?.amount, '10'); for (const name of ['actual_incurred', 'source_reviewed', 'fresh', 'evidence_version_ids']) assert.equal(name in restored, false);
  assert.throws(() => businessCostBody(b, costContent(restored.financial!, financialEvidence()), input.source, input.reason, true, true));
  for (const changed of [{ version: 8 }, { revision: sourceID }, { hash: 'b'.repeat(64) }, { id: 'purchase:' + sourceID }]) assert.equal(drafts.open(auth, { ...target, ...changed })!.read(), undefined);
  drafts.open({ ...auth, authorizationVersion: 3 }, target); assert.equal(handle.read(), undefined);
});
test('company expense input needs explicit allocation and fresh exact source context', () => {
  const b = basis(), seed = businessCostSeed(b); assert.equal(seed.amount, null); assert.equal(seed.occurred_on, null); assert.deepEqual(seed.allocations, []);
  const input = { ...financialInput('cost', undefined, 'manual:once'), title: 'Actual', date: '2025-02-10', currency: 'HKD', amount: '0.01', category: 'service', reason: 'Reviewed', allocations: [{ project: '', amount: '0.01' }] }, evidence = { ...financialEvidence(), party: { party_id: sourceID, revision_id: revision }, files: [revision] };
  const body = businessCostBody(b, costContent(input, evidence), input.source, input.reason, true, true); assert.deepEqual(body.allocations, [{ project_id: null, amount: '0.01' }]); assert.equal(body.source.version, 7);
  assert.throws(() => checkedBusinessCostBasis(b, 'purchase', sourceID)); assert.throws(() => businessCostBody(b, costContent(input, evidence), input.source, input.reason, false, true)); assert.equal(businessCostReceipt({ ...body, version: 1, state: 'draft' }, b, body), false);
});
