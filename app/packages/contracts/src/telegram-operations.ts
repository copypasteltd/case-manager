import type { components } from './api';
type S = components['schemas'];
export type TelegramOperation = S['TelegramOperation'];
export type TelegramOperationDetails = S['TelegramOperationDetails'];
export type TelegramOperationReconcile = S['TelegramOperationReconcile'];
export const telegramDeliveryStates = {
  pending: '等待发送', sending: '正在发送', retry_wait: '等待重试', sent: 'Bot已确认送达',
  delivery_unknown: '投递结果未知', confirmed_sent: '人工对账为已发送', failed: '发送失败',
  superseded: '已合并到后续阶段', suppressed: '已停止发送', abandoned: '人工已放弃重发',
} as const;
export const telegramDeliveryPhases = { accepted: '受理通知', progress: '进度通知', result: '结果通知' } as const;
export const telegramReconcileActions = [
  ['abandon', '保留未知事实，放弃重发'], ['confirm_sent', '已找到原群对应消息'],
  ['retry_confirmed_not_sent', '已核对未发送，允许重试'],
] as const;
export function telegramOperationsPath(state = '', channel = '') {
  const query = new URLSearchParams({ limit: '20' });
  if (state) query.set('state', state);
  if (channel.trim()) query.set('channel_id', channel.trim());
  return '/telegram/operations?' + query;
}
export function telegramOperationInput(item: TelegramOperation, action: TelegramOperationReconcile['action'], message: string, reason: string, reviewed: boolean, duplicate: boolean): TelegramOperationReconcile {
  if (item.state !== 'delivery_unknown' || !reviewed) throw new Error('请先核对原群、话题和当前未知投递。');
  if (!reason.trim() || [...reason.trim()].length > 200) throw new Error('请填写不超过200字的处理原因。');
  if (action === 'retry_confirmed_not_sent' && (!item.retry_authorized || !duplicate)) throw new Error('当前不可重发，或尚未确认可能重复的风险。');
  const id = Number(message);
  if (action === 'confirm_sent' && (!/^[1-9][0-9]*$/.test(message) || !Number.isSafeInteger(id))) throw new Error('请填写原群对应消息的完整正整数编号。');
  return { expected_version: item.version, action, message_id: action === 'confirm_sent' ? id : null,
    reason: reason.trim(), reviewed_original_channel: true, duplicate_risk_accepted: action === 'retry_confirmed_not_sent' };
}
