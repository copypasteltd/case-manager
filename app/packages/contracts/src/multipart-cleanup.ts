import type { components } from './api';

type Schemas = components['schemas'];
export type CleanupCandidate = Schemas['CleanupCandidate'];
export type CleanupPlan = Schemas['CleanupPlan'];
export type CleanupReceipt = Schemas['CleanupReceipt'];
export type CleanupSelection = Schemas['CleanupCreate']['items'][number];
const id = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const hash = /^[0-9a-f]{64}$/;
function requireValue(ok: unknown, message: string): asserts ok { if (!ok) throw new Error(message); }
function version(value: unknown): value is number { return Number.isSafeInteger(value) && Number(value) > 0; }

export const cleanupStates: Record<string, string> = {
  observing: '观察中', approved: '已批准', cancelled: '已撤销', queued: '已排队', running: '正在执行',
  retry_wait: '等待重试', succeeded: '已确认分片不存在', failed: '执行失败', blocked: '已阻断',
  started: '开始核对', confirmed_absent: '已确认分片不存在', retry_scheduled: '已安排重试',
};
export const cleanupBlockers: Record<string, string> = {
  POLICY_REQUIRED: '尚未发布分片保留制度', POLICY_DISABLED: '分片清理制度已停用', RETENTION_PERIOD_ACTIVE: '仍在保留期内',
  UPLOAD_STATE_UNSUPPORTED: '上传正在处理或状态不支持', UPLOAD_OR_PERMIT_ACTIVE: '上传或分片许可尚未过期',
  MULTIPART_ID_REQUIRED: '尚无准确分片上传标识', FINALIZATION_OR_FORMAL_CONTENT: '已进入最终校验或形成正式内容',
  SOURCE_CHANGED: '上传来源已改变', CLEANUP_BACKEND_UNAVAILABLE: '清理存储配置未就绪', UPLOAD_JOB_ACTIVE: '仍有上传任务在处理',
  CLEANUP_RESERVED_OR_CONFIRMED: '已被其他批准清单占用或确认清理', RETENTION_HOLD: '文件存在保留约束',
};

export function cleanupCreate(candidates: readonly CleanupCandidate[], selection: readonly CleanupSelection[], reason: string): Schemas['CleanupCreate'] {
  const text = reason.trim();
  requireValue(text && Array.from(text).length <= 200, '请填写不超过200字的清理原因。');
  requireValue(selection.length > 0 && selection.length <= 50, '请明确选择1至50项候选上传。');
  const seen = new Set<string>();
  for (const item of selection) {
    requireValue(id.test(item.upload_id) && version(item.expected_version) && !seen.has(item.upload_id), '候选选择无效或重复，请刷新后重新核对。');
    seen.add(item.upload_id);
    const current = candidates.find(row => row.upload_id === item.upload_id);
    requireValue(current?.eligible && current.blocking_reasons.length === 0 && current.version === item.expected_version, '候选状态或修订已改变，请刷新并重新选择。');
  }
  return { items: selection.map(row => ({ ...row })).sort((a, b) => a.upload_id.localeCompare(b.upload_id)), reason: text };
}

// Validate exact membership before rendering a receipt or accepting a command response.
export function checkedCleanupPlan(value: CleanupPlan, folderID: string, planID?: string): CleanupPlan {
  requireValue(value && id.test(value.id) && (!planID || value.id === planID) && version(value.version), '清理清单回执无效。');
  requireValue(value.snapshot?.folder_id === folderID && version(value.snapshot.folder_version) && hash.test(value.snapshot_sha256), '清理清单不属于当前目录。');
  requireValue(['observing', 'approved', 'cancelled'].includes(value.state), '清理清单状态无效。');
  const source = value.snapshot.items;
  requireValue(Array.isArray(source) && source.length > 0 && source.length <= 50 && Array.isArray(value.items) && source.length === value.items.length, '清理清单的固定候选与执行项不完整。');
  const sources = new Set<string>(), items = new Set<string>(), itemIDs = new Set<string>();
  for (const row of source) {
    requireValue(id.test(row.upload_id) && version(row.version) && hash.test(row.source_sha256) && hash.test(row.backend_sha256) && row.eligible && row.blocking_reasons.length === 0 && row.policy?.kind === 'multipart' && row.policy.enabled, '清理清单包含无效的固定候选。');
    sources.add(row.upload_id);
  }
  for (const row of value.items) {
    requireValue(id.test(row.id) && version(row.version) && version(row.generation) && Number.isSafeInteger(row.attempts) && row.attempts >= 0 && sources.has(row.upload_id) && ['observing', 'queued', 'running', 'retry_wait', 'succeeded', 'failed', 'blocked'].includes(row.state), '清理执行项不属于固定候选。');
    requireValue(row.state !== 'succeeded' || Number.isFinite(Date.parse(row.confirmed_at ?? '')), '成功状态缺少确认回执时间。');
    items.add(row.upload_id); itemIDs.add(row.id);
  }
  requireValue(sources.size === source.length && items.size === source.length && itemIDs.size === source.length, '清理清单存在重复项。');
  const start = Date.parse(value.created_at), ready = Date.parse(value.ready_after), expiry = Date.parse(value.expires_at);
  requireValue(Number.isFinite(start) && ready - start >= 86400000 && expiry > ready, '清理观察期或有效期无效。');
  return value;
}

export function cleanupDecision(plan: CleanupPlan, reason: string, acknowledged: boolean, action: 'approve' | 'cancel', now = Date.now()): Schemas['CleanupDecision'] {
  checkedCleanupPlan(plan, plan.snapshot.folder_id, plan.id);
  const text = reason.trim();
  requireValue(acknowledged, '请核对并明确确认此固定清单。');
  requireValue(text && Array.from(text).length <= 200, '请填写不超过200字的操作原因。');
  requireValue(plan.state !== 'cancelled', '此清单已撤销，请刷新资料。');
  if (action === 'approve') requireValue(plan.state === 'observing' && now >= Date.parse(plan.ready_after) && now < Date.parse(plan.expires_at), '清单尚在观察期、已过期或已经批准，请刷新资料。');
  return { expected_version: plan.version, snapshot_sha256: plan.snapshot_sha256, reason: text };
}

export function cleanupProgress(plan: CleanupPlan) {
  const confirmed = plan.items.filter(row => row.state === 'succeeded' && row.confirmed_at).length;
  const failed = plan.items.filter(row => ['failed', 'blocked'].includes(row.state)).length;
  return { total: plan.items.length, confirmed, failed, pending: plan.items.length - confirmed - failed };
}

export function checkedCleanupReceipts(rows: CleanupReceipt[]): CleanupReceipt[] {
  const seen = new Set<string>();
  for (const row of rows) {
    requireValue(id.test(row.id) && !seen.has(row.id) && version(row.generation) && version(row.attempt) && ['started', 'confirmed_absent', 'retry_scheduled', 'failed', 'blocked'].includes(row.outcome) && Number.isFinite(Date.parse(row.created_at)), '执行回执无效，请刷新核对。');
    seen.add(row.id);
  }
  return rows;
}
