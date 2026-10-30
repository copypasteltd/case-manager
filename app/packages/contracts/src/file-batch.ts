import type { components } from './api';
type S = components['schemas'];
export type FileBatchPlan = S['FileBatchPlan'];
export type BatchAction = S['FileBatchCreate']['action'];
export const batchActions = { move: '批量移动', trash: '批量移入回收站', restore: '批量恢复' };
export const batchStates = { pending: '待人工核对', executed: '整批已完成', cancelled: '计划已撤销' };
export const batchBlockers: Record<string, string> = {
  file_state_changed: '文件状态与本次操作不符。', project_read_only: '项目当前只读。',
  retention_hold: '文件存在有效保留锁。', already_in_target: '文件已在目标目录。', name_conflict: '目标存在同名文件或目录。',
};
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const id = (v: unknown): v is string => typeof v === 'string' && uuid.test(v);
const nullableID = (v: unknown) => v === null || id(v);
const version = (v: unknown): v is number => Number.isSafeInteger(v) && Number(v) > 0 && Number(v) < Number.MAX_SAFE_INTEGER;
const text = (v: unknown): v is string => typeof v === 'string' && v.trim().length > 0 && [...v].length <= 200;
const instant = (v: unknown): v is string => typeof v === 'string' && Number.isFinite(Date.parse(v));
const owns = (v: object, k: unknown) => typeof k === 'string' && Object.prototype.hasOwnProperty.call(v, k);
const classification = (v: unknown) => ['internal', 'project_restricted', 'confidential'].includes(String(v));
function fail(): never { throw new Error('批量清单或回执与当前资料不一致，请刷新核对本次结果。'); }
function file(v: unknown): v is S['FileBatchFile'] {
  return record(v) && id(v.id) && version(v.version) && text(v.name) && id(v.folder_id)
    && nullableID(v.project_id) && nullableID(v.creator_id) && nullableID(v.current_version_id)
    && classification(v.classification) && ['active', 'trashed'].includes(String(v.state));
}
function entry(v: unknown): v is S['AgentOrganizationEntry'] {
  return record(v) && v.kind === 'folder' && id(v.id) && version(v.version) && text(v.name)
    && nullableID(v.parent_id) && nullableID(v.project_id) && classification(v.classification);
}
function path(v: unknown, parent: string | null, project: string | null) {
  if (!Array.isArray(v) || v.length > 32) fail();
  const seen = new Set<string>();
  for (const p of v) {
    if (!entry(p) || p.id !== parent || p.project_id !== project || seen.has(p.id)) fail();
    seen.add(p.id); parent = p.parent_id;
  }
  if (parent !== null) fail();
}
function selection(v: unknown): v is S['FileBatchSelection'] {
  return record(v) && id(v.id) && version(v.expected_version) && nullableID(v.current_version_id);
}
export function fileBatchCreate(action: BatchAction, selected: S['File'][], target: S['Folder'] | undefined, reason: string): S['FileBatchCreate'] {
  if (!owns(batchActions, action) || !text(reason) || selected.length < 1 || selected.length > 50 || new Set(selected.map(v => v.id)).size !== selected.length) throw new Error('请选择1至50个不同文件，并填写操作原因。');
  for (const f of selected) {
    if (!file(f) || f.state !== (action === 'restore' ? 'trashed' : 'active') || !(action === 'move' ? f.can_move : action === 'trash' ? f.can_trash : f.can_restore)) throw new Error('文件状态或操作权限已改变，请刷新后重新选择。');
  }
  if (action === 'move' && (!target || !id(target.id) || !version(target.version) || target.state !== 'active' || !target.can_create || selected.some(f => f.project_id !== target.project_id || f.classification !== target.classification || f.folder_id === target.id))) throw new Error('请选择同项目、同分类且不同于来源的可写目标目录。');
  return { action, files: [...selected].sort((a, b) => a.id.localeCompare(b.id)).map(f => ({ id: f.id, expected_version: f.version, current_version_id: f.current_version_id })), target: action === 'move' ? { id: target!.id, expected_version: target!.version } : null, reason };
}
export function checkedFileBatch(raw: unknown, owner: string, planID?: string, sourceID?: string): FileBatchPlan {
  if (!record(raw) || !id(raw.id) || planID !== undefined && raw.id !== planID || raw.creator_id !== owner || !id(raw.creator_id)
    || !owns(batchStates, raw.state) || !version(raw.version) || !/^[a-f0-9]{64}$/.test(String(raw.plan_hash))
    || !instant(raw.created_at) || !instant(raw.expires_at) || Date.parse(raw.expires_at) <= Date.parse(raw.created_at)
    || Date.parse(raw.expires_at) - Date.parse(raw.created_at) > 1800001
    || typeof raw.current_basis_matches !== 'boolean' || typeof raw.can_execute !== 'boolean'
    || !record(raw.input) || !record(raw.snapshot) || !Array.isArray(raw.results)) fail();
  const input = raw.input, snapshot = raw.snapshot;
  if (!owns(batchActions, input.action) || !text(input.reason) || !Array.isArray(input.files) || input.files.length < 1 || input.files.length > 50
    || !input.files.every(selection) || !Array.isArray(snapshot.items) || snapshot.items.length !== input.files.length) fail();
  if (input.action === 'move') {
    if (!record(input.target) || !id(input.target.id) || !version(input.target.expected_version) || !record(snapshot.target)
      || !entry(snapshot.target.folder) || snapshot.target.folder.id !== input.target.id || snapshot.target.folder.version !== input.target.expected_version) fail();
    path(snapshot.target.path, snapshot.target.folder.parent_id, snapshot.target.folder.project_id);
  } else if (input.target !== null || snapshot.target !== null) fail();
  const seen = new Set<string>(); let blocked = false;
  for (let index = 0; index < snapshot.items.length; index++) {
    const item = snapshot.items[index], selected = input.files[index];
    if (!record(item) || !file(item.file) || !selection(selected) || seen.has(item.file.id)
      || index > 0 && String(input.files[index - 1].id) >= item.file.id
      || item.file.id !== selected.id || item.file.version !== selected.expected_version || item.file.current_version_id !== selected.current_version_id
      || typeof item.retention_held !== 'boolean' || !Array.isArray(item.blocking_reasons) || item.blocking_reasons.length > 6
      || !item.blocking_reasons.every(v => owns(batchBlockers, v)) || new Set(item.blocking_reasons).size !== item.blocking_reasons.length) fail();
    path(item.path, item.file.folder_id, item.file.project_id); seen.add(item.file.id); blocked ||= item.blocking_reasons.length > 0;
    if (item.file.state !== (input.action === 'restore' ? 'trashed' : 'active') && !item.blocking_reasons.includes('file_state_changed')
      || input.action === 'trash' && item.retention_held && !item.blocking_reasons.includes('retention_hold')) fail();
    if (input.action === 'move' && record(snapshot.target) && entry(snapshot.target.folder)
      && (item.file.project_id !== snapshot.target.folder.project_id || item.file.classification !== snapshot.target.folder.classification)) fail();
  }
  if (sourceID !== undefined && !seen.has(sourceID)) fail();
  if (raw.state === 'pending') {
    if (raw.version !== 1 || raw.completed_at !== null || raw.decision_reason !== null || raw.results.length !== 0
      || raw.can_execute && (!raw.current_basis_matches || blocked)) fail();
  } else {
    if (raw.version !== 2 || !instant(raw.completed_at) || !text(raw.decision_reason) || raw.can_execute || raw.current_basis_matches) fail();
    if (raw.state === 'cancelled' ? raw.results.length !== 0 : raw.results.length !== snapshot.items.length || blocked) fail();
    for (let i = 0; i < raw.results.length; i++) {
      const result = raw.results[i], before = (snapshot.items[i] as S['FileBatchItem']).file;
      if (!file(result) || result.id !== before.id || result.version !== before.version + 1 || result.name !== before.name
        || result.project_id !== before.project_id || result.creator_id !== before.creator_id || result.classification !== before.classification
        || result.current_version_id !== before.current_version_id || result.state !== (input.action === 'trash' ? 'trashed' : 'active')
        || result.folder_id !== (input.action === 'move' ? (snapshot.target as S['FileBatchDestination']).folder.id : before.folder_id)) fail();
    }
  }
  return raw as unknown as FileBatchPlan;
}
export function batchInputMatches(a: S['FileBatchCreate'], b: S['FileBatchCreate']) {
  return a.action === b.action && a.reason === b.reason && a.target?.id === b.target?.id && a.target?.expected_version === b.target?.expected_version
    && a.files.length === b.files.length && a.files.every((f, i) => f.id === b.files[i].id && f.expected_version === b.files[i].expected_version && f.current_version_id === b.files[i].current_version_id);
}
export function fileBatchDecision(p: FileBatchPlan, reason: string, ack: boolean, action: 'execute' | 'cancel'): S['FileBatchDecision'] {
  checkedFileBatch(p, p.creator_id, p.id);
  if (p.state !== 'pending' || action === 'execute' && !p.can_execute || !text(reason) || !ack) throw new Error('请重新核对固定清单，填写原因并明确确认本次操作。');
  return { expected_version: p.version, plan_hash: p.plan_hash, reason, acknowledged: true };
}
