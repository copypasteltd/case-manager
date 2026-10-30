import type { components } from './api';
type Replica = components['schemas']['FileReplica'];
const invalid = () => new Error('副本资料或重试结果尚未核实，请刷新存储记录后重试。');
const uuid = (value: unknown): value is string => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);

export function replicaRetryInput(replica: Replica, reason: string) {
  const text = reason.trim();
  if (!uuid(replica.id) || !Number.isSafeInteger(replica.version) || replica.version < 1 || replica.version >= Number.MAX_SAFE_INTEGER || !['oss', 'r2'].includes(replica.provider) || !['failed', 'missing'].includes(replica.state)) throw invalid();
  if (!text || [...text].length > 200) throw new Error('请填写1至200字的重试原因。');
  return { expected_version: replica.version, reason: text };
}

export function checkedReplicaRetry(value: Replica, before: Replica): Replica {
  if (!value || value.id !== before.id || value.provider !== before.provider || !Number.isSafeInteger(value.version) || value.version !== before.version + 1 || value.state !== 'queued' || value.actual_sha256 !== null || value.verified_at !== null || value.error_code !== null || !Number.isFinite(Date.parse(value.updated_at))) throw invalid();
  return value;
}
