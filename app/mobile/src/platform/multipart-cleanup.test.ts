import { strict as assert } from 'node:assert';
import { describe, it } from 'node:test';
import { checkedCleanupPlan, checkedCleanupReceipts, cleanupCreate, cleanupDecision, cleanupProgress } from '../../../packages/contracts/src/multipart-cleanup';
import { candidate, cleanupID as id, cleanupNow as now, cleanupPlan as plan } from '../../../packages/contracts/test/multipart-cleanup-fixture';

describe('four client multipart administration', () => {
  it('blocks stale selections and never silently substitutes refreshed revisions', () => {
    const row = candidate(), selection = [{ upload_id: row.upload_id, expected_version: row.version }];
    assert.throws(() => cleanupCreate([{ ...row, version: 3 }], selection, 'review')); assert.throws(() => cleanupCreate([{ ...row, eligible: false }], selection, 'review'));
    assert.equal(cleanupCreate([row], selection, 'review').items[0].expected_version, 2);
  });
  it('checks exact folder, plan, upload and policy membership in command receipts', () => {
    const p = plan(); assert.throws(() => checkedCleanupPlan(p, id(99))); assert.throws(() => checkedCleanupPlan(p, id(4), id(99)));
    assert.throws(() => checkedCleanupPlan({ ...p, snapshot: { ...p.snapshot, items: [{ ...candidate(), policy: null }] } }, id(4)));
    assert.throws(() => checkedCleanupPlan({ ...p, items: [{ ...p.items[0], upload_id: id(99) }] }, id(4)));
  });
  it('does not permit a gesture to bypass explicit confirmation or observation', () => {
    assert.throws(() => cleanupDecision(plan(), 'review', false, 'approve', now)); assert.throws(() => cleanupDecision(plan(), 'review', true, 'approve', 0));
    assert.equal(cleanupDecision(plan(), 'review', true, 'approve', now).snapshot_sha256, plan().snapshot_sha256);
  });
  it('preserves cancelled history and distinguishes queued work from completion', () => {
    const p = plan(); assert.equal(cleanupProgress({ ...p, state: 'approved', items: [{ ...p.items[0], state: 'queued' }] }).confirmed, 0);
    assert.throws(() => checkedCleanupPlan({ ...p, items: [{ ...p.items[0], state: 'succeeded', confirmed_at: null }] }, id(4)));
    assert.equal(cleanupProgress({ ...p, state: 'cancelled', items: [{ ...p.items[0], state: 'succeeded', confirmed_at: p.ready_after }] }).confirmed, 1);
  });
  it('requires a bounded reason and keeps exact large byte metadata', () => {
    const row = candidate(), selection = [{ upload_id: row.upload_id, expected_version: 2 }];
    assert.throws(() => cleanupCreate([row], selection, ' ')); assert.throws(() => cleanupCreate([row], selection, '字'.repeat(201)));
    assert.equal(checkedCleanupPlan(plan(), id(4)).snapshot.items[0].size_bytes, '120000000000');
  });
  it('rejects duplicate receipts and a nonpositive execution generation', () => {
    const r = { id: id(9), generation: 1, attempt: 1, outcome: 'confirmed_absent' as const, error_code: null, created_at: plan().ready_after };
    assert.equal(checkedCleanupReceipts([r]).length, 1); assert.throws(() => checkedCleanupReceipts([r, r])); assert.throws(() => checkedCleanupReceipts([{ ...r, generation: 0 }]));
  });
});
