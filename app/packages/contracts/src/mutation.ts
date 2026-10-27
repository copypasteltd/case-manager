import { taskCreationFields, taskCreationReceipt } from './task-creation';
export { makeTaskCreation, type TaskCreationValues } from './task-creation';
import { concernMutationFields, concernMutationReceipt, concernStates, isConcernMutation } from './concern-mutation';
export { makeConcernMutation, concernStates, concernTransitions, isConcernMutation, type ConcernTool, type ConcernValues } from './concern-mutation';
import type { components } from './api';

export type Mutation = components['schemas']['AgentMutationInput'];
export type MutationView = components['schemas']['AgentMutationView'];
export type MutationTool = Mutation['tool'];
export const mutationTools: readonly (readonly [MutationTool, string])[] = [['create_task', '创建任务或子任务'], ['create_risk', '创建项目风险'], ['update_risk', '修改风险资料与状态'], ['create_issue', '创建项目问题'], ['update_issue', '修改问题资料与状态'], ['create_deliverable', '创建交付草稿'], ['update_deliverable', '修改交付资料与关联'], ['create_milestone', '创建里程碑'], ['update_project', '修改项目资料'], ['update_task', '修改任务资料'], ['transition_task', '推进任务状态'], ['update_milestone', '修改里程碑资料'], ['transition_milestone', '推进里程碑状态'], ['transition_project', '推进项目阶段']];
export function mutationCreates(tool: MutationTool): boolean { return tool.startsWith('create_'); }
export function mutationTargetType(tool: MutationTool): 'project' | 'task' | 'milestone' | 'deliverable' | 'risk' | 'issue' { if (isConcernMutation(tool)) return tool.startsWith('create_') ? 'project' : tool === 'update_risk' ? 'risk' : 'issue'; return tool === 'update_deliverable' ? 'deliverable' : tool === 'create_task' || tool === 'create_deliverable' || tool === 'create_milestone' || tool.endsWith('_project') ? 'project' : tool.endsWith('_milestone') ? 'milestone' : 'task'; }
export interface MutationField { name: string; label: string; kind: 'name' | 'text' | 'date' | 'instant' | 'choice' | 'member' | 'milestone' | 'task'; nullable?: boolean; options?: readonly (readonly [string, string])[] }
const actions = [['start', '开始处理'], ['review', '提交验收'], ['return', '退回处理'], ['complete', '完成无需人工验收的任务'], ['block', '标记阻塞'], ['resume', '恢复处理'], ['cancel', '取消任务'], ['reopen', '重新打开']] as const;
const projectActions = [['activate','启动项目'],['hold','暂停项目'],['resume','恢复项目'],['close','进入收尾'],['cancel','取消项目'],['reopen','重新打开项目']] as const;
const milestoneActions = [['achieve','依据关联事项确认达成'],['miss','标记已逾期未达成'],['cancel','取消里程碑'],['reopen','重新打开里程碑']] as const;
const states: Record<string, string> = { submitted:'已提交验收',accepted:'已验收',changes_requested:'退回修改', draft: '筹备中', active: '进行中', on_hold: '暂停', closing: '收尾中', archived: '已归档', cancelled: '已取消', todo: '待处理', in_progress: '处理中', in_review: '待验收', done: '已完成', blocked: '受阻', open:'未达成', achieved:'已达成', missed:'逾期未达成' };
export const mutationFields: Record<MutationTool, readonly MutationField[]> = {
  ...concernMutationFields,
  create_task: taskCreationFields,
  create_deliverable: [{name:'name',label:'交付名称',kind:'name'},{name:'acceptance_criteria',label:'验收准则',kind:'text'},{name:'owner_id',label:'交付负责人',kind:'member'},{name:'milestone_id',label:'关联里程碑',kind:'milestone',nullable:true}],
  update_deliverable: [{name:'name',label:'交付名称',kind:'name'},{name:'acceptance_criteria',label:'验收准则',kind:'text'},{name:'owner_id',label:'交付负责人',kind:'member'},{name:'milestone_id',label:'关联里程碑',kind:'milestone',nullable:true}],
  create_milestone: [{name:'title',label:'里程碑标题',kind:'name'},{name:'owner_id',label:'里程碑负责人',kind:'member'},{name:'due_at',label:'目标时刻',kind:'instant'}],
  update_project: [{ name: 'name', label: '项目名称', kind: 'name' }, { name: 'scope', label: '项目范围说明', kind: 'text' }, { name: 'type', label: '项目类型', kind: 'name' }, { name: 'start_date', label: '开始日期', kind: 'date', nullable: true }, { name: 'due_date', label: '计划完成日期', kind: 'date', nullable: true }],
  update_task: [{ name: 'title', label: '任务标题', kind: 'name' }, { name: 'body', label: '任务说明', kind: 'text' }, { name: 'priority', label: '优先级', kind: 'choice', options: [['low', '低'], ['normal', '普通'], ['high', '高'], ['urgent', '紧急']] }, {name:'milestone_id',label:'关联里程碑',kind:'milestone',nullable:true}, { name: 'start_at', label: '开始时刻', kind: 'instant', nullable: true }, { name: 'due_at', label: '截止时刻', kind: 'instant', nullable: true }],
  transition_task: [{ name: 'action', label: '任务动作', kind: 'choice', options: actions }],
  update_milestone: [{name:'title',label:'里程碑标题',kind:'name'},{name:'owner_id',label:'里程碑负责人',kind:'member'},{name:'due_at',label:'目标时刻',kind:'instant'}],
  transition_milestone: [{name:'action',label:'里程碑动作',kind:'choice',options:milestoneActions}],
  transition_project: [{name:'action',label:'项目阶段动作',kind:'choice',options:projectActions}],
};
export function mutationValue(value: unknown, field?: string, tool?: MutationTool): string {
  if (value === null || value === undefined) return '未设置';
  if (field === 'state' && value === 'draft' && tool === 'update_deliverable') return '草稿';
  if (field === 'state' && tool && isConcernMutation(tool) && typeof value === 'string') return concernStates[value] ?? value;
  if (field === 'severity' || field === 'probability') return ({ low: '低', medium: '中', high: '高', critical: '严重' } as Record<string,string>)[String(value)] ?? String(value);
  if (field === 'state' && typeof value === 'string') return states[value] ?? value;
  if (field === 'action') return (tool === 'transition_project' ? projectActions : tool === 'transition_milestone' ? milestoneActions : actions).find(item => item[0] === value)?.[1] ?? String(value);
  if (field === 'review_required' && typeof value === 'boolean') return value ? '需要人工验收' : '无需人工验收';
  if (field === 'priority') return mutationFields.update_task.find(item => item.name === field)?.options?.find(item => item[0] === value)?.[1] ?? String(value);
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}
export function mutationChanges(input: Mutation): readonly { field: string; label: string; value: string }[] {
  return Object.entries(input.arguments).filter(([name]) => !['id', 'expected_version', 'reason'].includes(name)).map(([name, value]) => ({ field: name, label: mutationFields[input.tool].find(item => item.name === name)?.label ?? name, value: value === null ? (mutationCreates(input.tool) ? '明确不设置' : '清空') : mutationValue(value, name, input.tool) }));
}
export function makeMutation(tool: MutationTool, target: { id: string; version: number }, field: string, value: string, clear: boolean, reason: string): Mutation {
  if (tool === 'create_task') throw new Error('创建任务需要完整核对九项资料及人工验收要求。');
  if (isConcernMutation(tool)) throw new Error('风险与问题必须完整核对所有资料，不能发送部分修改。');
  if (tool === 'create_deliverable') throw new Error('创建交付草稿需要完整核对名称、验收准则、负责人和里程碑关联。');
  if (tool === 'update_deliverable') throw new Error('交付更新需要完整核对名称、验收准则、负责人和里程碑关联。');
  if (tool === 'create_milestone') throw new Error('创建里程碑需要完整填写标题、负责人和目标时刻。');
  const definition = mutationFields[tool]?.find(item => item.name === field);
  if (!definition || !/^[a-f0-9-]{36}$/.test(target.id) || !Number.isSafeInteger(target.version) || target.version < 1) throw new Error('请重新读取准确对象和基础版本。');
  if (!reason.trim() || [...reason].length > 200) throw new Error('请填写 1 至 200 字的更新原因。');
  if (clear && !definition.nullable) throw new Error('此字段不能清空。');
  if (!clear) {
    if (definition.kind === 'name' && (!value.trim() || [...value].length > 200)) throw new Error('请填写 1 至 200 字的名称。');
    if (definition.kind === 'text' && [...value].length > 20000) throw new Error('说明不能超过 20000 字。');
    if (definition.options && !definition.options.some(option => option[0] === value)) throw new Error('请明确选择当前可用动作或取值。');
    if (definition.kind === 'date' && (!/^\d{4}-\d{2}-\d{2}$/.test(value) || !Number.isFinite(Date.parse(value)) || new Date(value).toISOString().slice(0, 10) !== value)) throw new Error('日期请使用有效的 YYYY-MM-DD。');
    if (definition.kind === 'instant' && (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})$/.test(value) || !Number.isFinite(Date.parse(value)))) throw new Error('时刻请包含时区，例如 2026-12-01T18:00:00+08:00。');
    if ((definition.kind === 'member' || definition.kind === 'milestone') && !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value)) throw new Error('请选择实际项目成员或同项目里程碑。');
  }
  return { tool, arguments: { id: target.id, expected_version: target.version, reason, [field]: clear ? null : value } };
}
export function makeMilestoneCreation(target: {id:string;version:number}, title:string, owner:string, due:string, reason:string): Mutation {
  // Reuse the same field validation, but never expose partial creation authority.
  for (const [field,value] of [['title',title],['owner_id',owner],['due_at',due]]) makeMutation('update_milestone',target,field,value,false,reason);
  return {tool:'create_milestone',arguments:{id:target.id,expected_version:target.version,title,owner_id:owner,due_at:due,reason}};
}
export function makeDeliverableUpdate(target:{id:string;version:number}, name:string, criteria:string, owner:string, milestone:string|null, reason:string):Mutation {
  makeMutation('update_project',target,'name',name,false,reason);
  makeMutation('update_task',target,'body',criteria,false,reason);
  if (!criteria.trim()) throw new Error('请填写验收准则。');
  makeMutation('update_milestone',target,'owner_id',owner,false,reason);
  makeMutation('update_task',target,'milestone_id',milestone??'',milestone===null,reason);
  return {tool:'update_deliverable',arguments:{id:target.id,expected_version:target.version,name,acceptance_criteria:criteria,owner_id:owner,milestone_id:milestone,reason}};
}
export function makeDeliverableCreation(project:{id:string;version:number}, name:string, criteria:string, owner:string, milestone:string|null, reason:string):Mutation {
  const complete=makeDeliverableUpdate(project,name,criteria,owner,milestone,reason);
  return {tool:'create_deliverable',arguments:complete.arguments};
}
function canonical(value: unknown): string {
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']';
  if (value && typeof value === 'object') return '{' + Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => JSON.stringify(key) + ':' + canonical(item)).join(',') + '}';
  return JSON.stringify(value);
}
export function mutationReceipt(run: components['schemas']['Run'], input?: Mutation): boolean {
  const m = run.mutation;
  if (!input) return !m;
  if (!m || m.run_id !== run.id || m.tool !== input.tool || m.resource_id !== input.arguments.id || m.resource_type !== mutationTargetType(input.tool) || m.base_version !== input.arguments.expected_version || !/^[a-f0-9]{64}$/.test(m.intent_sha256) || canonical(m.arguments) !== canonical(input.arguments) || m.before?.id !== m.resource_id || m.before?.version !== m.base_version) return false;
  if (m.status === 'authorized') return m.operation_id === null && m.result_version === null && m.result === null;
  if (m.tool === 'create_task') return taskCreationReceipt(m, run.actor_id);
  if (isConcernMutation(m.tool)) return concernMutationReceipt(m, run.actor_id);
  if (m.tool === 'create_deliverable') return m.status === 'committed' && typeof m.operation_id === 'string' && /^[a-f0-9-]{36}$/.test(m.operation_id)
    && m.result_version === 1 && m.result?.version === 1 && typeof m.result.id === 'string' && /^[a-f0-9-]{36}$/.test(m.result.id) && m.result.id !== m.resource_id
    && m.result.revision_id === null && Object.keys(m.result).length === 3;
  if (m.tool === 'create_milestone') return m.status === 'committed' && typeof m.operation_id === 'string' && /^[a-f0-9-]{36}$/.test(m.operation_id)
    && m.result_version === 1 && m.result?.version === 1 && typeof m.result.id === 'string' && /^[a-f0-9-]{36}$/.test(m.result.id) && m.result.id !== m.resource_id
    && m.result.project_id === m.resource_id && m.result.state === 'open' && m.result.title === String(m.arguments.title).trim()
    && m.result.owner_id === m.arguments.owner_id && Date.parse(String(m.result.due_at)) === Date.parse(String(m.arguments.due_at));
  if (!(m.status === 'committed' && typeof m.operation_id === 'string' && /^[a-f0-9-]{36}$/.test(m.operation_id) && m.result_version === m.base_version + 1 && m.result?.id === m.resource_id && m.result?.version === m.result_version)) return false;
  if (m.tool === 'update_deliverable') return m.result.revision_id === m.before.current_revision_id;
  if (m.tool === 'transition_project' || m.tool === 'transition_milestone') {
    const expected: Record<string,string> = m.tool === 'transition_project' ? {activate:'active',hold:'on_hold',resume:'active',close:'closing',cancel:'cancelled',reopen:'active'} : {achieve:'achieved',miss:'missed',cancel:'cancelled',reopen:'open'};
    return m.result.state === expected[String(m.arguments.action)] && Object.prototype.hasOwnProperty.call(expected,String(m.arguments.action));
  }
  if (m.tool === 'update_milestone') {
    for (const field of ['title','owner_id','due_at'] as const) {
      const wanted = m.arguments[field];
      if (wanted === undefined) continue;
      if (field === 'due_at') { if (Date.parse(String(m.result[field])) !== Date.parse(String(wanted))) return false; }
      else if (m.result[field] !== (field === 'title' ? String(wanted).trim() : wanted)) return false;
    }
  }
  return true;
}
