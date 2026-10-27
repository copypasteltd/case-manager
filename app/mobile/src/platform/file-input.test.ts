import assert from 'node:assert/strict';
import test from 'node:test';
import { canChooseFolder, createFolderPayload, fileStatePayload, fileStateReceipt, folderCreateReceipt, movePayload, moveReceipt, storageName, type Folder, type StoredFile } from './file-input';
import { MemoryDrafts } from './drafts';

const id = (n: number) => `10000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
const folder = { id: id(1), version: 3, parent_id: id(2), project_id: id(3), creator_id: id(4), name: '交付资料', classification: 'project_restricted', state: 'active', can_create: true, can_update: true, can_upload: true, created_at: '2026-10-28T01:00:00Z', updated_at: '2026-10-28T02:00:00Z' } as Folder;
const destination = { ...folder, id: id(5), name: '已核对资料' };
const file = { id: id(6), version: 7, folder_id: folder.id, project_id: folder.project_id, creator_id: id(4), name: '项目说明.md', classification: folder.classification, state: 'active', current_version_id: id(7), legal_hold: false, can_move: true, can_trash: true, can_restore: false, created_at: folder.created_at, updated_at: folder.updated_at } as StoredFile;
const why = '经项目负责人核对后整理';

test('storage names preserve Unicode spelling and reject path, control, reserved and normalized unsafe forms', () => {
  for (const name of ['香港项目🙂', 'ＡＢＣ说明.md', 'Résumé', '供应商 报价', '🙂'.repeat(200)]) assert.equal(storageName(name), name);
  for (const name of ['', ' leading', 'trailing\u0085', '.', '..', 'name.', 'path/name', 'path\\name', 'data：one', 'bad\u200bname', 'x\u0000y', 'CON', 'com1.txt', 'ＬＰＴ９.pdf', 'x'.repeat(201), '🙂'.repeat(201)]) assert.throws(() => storageName(name));
});

test('new folders bind actual parent and inherited classification without injecting a project or writable flags', () => {
  const body = createFolderPayload('新资料🙂', folder.classification, folder);
  assert.deepEqual(body, { name: '新资料🙂', classification: folder.classification, parent_id: folder.id });
  assert.deepEqual(createFolderPayload('公司资料', 'internal'), { name: '公司资料', classification: 'internal' });
  for (const parent of [{ ...folder, can_create: false }, { ...folder, state: 'trashed' }, { ...folder, version: 0 }]) assert.throws(() => createFolderPayload('新资料', folder.classification, parent as Folder));
  assert.throws(() => createFolderPayload('新资料', 'internal', folder));
  assert.throws(() => createFolderPayload('新资料', '__proto__'));
});

test('move freezes source version, exact name, selected destination ID and reason without broadening intent', () => {
  assert.deepEqual(movePayload('file', file, '交付终稿.md', why, destination), { expected_version: file.version, new_name: '交付终稿.md', target_folder_id: destination.id, reason: why });
  assert.deepEqual(movePayload('folder', folder, '改名资料', why), { expected_version: folder.version, new_name: '改名资料', reason: why });
  assert.equal(file.name, '项目说明.md'); assert.equal(destination.name, '已核对资料');
});

test('destination selection rejects wrong project/classification, hidden write permission, self and malformed identities', () => {
  assert.equal(canChooseFolder('folder', folder, destination), true);
  for (const target of [{ ...destination, project_id: null }, { ...destination, project_id: id(99) }, { ...destination, classification: 'internal' }, { ...destination, can_create: false }, { ...destination, state: 'trashed' }, { ...destination, id: 'not-an-id' }]) {
    assert.equal(canChooseFolder('folder', folder, target as Folder), false);
    assert.throws(() => movePayload('folder', folder, '改名', why, target as Folder));
  }
  assert.equal(canChooseFolder('folder', folder, folder), false);
  assert.throws(() => movePayload('folder', folder, '改名', why, folder));
});

test('project roots may be renamed but never moved; unchanged requests and missing current write permission are refused', () => {
  const root = { ...folder, parent_id: null };
  assert.equal(movePayload('folder', root, '新的项目根目录名', why).new_name, '新的项目根目录名');
  assert.throws(() => movePayload('folder', root, '新的项目根目录名', why, destination));
  assert.throws(() => movePayload('file', file, file.name, why));
  assert.throws(() => movePayload('file', file, file.name, why, folder));
  for (const before of [{ ...file, can_move: false }, { ...file, state: 'trashed' }, { ...file, version: Number.MAX_SAFE_INTEGER }]) assert.throws(() => movePayload('file', before as StoredFile, '终稿.md', why));
  for (const reason of ['', '\u0085\u3000', 'x'.repeat(201), '🙂'.repeat(201)]) assert.throws(() => movePayload('file', file, '终稿.md', reason));
});

test('folder creation receipts must match the exact parent, scope, name, creator and initial version', () => {
  const body = createFolderPayload('新目录', folder.classification, folder);
  const receipt = { ...folder, id: id(10), version: 1, parent_id: folder.id, name: body.name };
  assert.equal(folderCreateReceipt(receipt, body, folder.creator_id!, folder), true);
  for (const patch of [{ parent_id: destination.id }, { project_id: null }, { classification: 'internal' }, { creator_id: id(99) }, { name: 'Other' }, { state: 'trashed' }, { version: 2 }, { id: folder.id }, { created_at: 'bad' }]) assert.equal(folderCreateReceipt({ ...receipt, ...patch }, body, folder.creator_id!, folder), false);
  assert.equal(folderCreateReceipt({}, body, folder.creator_id!, folder), false);
});

test('file move receipts cannot claim success while changing formal content, lifecycle, owner or namespace', () => {
  const body = movePayload('file', file, '交付终稿.md', why, destination);
  const receipt = { ...file, version: file.version + 1, name: body.new_name!, folder_id: destination.id };
  assert.equal(moveReceipt(receipt, 'file', file, body), true);
  for (const patch of [{ id: id(99) }, { version: file.version }, { version: file.version + 2 }, { folder_id: folder.id }, { name: 'Other' }, { project_id: null }, { classification: 'internal' }, { creator_id: id(99) }, { state: 'trashed' }, { legal_hold: true }, { current_version_id: id(99) }, { created_at: '2026-01-01T00:00:00Z' }]) assert.equal(moveReceipt({ ...receipt, ...patch }, 'file', file, body), false);
});

test('folder move receipts preserve the actual folder identity and require the selected parent', () => {
  const body = movePayload('folder', folder, '改名资料', why, destination);
  const receipt = { ...folder, version: folder.version + 1, name: body.new_name!, parent_id: destination.id };
  assert.equal(moveReceipt(receipt, 'folder', folder, body), true);
  assert.equal(moveReceipt({ ...receipt, parent_id: folder.parent_id }, 'folder', folder, body), false);
  assert.equal(moveReceipt({ ...receipt, project_id: id(99) }, 'folder', folder, body), false);
});

test('trash and restore use exact original commands, current capability and state without inventing history restoration', () => {
  assert.deepEqual(fileStatePayload(file, 'trash', why), { expected_version: file.version, action: 'trash', reason: why });
  const trashed = { ...file, state: 'trashed' as const, can_trash: false, can_restore: true };
  assert.deepEqual(fileStatePayload(trashed, 'restore', why), { expected_version: file.version, mode: 'untrash', reason: why });
  for (const before of [{ ...file, legal_hold: true }, { ...file, can_trash: false }, trashed]) assert.throws(() => fileStatePayload(before, 'trash', why));
  assert.throws(() => fileStatePayload(file, 'restore', why));
  assert.throws(() => fileStatePayload({ ...trashed, can_restore: false }, 'restore', why));
});

test('trash/restore receipts must preserve original name, location and exact formal version', () => {
  const trashed = { ...file, state: 'trashed' as const, version: file.version + 1 };
  assert.equal(fileStateReceipt(trashed, file, 'trash'), true);
  assert.equal(fileStateReceipt({ ...trashed, state: 'active', version: trashed.version + 1 }, trashed, 'restore'), true);
  for (const patch of [{ state: 'active' }, { current_version_id: null }, { folder_id: destination.id }, { name: 'Renamed' }, { version: file.version }, { legal_hold: true }]) assert.equal(fileStateReceipt({ ...trashed, ...patch }, file, 'trash'), false);
  for (const value of [null, [], true, 'success', {}]) assert.equal(fileStateReceipt(value, file, 'trash'), false);
});

test('organizing drafts retain whitelisted text only and never restore destination evidence or confirmation', () => {
  const store = new MemoryDrafts(() => ({ authenticated: true, epoch: 1 }));
  const authority = { epoch: 1, userID: id(4), authorizationVersion: 1 };
  const handle = store.open(authority, { kind: 'file-organize', id: file.id, version: file.version })!;
  assert.equal(handle.write({ name: '新名称.md', reason: why, action: 'move', target: destination, confirmed: true } as never), true);
  assert.deepEqual(handle.read(), { name: '新名称.md', reason: why, action: 'move' });
  assert.equal(handle.write({ name: '🙂'.repeat(200), reason: why, action: 'move' }), true);
  assert.equal(handle.read()!.name, '🙂'.repeat(200));
  assert.equal(store.open(authority, { kind: 'folder-organize', id: file.id, version: file.version })!.read(), undefined);
  assert.equal(store.open(authority, { kind: 'file-organize', id: file.id, version: file.version + 1 })!.read(), undefined);
});
