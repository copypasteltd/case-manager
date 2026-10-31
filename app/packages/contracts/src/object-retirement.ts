import type { components } from './api';
import { checkedCleanupReceipts } from './multipart-cleanup';
type Schemas = components['schemas'];
export type RetirementCandidate = Schemas['RetirementCandidate'];
export type RetirementPlan = Schemas['RetirementPlan'];
export type RetirementItem = Schemas['RetirementItem'];
export type RetirementAuthorization = Schemas['RetirementAuthorization'];
export { checkedCleanupReceipts };
const id = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const hash = /^[0-9a-f]{64}$/;
function check(value: unknown, message: string): asserts value { if (!value) throw new Error(message); }
const revision = (v: unknown) => Number.isSafeInteger(v) && Number(v) > 0;
export const retirementKinds = { history: '历史正式原件', committed_draft: '已提交上传的草稿原件' };
export const retirementStates: Record<string, string> = { observing: '观察中', approved: '已批准', completed: '全部原件已清理', cancelled: '已停止', queued: '已排队', running: '核对与执行中', retry_wait: '等待重试', failed: '执行失败', blocked: '受保护或已停止', succeeded: '已确认原件不存在', started: '开始核对原件', confirmed_absent: '已确认原件不存在', retry_scheduled: '已安排重试' };
export const retirementBlockers: Record<string, string> = {
 EMPTY_ORIGINAL_REVIEW_REQUIRED: '空原件需另行核对', POLICY_REQUIRED: '尚未发布对应保留制度', POLICY_DISABLED: '保留制度未启用', RETENTION_PERIOD_ACTIVE: '仍在保留期内', CURRENT_VERSION: '当前正式版本受保护',
 LEGAL_HOLD: '存在法务保留', RETENTION_HOLD: '存在有效保留约束', BUSINESS_REFERENCE: '仍有固定业务引用', RESTORE_ACTIVE: '仍有历史恢复任务', UPLOAD_ACTIVE: '仍有上传依赖', DOWNLOAD_PERMIT_ACTIVE: '下载许可尚未过期',
 REPLICA_EVIDENCE_INCOMPLETE: '缺少双云完整校验证据', DERIVED_OBJECT_REVIEW_REQUIRED: '派生存储对象需单独核对', ORIGINAL_UNAVAILABLE: '原件已停用或当前不可用', COMMITTED_DRAFT_REQUIRED: '草稿尚未完成正式提交', UPLOAD_OR_PERMIT_ACTIVE: '上传或原许可仍在有效期', UPLOAD_JOB_ACTIVE: '上传任务尚未结束', AGENT_RECOVERY_SOURCE: '秘书恢复来源继续保留', FORMAL_OBJECT_PROTECTED: '草稿键与正式对象重合', SOURCE_CHANGED: '原件来源证据不完整或已改变', CLEANUP_BACKEND_UNAVAILABLE: '未配置对应清理位置', NATIVE_OBJECT_VERSION_REQUIRED: 'OSS原件需要真实原生版本号', EXCLUSIVE_NAMESPACE_REQUIRED: 'R2清理需要明确的独占目录配置', ORIGINAL_ALREADY_RETIRED: '此原件已批准清理', SHARED_OBJECT_PROTECTED: '仍有其他版本使用此对象', OBJECT_WRITE_ACTIVE: '仍有对象写入任务',
};
export function checkedRetirementPlan(value: RetirementPlan, fileID: string, planID?: string): RetirementPlan {
 check(value && id.test(value.id) && revision(value.version) && (!planID || value.id === planID), '原件清单回执无效。');
 const s = value.snapshot;
 check(s && s.file_id === fileID && id.test(s.target_id) && revision(s.file_version) && revision(s.seq) && Object.prototype.hasOwnProperty.call(retirementKinds, s.kind) && hash.test(s.source_sha256) && hash.test(value.snapshot_sha256), '原件清单与所选文件不一致。');
 check(s.eligible && s.blocking_reasons.length === 0 && s.policy?.enabled && s.policy.kind === (s.kind === 'history' ? 'formal_history' : 'draft'), '固定清单缺少有效制度或仍受保护。');
 check(['observing', 'approved', 'completed', 'cancelled'].includes(value.state), '原件清单状态无效。');
 const expected = s.kind === 'history' ? 2 : 1;
 check(Array.isArray(value.items) && value.items.length === expected, '固定原件执行项不完整。');
 const ids = new Set<string>(), providers = new Set<string>();
 for (let i = 0; i < value.items.length; i++) {
  const row = value.items[i];
  check(id.test(row.id) && !ids.has(row.id) && row.ordinal === i && revision(row.version) && revision(row.generation) && Number.isSafeInteger(row.attempts) && row.attempts >= 0 && ['oss','r2'].includes(row.provider) && !providers.has(row.provider) && ['observing','queued','running','retry_wait','blocked','failed','succeeded'].includes(row.state), '原件执行范围、次序或修订无效。');
  check((row.state === 'succeeded') === !!row.confirmed_at && (!row.confirmed_at || Number.isFinite(Date.parse(row.confirmed_at))), '原件成功状态缺少有效确认时间。');
  ids.add(row.id); providers.add(row.provider);
 }
 check(s.kind !== 'history' || providers.has('oss') && providers.has('r2'), '历史原件必须分别核对OSS与R2。');
 const created = Date.parse(value.created_at), ready = Date.parse(value.ready_after), expiry = Date.parse(value.expires_at);
 check(Number.isFinite(created) && ready - created >= 86400000 && expiry > ready, '原件观察期或批准期限无效。');
 if (value.authorization) checkedRetirementAuthorization(value.authorization,value);
 check(value.state !== 'completed' || value.items.every(row => row.state === 'succeeded'), '尚有未完成的原件，不能显示清理完成。');
 return value;
}
export function retirementCreate(candidate: RetirementCandidate, reason: string): Schemas['RetirementCreate'] {
 const text = reason.trim();
 check(candidate.eligible && candidate.blocking_reasons.length === 0 && id.test(candidate.target_id) && revision(candidate.file_version), '候选仍受保护，请刷新核对。');
 check(text && Array.from(text).length <= 200, '请填写不超过200字的清理原因。');
 return { expected_file_version: candidate.file_version, kind: candidate.kind, target_id: candidate.target_id, reason: text };
}
export function retirementDecision(plan: RetirementPlan, reason: string, acknowledged: boolean, action: 'approve' | 'cancel', now = Date.now()): Schemas['CleanupDecision'] {
 checkedRetirementPlan(plan, plan.snapshot.file_id, plan.id);
 const text = reason.trim(); check(acknowledged && text && Array.from(text).length <= 200, '请核对不可逆影响并填写操作依据。');
 check(['observing', 'approved'].includes(plan.state), '此清单已经结束，请刷新核对。');
 if (action === 'approve') check(plan.state === 'observing' && now >= Date.parse(plan.ready_after) && now < Date.parse(plan.expires_at), '观察期未结束、批准期限已过或此清单已经批准。');
 return { expected_version: plan.version, snapshot_sha256: plan.snapshot_sha256, reason: text };
}

