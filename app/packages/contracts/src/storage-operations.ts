import type { components } from './api';
type S = components['schemas'];
export type StorageItem = S['StorageReplicaOperation'];
export type StorageDetails = S['StorageReplicaDetails'];
export type StorageInput = S['StorageReplicaRetry'];
export type StorageReceipt = S['StorageReplicaReceipt'];
export const providers = { oss: 'OSS', r2: 'Cloudflare R2' } as const;
export const states = { queued: '等待复制', copying: '复制中', verifying: '核验中', verified: '已核验', retry_wait: '等待自动重试', failed: '复制失败', missing: '副本缺失' } as const;
export const jobStates = { queued: '任务排队', running: '任务执行中', succeeded: '任务完成', retry_wait: '任务等待重试', dead: '任务已停止', cancelled: '任务已取消' } as const;
export const blocks = { NO_FILE_RETRY_AUTHORITY: '当前账号没有此文件的复制重试权限。', ORIGINAL_RETIRED: '此原件已进入清理，不能重新复制。', REPLICA_ALREADY_VERIFIED: '此副本已核验，无需重试。', REPLICA_JOB_ACTIVE: '原任务仍在处理，请先核对当前进展。', REPLICA_STATE_CONFLICT: '此状态不支持人工重试。', STORAGE_REPAIR_PLAN_REQUIRED: '主存储已变更，需要另行建立恢复计划。', REPLICA_SOURCE_UNAVAILABLE: '缺少可核验的来源副本。', READY_FOR_REVIEW: '可以核对当前版本后安排重试。' } as const;
export const retryNotice = '重试只重新安排此版本的复制；排队回执不代表复制或核验成功。当前权限、原件和任务状态会在保存时再次核对。';
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[1-8][a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/i.test(v);
const invalid = () => new Error('重试资料或回执尚未核实，请刷新副本详情及人工记录。');
function filter(v: string, options: object) { if (v && !Object.prototype.hasOwnProperty.call(options, v)) throw new Error('请选择有效的存储端和状态。'); return v; }
export function storagePath(provider = '', state = '', jobState = '') {
  return '/storage/replicas?' + [['limit', '20'], ['provider', filter(provider, providers)], ['state', filter(state, states)], ['job_state', filter(jobState, jobStates)]].filter(([, v]) => v).map(([k, v]) => k + '=' + encodeURIComponent(v)).join('&');
}
export function storageHistoryPath(id: string) { if (!uuid(id)) throw invalid(); return '/storage/replica-retries?limit=20&replica_id=' + encodeURIComponent(id); }
export function storageRetryInput(value: StorageDetails, reason: string, reviewed: boolean): StorageInput {
  const v = value.item, note = reason.trim();
  if (!reviewed || !value.can_retry || value.retry_block !== 'READY_FOR_REVIEW') throw new Error('请先核对可重试副本、版本及现有任务。');
  if (!uuid(v.id) || !uuid(v.file_id) || !uuid(v.file_version_id) || !Number.isSafeInteger(v.version) || v.version < 1 || v.version >= Number.MAX_SAFE_INTEGER || !/^[a-f0-9]{64}$/.test(v.basis_sha256) || v.state === 'verified' || ['retiring', 'purged'].includes(v.availability) || v.provider === v.primary_provider || !v.source_available || v.job && !['dead', 'cancelled'].includes(v.job.state)) throw invalid();
  if (!note || [...note].length > 200) throw new Error('请填写1至200字的重试原因。');
  return { expected_version: v.version, basis_sha256: v.basis_sha256, reviewed: true, reason: note };
}
function canonical(v: unknown): unknown { return v && typeof v === 'object' && !Array.isArray(v) ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => a.localeCompare(b)).map(([k, value]) => [k, canonical(value)])) : v; }
export function checkedStorageReceipt(value: unknown, reviewed: StorageDetails): StorageReceipt {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw invalid();
  const r = value as StorageReceipt, { basis_sha256, ...before } = reviewed.item;
  if (!uuid(r.id) || !uuid(r.actor_id) || !uuid(r.business_operation_id) || r.replica_id !== before.id || r.file_id !== before.file_id || r.file_version_id !== before.file_version_id || r.expected_version !== before.version || r.result_version !== before.version + 1 || r.basis_sha256 !== basis_sha256 || !Number.isFinite(Date.parse(r.created_at)) || JSON.stringify(canonical(r.before)) !== JSON.stringify(canonical(before))) throw invalid();
  const after = r.after;
  if (!after || after.id !== before.id || after.file_id !== before.file_id || after.file_version_id !== before.file_version_id || after.provider !== before.provider || after.version !== r.result_version || after.state !== 'queued' || after.error_code !== null || after.job !== null || after.file_revision !== before.file_revision || after.file_state !== before.file_state || after.availability !== before.availability || after.is_current !== before.is_current || after.storage_epoch !== before.storage_epoch || after.primary_provider !== before.primary_provider || !after.source_available || after.created_at !== before.created_at || !Number.isFinite(Date.parse(after.updated_at)) || Date.parse(after.updated_at) < Date.parse(before.updated_at)) throw invalid();
  return r;
}
