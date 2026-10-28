import type { CleanupCandidate, CleanupPlan } from '../src/multipart-cleanup';
export const cleanupID = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
export const cleanupNow = Date.parse('2026-10-29T00:00:00Z');
export function candidate(): CleanupCandidate {
  return { upload_id: cleanupID(1), version: 2, name: 'Synthetic abandoned upload.bin', size_bytes: '120000000000', state: 'uploading', created_at: '2026-10-20T00:00:00Z', expires_at: '2026-10-22T00:00:00Z', retain_until: '2026-10-21T00:00:00Z', policy: { id: cleanupID(2), version: 1, kind: 'multipart', enabled: true, retain_seconds: 86400, observation_seconds: 86400 }, source_sha256: 'a'.repeat(64), backend_sha256: 'b'.repeat(64), eligible: true, blocking_reasons: [] };
}
export function cleanupPlan(): CleanupPlan {
  return { id: cleanupID(3), version: 1, state: 'observing', snapshot: { folder_id: cleanupID(4), folder_version: 1, items: [candidate()] }, snapshot_sha256: 'c'.repeat(64), reason: 'Synthetic cleanup review', created_at: '2026-10-25T00:00:00Z', ready_after: '2026-10-26T00:00:00Z', expires_at: '2026-11-02T00:00:00Z', approved_at: null, approval_reason: null, cancelled_at: null, cancellation_reason: null, items: [{ id: cleanupID(5), upload_id: cleanupID(1), version: 1, generation: 1, state: 'observing', attempts: 0, last_error: null, confirmed_at: null, updated_at: '2026-10-25T00:00:00Z' }] };
}
