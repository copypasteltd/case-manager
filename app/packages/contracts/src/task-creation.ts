import type { components } from './api';
import type { MutationField } from './mutation';
import { trimMutationText, validMutationText, validMutationInstant, mutationInstantKey } from './mutation-validation';

type Mutation = components['schemas']['AgentMutationInput'];
type View = components['schemas']['AgentMutationView'];
export interface TaskCreationValues {
  title: string;
  body: string;
  assignee_id: string | null;
  parent_id: string | null;
  milestone_id: string | null;
  start_at: string | null;
  due_at: string | null;
  priority: string;
  review_required: boolean;
}
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
export const taskCreationFields: readonly MutationField[] = [
  { name: 'title', label: '任务标题', kind: 'name' },
  { name: 'body', label: '完整任务说明', kind: 'text' },
  { name: 'assignee_id', label: '任务负责人', kind: 'member', nullable: true },
  { name: 'parent_id', label: '父任务', kind: 'task', nullable: true },
  { name: 'milestone_id', label: '关联里程碑', kind: 'milestone', nullable: true },
  { name: 'start_at', label: '开始时刻', kind: 'instant', nullable: true },
  { name: 'due_at', label: '截止时刻', kind: 'instant', nullable: true },
  { name: 'priority', label: '优先级', kind: 'choice', options: [['low', '低'], ['normal', '普通'], ['high', '高'], ['urgent', '紧急']] },
  { name: 'review_required', label: '需要人工验收', kind: 'choice', options: [['true', '需要人工验收'], ['false', '无需人工验收']] },
];

export function makeTaskCreation(project: { id: string; version: number; state: string }, values: TaskCreationValues, reason: string): Mutation {
  if (!uuid.test(project.id) || !Number.isSafeInteger(project.version) || project.version < 1 || !['draft', 'active', 'on_hold'].includes(project.state)) throw new Error('请重新读取可写入项目及准确基础版本。');
  const fields = taskCreationFields.map(f => f.name);
  if (fields.some(f => !Object.prototype.hasOwnProperty.call(values, f)) || Object.keys(values).some(f => !fields.includes(f))) throw new Error('请完整核对九项任务资料；可空值须明确设置或不设置。');
  if (!validMutationText(reason, 200, true) || !validMutationText(values.title, 200, true) || !validMutationText(values.body, 20000)) throw new Error('请核对任务标题、完整说明和本次原因。');
  if (!['low', 'normal', 'high', 'urgent'].includes(values.priority) || typeof values.review_required !== 'boolean') throw new Error('请明确选择优先级与人工验收要求。');
  for (const id of [values.assignee_id, values.parent_id, values.milestone_id]) if (id !== null && (typeof id !== 'string' || !uuid.test(id))) throw new Error('请选择实际项目成员、同项目父任务与里程碑，或明确不设置。');
  for (const date of [values.start_at, values.due_at]) if (date !== null && !validMutationInstant(date)) throw new Error('起止时刻须为有效日期，含时区，最多六位小数秒。');
  if (values.start_at && values.due_at && mutationInstantKey(values.due_at) < mutationInstantKey(values.start_at)) throw new Error('截止时刻不能早于开始时刻。');
  const mutation: Mutation = { tool: 'create_task', arguments: { id: project.id, expected_version: project.version, reason, ...values } as Mutation['arguments'] };
  const bytes = [...JSON.stringify(mutation.arguments)].reduce((n, c) => n + (c.codePointAt(0)! > 0xffff ? 4 : c.charCodeAt(0) > 0x7ff ? 3 : c.charCodeAt(0) > 0x7f ? 2 : 1), 0);
  if (bytes > 65536) throw new Error('完整任务参数超过64KiB，请精简说明后重新核对。');
  return mutation;
}

export function taskCreationReceipt(m: View, actor: string): boolean {
  const r = m.result, a = m.arguments;
  if (m.tool !== 'create_task' || m.status !== 'committed' || !r || typeof m.operation_id !== 'string' || !uuid.test(m.operation_id) ||
    m.result_version !== 1 || r.version !== 1 || typeof r.id !== 'string' || !uuid.test(r.id) || r.id === m.resource_id || r.project_id !== m.resource_id ||
    r.creator_id !== actor || r.state !== 'todo' || r.title !== trimMutationText(String(a.title)) || r.body !== a.body || r.priority !== a.priority || r.review_required !== a.review_required) return false;
  for (const key of ['assignee_id', 'parent_id', 'milestone_id'] as const) if (r[key] !== a[key]) return false;
  for (const key of ['start_at', 'due_at'] as const) {
    if (a[key] === null ? r[key] != null : !validMutationInstant(a[key]) || !validMutationInstant(r[key]) || mutationInstantKey(a[key]) !== mutationInstantKey(r[key])) return false;
  }
  return true;
}
