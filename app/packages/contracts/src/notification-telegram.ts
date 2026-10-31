import type { components } from './api';
import { notificationCategories } from './notifications';
type S = components['schemas'];
export type TelegramPolicy = S['NotificationTelegramPolicy'];
export type TelegramSetting = S['NotificationTelegramSetting'];
export type TelegramSettings = S['NotificationTelegramSettings'];
export type TelegramPolicyInput = S['NotificationTelegramPolicyInput'];
export type TelegramReceipt = S['NotificationTelegramReceipt'];
export type TelegramDetails = S['NotificationTelegramReceiptDetails'];
export type TelegramReconcile = S['NotificationTelegramReconcileInput'];
export const telegramDeliveryStates: Record<TelegramReceipt['state'], string> = {
  pending: '待投递', sending: '发送中', retry_wait: '等待重试', sent: '服务商确认送达', delivery_unknown: '送达结果未知',
  confirmed_sent: '本人核对已送达', failed: '投递失败', suppressed: '已抑制', abandoned: '已终止',
};
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const botPattern = /^[1-9][0-9]{0,15}$/;
const sha = /^[a-f0-9]{64}$/;
function fail(): never { throw new Error('TG 提醒资料尚未核对，请刷新设置和投递回执。'); }
function integer(v: unknown, min = 1, max = Number.MAX_SAFE_INTEGER): v is number { return typeof v === 'number' && Number.isSafeInteger(v) && v >= min && v <= max; }
function timestamp(v: unknown) { return typeof v === 'string' && Number.isFinite(Date.parse(v)); }
function ids(...v: unknown[]) { return v.every(x => typeof x === 'string' && uuid.test(x)); }
function categoryList(v: unknown): v is TelegramPolicy['categories'] { return Array.isArray(v) && v.length <= 10 && new Set(v).size === v.length && v.every(x => typeof x === 'string' && Object.prototype.hasOwnProperty.call(notificationCategories, x)); }
function note(v: unknown): v is string { return typeof v === 'string' && [...v.trim()].length >= 1 && [...v.trim()].length <= 200; }
export function checkedTelegramPolicy(p: TelegramPolicy) {
  if (!p || !ids(p.id) || !botPattern.test(p.bot_id) || !integer(p.version) || typeof p.enabled !== 'boolean' || !categoryList(p.categories) || !note(p.reason) || !timestamp(p.created_at) ||
    (p.binding_id === null) !== (p.binding_version === null) || p.binding_id !== null && (!ids(p.binding_id) || !integer(p.binding_version)) || p.enabled && (!p.binding_id || !p.categories.length)) fail();
  return p;
}
export function checkedTelegramSettings(v: TelegramSettings) {
  if (!v || !Array.isArray(v.items) || v.items.length > 100 || new Set(v.items.map(x => x.bot_id)).size !== v.items.length) fail();
  for (const s of v.items) {
    if (!s || !botPattern.test(s.bot_id) || typeof s.binding_active !== 'boolean' || typeof s.effective !== 'boolean') fail();
    if (s.current !== null && checkedTelegramPolicy(s.current).bot_id !== s.bot_id) fail();
    if (s.effective && (!s.binding_active || !s.current?.enabled)) fail();
  }
  return v;
}
export function telegramPolicyInput(setting: TelegramSetting, enabled: boolean, categories: string[], reason: string): TelegramPolicyInput {
  if (!botPattern.test(setting.bot_id) || typeof enabled !== 'boolean' || !categoryList(categories) || enabled && (!setting.binding_active || !categories.length) || !note(reason)) throw new Error('请先确认本人绑定、通知类型及变更原因。');
  return { expected_version: setting.current?.version ?? 0, enabled, categories: [...categories].sort(), reason: reason.trim() };
}
export function checkedTelegramPolicyReceipt(v: TelegramPolicy, input: TelegramPolicyInput, bot: string) {
  checkedTelegramPolicy(v);
  if (v.bot_id !== bot || v.version !== input.expected_version + 1 || v.enabled !== input.enabled || JSON.stringify(v.categories) !== JSON.stringify(input.categories) || v.reason !== input.reason) fail();
  return v;
}
export function checkedTelegramReceipt(v: TelegramReceipt) {
  if (!v || !ids(v.id, v.source_delivery_id, v.thread_id, v.policy_id) || !botPattern.test(v.bot_id) || !integer(v.version) || !integer(v.attempts, 0, 5) || !Object.prototype.hasOwnProperty.call(telegramDeliveryStates, v.state) ||
    typeof v.reason_code !== 'string' || !/^[a-z0-9_]{1,80}$/.test(v.reason_code) || !timestamp(v.created_at) || !timestamp(v.updated_at) || !timestamp(v.next_attempt_at) ||
    v.payload_sha256 !== null && !sha.test(v.payload_sha256) || v.message_id !== null && !integer(v.message_id) || ['sent','confirmed_sent'].includes(v.state) && v.message_id === null) fail();
  return v;
}
export function checkedTelegramDetails(v: TelegramDetails) {
  const d = checkedTelegramReceipt(v.receipt);
  if (!Array.isArray(v.events) || v.events.length !== d.version) fail();
  v.events.forEach((e,i) => {
    if (!ids(e.id) || e.version !== i+1 || !Object.prototype.hasOwnProperty.call(telegramDeliveryStates,e.state) || !integer(e.attempts,0,5) || !timestamp(e.created_at) ||
      !/^[a-z0-9_]{1,80}$/.test(e.reason_code) || e.payload_sha256 !== null && !sha.test(e.payload_sha256) || e.message_id !== null && !integer(e.message_id) || e.actor_id !== null && !ids(e.actor_id) || e.reason !== null && !note(e.reason)) fail();
  });
  const last=v.events.at(-1); if (!last || ['state','version','attempts','reason_code','payload_sha256','message_id'].some(k => last[k as keyof typeof last] !== d[k as keyof TelegramReceipt])) fail();
  return v;
}
export function telegramReconcileInput(d: TelegramReceipt, action: TelegramReconcile['action'], message: string, reason: string): TelegramReconcile {
  checkedTelegramReceipt(d);
  if (!['confirm_sent','retry','abandon'].includes(action) || !note(reason) || action === 'confirm_sent' && d.state !== 'delivery_unknown' || action === 'retry' && (!['delivery_unknown','failed'].includes(d.state) || d.attempts >= 5) || action === 'abandon' && !['pending','retry_wait','delivery_unknown','failed'].includes(d.state)) throw new Error('请刷新投递状态，并核对本次处置方式和原因。');
  let message_id: number | null = null;
  if (action === 'confirm_sent') { if (!/^[1-9][0-9]{0,15}$/.test(message) || !integer(Number(message))) throw new Error('请填写已在本人 TG 私聊核对的消息号。'); message_id=Number(message); }
  else if (message.trim()) throw new Error('只有核对已送达时才填写消息号。');
  return { expected_version:d.version, action, message_id, reason:reason.trim() };
}
export function checkedTelegramReconcileReceipt(v: TelegramReceipt, old: TelegramReceipt, input: TelegramReconcile) {
  checkedTelegramReceipt(v);
  const state=input.action==='confirm_sent'?'confirmed_sent':input.action==='retry'?'retry_wait':'abandoned';
  if (v.version!==old.version+1 || v.state!==state || v.message_id!==input.message_id || v.attempts!==old.attempts || v.reason_code!=='human_'+input.action ||
    ['id','bot_id','source_delivery_id','thread_id','policy_id','payload_sha256','created_at'].some(k=>v[k as keyof TelegramReceipt]!==old[k as keyof TelegramReceipt])) fail();
  return v;
}
