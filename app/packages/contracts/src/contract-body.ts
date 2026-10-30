import { sha256 } from '@noble/hashes/sha2.js';
import type { components } from './api';

type S = components['schemas'];
export type ContractBodyInput = S['AgentContractBodyInput'];
export type ContractBodyView = S['AgentContractBodyView'];
const uuid = (value: string) => /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(value);
const digest = (value: string) => /^[a-f0-9]{64}$/.test(value);
const canonical = (value: unknown): string => Array.isArray(value) ? '[' + value.map(canonical).join(',') + ']' : value && typeof value === 'object' ? '{' + Object.keys(value).sort().map(key => JSON.stringify(key) + ':' + canonical((value as Record<string, unknown>)[key])).join(',') + '}' : JSON.stringify(value);

// Native-compatible UTF-8, including supplementary characters. No browser-only
// TextEncoder or lossy surrogate replacement may change the reviewed bytes.
export function contractBodyBytes(text: string): Uint8Array {
  const bytes: number[] = [];
  for (const char of text) {
    const code = char.codePointAt(0)!;
    if (code >= 0xd800 && code <= 0xdfff || code === 0) throw new Error('正文含有无效字符，请重新读取准确文件。');
    if (code < 0x80) bytes.push(code);
    else if (code < 0x800) bytes.push(0xc0 | code >> 6, 0x80 | code & 63);
    else if (code < 0x10000) bytes.push(0xe0 | code >> 12, 0x80 | code >> 6 & 63, 0x80 | code & 63);
    else bytes.push(0xf0 | code >> 18, 0x80 | code >> 12 & 63, 0x80 | code >> 6 & 63, 0x80 | code & 63);
  }
  return new Uint8Array(bytes);
}

function reason(value: string) {
  const clean = value.trim();
  if (!clean || [...clean].length > 200) throw new Error('请填写1至200字操作依据。');
  return clean;
}

export function makeContractBody(contract: S['Contract'], folder: S['Folder'], file: S['AgentFileCreateInput'], project: string | null, why: string, conversationClass: string): ContractBodyInput {
  if (!contract.can_edit || contract.lifecycle !== 'draft' || !['not_submitted', 'returned', 'withdrawn'].includes(contract.approval_state) || contract.project_id !== project || folder.project_id !== project || conversationClass !== contract.classification || folder.classification !== contract.classification || folder.id !== file.folder_id || folder.version !== file.expected_folder_version || folder.state !== 'active' || !uuid(contract.id) || !uuid(contract.current_revision_id) || !digest(contract.content_hash) || !Number.isSafeInteger(contract.version) || contract.version < 1) throw new Error('请选择会话范围内可编辑的合同，以及分类一致的准确新文件目录。');
  return { contract_id: contract.id, expected_version: contract.version, base_revision_id: contract.current_revision_id, content_hash: contract.content_hash, reason: reason(why) };
}

export function contractBodyProven(value: ContractBodyView): boolean {
  const input = value.input, before = value.before;
  if (!uuid(value.run_id) || !digest(value.intent_sha256) || !digest(value.file_intent_sha256) || !input || !before || before.kind !== 'contract' || before.id !== input.contract_id || !uuid(input.contract_id) || !uuid(input.base_revision_id) || !digest(input.content_hash) || before.version !== input.expected_version || before.current_revision_id !== input.base_revision_id || before.content_hash !== input.content_hash || before.lifecycle !== 'draft' || !['not_submitted', 'returned', 'withdrawn'].includes(before.approval_state) || !before.payload?.content || !Number.isSafeInteger(before.version) || before.version < 1) return false;
  if (value.status === 'authorized') return value.file === null && value.operation_id === null && value.result === null;
  const file = value.file, evidence = file?.evidence;
  if (!file || !evidence || !uuid(file.draft_id) || !uuid(evidence.file_id) || !uuid(evidence.version_id) || evidence.seq !== 1 || evidence.scan_state !== 'clean' || evidence.scan_waiver_ref !== null || !['text/plain', 'text/markdown'].includes(evidence.media_type) || !digest(evidence.sha256) || typeof file.content !== 'string' || !file.content.trim() || !evidence.name?.trim()) return false;
  try {
    const bytes = contractBodyBytes(file.content);
    if (bytes.length < 1 || bytes.length > 49152 || evidence.size_bytes !== String(bytes.length) || Array.from(sha256(bytes), byte => byte.toString(16).padStart(2, '0')).join('') !== evidence.sha256) return false;
  } catch { return false; }
  if (value.status === 'file_committed') return value.operation_id === null && value.result === null;
  const saved = value.result;
  return value.status === 'adopted' && !!value.operation_id && uuid(value.operation_id) && !!saved && saved.id === before.id && saved.kind === 'contract' && saved.version === before.version + 1 && uuid(saved.current_revision_id) && saved.current_revision_id !== before.current_revision_id && digest(saved.content_hash) && saved.project_id === before.project_id && saved.classification === before.classification && saved.lifecycle === 'draft' && canonical(saved.payload.content) === canonical({ ...before.payload.content as Record<string, unknown>, draft_file_version_id: evidence.version_id });
}

export function contractBodyAccepted(run: S['Run'], input?: ContractBodyInput): boolean {
  const value = run.contract_body;
  if (!input) return !value;
  if (!value || !run.file_create || run.file_create.recovery || run.file_update || run.mutation || run.oa_request || run.post_write || value.run_id !== run.id || value.file_intent_sha256 !== run.file_create.intent_sha256 || canonical(value.input) !== canonical(input) || !contractBodyProven(value)) return false;
  if (value.file && (value.file.evidence.name !== run.file_create.target.name || value.file.evidence.media_type !== run.file_create.target.media_type || !run.file_create.drafts.some(d => d.id === value.file!.draft_id && d.content_sha256 === value.file!.evidence.sha256 && d.result_version_id === value.file!.evidence.version_id))) return false;
  return run.file_create.target.classification === value.before.classification;
}

export function makeContractBodyAdopt(value: ContractBodyView, reviewed: boolean, why: string): S['AgentContractBodyAdopt'] {
  if (!reviewed || value.status !== 'file_committed' || !contractBodyProven(value)) throw new Error('请先核对合同原始资料和完整正文文件。');
  return { intent_sha256: value.intent_sha256, version_id: value.file!.evidence.version_id, content_sha256: value.file!.evidence.sha256, reviewed: true, reason: reason(why) };
}

export function contractBodyAdopted(value: ContractBodyView, original: ContractBodyView): boolean {
  return contractBodyProven(value) && value.status === 'adopted' && value.run_id === original.run_id && value.intent_sha256 === original.intent_sha256 && value.file_intent_sha256 === original.file_intent_sha256 && canonical(value.input) === canonical(original.input) && canonical(value.before) === canonical(original.before) && canonical(value.file) === canonical(original.file);
}

export function contractBodyFacts(value: ContractBodyView): [string, string][] {
  const content = value.before.payload.content as Record<string, unknown>;
  return [['合同', value.before.title], ['原始修订', String(value.before.version)], ['币种与金额', [content.currency, content.amount].filter(Boolean).join(' ') || '待明确'], ['起止日期', [content.start_date, content.end_date].map(v => typeof v === 'string' ? v : '待明确').join(' → ')], ['续约依据', typeof content.renewal_terms === 'string' && content.renewal_terms || '待明确'], ['终止依据', typeof content.termination_terms === 'string' && content.termination_terms || '待明确']];
}
