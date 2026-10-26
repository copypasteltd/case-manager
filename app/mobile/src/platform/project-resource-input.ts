import type { Schemas } from './session';
import { projectReadOnly } from './project-input';

export type ResourceKind = 'milestone' | 'risk' | 'issue';
export type ResourceItem = Schemas['Milestone'] | Schemas['Risk'] | Schemas['Issue'];
export type ResourceInput = { title: string; owner: string; due: string; description: string; severity: string; probability: string; mitigation: string; resolution: string };
export type ResourceCommand = { path: string; method: 'POST' | 'PATCH'; body: Record<string, unknown> };
const uuid = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(value);
const trim = (value: string) => value.replace(/^[\s\u0085]+|[\s\u0085]+$/g, '');
const limits: Record<keyof ResourceInput, number> = { title: 200, owner: 36, due: 40, description: 20000, severity: 8, probability: 8, mitigation: 20000, resolution: 20000 };
export const resourceLabels = { milestone: '里程碑', risk: '风险', issue: '问题' };
export const resourcePlural = { milestone: 'milestones', risk: 'risks', issue: 'issues' } as const;
export function copyResourceInput(value: unknown): ResourceInput | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return;
  const input = value as Record<string, unknown>, result = {} as ResourceInput;
  for (const key of Object.keys(limits) as (keyof ResourceInput)[]) {
    if (typeof input[key] !== 'string' || input[key].length > limits[key]) return;
    result[key] = input[key];
  }
  return result;
}
export const resourceInputCharacters = (input: ResourceInput) => Object.values(input).reduce((sum, value) => sum + value.length, 0);

