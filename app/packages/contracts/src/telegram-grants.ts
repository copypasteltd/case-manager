import type { components } from './api';
type S = components['schemas'];
export const telegramProjectTools = [
  'submit_deliverable', 'update_task_dependency', 'create_task', 'create_risk', 'update_risk', 'create_issue', 'update_issue',
  'create_deliverable', 'update_deliverable', 'create_milestone', 'update_project', 'update_task', 'transition_task',
  'transition_project', 'update_milestone', 'transition_milestone',
] as const;
export const grantStateLabels: Record<S['TelegramWriteGrant']['state'], string> = {
  pending_trigger: '等待本人原群触发', consumed: '触发已使用，请查实际结果', stopped: '执行已停止', expired: '触发已到期', invalidated: '原授权或业务依据已失效',
};
export const grantToolLabels: Record<typeof telegramProjectTools[number], string> = {
  submit_deliverable:'提交准确交付修订',update_task_dependency:'更改任务前置关系',create_task:'新建任务',create_risk:'新建风险',update_risk:'更新风险',
  create_issue:'新建问题',update_issue:'更新问题',create_deliverable:'新建交付',update_deliverable:'更新交付资料',create_milestone:'新建里程碑',
  update_project:'更新项目资料',update_task:'更新任务资料',transition_task:'更改任务状态',transition_project:'更改项目阶段',update_milestone:'更新里程碑资料',transition_milestone:'更改里程碑状态',
};
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v);
export function telegramGrantAccepted(v: S['TelegramWriteGrant'], conversation: string, input: S['TelegramWriteGrantInput']): boolean {
  const lifetime = Date.parse(v.expires_at) - Date.parse(v.created_at);
  return uuid(v.id) && uuid(v.run_id) && v.conversation_id === conversation && v.channel_id === input.channel_id && v.channel_version === input.channel_version
    && v.binding_id === input.binding_id && v.binding_version === input.binding_version && v.client_message_id === input.run.client_message_id
    && v.tool === input.run.mutation?.tool && telegramProjectTools.includes(v.tool) && v.state === 'pending_trigger' && v.run_state === 'queued'
    && v.run_version === 1 && /^[a-f0-9]{64}$/.test(v.input_sha256) && lifetime > 0 && lifetime <= 600000
    && v.trigger === '/task project-update authorize:' + v.id;
}
export function untriggeredGrant(v:S['TelegramWriteGrant']):boolean {
  return ['pending_trigger','invalidated','expired'].includes(v.state) && v.run_state === 'queued';
}
export function grantCancellationAccepted(v:S['Run'], grant:S['TelegramWriteGrant']):boolean {
  return v.id === grant.run_id && v.conversation_id === grant.conversation_id && v.version === grant.run_version + 1
    && !!v.cancel_requested_at && ['cancelled','running'].includes(v.state);
}
