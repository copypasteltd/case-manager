import { describe, expect, it } from 'vitest';
import { checkedCleanupPlan, checkedCleanupReceipts, cleanupCreate, cleanupDecision, cleanupProgress } from '@copypaste/contracts/multipart-cleanup';
import { candidate, cleanupID as id, cleanupNow as now, cleanupPlan as plan } from '../../packages/contracts/test/multipart-cleanup-fixture';

describe('fixed multipart cleanup and human decisions', () => {
  it('rejects an empty, duplicate, stale or blocked selection', () => {
    const row = candidate(), selected = { upload_id: row.upload_id, expected_version: row.version };
    for (const selection of [[], [selected, selected], [{ ...selected, expected_version: 1 }], [{ ...selected, upload_id: id(99) }]]) expect(() => cleanupCreate([row], selection, 'review')).toThrow();
    expect(() => cleanupCreate([{ ...row, eligible: false }], [selected], 'review')).toThrow();
    expect(() => cleanupCreate([{ ...row, blocking_reasons: ['RETENTION_HOLD'] }], [selected], 'review')).toThrow();
  });
  it('keeps exact selection revisions and preserves large byte sizes as strings', () => {
    const row = candidate(); expect(cleanupCreate([row], [{ upload_id: row.upload_id, expected_version: row.version }], ' review ').reason).toBe('review');
    expect(checkedCleanupPlan(plan(), id(4)).snapshot.items[0].size_bytes).toBe('120000000000');
  });
  it('rejects a different folder or plan and missing or repeated execution membership', () => {
    expect(() => checkedCleanupPlan(plan(), id(99))).toThrow(); expect(() => checkedCleanupPlan(plan(), id(4), id(99))).toThrow();
    const p = plan(); for (const value of [{ ...p, items: [] }, { ...p, items: [{ ...p.items[0], upload_id: id(99) }] }, { ...p, items: [...p.items, ...p.items], snapshot: { ...p.snapshot, items: [...p.snapshot.items, ...p.snapshot.items] } }]) expect(() => checkedCleanupPlan(value, id(4))).toThrow();
  });
  it('does not accept a short observation window or an invented successful result', () => {
    const p = plan(); expect(() => checkedCleanupPlan({ ...p, ready_after: p.created_at }, id(4))).toThrow();
    expect(() => checkedCleanupPlan({ ...p, items: [{ ...p.items[0], state: 'succeeded' }] }, id(4))).toThrow();
  });
  it('requires an explicit acknowledgement, reason and current approval window', () => {
    const p = plan(); expect(() => cleanupDecision(p, 'review', false, 'approve', now)).toThrow(); expect(() => cleanupDecision(p, '', true, 'approve', now)).toThrow();
    expect(() => cleanupDecision(p, 'review', true, 'approve', Date.parse(p.ready_after) - 1)).toThrow(); expect(() => cleanupDecision(p, 'review', true, 'approve', Date.parse(p.expires_at))).toThrow();
    expect(cleanupDecision(p, 'review', true, 'approve', now).snapshot_sha256).toBe(p.snapshot_sha256);
  });
  it('allows cancellation during observation and after expiry without accepting approval', () => {
    const p = plan(); expect(cleanupDecision(p, 'stop', true, 'cancel', Date.parse(p.created_at)).expected_version).toBe(1);
    expect(cleanupDecision(p, 'stop', true, 'cancel', Date.parse(p.expires_at) + 1).expected_version).toBe(1);
    expect(() => cleanupDecision({ ...p, state: 'cancelled' }, 'stop', true, 'cancel', now)).toThrow();
  });
  it('counts only confirmed absence as completion, including after cancellation', () => {
    const p = plan(); expect(cleanupProgress({ ...p, state: 'approved', items: [{ ...p.items[0], state: 'running' }] })).toEqual({ total: 1, confirmed: 0, failed: 0, pending: 1 });
    expect(cleanupProgress({ ...p, state: 'cancelled', items: [{ ...p.items[0], state: 'succeeded', confirmed_at: p.ready_after }] }).confirmed).toBe(1);
  });
  it('rejects forged, repeated and incomplete receipt records', () => {
    const row = { id: id(8), generation: 1, attempt: 1, outcome: 'started' as const, error_code: null, created_at: plan().created_at };
    expect(checkedCleanupReceipts([row])).toHaveLength(1); expect(() => checkedCleanupReceipts([row, row])).toThrow(); expect(() => checkedCleanupReceipts([{ ...row, attempt: 0 }])).toThrow();
  });
});
