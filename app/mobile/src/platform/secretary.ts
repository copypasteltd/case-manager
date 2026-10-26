import type { Schemas } from './session';

export const runNames: Record<Schemas['Run']['state'], string> = { queued: '等待处理', running: '正在处理', awaiting_approval: '等待计划批准', committing: '正在核对正式提交', succeeded: '已完成', failed: '未完成', cancelled: '已取消', interrupted: '已中断，需核对结果' };
export function runName(run: Schemas['Run']) {
  if(run.state!=='succeeded') return runNames[run.state];
  const outcomes:Record<string,string>={draft:'已回复，草稿待处理',pending:'已回复，事项待处理',awaiting_input:'已回复，等待补充资料',completed:'已回复，本次事项已完成',conflict:'已回复，需核对冲突',denied:'已回复，事项权限不足',failed:'已回复，事项未完成'};
  return outcomes[run.outcome??'']??'已回复，请核对事项结果';
}
export const eventNames: Record<Schemas['RunEvent']['type'], string> = { 'run.queued': '任务已受理', 'run.started': '开始处理', 'run.cancel_requested': '已请求取消，等待核对', 'run.cancelled': '已确认取消', 'run.interrupted': '处理已中断', 'run.failed': '任务未完成', 'run.completed': '处理完成', 'run.awaiting_approval': '等待计划批准', 'run.committing': '正在核对正式提交' };
export const skillNames: Record<string, string> = { 'project-create': '新建项目', 'project-update': '更新项目', 'file-update': '新建或更新文件', 'information-organize': '整理资料', 'oa-assist': '协助业务申请', 'subscription-review': '核对订阅支出', 'project-briefing': '整理项目简报', 'incident-summary': '整理运行事件' };
export function runtimeReason(reason: string | null) {
  return ({ CHANNEL_MANAGED: '此会话由TG入口管理，请从指定群话题发送新的请求。', CONVERSATION_READ_ONLY: '会话已归档，请先恢复再发送。', RUN_NOT_ALLOWED: '当前权限只允许查阅会话。', CONTEXT_UNAVAILABLE: '当前会话的共同阅读范围或 AI 处理许可尚未通过核对。', RUNTIME_UNAVAILABLE: '秘书执行服务暂未启用；已有会话和消息可以继续查阅。' } as Record<string, string>)[reason ?? ''] ?? '当前执行配置尚未确认，请重新读取。';
}
export function runReceipt(value: unknown, expected: { conversation: string; actor: string; profile: string; message: string }): value is Schemas['Run'] {
  if (!value || typeof value !== 'object') return false;
  const run = value as Schemas['Run'];
  return /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(run.id) && run.conversation_id === expected.conversation && run.actor_id === expected.actor && run.profile_id === expected.profile && run.client_message_id === expected.message && Number.isSafeInteger(run.version) && run.version >= 1 && run.last_event_seq === run.version && Object.prototype.hasOwnProperty.call(runNames, run.state) && typeof run.can_cancel === 'boolean';
}
export function messageText(value: Schemas['Message']) {
  return value.visibility === 'unavailable' ? '此消息所依赖的资料已变化或当前不可查阅，正文与引用已隐藏。' : value.body;
}
