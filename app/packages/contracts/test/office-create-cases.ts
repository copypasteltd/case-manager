import assert from 'node:assert/strict';
import { fileCreateFormats, fileCreateReceipt, makeFileCreate, makeFileUpdate, officeCreateKind, type FileCreate, type FileCreateView } from '../src/file-update';
import type { components } from '../src/api';
type Schemas = components['schemas'];
const id = (n: number) => `10000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
const folder = { id: id(1), version: 4, state: 'active', project_id: id(2) } as Schemas['Folder'];
const media = fileCreateFormats[2][0];
const input = () => makeFileCreate(folder, '项目说明.docx', media, id(2));
function staged(): FileCreateView {
  return { run_id: id(3), intent_sha256: 'a'.repeat(64), status: 'authorized', operation_id: null, result: null,
    target: { folder_id: id(1), folder_version: 4, name: '项目说明.docx', media_type: media, classification: 'project_restricted' },
    drafts: [{ id: id(4), content_sha256: 'b'.repeat(64), size_bytes: '4567', state: 'ready', scan_state: 'clean', plan_hash: 'c'.repeat(64), result_version_id: null,
      diff: { start_char: 0, removed_chars: 0, added_chars: 17, removed_sha256: 'e'.repeat(64), added_sha256: 'd'.repeat(64), before: '', after: '[paragraph] 合成Office文字摘录🙂', before_complete: true, after_complete: true, line_ending: 'lf', utf8_bom: false,
        office: { adapter: 'agent-office-create-v1', kind: 'docx', template_sha256: 'f'.repeat(64), workspace_sha256: '1'.repeat(64), unit_count: 3, warnings: ['核对完整文件与版式。'] } } }] };
}
const run = (view: FileCreateView) => ({ id: id(3), file_create: view } as Schemas['Run']);
export const officeCreateCases: Array<readonly [string, () => void]> = [
  ['human format and extension selection supports all five formats without implicit overwrite or unsafe paths', () => {
    for (const [type, , pattern] of fileCreateFormats) {
      const extension = officeCreateKind(type) ?? (type === 'text/plain' ? 'txt' : 'md');
      const value = makeFileCreate(folder, `合成资料.${extension}`, type, id(2));
      assert.equal(value.media_type, type); assert.equal(value.expected_folder_version, 4); assert.match(value.name, pattern);
    }
    for (const name of ['wrong.xlsx', 'macro.docm', '../secret.docx', 'x\\secret.docx', ' leading.docx']) assert.throws(() => makeFileCreate(folder, name, media, id(2)));
    assert.throws(() => makeFileCreate({ ...folder, state: 'trashed' }, 'a.docx', media, id(2)));
    assert.throws(() => makeFileCreate(folder, 'a.docx', media, id(9)));
    assert.throws(() => makeFileCreate(folder, 'a.pdf', 'application/pdf' as FileCreate['media_type'], id(2)));
  }],
  ['Office authorization binds actual generated format, workspace/template proof and bounded artifact size', () => {
    const value = staged(); assert.equal(fileCreateReceipt(run(value), input()), true);
    const patches: Array<(v: FileCreateView) => void> = [
      v => { delete v.drafts[0].diff.office; },
      v => { v.drafts[0].diff.office!.kind = 'xlsx'; },
      v => { v.drafts[0].diff.office!.adapter = 'other' as never; },
      v => { v.drafts[0].diff.office!.template_sha256 = 'bad'; },
      v => { v.drafts[0].diff.office!.workspace_sha256 = 'A'.repeat(64); },
      v => { v.drafts[0].diff.office!.unit_count = 2001; },
      v => { v.drafts[0].diff.office!.warnings = ['x'.repeat(2001)]; },
      v => { v.drafts[0].size_bytes = '1048577'; },
      v => { v.drafts[0].size_bytes = '0'; },
      v => { v.target.folder_version = 5; },
    ];
    for (const patch of patches) { const changed = structuredClone(value); patch(changed); assert.equal(fileCreateReceipt(run(changed), input()), false); }
  }],
  ['formal Office receipt requires scanned first-version binary hash and exact draft plan, not the text projection hash', () => {
    const value = staged(); value.status = 'committed'; value.operation_id = id(5); value.drafts[0].result_version_id = id(6);
    value.result = { committed: true, file: { id: id(7), folder_id: id(1), name: '项目说明.docx', classification: 'project_restricted', current_version_id: id(6) }, file_version: { id: id(6), file_id: id(7), seq: 1, base_version_id: null, source: 'agent', scan_state: 'clean', media_type: media, sha256: 'b'.repeat(64), size_bytes: '4567' } } as Schemas['FileCommitResult'];
    assert.notEqual(value.drafts[0].content_sha256, value.drafts[0].diff.added_sha256);
    assert.equal(fileCreateReceipt(run(value), input()), true);
    for (const patch of [{ sha256: 'd'.repeat(64) }, { seq: 2 }, { scan_state: 'waived' }, { media_type: 'text/markdown' }, { size_bytes: '4568' }]) {
      const changed = structuredClone(value); Object.assign(changed.result!.file_version, patch); assert.equal(fileCreateReceipt(run(changed), input()), false);
    }
    const unknown = structuredClone(value); unknown.operation_id = null; assert.equal(fileCreateReceipt(run(unknown), input()), false);
  }],
  ['creation does not broaden existing Office update authority or text-only request behavior', () => {
    const file = { id: id(7), version: 1, project_id: id(2), state: 'active', current_version_id: id(6) } as Schemas['File'];
    const version = { id: id(6), file_id: id(7), scan_state: 'clean', media_type: media, size_bytes: '4567' } as Schemas['FileVersion'];
    assert.throws(() => makeFileUpdate(file, version, id(2)));
    const plain = makeFileCreate(folder, '原有纪要.md', 'text/markdown', id(2));
    const view = { ...staged(), target: { ...staged().target, name: plain.name, media_type: plain.media_type }, drafts: [] };
    assert.equal(fileCreateReceipt(run(view), plain), true);
    assert.equal(officeCreateKind('text/plain'), null); assert.equal(officeCreateKind('application/pdf'), null);
  }],
];