// Plain dates are always Hong Kong wall time, never the device time zone.
// Keep all six PostgreSQL microsecond digits when comparing command receipts.
export function resourceTime(value: string): string {
  let text = value.trim();
  if (/^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?$/.test(text)) text = text.replace(' ', 'T') + '+08:00';
  const match = /^(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,6}))?)?(Z|[+-](\d{2}):(\d{2}))$/.exec(text);
  if (!match || Number(match[1].slice(0, 4)) < 1 || Number(match[2]) > 23 || Number(match[3]) > 59 || Number(match[4] ?? 0) > 59 || Number(match[7] ?? 0) > 23 || Number(match[8] ?? 0) > 59) throw new Error('请填写有效的香港日期时间，例如2028-02-29 18:00；含时区输入须使用有效时区。');
  const local = match[1] + 'T' + match[2] + ':' + match[3] + ':' + (match[4] ?? '00');
  if (!Number.isFinite(Date.parse(local + 'Z')) || new Date(local + 'Z').toISOString().slice(0, 19) !== local) throw new Error('日期时间不存在，请核对月份、日期和时间。');
  const actual = new Date(local + match[6]);
  if (!Number.isFinite(actual.valueOf()) || actual.getUTCFullYear() < 1 || !/^\d{4}-/.test(actual.toISOString())) throw new Error('日期时间超出允许范围。');
  const fraction = (match[5] ?? '').padEnd(6, '0');
  return actual.toISOString().slice(0, 19) + '.' + fraction + 'Z';
}
export function resourceTimeInput(value?: string | null): string {
  if (!value) return '';
  const exact = resourceTime(value), wall = new Date(Date.parse(exact.slice(0, 19) + 'Z') + 8 * 3600000).toISOString().slice(0, 19).replace('T', ' ');
  const fraction = exact.slice(20, 26).replace(/0+$/, '');
  return wall + (fraction ? '.' + fraction : '');
}
export function currentResourceInput(project: Schemas['Project'], item?: ResourceItem): ResourceInput {
  return { title: item?.title ?? '', owner: item ? item.owner_id ?? '' : project.owner_id, due: resourceTimeInput(item?.due_at),
    description: item && 'description' in item ? item.description ?? '' : '', severity: item && 'severity' in item ? item.severity ?? 'medium' : 'medium',
    probability: item && 'probability' in item ? item.probability ?? '' : '', mitigation: item && 'mitigation' in item ? item.mitigation ?? '' : '',
    resolution: item && 'resolution' in item ? item.resolution ?? '' : '' };
}
export function resourceEditable(kind: ResourceKind, item: ResourceItem) {
  return kind === 'milestone' ? ['open', 'missed'].includes(item.state) : !['closed', 'realized'].includes(item.state);
}
function writable(project: Schemas['Project'], item?: ResourceItem) {
  if (!uuid(project.id) || projectReadOnly(project) || item && (item.project_id !== project.id || !uuid(item.id) || !Number.isSafeInteger(item.version) || item.version < 1)) throw new Error('当前项目、事项版本或阶段不允许操作，请重新读取。');
}
export function resourcePayload(kind: ResourceKind, input: ResourceInput, project: Schemas['Project'], members: Schemas['ProjectMembers'], item?: ResourceItem): ResourceCommand {
  writable(project, item);
  if (!copyResourceInput(input) || !trim(input.title)) throw new Error('请填写有效标题，并核对文字长度。');
  if (item && !resourceEditable(kind, item)) throw new Error('当前事项须先重开；已发生风险请在关联问题中跟进。');
  if (!uuid(input.owner) || !members.items.some(user => user.id === input.owner && user.status === 'active')) throw new Error('请选择本项目当前有效成员作为负责人。');
  const body: Record<string, unknown> = { ...(item ? { expected_version: item.version } : {}), title: trim(input.title), owner_id: input.owner };
  if (input.due) body.due_at = resourceTime(input.due);
  else if (kind === 'milestone') throw new Error('里程碑需要明确的目标时间。');
  else if (item?.due_at) throw new Error('当前事项的截止时间可修改，暂不支持清空；请保留原值或填写新时间。');
  if (kind !== 'milestone') {
    if (!['low', 'medium', 'high', 'critical'].includes(input.severity)) throw new Error('请选择有效的影响程度。');
    Object.assign(body, { description: input.description, severity: input.severity });
    if (kind === 'risk') {
      if (input.probability && !['low', 'medium', 'high'].includes(input.probability)) throw new Error('请选择有效的发生概率。');
      if (!input.probability && item && 'probability' in item && item.probability) throw new Error('当前发生概率可修改，暂不支持清空。');
      if (input.probability) body.probability = input.probability;
      body.mitigation = input.mitigation;
    } else body.resolution = input.resolution;
  }
  return { path: item ? '/' + resourcePlural[kind] + '/' + item.id : '/projects/' + project.id + '/' + resourcePlural[kind], method: item ? 'PATCH' : 'POST', body };
}
const concernTransitions: Record<'risk' | 'issue', Record<string, string[]>> = {
  risk: { open: ['mitigating', 'closed', 'realized'], mitigating: ['closed', 'realized'], closed: ['open'], realized: [] },
  issue: { open: ['investigating', 'resolved'], investigating: ['resolved'], resolved: ['closed', 'investigating'], closed: ['open'] },
};
export const resourceActionLabels: Record<string, string> = { achieve: '标记里程碑达成', miss: '标记里程碑错过', cancel: '取消里程碑', reopen: '重开里程碑', mitigating: '开始风险应对', closed: '关闭事项', realized: '风险已发生，转为问题', open: '重新打开事项', investigating: '开始或继续调查', resolved: '标记问题已解决' };
export function resourceTransitions(kind: ResourceKind, item: ResourceItem): string[] {
  if (kind !== 'milestone') return concernTransitions[kind][item.state] ?? [];
  if (item.state === 'open') return ['achieve', ...('overdue' in item && item.overdue ? ['miss'] : []), 'cancel'];
  if (item.state === 'missed') return ['achieve', 'cancel', 'reopen'];
  return ['achieved', 'cancelled'].includes(item.state) ? ['reopen'] : [];
}
export function resourceTransition(kind: ResourceKind, item: ResourceItem, project: Schemas['Project'], action: string, reason: string, resolution: string, actions: string[]): ResourceCommand {
  writable(project, item);
  if (!resourceTransitions(kind, item).includes(action) || !trim(reason) || trim(reason).length > 200) throw new Error('请选择当前有效的状态操作，并填写200字以内的原因。');
  if (kind === 'risk' && action === 'realized' && !actions.includes('issue.create')) throw new Error('风险转为问题同时需要本项目的问题创建权限。');
  const body: Record<string, unknown> = { expected_version: item.version, reason: trim(reason) };
  if (kind === 'milestone') body.action = action;
  else {
    body.state = action;
    if (kind === 'issue' && ['resolved', 'closed'].includes(action)) {
      if (!trim(resolution) || resolution.length > 20000) throw new Error('解决或关闭问题前，请填写实际解决说明。');
      body.resolution = resolution;
    }
  }
  return { path: '/' + resourcePlural[kind] + '/' + item.id + (kind === 'milestone' ? '/transition' : ''), method: kind === 'milestone' ? 'POST' : 'PATCH', body };
}
const sameTime = (left?: string | null, right?: string | null) => left && right ? resourceTime(left) === resourceTime(right) : !left && !right;
export function resourceReceipt(value: unknown, kind: ResourceKind, input: ResourceInput, project: Schemas['Project'], before?: ResourceItem): value is ResourceItem {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const actual = value as ResourceItem;
  try {
    if (!uuid(actual.id) || actual.project_id !== project.id || actual.version !== (before ? before.version + 1 : 1) || before && actual.id !== before.id
      || actual.state !== (before?.state ?? 'open') || actual.title !== trim(input.title) || (actual.owner_id ?? '') !== input.owner || !sameTime(actual.due_at, input.due)) return false;
    if (kind === 'milestone') return true;
    const concern = actual as Schemas['Risk'] & Schemas['Issue'];
    if ((concern.description ?? '') !== input.description || concern.severity !== input.severity) return false;
    if (kind === 'issue') return (concern.resolution ?? '') === input.resolution;
    return (concern.probability ?? '') === input.probability && (concern.mitigation ?? '') === input.mitigation
      && (concern.issue_id ?? null) === (before && 'issue_id' in before ? before.issue_id ?? null : null);
  } catch { return false; }
}
export function resourceTransitionReceipt(value: unknown, kind: ResourceKind, before: ResourceItem, action: string, resolution: string): value is ResourceItem {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const actual = value as ResourceItem;
  const state = kind === 'milestone' ? ({ achieve: 'achieved', miss: 'missed', cancel: 'cancelled', reopen: 'open' } as Record<string, string>)[action] : action;
  if (!resourceTransitions(kind, before).includes(action) || actual.id !== before.id || actual.project_id !== before.project_id || actual.version !== before.version + 1 || actual.state !== state) return false;
  const input = currentResourceInput({ owner_id: before.owner_id ?? '' } as Schemas['Project'], before);
  if (kind === 'issue' && ['resolved', 'closed'].includes(action)) input.resolution = resolution;
  const baseline = { ...before, state } as ResourceItem;
  if (kind === 'risk' && action === 'realized') {
    if (!('issue_id' in actual) || !uuid(actual.issue_id) || actual.issue_id === actual.id) return false;
    Object.assign(baseline, { issue_id: actual.issue_id });
  }
  return resourceReceipt(actual, kind, input, { id: before.project_id } as Schemas['Project'], baseline);
}