export function retirementCanRenew(plan: RetirementPlan): boolean {
 return plan.state === 'approved' && plan.items.some(row => row.state !== 'succeeded') && plan.items.every(row => ['succeeded','blocked','failed'].includes(row.state));
}
export function checkedRetirementAuthorization(value: RetirementAuthorization, plan: RetirementPlan): RetirementAuthorization {
 check(value && id.test(value.id) && revision(value.sequence) && revision(value.plan_version) && value.plan_version <= plan.version && value.snapshot_sha256 === plan.snapshot_sha256 && id.test(value.authorized_by) && ['web','android','wechat','douyin'].includes(value.channel) && Number.isFinite(Date.parse(value.authorized_at)) && Date.parse(value.authorized_at) >= Date.parse(plan.approved_at ?? '') && value.reason?.trim() && Array.from(value.reason).length <= 200, '重新授权记录与固定清单不一致。');
 check(Array.isArray(value.items) && value.items.length >= 1 && value.items.length <= plan.items.length, '重新授权范围无效。');
 const seen = new Set<string>();
 for (const row of value.items) {
  const item = plan.items.find(v => v.id === row.item_id);
  check(item && !seen.has(row.item_id) && revision(row.previous_version) && row.previous_version < item.version && revision(row.generation) && row.generation >= 2 && row.generation <= item.generation, '重新授权记录中的原件或修订无效。');
  seen.add(row.item_id);
 }
 return value;
}
export function retirementRenewal(plan: RetirementPlan, reason: string, acknowledged: boolean): Schemas['RetirementRenewal'] {
 checkedRetirementPlan(plan,plan.snapshot.file_id,plan.id);
 const text=reason.trim();
 check(retirementCanRenew(plan), '仅可在全部剩余任务停止后重新授权，请刷新核对。');
 check(acknowledged && text && Array.from(text).length <= 200, '请核对剩余范围、不可逆影响并填写重新授权依据。');
 return {expected_version:plan.version,snapshot_sha256:plan.snapshot_sha256,reason:text,items:plan.items.filter(v => v.state !== 'succeeded').map(v => ({id:v.id,expected_version:v.version}))};
}
export function checkedRetirementRenewalResult(before: RetirementPlan, after: RetirementPlan): boolean {
 checkedRetirementPlan(after,before.snapshot.file_id,before.id);
 if (after.version !== before.version + 1 || after.state !== 'approved' || after.snapshot_sha256 !== before.snapshot_sha256 || !after.authorization || after.authorization.sequence !== (before.authorization?.sequence ?? 0)+1 || after.authorization.plan_version !== after.version || after.approved_at !== before.approved_at || after.approval_reason !== before.approval_reason) return false;
 const remaining=before.items.filter(v => v.state !== 'succeeded');
 return after.authorization.items.length === remaining.length && before.items.every(row => {
  const next=after.items.find(v => v.id === row.id);
  if (!next) return false;
  if (row.state === 'succeeded') return next.state === row.state && next.version === row.version && next.generation === row.generation && next.confirmed_at === row.confirmed_at;
  return next.state === 'queued' && next.version === row.version + 1 && next.generation === row.generation + 1 && after.authorization!.items.some(v => v.item_id === row.id && v.previous_version === row.version && v.generation === next.generation);
 });
}
