import type { components } from './api';
type S = components['schemas'];
export type ProjectBusinessContext = S['ProjectBusinessContext'];
export const projectBusinessKinds = ['contracts', 'purchases', 'claims', 'costs', 'payments', 'subscriptions'] as const;
export type ProjectBusinessKind = typeof projectBusinessKinds[number];
export type BusinessSelection = { projectID: string | null; companyOnly: boolean };
export const projectBusinessLabels: Record<ProjectBusinessKind, string> = { contracts: '合同', purchases: '采购', claims: '报销', costs: '实际费用', payments: '实际资金', subscriptions: '订阅' };
export const projectBusinessActions: Record<ProjectBusinessKind, string> = { contracts: 'contract.create', purchases: 'purchase.create', claims: 'claim.create', costs: 'cost.record', payments: 'payment.record', subscriptions: 'subscription.create' };
const invalid = () => new Error('项目业务范围尚未核实，请从项目重新打开。');
export function businessProjectID(value: unknown): string {
  if (typeof value !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)) throw invalid();
  return value.toLowerCase();
}
export function businessSelection(project: unknown, companyOnly: unknown): BusinessSelection {
  const projectID = project === undefined || project === null ? null : businessProjectID(project);
  if (companyOnly !== undefined && companyOnly !== null && companyOnly !== 'true' && companyOnly !== 'false') throw invalid();
  if (projectID && companyOnly === 'true') throw invalid();
  return { projectID, companyOnly: companyOnly === 'true' };
}
export function businessFilters(selection: BusinessSelection, kind?: ProjectBusinessKind): Record<string, string> {
  if (selection.projectID) return { project_id: businessProjectID(selection.projectID), ...(kind === 'costs' || kind === 'payments' ? { project_records: 'true' } : {}) };
  return selection.companyOnly ? { company_only: 'true' } : {};
}
export function checkedBusinessContext(value: ProjectBusinessContext, project: string): ProjectBusinessContext {
  if (!value || value.scope !== 'project_business' || !Number.isFinite(Date.parse(value.as_of)) || !value.project || value.project.id !== businessProjectID(project) || !Number.isSafeInteger(value.project.version) || value.project.version < 1 || !['draft', 'active', 'on_hold', 'closing', 'archived', 'cancelled'].includes(value.project.state) || !['internal', 'project_restricted', 'confidential'].includes(value.project.classification ?? '') || !Array.isArray(value.capabilities) || value.capabilities.length !== 6 || new Set(value.capabilities.map(item => item?.kind)).size !== 6 || projectBusinessKinds.some(kind => !value.capabilities.some(item => item.kind === kind))) throw invalid();
  for (const item of value.capabilities) {
    if (typeof item.can_create !== 'boolean' || item.can_create && item.blocked_reason !== null || !item.can_create && !['project_read_only', 'classification_requires_grant', 'not_authorized'].includes(item.blocked_reason ?? '')) throw invalid();
    if (['closing', 'archived', 'cancelled'].includes(value.project.state) && (item.can_create || item.blocked_reason !== 'project_read_only')) throw invalid();
    if (!['closing', 'archived', 'cancelled'].includes(value.project.state) && value.project.classification === 'confidential' && (item.can_create || item.blocked_reason !== 'classification_requires_grant')) throw invalid();
  }
  return value;
}
export function projectBusinessURL(kind: ProjectBusinessKind, project: string, client: 'web' | 'mobile', create = false): string {
  if (!projectBusinessKinds.includes(kind) || !['web', 'mobile'].includes(client)) throw invalid();
  const id = businessProjectID(project);
  if (client === 'web') return '/' + kind + '?project_id=' + id + (create ? '&create=1' : '');
  if (!create) return '/pages/list/index?kind=' + kind + '&project_id=' + id;
  if (kind === 'contracts' || kind === 'purchases' || kind === 'claims') return '/pages/business/index?kind=' + ({ contracts: 'contract', purchases: 'purchase', claims: 'claim' }[kind]) + '&project_id=' + id;
  return '/pages/finance/index?kind=' + ({ costs: 'cost', payments: 'payment', subscriptions: 'subscription' } as const)[kind as 'costs' | 'payments' | 'subscriptions'] + '&action=create&project_id=' + id;
}
export function projectBusinessHome(project: string, client: 'web' | 'mobile'): string {
  const id = businessProjectID(project);
  return client === 'web' ? '/projects/' + id + '?tab=overview' : '/pages/project/index?id=' + id + '&section=overview';
}
