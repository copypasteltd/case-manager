import type { components } from './api';
type Schemas = components['schemas'];
export type Notification = Schemas['Notification'];
export const notificationCategories: Record<Notification['category'], string> = {
  mention: '讨论提及', task: '任务', approval: '待审批', request_result: '申请进展', contract_due: '合同到期',
  subscription_due: '订阅账期', replica_failure: '文件副本', incident: '服务事件', secretary: '秘书结果',
};
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
function id(value: unknown): string {
  if (typeof value !== 'string' || !uuid.test(value)) throw new Error('通知地址无效，请刷新后重新打开。');
  return value.toLowerCase();
}
// Only these business routes may be opened. A returned URL never becomes an
// unrestricted redirect, in either the Web client or a mini-program.
export function notificationDestination(target: Schemas['NotificationTarget']) {
  const resource = id(target.id);
  if (target.comment_id !== undefined && target.type !== 'task') throw new Error('讨论地址与资料类型不一致。');
  let web: string, mobile: string;
  switch (target.type) {
    case 'task': web = '/projects/' + id(target.container_id) + '?task=' + resource; mobile = '/pages/detail/index?kind=task&id=' + resource; if (target.comment_id !== undefined) { const comment = id(target.comment_id); web += '&comment=' + comment; mobile += '&comment_id=' + comment; } break;
    case 'request': web = '/requests/' + resource; mobile = '/pages/detail/index?kind=request&id=' + resource; break;
    case 'contract': web = '/contracts/' + resource; mobile = '/pages/business/index?kind=contract&id=' + resource; break;
    case 'subscription_period': id(target.container_id); web = '/subscription-periods/' + resource; mobile = '/pages/subscription-period/index?id=' + resource; break;
    case 'file': web = '/files/' + resource; mobile = '/pages/detail/index?kind=file&id=' + resource; break;
    case 'incident': web = '/incidents/' + resource; mobile = '/pages/status/index?kind=incident&id=' + resource; break;
    case 'agent_run': { const conversation = id(target.container_id); web = '/conversations/' + conversation + '?run=' + resource; mobile = '/pages/secretary/index?kind=conversation&id=' + conversation + '&run=' + resource; break; }
    default: throw new Error('暂不支持这个通知地址，请从工作台查看相关资料。');
  }
  if (target.web_path !== web) throw new Error('通知地址与资料不一致，请刷新后重新打开。');
  return { web, mobile };
}
export function notificationReadInput(value: Notification): Schemas['NotificationRead'] {
  if (!Number.isSafeInteger(value.delivery_seq) || value.delivery_seq < 1 || !Number.isSafeInteger(value.read_seq) || value.read_seq < 0 || value.read_seq > value.delivery_seq || value.read !== (value.read_seq === value.delivery_seq)) throw new Error('通知序号无效，请刷新后核对。');
  return { delivery_seq: value.delivery_seq };
}
export function notificationCount(count: number, complete: boolean) {
  if (!Number.isSafeInteger(count) || count < 0) return '待核对';
  if (complete) return String(count);
  return count > 0 ? count + '+' : '待核对';
}
export function mergeNotifications(items: readonly Notification[]) {
  const latest = new Map<string, Notification>();
  for (const item of items) {
    const previous = latest.get(item.id);
    if (!previous || item.delivery_seq > previous.delivery_seq || (item.delivery_seq === previous.delivery_seq && item.read_seq >= previous.read_seq)) latest.set(item.id, item);
  }
  return [...latest.values()].sort((a, b) => b.updated_at.localeCompare(a.updated_at) || b.id.localeCompare(a.id));
}

