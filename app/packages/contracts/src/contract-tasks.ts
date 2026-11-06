import type { components } from './api';

type Schemas = components['schemas'];
type Contract = Schemas['Contract'];
type Node = Schemas['ContractMilestone'];
export type ContractTask = Pick<Schemas['Task'], 'id' | 'version' | 'title' | 'state' | 'project_id'>;
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const version = (value: number) => Number.isSafeInteger(value) && value > 0;

export function contractTaskInput(contract: Contract, node: Node, task: ContractTask, reason: string, acknowledge: boolean): Schemas['ContractMilestoneTaskInput'] {
  if (!uuid.test(contract.id) || !uuid.test(node.id) || !uuid.test(task.id) || !uuid.test(task.project_id) || !version(contract.version) || !version(task.version)
    || node.contract_id !== contract.id || !contract.active_revision_id || node.revision_id !== contract.active_revision_id) throw new Error('合同、节点或任务依据已变化，请重新读取后核对。');
  const text = reason.trim();
  if (!text || [...text].length > 200) throw new Error('请填写200字以内的本次操作依据。');
  if (contract.project_id !== task.project_id && !acknowledge) throw new Error('合同与任务范围不同，请核对两侧用途和权限后明确确认。');
  return { contract_version: contract.version, contract_revision_id: node.revision_id, task_id: task.id, task_version: task.version, acknowledge_scope_difference: acknowledge, reason: text };
}

export function contractTaskReceipt(value: unknown, contract: Contract, node: Node, task: ContractTask, body: Schemas['ContractMilestoneTaskInput'], root?: string): value is Schemas['ContractMilestoneTaskEvent'] {
  if (!value || typeof value !== 'object') return false;
  const row = value as Schemas['ContractMilestoneTaskEvent'];
  return uuid.test(row.id ?? '') && version(row.seq) && row.contract_id === contract.id && row.milestone_id === node.id
    && row.contract_revision_id === body.contract_revision_id && row.contract_version === body.contract_version
    && row.action === (root ? 'revoke' : 'link') && row.source_link_id === (root ?? null)
    && row.task_id === task.id && row.task_version === task.version && row.task_project_id === task.project_id
    && row.task_title === task.title && row.task_state === task.state && row.reason === body.reason
    && row.cross_scope === (contract.project_id !== task.project_id) && row.scope_difference_acknowledged === body.acknowledge_scope_difference
    && !row.revoked && uuid.test(row.actor_id ?? '') && Number.isFinite(Date.parse(row.created_at));
}
