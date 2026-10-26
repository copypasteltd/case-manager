import type { Schemas } from './session';
import { projectReadOnly } from './project-input';
import { resourceTime, resourceTimeInput } from './project-resource-input';

export type TaskInput = { title: string; body: string; assignee: string; parent: string; milestone: string; start: string; due: string; priority: string; review: string };
type Task = Schemas['Task'];
type Project = Schemas['Project'];
type References = { parent?: Task; milestone?: Schemas['Milestone'] };
const limits: Record<keyof TaskInput, number> = { title: 200, body: 20000, assignee: 36, parent: 36, milestone: 36, start: 40, due: 40, priority: 6, review: 8 };
const uuid = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(value);
const trim = (value: string) => value.replace(/^[\s\u0085]+|[\s\u0085]+$/g, '');
const sameTime = (left?: string | null, right?: string | null) => left && right ? resourceTime(left) === resourceTime(right) : !left && !right;

export function copyTaskInput(value: unknown): TaskInput | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return;
  const fields = value as Record<string, unknown>, copy = {} as TaskInput;
  for (const key of Object.keys(limits) as (keyof TaskInput)[]) {
    if (typeof fields[key] !== 'string' || fields[key].length > limits[key]) return;
    copy[key] = fields[key];
  }
  return copy;
}
export const taskInputCharacters = (input: TaskInput) => Object.values(input).reduce((sum, field) => sum + field.length, 0);
export function currentTaskInput(task?: Task, parent?: Task): TaskInput {
  return { title: task?.title ?? '', body: task?.body ?? '', assignee: task?.assignee_id ?? '', parent: task?.parent_id ?? parent?.id ?? '',
    milestone: task?.milestone_id ?? '', start: resourceTimeInput(task?.start_at), due: resourceTimeInput(task?.due_at),
    priority: task?.priority ?? 'normal', review: task?.review_required ? 'required' : 'direct' };
}
export const taskEditable = (task: Task) => ['todo', 'in_progress', 'in_review', 'blocked'].includes(task.state);
export function taskWritable(project: Project, members: Schemas['ProjectMembers'], me: Schemas['Me']) {
  return !projectReadOnly(project) && members.memberships?.find(member => member.user_id === me.user.id)?.project_role !== 'viewer';
}
function context(project: Project, before?: Task) {
  if (!uuid(project.id) || projectReadOnly(project) || before && (!uuid(before.id) || before.project_id !== project.id || !Number.isSafeInteger(before.version) || before.version < 1 || !taskEditable(before))) {
    throw new Error('当前项目、任务阶段或版本不允许操作，请先重新读取。');
  }
}
export function taskPayload(input: TaskInput, project: Project, members: Schemas['ProjectMembers'], me: Schemas['Me'], before?: Task, references: References = {}): Schemas['TaskCreate'] | Schemas['TaskPatch'] {
  context(project, before);
  if (!taskWritable(project, members, me) || !me.actions.includes(before ? 'task.update' : 'task.create')) throw new Error('当前身份与项目角色不允许保存任务。');
  if (!copyTaskInput(input) || !trim(input.title) || !['low', 'normal', 'high', 'urgent'].includes(input.priority) || !['direct', 'required'].includes(input.review)) throw new Error('请核对任务名称、文字长度、优先级和完成要求。');
  const assignmentChanged = input.assignee !== (before?.assignee_id ?? '');
  if (input.assignee && (!uuid(input.assignee) || !members.items.some(user => user.id === input.assignee && user.status === 'active'))) throw new Error('负责人须为本项目当前有效成员；原负责人不可用时请有权人员重新分配。');
  if (!me.actions.includes('task.assign') && (before ? assignmentChanged : input.assignee && input.assignee !== me.user.id)) throw new Error('分配或清空负责人需要任务分配权限；创建时可选择本人或暂不分配。');
  for (const key of ['parent', 'milestone'] as const) {
    const selected = input[key], original = key === 'parent' ? before?.parent_id : before?.milestone_id;
    if (selected && (!uuid(selected) || selected === before?.id)) throw new Error('关联标识无效，父任务不能是当前任务。');
    if (selected && selected !== original) {
      const reference = references[key];
      if (!reference || reference.id !== selected || reference.project_id !== project.id || (key === 'parent' ? ['done', 'cancelled'] : ['achieved', 'cancelled']).includes(reference.state)) throw new Error('请选择同项目当前可读且未结束的' + (key === 'parent' ? '父任务。' : '里程碑。'));
    }
  }
  const start = input.start ? resourceTime(input.start) : null, due = input.due ? resourceTime(input.due) : null;
  // Canonical strings retain six microsecond digits and sort chronologically.
  if (start && due && due < start) throw new Error('截止时间不能早于开始时间。');
  const common = { title: trim(input.title), body: input.body, priority: input.priority as Schemas['TaskCreate']['priority'], review_required: input.review === 'required',
    parent_id: input.parent || null, milestone_id: input.milestone || null, start_at: start, due_at: due };
  return before ? { expected_version: before.version, ...common, ...(assignmentChanged ? { assignee_id: input.assignee || null } : {}) }
    : { ...common, assignee_id: input.assignee || null };
}
export function taskReceipt(value: unknown, input: TaskInput, project: Project, before?: Task): value is Task {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  try {
    const actual = value as Task;
    return uuid(actual.id) && actual.project_id === project.id && actual.version === (before ? before.version + 1 : 1)
      && (!before || actual.id === before.id) && actual.state === (before?.state ?? 'todo') && actual.title === trim(input.title)
      && actual.body === input.body && actual.priority === input.priority && actual.review_required === (input.review === 'required')
      && (actual.assignee_id ?? '') === input.assignee && (actual.parent_id ?? '') === input.parent && (actual.milestone_id ?? '') === input.milestone
      && sameTime(actual.start_at, input.start) && sameTime(actual.due_at, input.due);
  } catch { return false; }
}
export function dependencyPayload(task: Task, predecessor: Task, project: Project, action: string): Schemas['TaskDependency'] {
  context(project, task);
  if (!uuid(predecessor.id) || predecessor.id === task.id || predecessor.project_id !== task.project_id || !['add', 'remove'].includes(action)) throw new Error('请选择同项目的其他可读任务，并明确添加或移除。');
  if (action === 'add' && !['todo', 'blocked'].includes(task.state) && predecessor.state !== 'done') throw new Error('进行中的任务不能新增未完成前置任务，请先按实际情况标记受阻。');
  return { expected_version: task.version, predecessor_id: predecessor.id, action: action as 'add' | 'remove' };
}
export function dependencyTaskReceipt(value: unknown, task: Task): value is Task {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const actual = value as Task;
  try {
    return actual.id === task.id && actual.project_id === task.project_id && actual.version === task.version + 1
      && actual.state === task.state && actual.title === task.title && actual.body === task.body && actual.priority === task.priority
      && actual.review_required === task.review_required && (actual.assignee_id ?? null) === (task.assignee_id ?? null)
      && (actual.parent_id ?? null) === (task.parent_id ?? null) && (actual.milestone_id ?? null) === (task.milestone_id ?? null)
      && sameTime(actual.start_at, task.start_at) && sameTime(actual.due_at, task.due_at);
  } catch { return false; }
}
export function dependencyProof(edges: unknown, current: Task, receipt: Task, predecessor: Task, action: 'add' | 'remove'): boolean {
  if (!edges || typeof edges !== 'object' || !Array.isArray((edges as Schemas['TaskDependencies']).items)) return false;
  if (current.version !== receipt.version || !dependencyTaskReceipt(current, { ...receipt, version: receipt.version - 1 })) return false;
  const items = (edges as Schemas['TaskDependencies']).items;
  if (items.some(edge => !uuid(edge.predecessor_id) || typeof edge.title !== 'string' || typeof edge.state !== 'string') || new Set(items.map(edge => edge.predecessor_id)).size !== items.length) return false;
  return items.some(edge => edge.predecessor_id === predecessor.id) === (action === 'add');
}
