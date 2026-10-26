import { MobileError, type Schemas } from './session';

export const closeoutLabels = {
  tasks: '任务', milestones: '里程碑', deliverables: '交付与验收', risks: '风险', issues: '问题', changes: '项目变更',
  budgets: '预算版本', contracts: '合同与履约', purchases: '采购履约', claims: '报销', costs: '实际费用与结算',
  payments: '资金与退款', subscriptions: '订阅与账期',
};
export type CloseoutKind = keyof typeof closeoutLabels;
export type ArchiveNotes = { summary: string; lessons: string; followUp: string; reason: string };
type Preview = Schemas['ProjectCloseout'];
type Project = Schemas['Project'];
type Archive = Schemas['ProjectArchive'];
type Group = Schemas['ProjectCloseoutGroup'];
type ArchiveFile = Pick<Schemas['BusinessFileReference'], 'file_id' | 'version_id' | 'seq' | 'name' | 'sha256' | 'size_bytes'>;
export type ProjectAction = 'activate' | 'hold' | 'resume' | 'close' | 'cancel' | 'reopen';
const kinds = Object.keys(closeoutLabels) as CloseoutKind[];
const uuid = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(value);
const version = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value > 0;
const fingerprint = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{32}$/.test(value);
const object = (value: unknown): value is Record<string, unknown> => Boolean(value && typeof value === 'object' && !Array.isArray(value));
const trim = (value: string) => value.replace(/^[\s\u0085]+|[\s\u0085]+$/g, '');
const text = (value: unknown, limit: number) => typeof value === 'string' && trim(value).length > 0 && value.length <= limit;
const instant = (value: unknown) => typeof value === 'string' && Number.isFinite(Date.parse(value));

function projectValid(value: unknown): value is Project {
  return object(value) && uuid(value.id) && version(value.version) && uuid(value.owner_id)
    && text(value.name, 200) && text(value.project_no, 200) && typeof value.scope === 'string'
    && ['draft', 'active', 'on_hold', 'closing', 'archived', 'cancelled'].includes(value.state as string);
}
function groupsValid(value: unknown, complete = false): value is Group[] {
  if (!Array.isArray(value) || value.length !== kinds.length || new Set(value.map(g => g?.kind)).size !== kinds.length) return false;
  let count = 0;
  for (const g of value) {
    if (!object(g) || !kinds.includes(g.kind as CloseoutKind) || !Array.isArray(g.items)) return false;
    if (g.visibility === 'restricted') {
      if (complete || g.count !== null || g.open_count !== null || g.items.length !== 0) return false;
    } else if (g.visibility === 'complete') {
      if (g.count !== g.items.length || !Number.isSafeInteger(g.open_count) || (g.open_count as number) < 0
        || g.open_count !== g.items.filter(i => i?.blocking === true).length || new Set(g.items.map(i => i?.id)).size !== g.items.length) return false;
      if (g.items.some(i => !object(i) || i.kind !== g.kind || !uuid(i.id) || !version(i.version)
        || typeof i.title !== 'string' || typeof i.state !== 'string' || typeof i.blocking !== 'boolean')) return false;
      count += g.items.length;
    } else return false;
  }
  return count <= 5000;
}
export function currentCloseout(value: unknown, projectID: string): Preview {
  if (!object(value) || !projectValid(value.project) || value.project.id !== projectID || !instant(value.as_of)
    || !groupsValid(value.groups) || typeof value.can_close !== 'boolean' || typeof value.can_archive !== 'boolean') {
    throw new MobileError('INVALID_RESPONSE', '结项核查资料不完整或不属于当前项目，请重新读取。');
  }
  const complete = value.groups.every(g => g.visibility === 'complete');
  const blocked = value.groups.some(g => (g.open_count ?? 0) > 0);
  if (value.visibility === 'complete' ? !complete || !fingerprint(value.fingerprint)
    : value.visibility !== 'restricted' || complete || value.fingerprint !== null || value.can_close || value.can_archive) {
    throw new MobileError('INVALID_RESPONSE', '当前权限与核查范围尚未核实，不能将缺项当作通过。');
  }
  if (value.can_close && (blocked || !['active', 'on_hold'].includes(value.project.state))
    || value.can_archive && (blocked || value.project.state !== 'closing')) {
    throw new MobileError('INVALID_RESPONSE', '结项动作与当前实际阶段或待处置事项不一致。');
  }
  return value as Preview;
}
export function closeoutWriter(members: Schemas['ProjectMembers'], me: Schemas['Me']) {
  return members.memberships?.find(member => member.user_id === me.user.id)?.project_role !== 'viewer';
}
export function projectActions(basis: Preview, members: Schemas['ProjectMembers'], me: Schemas['Me']): { action: ProjectAction; label: string }[] {
  if (!closeoutWriter(members, me)) return [];
  const state = basis.project.state, actions: { action: ProjectAction; label: string }[] = [];
  if (me.actions.includes('project.update')) {
    if (state === 'draft') actions.push({ action: 'activate', label: '启动项目' });
    if (state === 'active') actions.push({ action: 'hold', label: '暂停项目' });
    if (state === 'on_hold' || state === 'closing') actions.push({ action: 'resume', label: '恢复进行' });
    if (basis.can_close) actions.push({ action: 'close', label: '进入结项核查阶段' });
    if (['draft', 'active', 'on_hold'].includes(state)) actions.push({ action: 'cancel', label: '取消项目' });
  }
  if (['archived', 'cancelled'].includes(state) && me.actions.includes('project.reopen')) actions.push({ action: 'reopen', label: '重新打开项目' });
  return actions;
}
export function projectTransition(basis: Preview, members: Schemas['ProjectMembers'], me: Schemas['Me'], action: string, reason: string) {
  currentCloseout(basis, basis.project.id);
  if (!projectActions(basis, members, me).some(choice => choice.action === action) || !text(reason, 200)) throw new Error('请核对当前项目阶段、动作权限与200字以内的原因。');
  return { expected_version: basis.project.version, action: action as ProjectAction, reason: trim(reason) };
}
export function projectStateReceipt(value: unknown, before: Project, state: Project['state']): value is Project {
  if (!projectValid(value) || value.id !== before.id || value.version !== before.version + 1 || value.state !== state) return false;
  return (['name', 'scope', 'owner_id', 'project_no', 'type', 'classification', 'template_id', 'template_version',
    'creator_id', 'initialization_state', 'start_date', 'due_date'] as const).every(key => (value[key] ?? null) === (before[key] ?? null));
}
export const transitionState = (action: ProjectAction): Project['state'] => ({ activate: 'active', hold: 'on_hold', resume: 'active', close: 'closing', cancel: 'cancelled', reopen: 'active' } as const)[action];

