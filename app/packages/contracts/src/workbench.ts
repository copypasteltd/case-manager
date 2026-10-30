import type { components } from './api';
type S = components['schemas'];
export type WorkbenchSummary = S['WorkbenchSummary'];
export type WorkbenchGroup = S['WorkbenchGroup'];
export type WorkbenchItem = S['WorkbenchItem'];
export const workbenchKinds = ['approvals', 'contracts', 'subscription_periods', 'incidents', 'file_attention'] as const;
export type WorkbenchKind = typeof workbenchKinds[number];
export const workbenchLabels: Record<WorkbenchKind, string> = { approvals: '待我审批', contracts: '我负责的合同到期', subscription_periods: '我负责的订阅待核对', incidents: '我负责的未结事件', file_attention: '我创建的文件存储待办' };
export const workbenchHints: Record<WorkbenchKind, string> = {
  approvals: '当前修订、当前审批轮次中，分配给你的有效步骤。',
  contracts: '生效合同的正式生效修订；包含已到期及未来30天到期的合同。',
  subscription_periods: '已生成且需要你核对的账期，不表示欠款或已扣费。',
  incidents: '分配给你且尚未解决的服务事件，仅显示全部影响服务均可查阅的事项。',
  file_attention: '当前版本不可用、副本异常或尚待验证；完成验证后消除。仅显示你创建且当前可读的文件。',
};
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
const integer = (v: unknown, min = 0): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= min;
const day = (v: unknown): v is string => typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v) && Number.isFinite(Date.parse(v + 'T00:00:00Z')) && new Date(v + 'T00:00:00Z').toISOString().slice(0, 10) === v;
const clock = (v: unknown): v is string => typeof v === 'string' && /^\d{4}-\d{2}-\d{2}T/.test(v) && Number.isFinite(Date.parse(v));
const invalid = () => new Error('工作台业务汇总尚未核实，请刷新后重试。');

export function checkedWorkbench(v: WorkbenchSummary): WorkbenchSummary {
  if (!v || v.scope !== 'my_readable_work' || v.contract_timezone !== 'Asia/Hong_Kong' || !clock(v.as_of) || !day(v.business_date) || !day(v.contract_through) || Date.parse(v.contract_through + 'T00:00:00Z') - Date.parse(v.business_date + 'T00:00:00Z') !== 30 * 86400000 || new Date(Date.parse(v.as_of) + 8 * 3600000).toISOString().slice(0, 10) !== v.business_date || !Array.isArray(v.groups) || v.groups.length !== workbenchKinds.length) throw invalid();
  const kinds = new Set(v.groups.map(g => g?.kind));
  if (kinds.size !== workbenchKinds.length || workbenchKinds.some(kind => !kinds.has(kind))) throw invalid();
  for (const group of v.groups) {
    if (!group || !integer(group.count) || !Array.isArray(group.items) || group.items.length !== Math.min(group.count, 5) || typeof group.has_more !== 'boolean' || group.has_more !== (group.count > group.items.length) || new Set(group.items.map(item => item?.id)).size !== group.items.length) throw invalid();
    for (const item of group.items) {
      if (!item || !uuid(item.id) || !uuid(item.revision_id) || !integer(item.version, 1) || typeof item.title !== 'string' || !item.title.trim() || [...item.title].length > 200 || !clock(item.updated_at) || item.related_id !== null && !uuid(item.related_id)) throw invalid();
      if (group.kind === 'approvals' && (item.state !== 'active' || item.due_on !== null || !uuid(item.related_id))) throw invalid();
      if (group.kind === 'contracts' && (!day(item.due_on) || item.due_on > v.contract_through || item.related_id !== null || item.state !== (item.due_on < v.business_date ? 'overdue' : 'due_soon'))) throw invalid();
      if (group.kind === 'incidents' && (!['open', 'investigating', 'monitoring'].includes(item.state) || item.due_on !== null || item.related_id !== null)) throw invalid();
      if (group.kind === 'file_attention' && (!['file_unavailable', 'replica_attention', 'replica_pending'].includes(item.state) || item.due_on !== null || item.related_id !== null)) throw invalid();
      if (group.kind === 'subscription_periods' && (item.state !== 'awaiting_confirmation' || !day(item.due_on) || !uuid(item.related_id))) throw invalid();
    }
    for (let i = 1; i < group.items.length; i++) {
      const before = group.items[i - 1], after = group.items[i];
      if ((before.due_on ?? '9999-99-99') > (after.due_on ?? '9999-99-99') || before.due_on === after.due_on && (Date.parse(before.updated_at) < Date.parse(after.updated_at) || Date.parse(before.updated_at) === Date.parse(after.updated_at) && before.id.toLowerCase() < after.id.toLowerCase())) throw invalid();
    }
  }
  return v;
}

export function workbenchURL(kind: WorkbenchKind, item: WorkbenchItem, client: 'web' | 'mobile'): string {
  if (!workbenchKinds.includes(kind) || !uuid(item.id) || !['web', 'mobile'].includes(client)) throw invalid();
  if (client === 'web') return ({ approvals: '/requests/', contracts: '/contracts/', subscription_periods: '/subscription-periods/', incidents: '/incidents/', file_attention: '/files/' }[kind]) + item.id;
  return ({ approvals: '/pages/detail/index?kind=request&id=', contracts: '/pages/business/index?kind=contract&id=', subscription_periods: '/pages/subscription-period/index?id=', incidents: '/pages/status/index?kind=incident&id=', file_attention: '/pages/files/index?file_id=' }[kind]) + item.id;
}

export function workbenchState(item: WorkbenchItem): string {
  return ({ active: '待审批', overdue: '合同已到期', due_soon: '合同即将到期', awaiting_confirmation: '账期待核对', open: '事件待处理', investigating: '事件调查中', monitoring: '事件观察中', file_unavailable: '文件当前版本不可用', replica_attention: '文件副本需要关注', replica_pending: '文件副本待验证' } as Record<string, string>)[item.state] ?? '请核对';
}
