import type { components } from './api';

type S = components['schemas'];
export type CatalogItem = S['AgentProjectCatalogItem'];
export type ProjectSelection = { owner?: CatalogItem; members: CatalogItem[]; template?: CatalogItem; type: string; start: string; due: string };
export const emptyProjectSelection = (): ProjectSelection => ({ members: [], type: 'general', start: '', due: '' });
export function projectPlanProject(actor: string, name: string, scope: string, classification: 'internal' | 'project_restricted', choice: ProjectSelection): S['AgentPlanProject'] {
  const valid = (item: CatalogItem, type: CatalogItem['type']) => item.type === type && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(item.id) && item.version > 0 && /^[0-9a-f]{64}$/.test(item.source_sha256);
  if ((choice.owner && !valid(choice.owner, 'agent_project_person')) || choice.members.some(v => !valid(v, 'agent_project_person')) || (choice.template && !valid(choice.template, 'agent_project_template'))) throw new Error('请选择当前有权查询的人员和已发布模板。');
  const day = (v: string) => !v || (/^(?!0000)\d{4}-\d{2}-\d{2}$/.test(v) && Number.isFinite(Date.parse(v + 'T00:00:00Z')) && new Date(v + 'T00:00:00Z').toISOString().slice(0, 10) === v);
  if (!day(choice.start) || !day(choice.due) || (choice.start && choice.due && choice.start > choice.due)) throw new Error('请填写有效日期，截止日期应不早于开始日期。');
  const owner = choice.owner?.id ?? actor;
  const members = [...new Set(choice.members.map(v => v.id).filter(id => id !== owner))].sort();
  if (members.length > 30 || !name.trim() || !choice.type.trim()) throw new Error('请核对项目名称、类型及最多30名额外成员。');
  const typeBytes = [...choice.type.trim()].reduce((n, c) => n + (c.codePointAt(0)! <= 0x7f ? 1 : c.codePointAt(0)! <= 0x7ff ? 2 : c.codePointAt(0)! <= 0xffff ? 3 : 4), 0);
  if (typeBytes > 100) throw new Error('项目类型应不超过100字节，请缩短名称。');
  return { name: name.trim(), scope, classification, owner_id: owner, member_ids: members, type: choice.type.trim(),
    ...(choice.start ? { start_date: choice.start } : {}), ...(choice.due ? { due_date: choice.due } : {}), ...(choice.template ? { template_id: choice.template.id } : {}) };
}
export function catalogSourceMatches(view: S['AgentProjectCatalogDocument'], source: Pick<S['AgentSourceSnapshot'], 'type' | 'id' | 'resource_version' | 'sha256'>): boolean {
  return view.item.id === source.id && view.item.type === source.type && view.item.version === source.resource_version && view.item.source_sha256 === source.sha256 && view.untrusted_data === true;
}
export function projectPlanMatches(plan: S['AgentPlan'], request: S['AgentProjectPlanCreate']): boolean {
  const a = plan.project, b = request.project;
  return a.name === b.name && a.scope === b.scope && a.owner_id === b.owner_id && a.classification === b.classification && (a.type ?? 'general') === (b.type ?? 'general')
    && (a.start_date ?? '') === (b.start_date ?? '') && (a.due_date ?? '') === (b.due_date ?? '') && (a.template_id ?? '') === (b.template_id ?? '')
    && JSON.stringify([...(a.member_ids ?? [])].sort()) === JSON.stringify([...(b.member_ids ?? [])].sort());
}
