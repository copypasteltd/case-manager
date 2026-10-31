import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
function expect(value: unknown) { return { toBe: (other: unknown) => assert.equal(value, other), toEqual: (other: unknown) => assert.deepEqual(value, other), toThrow: () => assert.throws(value as () => unknown), toHaveLength: (size: number) => assert.equal((value as { length: number }).length, size), toContain: (text: string) => assert.ok((value as string).includes(text)) }; }
import { storagePath, storageHistoryPath, storageRetryInput, checkedStorageReceipt, type StorageDetails, type StorageReceipt } from '../../../packages/contracts/src/storage-operations';
const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const details = (): StorageDetails => ({ can_open: true, can_retry: true, retry_block: 'READY_FOR_REVIEW', item: { id: id(1), version: 4, provider: 'r2', state: 'failed', file_id: id(2), file_version_id: id(3), file_revision: 2, file_state: 'active', availability: 'available', is_current: true, storage_epoch: 1, primary_provider: 'oss', source_available: true, error_code: 'REPLICA_HASH_MISMATCH', job: { id: id(4), state: 'dead', attempt: 3, max_attempts: 3, updated_at: '2026-10-31T01:00:00Z' }, created_at: '2026-10-30T01:00:00Z', updated_at: '2026-10-31T01:00:00Z', basis_sha256: 'a'.repeat(64) } });
function receipt(d = details()): StorageReceipt {
  const { basis_sha256, ...before } = d.item;
  return { id: id(5), actor_id: id(6), business_operation_id: id(7), replica_id: before.id, file_id: before.file_id, file_version_id: before.file_version_id, expected_version: before.version, result_version: before.version + 1, basis_sha256, before, after: { ...before, version: before.version + 1, state: 'queued', error_code: null, job: null, updated_at: '2026-10-31T01:01:00Z' }, created_at: '2026-10-31T01:01:00Z' };
}
describe('reviewed file replication operations', () => {
  it('restricts query filters and exact replica history IDs', () => {
    expect(storagePath('r2', 'failed', 'dead')).toBe('/storage/replicas?limit=20&provider=r2&state=failed&job_state=dead');
    expect(storageHistoryPath(id(1))).toContain('replica_id=' + id(1));
    for (const value of ['__proto__', 'bucket', 'r2&secret=x']) expect(() => storagePath(value)).toThrow();
    expect(() => storageHistoryPath('../files')).toThrow();
  });
  it('requires explicit review, current retry authority and a bounded human reason', () => {
    expect(storageRetryInput(details(), ' Human review ', true)).toEqual({ expected_version: 4, basis_sha256: 'a'.repeat(64), reviewed: true, reason: 'Human review' });
    expect(() => storageRetryInput(details(), 'Review', false)).toThrow();
    expect(() => storageRetryInput({ ...details(), can_retry: false }, 'Review', true)).toThrow();
    for (const reason of [' ', '字'.repeat(201)]) expect(() => storageRetryInput(details(), reason, true)).toThrow();
    expect(storageRetryInput(details(), '😀'.repeat(200), true).reason).toHaveLength(400);
  });
  it('rejects stale or unsafe replica and active job snapshots', () => {
    for (const change of [{ state: 'verified' }, { availability: 'purged' }, { source_available: false }, { primary_provider: 'r2' }, { version: 0 }, { version: Number.MAX_SAFE_INTEGER }, { basis_sha256: 'invalid' }, { job: { ...details().item.job!, state: 'running' } }]) {
      const d = details(); Object.assign(d.item, change); expect(() => storageRetryInput(d, 'Review', true)).toThrow();
    }
  });
  it('accepts the exact historical queue receipt and JSONB key reordering', () => {
    const r = receipt();
    r.before = Object.fromEntries(Object.entries(r.before).reverse()) as StorageReceipt['before'];
    r.before.job = Object.fromEntries(Object.entries(r.before.job!).reverse()) as StorageReceipt['before']['job'];
    expect(checkedStorageReceipt(r, details())).toBe(r);
  });
  it('rejects wrong targets, missing operations and changed versions or review basis', () => {
    for (const change of [{ file_id: id(9) }, { replica_id: id(9) }, { file_version_id: id(9) }, { business_operation_id: '' }, { result_version: 7 }, { basis_sha256: 'b'.repeat(64) }, { created_at: 'invalid' }]) {
      const r = receipt(); Object.assign(r, change); expect(() => checkedStorageReceipt(r, details())).toThrow();
    }
  });
  it('rejects changed before-state even when replica version has not changed', () => {
    for (const change of [{ state: 'missing' }, { file_revision: 3 }, { job: { ...details().item.job!, updated_at: '2026-10-31T02:00:00Z' } }]) {
      const r = receipt(); Object.assign(r.before, change); expect(() => checkedStorageReceipt(r, details())).toThrow();
    }
  });
  it('never accepts verified state or an unbound target as proof of queuing', () => {
    for (const change of [{ state: 'verified' }, { file_id: id(9) }, { provider: 'oss' }, { version: 6 }, { error_code: 'OTHER_STORAGE_FAILURE' }, { job: details().item.job }, { source_available: false }, { updated_at: '2026-10-30T01:00:00Z' }]) {
      const r = receipt(); Object.assign(r.after, change); expect(() => checkedStorageReceipt(r, details())).toThrow();
    }
    for (const value of [null, [], {}]) expect(() => checkedStorageReceipt(value, details())).toThrow();
  });
});
