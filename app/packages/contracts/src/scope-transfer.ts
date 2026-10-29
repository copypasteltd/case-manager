import type { components } from './api';
type Schemas = components['schemas'];
export type ScopeTransfer = Schemas['ScopeTransfer'];
export type TransferResource = Schemas['File'] | Schemas['Folder'];
export const transferStates = { pending: '待人工核对', executed: '已完成跨范围移动', cancelled: '已撤销计划' };
export const transferBlockers: Record<string, string> = {
  RETENTION_HOLD: '存在有效保留锁，需由原业务解除后重新拟定。', BUSINESS_REFERENCE: '正式业务仍引用此文件，须先按原业务流程处理；计划不解除业务引用。',
  UPLOAD_ACTIVE: '存在未结束的上传，请先处理上传任务。', RESTORE_ACTIVE: '存在未结束的历史恢复任务。', DOWNLOAD_PERMIT_ACTIVE: '已发放的下载许可仍有效，请待其结束后重新拟定。',
  CLASSIFICATION_DOWNGRADE: '目标分类低于来源分类，不能通过移动降低分类。', CALLER_TARGET_READ_REQUIRED: '当前员工尚无目标范围的完整读取权限。',
};
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;
const channels = ['web', 'wechat', 'douyin', 'android', 'telegram'];
const classes = ['internal', 'project_restricted', 'confidential'];
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const id = (v: unknown): v is string => typeof v === 'string' && uuid.test(v);
const nullableID = (v: unknown) => v === null || id(v);
const count = (v: unknown): v is number => Number.isSafeInteger(v) && Number(v) >= 0 && Number(v) < Number.MAX_SAFE_INTEGER;
const version = (v: unknown): v is number => count(v) && v > 0;
const text = (v: unknown): v is string => typeof v === 'string' && [...v].length > 0 && [...v].length <= 200;
const instant = (v: unknown): v is string => typeof v === 'string' && Number.isFinite(Date.parse(v));
const owns = (v: object, key: string) => Object.prototype.hasOwnProperty.call(v, key);
function fail(): never { throw new Error('跨范围计划与当前资料不一致，请刷新核对；未确认执行结果。'); }
function entry(v: unknown): v is Schemas['AgentOrganizationEntry'] & Record<string, unknown> {
  return record(v) && ['file', 'folder'].includes(String(v.kind)) && id(v.id) && version(v.version) && text(v.name)
    && nullableID(v.parent_id) && nullableID(v.project_id) && classes.includes(String(v.classification));
}
function name(value: string) {
  const normalized = value.normalize('NFKC').toLowerCase();
  if (!text(value) || value.trim() !== value || [value, normalized].some(v => /[\\/<>:"|?*\p{Cc}\p{Cf}\p{Cs}]/u.test(v))
    || ['.', '..'].includes(normalized) || /[. ]$/.test(normalized) || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/.test(normalized)) throw new Error('请填写有效的实际名称，避免路径或平台保留字符。');
}
function reason(value: string) { if (!text(value) || !value.trim()) throw new Error('请填写1至200字符的实际操作原因。'); }

export function scopeTransferCreate(kind: 'file' | 'folder', source: TransferResource, target: Schemas['Folder'], newName: string, why: string): Schemas['ScopeTransferCreate'] {
  name(newName); reason(why);
  if (!id(source.id) || !version(source.version) || source.state !== 'active'
    || !(kind === 'file' ? (source as Schemas['File']).can_move : (source as Schemas['Folder']).can_update)) throw new Error('来源资料已改变或当前无权移动，请刷新核对。');
  if (!id(target.id) || !version(target.version) || !target.can_create || target.state !== 'active' || target.id === source.id) throw new Error('请选择当前可写入的实际目标目录。');
  if (kind === 'folder' && (source as Schemas['Folder']).parent_id === null) throw new Error('资料根目录不能跨范围移动，请选择其内的资料。');
  if (source.project_id === target.project_id && source.classification === target.classification) throw new Error('同项目、同分类请使用普通改名或移动。');
  return { kind, resource_id: source.id, expected_version: source.version, target_folder_id: target.id, expected_target_version: target.version, new_name: newName, reason: why };
}

export function checkedScopeTransfer(raw: unknown, kind: 'file' | 'folder', resourceID: string, planID?: string): ScopeTransfer {
  if (!record(raw) || !id(raw.id) || planID && raw.id !== planID || !id(raw.creator_id) || !version(raw.version)
    || !owns(transferStates, String(raw.state)) || !hash.test(String(raw.plan_hash)) || !instant(raw.created_at) || !instant(raw.expires_at)
    || typeof raw.current_basis_matches !== 'boolean' || typeof raw.can_execute !== 'boolean' || !record(raw.input) || !record(raw.snapshot)) fail();
  const input = raw.input, snapshot = raw.snapshot;
  if (input.kind !== kind || input.resource_id !== resourceID || !version(input.expected_version) || !id(input.target_folder_id) || !version(input.expected_target_version)
    || !(input.new_name === null || text(input.new_name)) || !text(input.reason) || !entry(snapshot.source) || !entry(snapshot.target)
    || snapshot.source.kind !== kind || snapshot.source.id !== resourceID || snapshot.source.version !== input.expected_version
    || snapshot.target.kind !== 'folder' || snapshot.target.id !== input.target_folder_id || snapshot.target.version !== input.expected_target_version
    || snapshot.new_name !== (input.new_name ?? snapshot.source.name) || !Array.isArray(snapshot.items) || snapshot.items.length < 1 || snapshot.items.length > 100) fail();
  for (const path of [snapshot.source_path, snapshot.target_path]) if (!Array.isArray(path) || path.length > 32 || !path.every(v => entry(v) && v.kind === 'folder') || new Set(path.map(v => (v as Schemas['AgentOrganizationEntry']).id)).size !== path.length) fail();
  const seen = new Set<string>(); let root = false, blocked = false;
  for (const value of snapshot.items) {
    if (!record(value) || !entry(value) || !['active', 'trashed'].includes(String(value.state)) || !nullableID(value.creator_id)
      || value.project_id !== snapshot.source.project_id || value.new_classification !== snapshot.target.classification || !nullableID(value.current_version_id)
      || !count(value.content_versions) || !hash.test(String(value.audience_sha256)) || !Array.isArray(value.blocking_reasons) || value.blocking_reasons.length > 10
      || !value.blocking_reasons.every(v => typeof v === 'string' && owns(transferBlockers, v))
      || !Array.isArray(value.visibility_changes) || value.visibility_changes.length > 1000 || seen.has(value.id)) fail();
    seen.add(value.id);
    if (value.kind === 'file' ? !hash.test(String(value.content_sha256)) : value.content_sha256 !== '' || value.content_versions !== 0 || value.current_version_id !== null) fail();
    for (const field of ['human_before', 'human_after', 'ai_before', 'ai_after']) if (!count(value[field]) || Number(value[field]) > 1000) fail();
    if (value.id === resourceID) {
      if (value.kind !== kind || value.version !== input.expected_version || value.name !== snapshot.source.name || value.classification !== snapshot.source.classification || value.parent_id !== snapshot.source.parent_id) fail();
      root = true;
    }
    const changes = new Set<string>(); let humanDelta = 0, aiDelta = 0;
    for (const change of value.visibility_changes) {
      if (!record(change) || !id(change.user_id) || !text(change.display_name) || !channels.includes(String(change.channel))
        || typeof change.ai !== 'boolean' || typeof change.before !== 'boolean' || typeof change.after !== 'boolean' || change.before === change.after) fail();
      const key = change.user_id + '/' + change.channel + '/' + change.ai;
      if (changes.has(key)) fail(); changes.add(key);
      const delta = Number(change.after) - Number(change.before); if (change.ai) aiDelta += delta; else humanDelta += delta;
    }
    if (Number(value.human_after) - Number(value.human_before) !== humanDelta || Number(value.ai_after) - Number(value.ai_before) !== aiDelta) fail();
    blocked ||= value.blocking_reasons.length > 0;
  }
  if (!root || kind === 'file' && snapshot.items.length !== 1 || raw.can_execute && (raw.state !== 'pending' || !raw.current_basis_matches || blocked)) fail();
  if (raw.state === 'pending' ? raw.version !== 1 || raw.completed_at !== null || raw.decision_reason !== null
    : raw.version !== 2 || !instant(raw.completed_at) || !text(raw.decision_reason) || raw.can_execute) fail();
  return raw as unknown as ScopeTransfer;
}

export function scopeTransferDecision(plan: ScopeTransfer, why: string, acknowledged: boolean, action: 'execute' | 'cancel'): Schemas['ScopeTransferDecision'] {
  checkedScopeTransfer(plan, plan.input.kind, plan.input.resource_id, plan.id); reason(why);
  if (!acknowledged || plan.state !== 'pending' || action === 'execute' && !plan.can_execute) throw new Error('请重新读取计划，核对全部权限变化和阻塞原因，再明确确认本次操作。');
  return { expected_version: plan.version, plan_hash: plan.plan_hash, reason: why };
}
