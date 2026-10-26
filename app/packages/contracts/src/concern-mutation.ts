import type { components } from './api';
import type { MutationField } from './mutation';

export type ConcernTool = 'create_risk' | 'update_risk' | 'create_issue' | 'update_issue';
type Mutation = components['schemas']['AgentMutationInput'];
type View = components['schemas']['AgentMutationView'];
export interface ConcernValues {
  title: string;
  description: string;
  owner_id: string;
  severity: string;
  due_at: string | null;
  probability?: string | null;
  mitigation?: string;
  resolution?: string;
  state?: string;
}
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const severities = [['low', '低'], ['medium', '中'], ['high', '高'], ['critical', '严重']] as const;
export const concernTransitions: Readonly<Record<'risk' | 'issue', Readonly<Record<string, readonly string[]>>>> = {
  risk: { open: ['mitigating', 'closed', 'realized'], mitigating: ['closed', 'realized'], closed: ['open'], realized: [] },
  issue: { open: ['investigating', 'resolved'], investigating: ['resolved'], resolved: ['closed', 'investigating'], closed: ['open'] },
};
export const concernStates: Readonly<Record<string, string>> = { open: '待处理', mitigating: '缓解中', realized: '已发生并转成问题', investigating: '调查中', resolved: '已解决', closed: '已关闭' };
const base: readonly MutationField[] = [
  { name: 'title', label: '事项标题', kind: 'name' },
  { name: 'description', label: '完整说明', kind: 'text' },
  { name: 'owner_id', label: '项目成员负责人', kind: 'member' },
  { name: 'severity', label: '严重程度', kind: 'choice', options: severities },
  { name: 'due_at', label: '截止时刻', kind: 'instant', nullable: true },
];
const risk: readonly MutationField[] = [...base,
  { name: 'probability', label: '发生概率', kind: 'choice', nullable: true, options: severities.slice(0, 3) },
  { name: 'mitigation', label: '缓解措施', kind: 'text' },
];
const issue: readonly MutationField[] = [...base, { name: 'resolution', label: '解决说明', kind: 'text' }];
const state: MutationField = { name: 'state', label: '明确的新状态', kind: 'choice' };
export const concernMutationFields: Readonly<Record<ConcernTool, readonly MutationField[]>> = {
  create_risk: risk, update_risk: [...risk, state], create_issue: issue, update_issue: [...issue, state],
};
export function isConcernMutation(tool: string): tool is ConcernTool {
  return tool === 'create_risk' || tool === 'update_risk' || tool === 'create_issue' || tool === 'update_issue';
}
// Match Go strings.TrimSpace, including U+0085 and excluding U+FEFF.
function trim(value: string): string {
  return value.replace(/^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g, '');
}
function text(value: unknown, maximum: number, required = false): value is string {
  if (typeof value !== 'string' || [...value].length > maximum || required && !trim(value)) return false;
  return ![...value].some(c => c.length === 1 && c.charCodeAt(0) >= 0xd800 && c.charCodeAt(0) <= 0xdfff);
}
function instant(value: unknown): value is string {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T([01]\d|2[0-3]):[0-5]\d:[0-5]\d(\.\d{1,6})?(Z|[+-]\d{2}:\d{2})$/.test(value) || value.startsWith('0000-') || !Number.isFinite(Date.parse(value))) return false;
  const day = new Date(value.slice(0, 10) + 'T00:00:00Z');
  return Number.isFinite(day.getTime()) && day.toISOString().slice(0, 10) === value.slice(0, 10);
}
function instantKey(value: string): string {
  const fraction = /\.(\d+)(?:Z|[+-])/.exec(value)?.[1] ?? '';
  return new Date(value).toISOString().slice(0, 19) + '.' + fraction.padEnd(6, '0') + 'Z';
}
export function makeConcernMutation(tool: ConcernTool, target: { id: string; version: number; state?: string }, values: ConcernValues, reason: string): Mutation {
  const creating = tool.startsWith('create_'), kind = tool.endsWith('_risk') ? 'risk' : 'issue';
  if (!isConcernMutation(tool) || !uuid.test(target.id) || !Number.isSafeInteger(target.version) || target.version < 1 || !text(reason, 200, true)) throw new Error('请重新读取准确对象、版本并填写操作原因。');
  const required = concernMutationFields[creating ? tool : kind === 'risk' ? 'create_risk' : 'create_issue'].map(f => f.name);
  if (required.some(k => !Object.prototype.hasOwnProperty.call(values, k)) || Object.keys(values).some(k => !required.includes(k) && (creating || k !== 'state'))) throw new Error('请完整核对所有事项资料；截止时刻和风险概率必须明确设置或清空。');
  if (!text(values.title, 200, true) || !text(values.description, 20000) || !uuid.test(values.owner_id) || !severities.some(s => s[0] === values.severity) || values.due_at !== null && !instant(values.due_at)) throw new Error('请核对事项标题、说明、实际成员、严重程度和含时区的有效时刻。');
  if (kind === 'risk' ? !text(values.mitigation, 20000) || values.probability !== null && !['low', 'medium', 'high'].includes(String(values.probability)) : !text(values.resolution, 20000)) throw new Error('请核对风险概率、缓解措施或问题解决说明。');
  if (creating ? !['draft', 'active', 'on_hold'].includes(target.state ?? '') : !target.state || !concernTransitions[kind][target.state]) throw new Error('请核对当前项目或事项是否仍可修改。');
  if (values.state !== undefined && (creating || !target.state || !concernTransitions[kind][target.state]?.includes(values.state))) throw new Error('此状态变更不符合当前事项状态；保持状态时请省略状态字段。');
  if (!creating && kind === 'risk' && target.state === 'realized' || !creating && target.state === 'closed' && values.state !== 'open') throw new Error('已发生的风险须在关联问题中跟进，已关闭事项须先重新打开。');
  if (kind === 'issue' && (values.state === 'resolved' || values.state === 'closed') && !trim(values.resolution ?? '')) throw new Error('解决或关闭问题必须填写解决说明。');
  const input: Mutation = { tool, arguments: { id: target.id, expected_version: target.version, reason, ...values } as Mutation['arguments'] };
  const bytes = [...JSON.stringify(input.arguments)].reduce((n, c) => n + (c.codePointAt(0)! > 0xffff ? 4 : c.charCodeAt(0) > 0x7ff ? 3 : c.charCodeAt(0) > 0x7f ? 2 : 1), 0);
  if (bytes > 65536) throw new Error('本次完整事项参数超过64KiB，请精简说明后重新核对。');
  return input;
}
export function concernMutationReceipt(m: View, actor?: string): boolean {
  if (!isConcernMutation(m.tool) || !m.result || m.status !== 'committed' || typeof m.operation_id !== 'string' || !uuid.test(m.operation_id)) return false;
  const r = m.result, a = m.arguments, creating = m.tool.startsWith('create_'), risk = m.tool.endsWith('_risk');
  if (typeof r.id !== 'string' || !uuid.test(r.id) || typeof r.creator_id !== 'string' || !uuid.test(r.creator_id)) return false;
  if (creating ? r.id === m.resource_id || r.version !== 1 || m.result_version !== 1 || r.project_id !== m.resource_id || r.creator_id !== actor || r.state !== 'open' : r.id !== m.resource_id || r.version !== m.base_version + 1 || m.result_version !== r.version || r.project_id !== m.before.project_id || r.creator_id !== m.before.creator_id || r.state !== (a.state ?? m.before.state)) return false;
  if (r.title !== trim(String(a.title)) || r.description !== a.description || r.owner_id !== a.owner_id || r.severity !== a.severity) return false;
  if (a.due_at === null ? r.due_at != null : !instant(r.due_at) || !instant(a.due_at) || instantKey(r.due_at) !== instantKey(a.due_at)) return false;
  if (risk) {
    if ((r.probability ?? null) !== a.probability || (r.mitigation ?? '') !== a.mitigation) return false;
    if (r.state === 'realized') return !creating && m.before.issue_id == null && typeof r.issue_id === 'string' && uuid.test(r.issue_id) && r.issue_id !== r.id && r.issue_id !== r.project_id;
    return r.issue_id == null;
  }
  return (r.resolution ?? '') === a.resolution && r.issue_id == null;
}