export type DeliveryJob = Schemas['NotificationDeliveryJob'];
export type RetryReceipt = Schemas['NotificationRetryReceipt'];
export const notificationRetryBlocks: Record<DeliveryJob['retry_block'], string> = {
  ready: '可核对后创建重试', not_authorized: '当前只有查询权限', not_failed: '尚未最终失败，按当前任务状态处理',
  already_retried: '已创建后续任务，请查看其结果', invalid_input: '固定输入无效，需要修复来源',
  rejected_event: '事件已被消费协议拒绝，需要修复来源', missing_event: '固定业务事件不存在', unsupported_event: '事件类型或版本尚不支持',
};
function metadata(value: unknown, keys: string[]) {
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).some(k => !keys.includes(k)) || keys.some(k => !Object.prototype.hasOwnProperty.call(value,k))) throw new Error('投递资料字段不完整，请刷新核对。');
}
function timestamp(value: unknown) {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) || !Number.isFinite(Date.parse(value))) throw new Error('投递资料时间无效。');
}
function smallInteger(value: unknown, maximum: number) { return Number.isSafeInteger(value) && Number(value) >= 0 && Number(value) <= maximum; }
export function checkedRetryReceipt(value: RetryReceipt, source?: string, actor?: string, event?: string): RetryReceipt {
  metadata(value, ['id','source_job_id','retry_job_id','business_event_id','created_by','created_at']);
  for (const v of [value.id,value.source_job_id,value.retry_job_id,value.business_event_id,value.created_by]) id(v);
  timestamp(value.created_at);
  if (value.source_job_id === value.retry_job_id || source && value.source_job_id !== source || actor && value.created_by !== actor || event && value.business_event_id !== event) throw new Error('重试回执与所选任务、事件或当前人员不一致。');
  return value;
}
export function checkedDeliveryJob(value: DeliveryJob, expected: string): DeliveryJob {
  metadata(value, ['job_id','kind','state','safe_code','attempt','max_attempts','first_failed_at','updated_at','available_at','revision','business_event_id','event_type','can_retry','retry_block','retry_id','retry_job_id','parent_job_id','result']);
  if (id(value.job_id) !== id(expected) || !['notification.deliver.v1','event.rejected.v1'].includes(value.kind) || !['queued','running','succeeded','retry_wait','dead','cancelled'].includes(value.state) || !/^[a-f0-9]{64}$/.test(value.revision) || typeof value.can_retry !== 'boolean' || !Object.prototype.hasOwnProperty.call(notificationRetryBlocks,value.retry_block)) throw new Error('任务状态或固定版本无效。');
  if (!smallInteger(value.max_attempts,16) || value.max_attempts < 1 || !smallInteger(value.attempt,value.max_attempts)) throw new Error('投递尝试次数无效。');
  for (const v of [value.business_event_id,value.retry_id,value.retry_job_id,value.parent_job_id]) if (v !== null) id(v);
  timestamp(value.updated_at); timestamp(value.available_at); if (value.first_failed_at !== null) timestamp(value.first_failed_at);
  if (value.safe_code !== null && !/^[A-Z][A-Z0-9_]{1,79}$/.test(value.safe_code) || value.event_type !== null && !/^[a-z][a-z0-9_.-]{0,79}$/.test(value.event_type)) throw new Error('投递技术标识无效。');
  if ((value.retry_id === null) !== (value.retry_job_id === null) || value.retry_job_id === value.job_id || value.parent_job_id === value.job_id || value.can_retry !== (value.retry_block === 'ready') || value.can_retry && (value.kind !== 'notification.deliver.v1' || value.state !== 'dead' || !value.business_event_id || !value.event_type || value.retry_job_id !== null)) throw new Error('重试状态相互矛盾，请刷新核对。');
  if (value.retry_block === 'already_retried' && !value.retry_job_id || value.retry_block === 'rejected_event' && value.kind !== 'event.rejected.v1') throw new Error('重试来源状态无效。');
  if (value.result !== null) {
    const r=value.result; metadata(r,['delivered','suppressed','outcome','safe_code','completed_at']); timestamp(r.completed_at);
    if (value.state !== 'succeeded' || !smallInteger(r.delivered,100) || !smallInteger(r.suppressed,100) || !['delivered','suppressed'].includes(r.outcome) || (r.outcome === 'delivered') !== (r.delivered > 0) || !/^[A-Z][A-Z0-9_]{1,79}$/.test(r.safe_code)) throw new Error('投递回执与任务状态不一致。');
  }
  return value;
}
export function notificationRetryInput(job: DeliveryJob, reason: string): Schemas['NotificationRetryInput'] {
  checkedDeliveryJob(job,job.job_id);
  const text=reason.trim();
  if (!job.can_retry || [...text].length < 1 || [...text].length > 200) throw new Error('请核对可重试任务并填写1至200字的原因。');
  return { expected_revision:job.revision, reason:text };
}
