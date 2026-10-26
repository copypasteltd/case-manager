import type { components } from './api';

type Schemas = components['schemas'];
export type FileUpdate = Schemas['AgentFileUpdateInput'];
export type FileUpdateView = Schemas['AgentFileUpdateView'];
export type FileCreate = Schemas['AgentFileCreateInput'];
export type FileCreateView = Schemas['AgentFileCreateView'];
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;

export function makeFileUpdate(file: Schemas['File'], version: Schemas['FileVersion'], project: string | null): FileUpdate {
  if (file.project_id !== project || file.state !== 'active' || file.current_version_id !== version.id || version.file_id !== file.id || !uuid.test(file.id) || !uuid.test(version.id) || !Number.isSafeInteger(file.version) || file.version < 1) throw new Error('请选择当前会话范围内的准确现行文件版本。');
  if (version.scan_state !== 'clean' || !['text/plain', 'text/markdown'].includes(version.media_type) || !/^\d+$/.test(version.size_bytes) || BigInt(version.size_bytes) < 1n || BigInt(version.size_bytes) > 1048576n) throw new Error('秘书编辑支持已扫描通过、1字节至1MiB的UTF-8文本或Markdown。');
  return { file_id: file.id, base_version_id: version.id, expected_file_version: file.version };
}

export function fileUpdateReceipt(run: Schemas['Run'], input?: FileUpdate): boolean {
  const value = run.file_update;
  if (!input) return !value;
  if (!value || value.run_id !== run.id || value.file_id !== input.file_id || value.base_version_id !== input.base_version_id || value.file_version !== input.expected_file_version || !hash.test(value.intent_sha256) || !value.target?.name || !Array.isArray(value.drafts) || value.drafts.length > 3) return false;
  if (value.status === 'authorized') return value.operation_id === null && value.result === null;
  const result = value.result, version = result?.file_version;
  return value.status === 'committed' && !!value.operation_id && uuid.test(value.operation_id) && result?.committed === true && result.file.id === input.file_id && result.file.version === input.expected_file_version + 1 && result.file.current_version_id === version?.id && version.file_id === input.file_id && version.base_version_id === input.base_version_id && version.source === 'agent' && version.scan_state === 'clean' && value.drafts.some(draft => draft.result_version_id === version.id && draft.content_sha256 === version.sha256 && !!draft.plan_hash && hash.test(draft.plan_hash));
}

export function makeFileCreate(folder: Schemas['Folder'], name: string, media: 'text/plain' | 'text/markdown', project: string | null): FileCreate {
  if (folder.project_id !== project || folder.state !== 'active' || !uuid.test(folder.id) || !Number.isSafeInteger(folder.version) || folder.version < 1) throw new Error('请选择当前会话范围内的有效目标目录。');
  if (!name.trim() || name !== name.trim() || /[\\/\u0000-\u001f]/.test(name) || !(media === 'text/plain' ? /\.txt$/i : /\.(md|markdown)$/i).test(name)) throw new Error('请填写与格式一致的.txt或.md／.markdown文件名。');
  return { folder_id: folder.id, expected_folder_version: folder.version, name, media_type: media };
}

export function fileCreateReceipt(run: Schemas['Run'], input?: FileCreate): boolean {
  const value = run.file_create;
  if (!input) return !value;
  if (!value || run.file_update || value.run_id !== run.id || !hash.test(value.intent_sha256) || value.target.folder_id !== input.folder_id || value.target.folder_version !== input.expected_folder_version || value.target.name !== input.name || value.target.media_type !== input.media_type || !Array.isArray(value.drafts) || value.drafts.length > 3) return false;
  if (value.status === 'authorized') return value.operation_id === null && value.result === null;
  const result = value.result, version = result?.file_version, file = result?.file;
  return value.status === 'committed' && !!value.operation_id && uuid.test(value.operation_id) && result?.committed === true && !!file && uuid.test(file.id) && file.folder_id === input.folder_id && file.name === input.name && file.classification === value.target.classification && file.current_version_id === version?.id && version.file_id === file.id && version.seq === 1 && version.base_version_id === null && version.media_type === input.media_type && version.source === 'agent' && version.scan_state === 'clean' && value.drafts.some(draft => draft.result_version_id === version.id && draft.content_sha256 === version.sha256 && draft.size_bytes === version.size_bytes && !!draft.plan_hash && hash.test(draft.plan_hash));
}

export function fileDraftState(state: string): string {
  const states: Record<string, string> = { created: '草稿上传尚未确认', uploading: '上传中', finalizing: '上传完成，等待校验', verifying: '校验中', scanning: '扫描中', promoting: '准备正式候选', ready: '候选已就绪', committed: '正式版本已提交', blocked: '草稿被阻断', failed: '草稿未完成', expired: '草稿已失效', aborted: '草稿已终止' };
  return states[state] ?? '状态待核对';
}

export function fileScanState(state: string): string {
  const states: Record<string, string> = { pending: '等待扫描', clean: '扫描通过', blocked: '扫描阻断', unscannable: '暂无法扫描', infected: '检出风险', failed: '扫描失败', unknown: '扫描尚未确认', waived: '人工豁免不能用于秘书提交' };
  return states[state] ?? '扫描状态待核对';
}
