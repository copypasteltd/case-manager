import type { components } from './api';
import { fileCreateReceipt, fileUpdateReceipt } from './file-update';

type Schemas = components['schemas'];
export type FileRecovery = Schemas['AgentFileRecoveryInput'];
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;
const stopped = new Set(['succeeded', 'failed', 'cancelled']);
const badScan = new Set(['quarantined', 'blocked', 'infected', 'unscannable', 'waived']);

export function fileRecoveryMessage(message: Schemas['Message']): boolean {
  return message.role === 'user' && message.visibility === 'available' && message.source_refs.some(ref => ref.type === 'agent_file_update' || ref.type === 'agent_file_create');
}

export function makeFileRecovery(run: Schemas['Run'], conversation: Schemas['Conversation'], actor: string, draftID: string, reviewed: boolean): FileRecovery {
  const value = run.file_update ?? run.file_create;
  if (!reviewed || conversation.type !== 'private' || conversation.creator_id !== actor || conversation.state !== 'active' || run.conversation_id !== conversation.id || run.actor_id !== actor || !uuid.test(run.id) || !stopped.has(run.state) || !value || !!run.file_update === !!run.file_create || value.run_id !== run.id || value.status !== 'authorized' || value.operation_id !== null || value.result !== null || (value.recovery?.depth ?? 0) >= 8) throw new Error('请选择本人私密会话中已经明确结束、尚未提交的文件草稿，并确认已核对。');
  const draft = value.drafts.find(v => v.id === draftID);
  if (!draft || !uuid.test(draft.id) || !hash.test(draft.content_sha256) || !/^\d+$/.test(draft.size_bytes) || BigInt(draft.size_bytes) < 1n || BigInt(draft.size_bytes) > ('base_version_id' in value ? 1048576n : 49152n) || ['committed', 'blocked'].includes(draft.state) || badScan.has(draft.scan_state) || draft.result_version_id !== null) throw new Error('草稿已提交、被扫描阻断或内容凭据不完整，不能恢复。');
  return { source_run_id: run.id, draft_id: draft.id, content_sha256: draft.content_sha256, reviewed: true };
}

export function fileRecoveryReceipt(run: Schemas['Run'], input?: FileRecovery): boolean {
  const value = run.file_update ?? run.file_create;
  if (!input) return !run.file_update?.recovery && !run.file_create?.recovery;
  const r = value?.recovery;
  if (!input.reviewed || !uuid.test(input.source_run_id) || !uuid.test(input.draft_id) || !hash.test(input.content_sha256) || !value || !!run.file_update === !!run.file_create || !r || r.source_run_id !== input.source_run_id || r.draft_id !== input.draft_id || r.content_sha256 !== input.content_sha256 || !hash.test(r.recovery_sha256) || !Number.isSafeInteger(r.depth) || r.depth < 1 || r.depth > 8 || run.id === r.source_run_id || !Array.isArray(value.drafts) || value.drafts.some(d => d.id === input.draft_id || !uuid.test(d.id) || d.content_sha256 !== input.content_sha256)) return false;
  const target = value.target;
  if (!uuid.test(run.id) || !target || !uuid.test(target.folder_id) || !target.name?.trim() || !['text/plain', 'text/markdown'].includes(target.media_type) || value.drafts.some(d => !/^\d+$/.test(d.size_bytes) || BigInt(d.size_bytes) < 1n || BigInt(d.size_bytes) > (run.file_update ? 1048576n : 49152n))) return false;
  if (run.file_update) {
    const { recovery: _recovery, ...plain } = run.file_update;
    if (!uuid.test(plain.file_id) || !uuid.test(plain.base_version_id) || !Number.isSafeInteger(plain.file_version) || plain.file_version < 1) return false;
    return fileUpdateReceipt({ ...run, file_update: plain }, { file_id: plain.file_id, base_version_id: plain.base_version_id, expected_file_version: plain.file_version });
  }
  const { recovery: _recovery, ...plain } = run.file_create!;
  if (!Number.isSafeInteger(target.folder_version) || target.folder_version! < 1) return false;
  return fileCreateReceipt({ ...run, file_create: plain }, { folder_id: plain.target.folder_id, expected_folder_version: plain.target.folder_version!, name: plain.target.name, media_type: plain.target.media_type });
}
