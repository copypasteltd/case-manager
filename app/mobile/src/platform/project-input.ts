import type { Schemas } from './session';

export type ProjectInput = { name: string; type: string; scope: string; start: string; due: string; classification: string };
type Project = Schemas['Project'];
type Template = Schemas['ProjectTemplate'];
const uuid = (value: unknown): value is string => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value);
const trim = (value: string) => value.replace(/^[\s\u0085]+|[\s\u0085]+$/g, '');
const limits: Record<keyof ProjectInput, number> = { name: 200, type: 100, scope: 20000, start: 10, due: 10, classification: 24 };
export function copyProjectInput(value: unknown): ProjectInput | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return;
  const input = value as Record<string, unknown>, result = {} as ProjectInput;
  for (const key of Object.keys(limits) as (keyof ProjectInput)[]) {
    if (typeof input[key] !== 'string' || input[key].length > limits[key]) return;
    result[key] = input[key];
  }
  return result;
}
export const projectInputCharacters = (input: ProjectInput) => Object.values(input).reduce((sum, value) => sum + value.length, 0);
export function currentProjectInput(project?: Project): ProjectInput {
  return { name: project?.name ?? '', type: project?.type ?? 'general', scope: project?.scope ?? '', start: project?.start_date ?? '', due: project?.due_date ?? '', classification: project?.classification ?? 'project_restricted' };
}
export const projectReadOnly = (project: Project) => ['closing', 'archived', 'cancelled'].includes(project.state);
export const projectControlled = (project: Project) => ['active', 'on_hold'].includes(project.state);
function validDate(value: string) {
  if (!value) return true;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value) || Number(value.slice(0, 4)) < 1) return false;
  const instant = new Date(value + 'T00:00:00Z');
  return !Number.isNaN(instant.valueOf()) && instant.toISOString().slice(0, 10) === value;
}
export function projectPayload(input: ProjectInput, me: Schemas['Me'], project?: Project, template?: Template): Schemas['ProjectCreate'] | Schemas['ProjectPatch'] {
  const copy = copyProjectInput(input);
  if (!copy || !trim(input.name) || !trim(input.type)) throw new Error('请填写有效项目名称、类型和范围资料。');
  if (!validDate(input.start) || !validDate(input.due) || input.start && input.due && input.start > input.due) throw new Error('请使用有效的 YYYY-MM-DD 日期，截止日期不能早于开始日期。');
  const common = { name: trim(input.name), type: trim(input.type) };
  if (project) {
    if (projectReadOnly(project)) throw new Error('当前项目阶段只允许查阅，请按原规则重开。');
    if (projectControlled(project)) {
      if (input.scope !== project.scope || input.start !== (project.start_date ?? '') || input.due !== (project.due_date ?? '')) throw new Error('进行中项目的范围和日期须经独立项目变更审批后实施。');
      return { expected_version: project.version, ...common };
    }
    return { expected_version: project.version, ...common, scope: input.scope, start_date: input.start || null, due_date: input.due || null };
  }
  if (!uuid(me.user.id) || !['internal', 'project_restricted'].includes(input.classification)) throw new Error('请核对本人身份和项目访问范围。');
  if (template && (!uuid(template.id) || template.state !== 'published' || !Number.isSafeInteger(template.revision) || template.revision < 1 || !/^[0-9a-f]{64}$/.test(template.content_hash))) throw new Error('请选择准确已发布的模板修订。');
  return { ...common, scope: input.scope, owner_id: me.user.id, classification: input.classification as 'internal' | 'project_restricted', ...(input.start ? { start_date: input.start } : {}), ...(input.due ? { due_date: input.due } : {}), ...(template ? { template_id: template.id } : {}) };
}
export function projectReceipt(value: unknown, input: ProjectInput, me: Schemas['Me'], before?: Project, template?: Template): value is Project {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const actual = value as Project;
  return uuid(actual.id) && actual.version === (before ? before.version + 1 : 1)
    && (!before || actual.id === before.id) && actual.name === trim(input.name) && actual.type === trim(input.type)
    && actual.scope === input.scope && (actual.start_date ?? '') === input.start && (actual.due_date ?? '') === input.due
    && actual.owner_id === (before?.owner_id ?? me.user.id) && actual.classification === (before?.classification ?? input.classification)
    && actual.state === (before?.state ?? 'draft') && typeof actual.project_no === 'string' && actual.project_no.length > 0
    && (actual.template_id ?? null) === (before?.template_id ?? template?.id ?? null)
    && (actual.template_version ?? null) === (before?.template_version ?? template?.revision ?? null)
    && ['pending', 'complete', 'partial_failed'].includes(actual.initialization_state ?? '');
}
export const memberRoleNames = { owner: '项目负责人', manager: '项目管理员', member: '项目成员', viewer: '只读成员' };
export function memberPayload(project: Project, members: Schemas['ProjectMembers'], me: Schemas['Me'], userID: string, action: string, reason: string): Schemas['ProjectMemberChange'] {
  if (projectReadOnly(project) || !uuid(userID) || userID === me.user.id || userID === project.owner_id) throw new Error('不能在此变更本人、项目负责人或只读项目的成员资格。');
  if (!reason.trim() || reason.trim().length > 200) throw new Error('请填写200字以内的成员变更原因。');
  if (action === 'remove') {
    if (!members.items.some(user => user.id === userID)) throw new Error('请先核对待移除的实际项目成员。');
    return { expected_version: project.version, user_id: userID, action: 'remove', reason: reason.trim() };
  }
  if (!['member', 'manager', 'viewer'].includes(action)) throw new Error('请选择有效项目角色。');
  return { expected_version: project.version, user_id: userID, action: 'set', project_role: action as 'member' | 'manager' | 'viewer', reason: reason.trim() };
}
export function ownerPayload(project: Project, userID: string): Schemas['ProjectPatch'] {
  if (projectReadOnly(project) || !uuid(userID) || userID === project.owner_id) throw new Error('请选择另一位实际有效用户，并核对项目当前阶段。');
  return { expected_version: project.version, owner_id: userID };
}
export function projectVersionReceipt(value: unknown, before: Project, owner = before.owner_id): value is Project {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const actual = value as Project;
  return actual.id === before.id && actual.version === before.version + 1 && actual.owner_id === owner && actual.name === before.name && actual.state === before.state;
}
