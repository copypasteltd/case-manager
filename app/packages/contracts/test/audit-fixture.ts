import { auditWindow, type AuditEvent, type AuditExport, type AuditPage } from '../src/audit';
export const auditID = '00000000-0000-4000-8000-000000000001';
export const otherAuditID = '00000000-0000-4000-8000-000000000002';
export function auditFixture() {
  const filter = auditWindow('2026-10-01', '2026-10-02');
  const event: AuditEvent = { id: auditID, occurred_at: '2026-10-01T04:00:00Z', actor_id: auditID, executor_id: auditID, channel: 'web', action: 'project.created', result: 'succeeded', resource_type: 'project', resource_id: otherAuditID, resource_version: '9007199254740993', request_id: 'synthetic-request', run_id: null, job_id: null, before_digest: null, after_digest: 'a'.repeat(64) };
  const page: AuditPage = { items: [event], next_cursor: null, filters: filter, disclosure: 'organization_metadata_v1' };
  const record: AuditExport = { id: auditID, filters: filter, disclosure: 'organization_metadata_v1', created_at: '2026-10-01T04:00:00Z', expires_at: '2026-10-02T04:00:00Z', row_count: 1, csv_bytes: 3, csv_sha256: 'a'.repeat(64), state: 'ready', can_download: true };
  return { filter, event, page, record };
}
