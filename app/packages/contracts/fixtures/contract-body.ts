import { createHash } from 'node:crypto';
import type { components } from '../src/api';
import { makeContractBody, type ContractBodyView } from '../src/contract-body';
type S = components['schemas'];
export const id = (number: number) => '11700000-0000-4000-8000-' + String(number).padStart(12, '0');
export const contract = { id: id(1), version: 3, current_revision_id: id(2), content_hash: 'a'.repeat(64), project_id: id(3), classification: 'project_restricted', title: 'Synthetic contract', lifecycle: 'draft', approval_state: 'not_submitted', can_edit: true } as S['Contract'];
export const folder = { id: id(4), version: 2, project_id: contract.project_id, classification: contract.classification, state: 'active' } as S['Folder'];
export const file = { folder_id: folder.id, expected_folder_version: folder.version, name: '完整合同.md', media_type: 'text/markdown' } as S['AgentFileCreateInput'];
export const input = makeContractBody(contract, folder, file, contract.project_id, 'Use exact facts', contract.classification);
export function authorized(): ContractBodyView {
  return { run_id: id(5), input, before: { ...contract, kind: 'contract', payload: { content: { title: contract.title, currency: 'HKD', amount: '12000', start_date: '2026-11-01', end_date: '2027-10-31', notes: 'Human commercial facts', draft_file_version_id: id(20) } } }, intent_sha256: 'b'.repeat(64), file_intent_sha256: 'c'.repeat(64), status: 'authorized', file: null, result: null, operation_id: null } as unknown as ContractBodyView;
}
export function committed(): ContractBodyView {
  const content = '\ufeff# 完整合同🙂\r\nHKD 12000\r\n' + '完整条款。\r\n'.repeat(240) + 'END OF COMPLETE BODY\r\n';
  return { ...authorized(), status: 'file_committed', file: { draft_id: id(6), content, evidence: { file_id: id(7), version_id: id(8), seq: 1, name: file.name, size_bytes: String(Buffer.byteLength(content)), sha256: createHash('sha256').update(content).digest('hex'), media_type: file.media_type, scan_state: 'clean', scan_waiver_ref: null } } };
}
export function adopted(): ContractBodyView {
  const value = committed();
  return { ...value, status: 'adopted', operation_id: id(9), result: { ...value.before, version: value.before.version + 1, current_revision_id: id(10), content_hash: 'd'.repeat(64), payload: { ...value.before.payload, content: { ...value.before.payload.content as object, draft_file_version_id: value.file!.evidence.version_id } } } } as unknown as ContractBodyView;
}
export function run(value: ContractBodyView): S['Run'] {
  return { id: value.run_id, contract_body: value, file_create: { run_id: value.run_id, intent_sha256: value.file_intent_sha256, target: { ...file, folder_id: folder.id, folder_version: folder.version, classification: folder.classification }, drafts: value.file ? [{ id: value.file.draft_id, content_sha256: value.file.evidence.sha256, result_version_id: value.file.evidence.version_id }] : [] } } as unknown as S['Run'];
}
