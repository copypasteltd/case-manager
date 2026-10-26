import { describe, expect, it } from 'vitest';
import { fileDraftState, fileScanState, fileUpdateReceipt, makeFileUpdate, type FileUpdateView } from '@copypaste/contracts/file-update';
import type { Schemas } from './api';

const id = '11111111-1111-4111-8111-111111111111', base = '22222222-2222-4222-8222-222222222222', runID = '33333333-3333-4333-8333-333333333333', next = '44444444-4444-4444-8444-444444444444';
const file = { id, version: 7, project_id: null, state: 'active', current_version_id: base } as Schemas['File'];
const version = { id: base, file_id: id, media_type: 'text/markdown', scan_state: 'clean', size_bytes: '1048576' } as Schemas['FileVersion'];
const input = makeFileUpdate(file, version, null);
const authorized = { run_id: runID, file_id: id, base_version_id: base, file_version: 7, intent_sha256: 'a'.repeat(64), target: { name: '准确文件.md' }, status: 'authorized', drafts: [], operation_id: null, result: null } as unknown as FileUpdateView;
function run(value: FileUpdateView) { return { id: runID, file_update: value } as Schemas['Run']; }

describe('exact file update scope and formal commit evidence', () => {
  it('pins current file metadata and immutable version without adding paths or automatic edits', () => {
    expect(input).toEqual({ file_id: id, base_version_id: base, expected_file_version: 7 });
    for (const altered of [{ ...file, current_version_id: next }, { ...file, project_id: next }, { ...file, state: 'trashed' }]) expect(() => makeFileUpdate(altered as Schemas['File'], version, null)).toThrow();
  });
  it('rejects unsupported formats, waived scans, empty and oversized bodies', () => {
    for (const altered of [{ ...version, scan_state: 'waived' as const }, { ...version, media_type: 'application/json' }, { ...version, size_bytes: '0' }, { ...version, size_bytes: '1048577' }, { ...version, size_bytes: 'invalid' }]) expect(() => makeFileUpdate(file, altered, null)).toThrow();
  });
  it('accepts a queued exact intent but rejects altered target and fake completion', () => {
    expect(fileUpdateReceipt(run(authorized), input)).toBe(true);
    for (const altered of [{ ...authorized, file_id: next }, { ...authorized, base_version_id: next }, { ...authorized, file_version: 8 }, { ...authorized, status: 'committed' }, { ...authorized, operation_id: next }]) expect(fileUpdateReceipt(run(altered as FileUpdateView), input)).toBe(false);
    expect(fileUpdateReceipt(run(authorized))).toBe(false);
  });
  it('requires matching scanned formal version, content and fixed plan while preserving pending replica truth', () => {
    const committed = { ...authorized, status: 'committed', operation_id: next, drafts: [{ result_version_id: next, content_sha256: 'b'.repeat(64), plan_hash: 'c'.repeat(64) }], result: { committed: true, file: { id, version: 8, current_version_id: next }, file_version: { id: next, file_id: id, base_version_id: base, source: 'agent', scan_state: 'clean', sha256: 'b'.repeat(64), replicas: [{ state: 'verified' }, { state: 'queued' }] } } } as unknown as FileUpdateView;
    expect(fileUpdateReceipt(run(committed), input)).toBe(true);
    const changed = structuredClone(committed); changed.result!.file_version.sha256 = 'd'.repeat(64);
    expect(fileUpdateReceipt(run(changed), input)).toBe(false);
    expect(fileDraftState('ready')).toBe('候选已就绪');
    expect(fileScanState('waived')).toBe('人工豁免不能用于秘书提交');
    expect(fileDraftState('untrusted-provider-detail')).toBe('状态待核对');
  });
});
