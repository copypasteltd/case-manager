import { MobileError, type Schemas } from './session';

export type Channel = Schemas['TelegramChannel'];
export type Challenge = Schemas['TelegramChallenge'];
export type Binding = Schemas['TelegramBinding'];
export const telegramSkills = { 'project-briefing': '项目简报', 'incident-summary': '服务事件摘要' };
export type ChannelInput = { bot: string; chat: string; topic: string; label: string; project: string;
  allowed: Channel['allowed_skill_ids']; defaultSkill: string; state: Channel['state']; delivery: boolean };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const numeric = /^[1-9][0-9]{0,15}$/;
const validVersion = (v: unknown) => Number.isSafeInteger(v) && Number(v) > 0;
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const stamp = (v: unknown) => typeof v === 'string' && Number.isFinite(Date.parse(v));
function invalid(message: string): never { throw new MobileError('INVALID_TELEGRAM_INPUT', message, 422); }
function reason(value: string) { const text = value.trim(); if (!text || [...text].length > 200) invalid('请填写不超过200字的明确原因。'); return text; }
export function telegramReason(value: string) { return reason(value); }
export function channelInput(value?: Channel): ChannelInput {
  return { bot: value?.bot_id ?? '', chat: value?.chat_id ?? '', topic: String(value?.topic_id ?? 0), label: value?.label ?? '',
    project: value?.project_id ?? '', allowed: [...(value?.allowed_skill_ids ?? ['project-briefing'])], defaultSkill: value?.default_skill_id ?? '',
    state: value?.state ?? 'paused', delivery: value?.delivery_enabled ?? false };
}
export function channelPayload(input: ChannelInput, current: Channel | undefined, explanation: string, fresh: boolean, checked: boolean): Schemas['TelegramChannelCreate'] | Schemas['TelegramChannelPatch'] {
  if (!fresh || !checked) invalid('请核对准确群、话题、范围与允许任务，并完成本人两步验证。');
  const bot = input.bot.trim(), chat = input.chat.trim(), topic = input.topic.trim(), label = input.label.trim(), project = input.project.trim();
  if (!numeric.test(bot) || !/^-[1-9][0-9]{0,15}$/.test(chat)) invalid('请选择明确Bot并填写准确数字群号；群号须保留负号。');
  if (!/^(0|[1-9][0-9]*)$/.test(topic) || !Number.isSafeInteger(Number(topic)) || Number(topic) > 2147483647) invalid('话题号须为0至2147483647的整数；无话题请明确填0。');
  if (!label || [...label].length > 200 || project && !uuid.test(project)) invalid('请核对入口名称与通过候选列表选择的项目。');
  if (!['active', 'paused'].includes(input.state) || !current && input.state !== 'paused') invalid('新入口必须先以暂停状态创建。');
  const allowed = [...input.allowed];
  if (!allowed.length || allowed.length > 2 || new Set(allowed).size !== allowed.length || allowed.some(id => !Object.prototype.hasOwnProperty.call(telegramSkills, id))) invalid('请明确选择一至两项允许任务。');
  if (input.defaultSkill && !allowed.includes(input.defaultSkill as keyof typeof telegramSkills)) invalid('默认任务必须属于本次允许任务。');
  const common = { label, project_id: project || null, allowed_skill_ids: allowed,
    default_skill_id: (input.defaultSkill || null) as Channel['default_skill_id'], delivery_enabled: input.delivery, reason: reason(explanation) };
  if (!current) return { ...common, bot_id: bot, chat_id: chat, topic_id: Number(topic) };
  if (!uuid.test(current.id) || !validVersion(current.version) || bot !== current.bot_id || chat !== current.chat_id || Number(topic) !== current.topic_id) invalid('原入口位置不可更换；请关闭并重新核对准确入口。');
  return { ...common, expected_version: current.version, state: input.state };
}
export function channelReceipt(value: unknown, body: Schemas['TelegramChannelCreate'] | Schemas['TelegramChannelPatch'], current?: Channel): value is Channel {
  if (!record(value) || typeof value.id !== 'string' || !uuid.test(value.id) || !validVersion(value.version) || value.disclosure_mode !== 'portal_only'
    || !stamp(value.created_at) || !stamp(value.updated_at) || !Array.isArray(value.allowed_skill_ids)) return false;
  const origin = current ?? body as Schemas['TelegramChannelCreate'];
  return (!current || value.id === current.id) && value.version === (current ? current.version + 1 : 1)
    && value.bot_id === origin.bot_id && value.chat_id === origin.chat_id && value.topic_id === origin.topic_id
    && value.label === body.label && value.project_id === body.project_id && value.default_skill_id === body.default_skill_id
    && value.delivery_enabled === body.delivery_enabled && value.state === (current ? (body as Schemas['TelegramChannelPatch']).state : 'paused')
    && JSON.stringify([...value.allowed_skill_ids].sort()) === JSON.stringify([...body.allowed_skill_ids].sort());
}
export function challengeReceipt(value: unknown, bot: Schemas['TelegramBot']): value is Challenge {
  return record(value) && typeof value.id === 'string' && uuid.test(value.id) && validVersion(value.version) && value.bot_id === bot.bot_id
    && value.telegram_user_id === null && value.claimed_at === null && value.confirmed_at === null && value.cancelled_at === null
    && stamp(value.created_at) && stamp(value.expires_at) && typeof value.start_token === 'string' && /^[A-Za-z0-9_-]{43}$/.test(value.start_token)
    && /^[A-Za-z][A-Za-z0-9_]{4,31}$/.test(bot.username) && value.start_url === 'https://t.me/' + bot.username + '?start=' + value.start_token;
}
export function bindingReceipt(value: unknown, challenge: Challenge): value is Binding {
  return record(value) && typeof value.id === 'string' && uuid.test(value.id) && validVersion(value.version) && value.state === 'active'
    && value.bot_id === challenge.bot_id && typeof value.telegram_user_id === 'string' && numeric.test(value.telegram_user_id)
    && value.telegram_user_id === challenge.telegram_user_id && stamp(value.created_at) && value.revoked_at === null;
}
export function endedReceipt(value: unknown, target: Challenge | Binding, kind: 'challenge' | 'binding'): boolean {
  return record(value) && value.id === target.id && value.version === target.version + 1 && value.bot_id === target.bot_id
    && value.telegram_user_id === target.telegram_user_id && (kind === 'binding' ? value.state === 'revoked' && stamp(value.revoked_at) : stamp(value.cancelled_at));
}
