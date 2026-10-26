import { MobileError, type Schemas } from './session';

export type OverviewKind = Schemas['ProjectOverviewGroup']['kind'];
export const overviewLabels: Record<OverviewKind, string> = {
  tasks: '任务', milestones: '里程碑', deliverables: '交付', risks: '风险', issues: '问题', changes: '变更', budgets: '预算修订',
  files: '文件', posts: '知识与日志', contracts: '合同', purchases: '采购', claims: '报销', costs: '实际费用', payments: '收付款', subscriptions: '订阅', services: '服务配置',
};
const states: Record<string, string> = {
  todo: '待开始', in_progress: '进行中', in_review: '待审核', done: '已完成', blocked: '受阻', cancelled: '已取消',
  open: '待处理', achieved: '已达成', missed: '已错过', mitigating: '应对中', closed: '已关闭', realized: '已转问题', investigating: '调查中', resolved: '已解决',
  draft: '草稿', submitted: '已提交', accepted: '已验收', changes_requested: '待修改', requested: '已申请', implemented: '已实施', rejected: '已驳回', withdrawn: '已撤回',
  approved: '已批准', superseded: '已取代', active: '使用中', published: '已发布', archived: '已归档', confirmed: '已确认', reversed: '已冲销', recorded: '已登记',
  ordering: '下单中', receiving: '收货中', completed: '已完成', terminated: '已终止', paused: '已暂停', signed: '已签署', performing: '履约中', returned: '已退回',
  forecast: '预计账期', awaiting_confirmation: '待核对', waived: '已豁免', void: '已作废', unknown: '未核实', not_submitted: '尚未送审',
};
export function overviewState(value: string): string { return states[value] ?? value; }

// Keep the database's decimal text, including six places and values beyond the
// JS integer range. Formatting neither adds currencies nor computes balances.
export function exactAmount(value: string | null | undefined): string {
  if (typeof value !== 'string' || !/^-?\d+(\.\d{1,6})?$/.test(value)) return '金额未核实';
  const [whole, fraction] = value.split('.');
  return whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',') + (fraction === undefined ? '' : '.' + fraction);
}
export function overviewClock(value: string): string {
  const instant = new Date(value);
  return Number.isFinite(instant.getTime()) ? new Date(instant.getTime() + 8 * 3600000).toISOString().slice(0, 19).replace('T', ' ') + ' · 香港' : '时间未核实';
}
function id(value: string): string {
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)) throw new MobileError('INVALID_RESOURCE', '资料地址无效，请从项目重新打开。');
  return value.toLowerCase();
}
const sections = new Set<OverviewKind>(['milestones', 'risks', 'issues', 'deliverables', 'changes', 'budgets']);
const listKinds = new Set<OverviewKind>(['tasks', 'files', 'posts', 'contracts', 'purchases', 'claims', 'costs', 'payments', 'subscriptions']);
export type FinancialKind = 'cost' | 'payment' | 'subscription' | 'change';
const financePaths: Record<FinancialKind, string> = { cost: 'costs', payment: 'payments', subscription: 'subscriptions', change: 'changes' };
export function financialKind(value: unknown): FinancialKind {
  if (typeof value !== 'string' || !Object.prototype.hasOwnProperty.call(financePaths, value)) throw new MobileError('INVALID_RESOURCE', '业务资料地址无效，请从列表重新打开。');
  return value as FinancialKind;
}
export function financialReadPath(kind: FinancialKind, resourceID: string): string { return '/' + financePaths[financialKind(kind)] + '/' + id(resourceID); }
export function financialURL(kind: FinancialKind, resourceID: string): string { return '/pages/finance/index?kind=' + financialKind(kind) + '&id=' + id(resourceID); }
export function overviewCollectionURL(projectID: string, kind: OverviewKind): string | null {
  const project = id(projectID);
  if (sections.has(kind)) return '/pages/project/index?id=' + project + '&section=' + kind;
  if (listKinds.has(kind)) return '/pages/list/index?kind=' + kind + '&project_id=' + project;
  // Service discovery has no project-filter contract. Only actual item links
  // may advertise an association; never show the global list as this project.
  return null;
}
export function overviewItemURL(projectID: string, kind: OverviewKind, resourceID: string): string {
  const project = id(projectID), resource = id(resourceID);
  if (kind === 'budgets') return overviewCollectionURL(project, kind)! + '&focus=' + resource;
  if (kind === 'costs') return financialURL('cost', resource);
  if (kind === 'payments') return financialURL('payment', resource);
  if (kind === 'subscriptions') return financialURL('subscription', resource);
  if (kind === 'changes') return financialURL('change', resource);
  if (kind === 'services') return '/pages/status/index?kind=service&id=' + resource;
  if (kind === 'contracts' || kind === 'purchases' || kind === 'claims') return '/pages/business/index?kind=' + ({ contracts: 'contract', purchases: 'purchase', claims: 'claim' }[kind]) + '&id=' + resource;
  if (kind === 'deliverables') return '/pages/deliverable/index?id=' + resource;
  const detail: Partial<Record<OverviewKind, string>> = { tasks: 'task', milestones: 'milestone', risks: 'risk', issues: 'issue', files: 'file', posts: 'post' };
  if (!detail[kind]) throw new MobileError('INVALID_RESOURCE', '这个业务入口尚未核实。');
  return '/pages/detail/index?kind=' + detail[kind] + '&id=' + resource;
}

// Reject a mismatched or incomplete envelope before mounting any factual
// content. Unknown authorization remains distinct from a readable empty set.
export function currentOverview(value: Schemas['ProjectOverview'], projectID: string, projectVersion: number): Schemas['ProjectOverview'] {
  const invalid = () => { throw new MobileError('INVALID_RESPONSE', '项目概览尚未核实，请重新读取。'); };
  if (value.project_id !== id(projectID) || value.project_version !== projectVersion || value.scope !== 'readable_records' || !Number.isFinite(new Date(value.as_of).getTime())) invalid();
  const kinds = new Set(value.groups.map(group => group.kind));
  if (value.groups.length !== Object.keys(overviewLabels).length || kinds.size !== value.groups.length || Object.keys(overviewLabels).some(kind => !kinds.has(kind as OverviewKind))) invalid();
  for (const group of value.groups) {
    if (!group.available) {
      if (group.count !== null || group.items.length || group.states.length || group.has_more) invalid();
      continue;
    }
    if (group.count === null || !Number.isSafeInteger(group.count) || group.count < 0 || group.items.length > 5 || group.items.length > group.count || group.has_more !== (group.count > group.items.length)) invalid();
    if (group.states.some(state => !Number.isSafeInteger(state.count) || state.count < 0) || group.states.reduce((sum, state) => sum + state.count, 0) !== group.count || new Set(group.states.map(state => state.state)).size !== group.states.length) invalid();
    if (new Set(group.items.map(item => item.id)).size !== group.items.length) invalid();
    for (const item of group.items) if (id(item.id) !== item.id || !Number.isSafeInteger(item.version) || item.version < 1) invalid();
  }
  return value;
}
