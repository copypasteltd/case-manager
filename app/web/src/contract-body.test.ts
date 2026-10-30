import { describe, expect, it } from 'vitest';
import { adopted, authorized, committed, contract, file, folder, id, input, run } from '../../packages/contracts/fixtures/contract-body';
import { contractBodyAccepted, contractBodyAdopted, contractBodyBytes, contractBodyProven, makeContractBody, makeContractBodyAdopt } from '@copypaste/contracts/contract-body';

describe('complete exact contract bodies and explicit human adoption', () => {
  it('requires an editable draft, exact scope and identical classification', () => {
    expect(makeContractBody(contract, folder, file, contract.project_id, ' review ', contract.classification)).toEqual({ ...input, reason: 'review' });
    for (const changed of [{ can_edit: false }, { lifecycle: 'effective' }, { approval_state: 'in_review' }, { version: 0 }, { project_id: null }, { content_hash: 'bad' }]) expect(() => makeContractBody({ ...contract, ...changed } as typeof contract, folder, file, contract.project_id, 'review', contract.classification)).toThrow();
    for (const changed of [{ version: 1 }, { state: 'trashed' }, { classification: 'internal' }, { project_id: null }]) expect(() => makeContractBody(contract, { ...folder, ...changed } as typeof folder, file, contract.project_id, 'review', contract.classification)).toThrow();
    expect(() => makeContractBody(contract, folder, file, contract.project_id, 'review', 'confidential')).toThrow();
  });
  it('binds a queued body to this run, exact contract input and new file intent', () => {
    expect(contractBodyAccepted(run(authorized()), input)).toBe(true);
    expect(contractBodyAccepted(run(authorized()))).toBe(false);
    const changed = run(authorized()); changed.file_create!.intent_sha256 = 'e'.repeat(64); expect(contractBodyAccepted(changed, input)).toBe(false);
    expect(contractBodyAccepted(run(authorized()), { ...input, expected_version: 4 })).toBe(false);
  });
  it('requires the complete original facts before declaring a body proven', () => {
    for (const changed of [{ id: id(44) }, { version: 4 }, { current_revision_id: id(45) }, { content_hash: 'f'.repeat(64) }, { lifecycle: 'effective' }]) { const value = authorized(); Object.assign(value.before, changed); expect(contractBodyProven(value)).toBe(false); }
    expect(contractBodyProven({ ...authorized(), file: committed().file })).toBe(false);
  });
  it('verifies full UTF-8 bytes beyond the old difference preview', () => {
    const value = committed(); expect(value.file!.content.length).toBeGreaterThan(1000); expect(contractBodyProven(value)).toBe(true); expect(contractBodyAccepted(run(value), input)).toBe(true);
    expect(Array.from(contractBodyBytes('香港🙂\r\n'))).toEqual([...Buffer.from('香港🙂\r\n')]);
    for (const text of ['\u0000', '\ud800']) expect(() => contractBodyBytes(text)).toThrow();
  });
  it('rejects altered, truncated, oversized, unscanned or mismatched file content', () => {
    for (const changed of [{ content: committed().file!.content.slice(0, 1000) }, { content: 'x'.repeat(49153) }]) { const value = committed(); Object.assign(value.file!, changed); expect(contractBodyProven(value)).toBe(false); }
    for (const changed of [{ sha256: 'e'.repeat(64) }, { size_bytes: '1' }, { seq: 2 }, { scan_state: 'pending' }, { media_type: 'application/pdf' }, { scan_waiver_ref: 'waiver' }]) { const value = committed(); Object.assign(value.file!.evidence, changed); expect(contractBodyProven(value)).toBe(false); }
  });
  it('never accepts a different draft, file name or unrelated write scope', () => {
    for (const changed of [{ mutation: {} }, { oa_request: {} }, { file_update: {} }, { post_write: {} }]) expect(contractBodyAccepted({ ...run(committed()), ...changed } as ReturnType<typeof run>, input)).toBe(false);
    const wrong = run(committed()); wrong.file_create!.drafts[0].result_version_id = id(77); expect(contractBodyAccepted(wrong, input)).toBe(false);
    const renamed = run(committed()); renamed.file_create!.target.name = 'other.md'; expect(contractBodyAccepted(renamed, input)).toBe(false);
  });
  it('requires human review, exact file digest and a bounded adoption reason', () => {
    expect(() => makeContractBodyAdopt(authorized(), true, 'review')).toThrow();
    expect(() => makeContractBodyAdopt(committed(), false, 'review')).toThrow();
    for (const why of [' ', 'x'.repeat(201)]) expect(() => makeContractBodyAdopt(committed(), true, why)).toThrow();
    const value = committed(); expect(makeContractBodyAdopt(value, true, ' review ')).toEqual({ intent_sha256: value.intent_sha256, version_id: value.file!.evidence.version_id, content_sha256: value.file!.evidence.sha256, reviewed: true, reason: 'review' });
  });
  it('accepts only a new contract revision with all other commercial facts preserved', () => {
    expect(contractBodyProven(adopted())).toBe(true); expect(contractBodyAdopted(adopted(), committed())).toBe(true);
    for (const changed of [{ version: 3 }, { project_id: null }, { lifecycle: 'effective' }, { current_revision_id: contract.current_revision_id }]) { const value = adopted(); Object.assign(value.result!, changed); expect(contractBodyProven(value)).toBe(false); }
    const altered = adopted(); (altered.result!.payload.content as Record<string, unknown>).amount = '99999'; expect(contractBodyProven(altered)).toBe(false);
  });
  it('does not accept an adoption receipt for a changed original basis or file', () => {
    const changed = adopted(); changed.intent_sha256 = 'f'.repeat(64); expect(contractBodyAdopted(changed, committed())).toBe(false);
    const swapped = adopted(); swapped.file!.evidence.file_id = id(33); expect(contractBodyAdopted(swapped, committed())).toBe(false);
    expect(() => makeContractBodyAdopt(adopted(), true, 'again')).toThrow();
  });
});
