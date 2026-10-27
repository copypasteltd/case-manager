import type { components } from './api';
import { validMutationText, validMutationInstant, mutationInstantKey } from './mutation-validation';

type Mutation = components['schemas']['AgentMutationInput'];
type View = components['schemas']['AgentMutationView'];
type Task = Pick<components['schemas']['Task'], 'id' | 'version' | 'project_id' | 'state'>;
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export function makeTaskDependency(task: Task, predecessor: Task, action: string, reason: string, current: readonly string[]): Mutation {
  for (const item of [task, predecessor]) {
    if (!uuid.test(item.id) || !uuid.test(item.project_id) || !Number.isSafeInteger(item.version) || item.version < 1) throw new Error('请重新读取两个准确任务和版本。');
  }
  if (task.id === predecessor.id || task.project_id !== predecessor.project_id) throw new Error('前置任务必须是同项目的另一项任务。');
  if (action !== 'add' && action !== 'remove') throw new Error('请明确选择新增或解除依赖。');
  if (!validMutationText(reason, 200, true)) throw new Error('请填写 1 至 200 字的更新原因。');
  if (['done', 'cancelled'].includes(task.state)) throw new Error('请重开后续任务后修改依赖。');
  if (current.includes(predecessor.id) === (action === 'add')) throw new Error('当前关系不会发生变化，请重新核对依赖。');
  if (action === 'add' && !['todo', 'blocked'].includes(task.state) && predecessor.state !== 'done') throw new Error('进行中的任务不能新增未完成前置任务。');
  return { tool: 'update_task_dependency', arguments: { id: task.id, expected_version: task.version, predecessor_id: predecessor.id, expected_predecessor_version: predecessor.version, action, reason } };
}

export function taskDependencyBasis(m: View): boolean {
  return typeof m.arguments.predecessor_id === 'string' && uuid.test(m.arguments.predecessor_id) && m.arguments.predecessor_id !== m.resource_id
    && Number.isSafeInteger(m.arguments.expected_predecessor_version) && Number(m.arguments.expected_predecessor_version) > 0
    && Object.keys(m.arguments).length === 6 && uuid.test(String(m.before.project_id))
    && ['add', 'remove'].includes(String(m.arguments.action)) && m.before.predecessor_id === m.arguments.predecessor_id
    && m.before.predecessor_version === m.arguments.expected_predecessor_version && typeof m.before.predecessor_title === 'string'
    && typeof m.before.predecessor_state === 'string' && /^[0-9a-f]{64}$/.test(String(m.before.predecessor_sha256))
    && m.before.dependency_present === (m.arguments.action === 'remove');
}

export function taskDependencyReceipt(m: View): boolean {
  const result = m.result;
  if (!taskDependencyBasis(m) || !result || result.project_id !== m.before.project_id || result.predecessor_id !== m.arguments.predecessor_id
    || result.predecessor_version !== m.arguments.expected_predecessor_version || result.dependency_action !== m.arguments.action
    || result.dependency_present !== (m.arguments.action === 'add')) return false;
  for (const field of ['title', 'body', 'state', 'priority', 'assignee_id', 'parent_id', 'milestone_id', 'review_required']) {
    if (result[field] !== m.before[field]) return false;
  }
  for (const field of ['start_at', 'due_at']) {
    if ((result[field] == null) !== (m.before[field] == null)) return false;
    if (result[field] != null && (!validMutationInstant(result[field]) || !validMutationInstant(m.before[field]) || mutationInstantKey(result[field]) !== mutationInstantKey(m.before[field]))) return false;
  }
  return true;
}