export function archivePayload(basis: Preview, members: Schemas['ProjectMembers'], me: Schemas['Me'], notes: ArchiveNotes,
  files: ArchiveFile[], business: boolean, financial: boolean, fresh: boolean): Schemas['ProjectArchiveInput'] {
  currentCloseout(basis, basis.project.id);
  if (!basis.can_archive || basis.visibility !== 'complete' || !fingerprint(basis.fingerprint)
    || !closeoutWriter(members, me) || !me.actions.includes('project.archive')) throw new Error('当前核查、项目阶段或归档权限不允许提交。');
  if (!text(notes.summary, 20000) || !text(notes.lessons, 20000) || !text(notes.followUp, 20000) || !text(notes.reason, 200)) throw new Error('请完整填写三项复盘内容和200字以内的原因；无后续事项也请明确说明。');
  if (!business || !financial || !fresh) throw new Error('请重新核实业务与财务两项声明，并完成本次两步验证。');
  if (files.length > 20 || files.some(f => !uuid(f.version_id) || !uuid(f.file_id)) || new Set(files.map(f => f.version_id)).size !== files.length) throw new Error('附件须为最多20个不同的实际正式版本。');
  return { expected_version: basis.project.version, expected_fingerprint: basis.fingerprint, summary: trim(notes.summary),
    lessons_learned: trim(notes.lessons), follow_up: trim(notes.followUp), reason: trim(notes.reason),
    business_verified: true, financial_verified: true, file_version_ids: files.map(f => f.version_id).sort() };
}
function archiveValid(value: unknown, projectID: string): value is Archive {
  if (!object(value) || !uuid(value.id) || value.project_id !== projectID || !version(value.project_version)
    || !text(value.project_name, 200) || !fingerprint(value.fingerprint) || !text(value.summary, 20000)
    || !text(value.lessons_learned, 20000) || !text(value.follow_up, 20000) || !text(value.reason, 200)
    || value.business_verified !== true || value.financial_verified !== true || !uuid(value.actor_id) || !text(value.actor_name, 200)
    || !instant(value.created_at) || !['current', 'changed'].includes(value.current_basis_state as string)
    || !groupsValid(value.groups, true) || value.groups.some(g => g.open_count !== 0)
    || !Array.isArray(value.file_version_ids) || value.file_version_ids.length > 20
    || new Set(value.file_version_ids).size !== value.file_version_ids.length || value.file_version_ids.some(id => !uuid(id))
    || !Array.isArray(value.attachments) || value.attachments.length !== value.file_version_ids.length) return false;
  return value.attachments.every((f, i) => object(f) && uuid(f.file_id) && f.version_id === (value.file_version_ids as string[])[i]
    && version(f.seq) && text(f.name, 200) && typeof f.size_bytes === 'string' && /^(0|[1-9][0-9]*)$/.test(f.size_bytes)
    && typeof f.sha256 === 'string' && /^[a-f0-9]{64}$/.test(f.sha256) && text(f.media_type, 200)
    && ['clean', 'waived'].includes(f.scan_state as string) && (f.scan_waiver_ref === null || uuid(f.scan_waiver_ref)));
}
export function currentArchives(value: unknown, projectID: string): Schemas['ProjectArchiveList'] {
  if (!object(value) || !Array.isArray(value.items) || value.items.length > 100
    || value.visibility === 'restricted' && (value.items.length !== 0 || value.next_cursor !== null)
    || !['complete', 'restricted'].includes(value.visibility as string)
    || value.next_cursor !== null && (typeof value.next_cursor !== 'string' || !value.next_cursor.length || value.next_cursor.length > 2048)
    || value.items.some(item => !archiveValid(item, projectID)) || new Set(value.items.map(item => item.id)).size !== value.items.length) {
    throw new MobileError('INVALID_RESPONSE', '归档历史未能完整核实，请重新读取；不展示旧缓存或不完整复盘。');
  }
  return value as Schemas['ProjectArchiveList'];
}
function sameGroups(actual: Group[], before: Group[]): boolean {
  return kinds.every(kind => {
    const a = actual.find(g => g.kind === kind)!, b = before.find(g => g.kind === kind)!;
    return a.visibility === b.visibility && a.count === b.count && a.open_count === b.open_count
      && a.items.every(item => { const old = b.items.find(i => i.id === item.id); return old
        && item.kind === old.kind && item.version === old.version && item.title === old.title && item.state === old.state && item.blocking === old.blocking; });
  });
}
export function archiveReceipt(value: unknown, basis: Preview, input: Schemas['ProjectArchiveInput'], files: ArchiveFile[], userID: string): value is Schemas['ProjectArchiveReceipt'] {
  if (!object(value) || !projectStateReceipt(value.project, basis.project, 'archived') || !archiveValid(value.archive, basis.project.id)) return false;
  const actual = value.archive;
  return actual.project_version === value.project.version && actual.project_name === basis.project.name
    && actual.fingerprint === input.expected_fingerprint && actual.summary === input.summary && actual.lessons_learned === input.lessons_learned
    && actual.follow_up === input.follow_up && actual.reason === input.reason && actual.actor_id === userID && actual.current_basis_state === 'current'
    && files.length === input.file_version_ids.length && files.every(f => input.file_version_ids.includes(f.version_id))
    && JSON.stringify(actual.file_version_ids) === JSON.stringify(input.file_version_ids) && sameGroups(actual.groups, basis.groups)
    && actual.attachments.every(f => (['file_id', 'version_id', 'seq', 'name', 'sha256', 'size_bytes'] as const).every(key => f[key] === files.find(selected => selected.version_id === f.version_id)?.[key]));
}
export function archiveProof(current: Preview, history: Schemas['ProjectArchiveList'], receipt: Schemas['ProjectArchiveReceipt'], basis: Preview,
  input: Schemas['ProjectArchiveInput'], files: ArchiveFile[], userID: string): boolean {
  try {
    currentCloseout(current, basis.project.id); currentArchives(history, basis.project.id);
    const record = history.items.find(item => item.id === receipt.archive.id);
    return history.visibility === 'complete' && current.visibility === 'complete' && Boolean(record)
      && archiveReceipt({ project: current.project, archive: record }, basis, input, files, userID)
      && sameGroups(current.groups, record!.groups) && record!.created_at === receipt.archive.created_at;
  } catch { return false; }
}
