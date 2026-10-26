import type { components } from './api';
type Schemas = components['schemas'];
export type Notification = Schemas['Notification'];
export const notificationCategories: Record<Notification['category'], string> = {
  task: '任务', approval: '待审批', request_result: '申请进展', contract_due: '合同到期',
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
  let web: string, mobile: string;
  switch (target.type) {
    case 'task': web = '/projects/' + id(target.container_id) + '?task=' + resource; mobile = '/pages/detail/index?kind=task&id=' + resource; break;
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
