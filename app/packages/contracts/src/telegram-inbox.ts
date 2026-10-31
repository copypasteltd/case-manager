import type { components } from './api';
type S = components['schemas'];
export type TelegramInboxOperation = S['TelegramInboxOperation'];
export type TelegramInboxDetails = S['TelegramInboxOperationDetails'];
export type TelegramInboxEvent = S['TelegramInboxOperationEvent'];
export type TelegramInboxWithdraw = S['TelegramInboxWithdraw'];
export const telegramInboxStates = { awaiting_executor: '等待秘书调度', dispatched: '已创建秘书任务', withdrawn: '已撤回' } as const;
export const telegramInboxSkills = { 'project-briefing': '项目简报', 'incident-summary': '服务事件摘要' } as const;
export const telegramInboxSources = { accepted: '实际接收', transition: '实际状态变化', retained_baseline: '启用台账时的现状快照' } as const;
export function telegramInboxReason(code: string) {
  const values: Record<string, string> = {
    executor_not_connected: '已接收，等待调度器接单', RUNTIME_UNAVAILABLE: '等待指定任务的有效执行配置',
    TELEGRAM_SKILL_UNAVAILABLE: '等待指定任务工具就绪', RUN_CAPACITY_EXCEEDED: '等待可用任务名额',
    CONVERSATION_RUN_ACTIVE: '等待此上下文的上一任务结束', TELEGRAM_SCOPE_CHANGED: '绑定或入口范围已变化',
    TELEGRAM_REQUEST_EXPIRED: '排队请求已超过30分钟有效期', manual_queue_withdrawn: '组织经办人撤回尚未启动的请求',
    channel_changed: '入口配置已变化，旧请求停止执行', binding_revoked: '本人绑定已撤销',
    group_review_required: '群成员或标识变化，等待重新核对',
  };
  return values[code] ?? code;
}
export function telegramInboxPath(state = '', channel = '', skill = '') {
  const pairs = [['limit', '20'], ['state', state], ['channel_id', channel.trim()], ['skill_id', skill]];
  return '/telegram/inbox-operations?' + pairs.filter(([, v]) => v).map(([k, v]) => k + '=' + encodeURIComponent(v)).join('&');
}
export function telegramInboxWithdrawInput(item: TelegramInboxOperation, reason: string, reviewed: boolean): TelegramInboxWithdraw {
  if (item.state !== 'awaiting_executor' || !item.withdrawable || item.run_id !== null || !reviewed) throw new Error('请核对尚未启动的准确入站请求。');
  if (!reason.trim() || [...reason.trim()].length > 200) throw new Error('请填写不超过200字的撤回原因。');
  return { expected_version: item.version, reviewed_request: true, reason: reason.trim() };
}
