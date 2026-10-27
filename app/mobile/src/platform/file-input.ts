import type { Schemas } from './session';

export type Folder = Schemas['Folder'];
export type StoredFile = Schemas['File'];
export type FileResource = Folder | StoredFile;
export type ResourceKind = 'file' | 'folder';
// The API's required-only anyOf alternatives generate an unknown TypeScript
// union. This exact supported command subset stays typed before transmission.
export type FileMove = { expected_version: number; new_name: string; reason: string; target_folder_id?: string };
export const classifications = { internal: '公司内部', project_restricted: '项目受限', confidential: '机密' } as const;
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const record = (v: unknown): v is Record<string, unknown> => Boolean(v) && typeof v === 'object' && !Array.isArray(v);
const nullableID = (v: unknown) => v === null || typeof v === 'string' && uuid.test(v);
const version = (v: number) => Number.isSafeInteger(v) && v > 0 && v < Number.MAX_SAFE_INTEGER;
const goTrim = (v: string) => v.replace(/^[\s\u0085]+|[\s\u0085]+$/gu, '');

// Preserve the entered spelling. Server NFKC/case folding remains authoritative
// for collisions; validation never silently renames a file or directory.
export function storageName(name: string): string {
  if (!name || [...name].length > 200 || goTrim(name) !== name) throw new Error('名称需为1至200字符，首尾不能有空白。');
  const normalized = name.normalize('NFKC').toLowerCase();
  for (const value of [name, normalized]) {
    if (/[\\/<>:"|?*\p{Cc}\p{Cf}\p{Cs}]/u.test(value)) throw new Error('名称不能包含路径分隔符、平台禁用字符或隐藏控制字符。');
  }
  if (normalized === '.' || normalized === '..' || /[. ]$/.test(normalized)) throw new Error('名称不能是点目录或以点和空格结尾。');
  if (/^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/.test(normalized)) throw new Error('名称为平台保留名，请更换。');
  return name;
}
function reason(value: string): string {
  if (!goTrim(value) || [...value].length > 200) throw new Error('请填写1至200字符的操作原因。');
  return value;
}
function source(value: FileResource) {
  if (!uuid.test(value.id) || !version(value.version) || !nullableID(value.project_id) || !Object.prototype.hasOwnProperty.call(classifications, value.classification)) throw new Error('来源资料不完整，请刷新后重新核对。');
}
export function canChooseFolder(kind: ResourceKind, before: FileResource, target: Folder): boolean {
  return target.state === 'active' && target.can_create === true && target.project_id === before.project_id
    && target.classification === before.classification && uuid.test(target.id)
    && (kind !== 'folder' || target.id !== before.id);
}
export function createFolderPayload(name: string, classification: string, parent?: Folder): Schemas['FolderCreate'] {
  storageName(name);
  if (!Object.prototype.hasOwnProperty.call(classifications, classification)) throw new Error('请选择实际资料分类。');
  if (parent) {
    source(parent);
    if (!parent.can_create || parent.state !== 'active' || classification !== parent.classification) throw new Error('当前目录不允许新建，或分类与父目录不一致。');
    return { name, classification: classification as Folder['classification'], parent_id: parent.id };
  }
  return { name, classification: classification as Folder['classification'] };
}
export function movePayload(kind: ResourceKind, before: FileResource, name: string, why: string, target?: Folder): FileMove {
  source(before); storageName(name); reason(why);
  if (before.state !== 'active' || !(kind === 'folder' ? (before as Folder).can_update : (before as StoredFile).can_move)) throw new Error('当前资料不允许改名或移动。');
  const currentParent = kind === 'folder' ? (before as Folder).parent_id : (before as StoredFile).folder_id;
  if (target && !canChooseFolder(kind, before, target)) throw new Error('目标须为同项目、同分类且可新建的实际目录。');
  if (target && kind === 'folder' && before.project_id !== null && (before as Folder).parent_id === null) throw new Error('项目资料根目录只能改名，不能移动。');
  if (name === before.name && (!target || target.id === currentParent)) throw new Error('名称和位置均未变化，请核对本次实际变更。');
  return { expected_version: before.version, new_name: name, reason: why, ...(target ? { target_folder_id: target.id } : {}) };
}
export function fileStatePayload(before: StoredFile, action: 'trash' | 'restore', why: string): Schemas['Transition'] | Schemas['RestoreFile'] {
  source(before); reason(why);
  if (action === 'trash') {
    if (!before.can_trash || before.state !== 'active' || before.legal_hold) throw new Error('当前文件不能移入回收站，请核对状态与保留要求。');
    return { expected_version: before.version, action: 'trash', reason: why };
  }
  if (!before.can_restore || before.state !== 'trashed') throw new Error('当前文件不能从回收站恢复。');
  return { expected_version: before.version, mode: 'untrash', reason: why };
}
function validResource(value: unknown): value is Record<string, unknown> {
  return record(value) && typeof value.id === 'string' && uuid.test(value.id)
    && Number.isSafeInteger(value.version) && Number(value.version) > 0 && typeof value.name === 'string'
    && nullableID(value.project_id) && nullableID(value.creator_id) && typeof value.classification === 'string'
    && Object.prototype.hasOwnProperty.call(classifications, value.classification) && typeof value.created_at === 'string' && Number.isFinite(Date.parse(value.created_at))
    && typeof value.updated_at === 'string' && Number.isFinite(Date.parse(value.updated_at));
}
export function folderCreateReceipt(value: unknown, body: Schemas['FolderCreate'], creatorID: string, parent?: Folder): value is Folder {
  return validResource(value) && value.version === 1 && value.state === 'active' && value.name === body.name
    && value.classification === body.classification && value.creator_id === creatorID
    && value.parent_id === (parent?.id ?? null) && value.project_id === (parent?.project_id ?? null)
    && (!parent || value.id !== parent.id);
}
export function moveReceipt(value: unknown, kind: ResourceKind, before: FileResource, body: FileMove): value is FileResource {
  if (!validResource(value) || value.id !== before.id || value.version !== before.version + 1 || value.state !== before.state
    || value.created_at !== before.created_at || value.creator_id !== before.creator_id
    || value.project_id !== before.project_id || value.classification !== before.classification || value.name !== body.new_name) return false;
  if (kind === 'folder') return value.parent_id === (body.target_folder_id ?? (before as Folder).parent_id);
  const file = before as StoredFile;
  return value.folder_id === (body.target_folder_id ?? file.folder_id) && value.current_version_id === file.current_version_id && value.legal_hold === file.legal_hold;
}
export function fileStateReceipt(value: unknown, before: StoredFile, action: 'trash' | 'restore'): value is StoredFile {
  return validResource(value) && value.id === before.id && value.version === before.version + 1
    && value.state === (action === 'trash' ? 'trashed' : 'active') && value.name === before.name
    && value.folder_id === before.folder_id && value.project_id === before.project_id && value.classification === before.classification
    && value.created_at === before.created_at && value.creator_id === before.creator_id
    && value.current_version_id === before.current_version_id && value.legal_hold === before.legal_hold;
}
