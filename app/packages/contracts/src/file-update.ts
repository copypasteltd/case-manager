import type { components } from './api';

type Schemas = components['schemas'];
export type FileUpdate = Schemas['AgentFileUpdateInput'];
export type FileUpdateView = Schemas['AgentFileUpdateView'];
export type FileCreate = Schemas['AgentFileCreateInput'];
export type FileCreateView = Schemas['AgentFileCreateView'];
export const fileCreateFormats = [
  ['text/markdown', 'Markdown（.md／.markdown）', /\.(md|markdown)$/i],
  ['text/plain', '文本（.txt）', /\.txt$/i],
  ['application/vnd.openxmlformats-officedocument.wordprocessingml.document', 'Word（.docx）', /\.docx$/i],
  ['application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 'Excel（.xlsx）', /\.xlsx$/i],
  ['application/vnd.openxmlformats-officedocument.presentationml.presentation', 'PowerPoint（.pptx）', /\.pptx$/i],
] as const;
export type FileCreateMedia = FileCreate['media_type'];
export function officeCreateKind(media: string): 'docx' | 'xlsx' | 'pptx' | null {
  const at = fileCreateFormats.findIndex(row => row[0] === media);
  return at < 2 ? null : (['docx', 'xlsx', 'pptx'] as const)[at - 2] ?? null;
}
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;

export function makeFileUpdate(file: Schemas['File'], version: Schemas['FileVersion'], project: string | null): FileUpdate {
  if (file.project_id !== project || file.state !== 'active' || file.current_version_id !== version.id || version.file_id !== file.id || !uuid.test(file.id) || !uuid.test(version.id) || !Number.isSafeInteger(file.version) || file.version < 1) throw new Error('请选择当前会话范围内的准确现行文件版本。');
  if (version.scan_state !== 'clean' || !fileCreateFormats.some(([media]) => media === version.media_type) || !/^\d+$/.test(version.size_bytes) || BigInt(version.size_bytes) < 1n || BigInt(version.size_bytes) > 1048576n) throw new Error('秘书编辑支持已扫描通过、1字节至1MiB的文本、Markdown、DOCX、XLSX或PPTX。');
  return { file_id: file.id, base_version_id: version.id, expected_file_version: file.version };
}

export function fileUpdateReceipt(run: Schemas['Run'], input?: FileUpdate): boolean {
  const value = run.file_update;
  if (!input) return !value;
  if (!value || value.recovery || value.run_id !== run.id || value.file_id !== input.file_id || value.base_version_id !== input.base_version_id || value.file_version !== input.expected_file_version || !hash.test(value.intent_sha256) || !value.target?.name || !Array.isArray(value.drafts) || value.drafts.length > 3) return false;
  if (value.status === 'authorized') return value.operation_id === null && value.result === null;
  const result = value.result, version = result?.file_version;
  return value.status === 'committed' && !!value.operation_id && uuid.test(value.operation_id) && result?.committed === true && result.file.id === input.file_id && result.file.version === input.expected_file_version + 1 && result.file.current_version_id === version?.id && version.file_id === input.file_id && version.base_version_id === input.base_version_id && version.source === 'agent' && version.scan_state === 'clean' && value.drafts.some(draft => draft.result_version_id === version.id && draft.content_sha256 === version.sha256 && !!draft.plan_hash && hash.test(draft.plan_hash));
}

export function makeFileCreate(folder: Schemas['Folder'], name: string, media: FileCreateMedia, project: string | null): FileCreate {
  if (folder.project_id !== project || folder.state !== 'active' || !uuid.test(folder.id) || !Number.isSafeInteger(folder.version) || folder.version < 1) throw new Error('请选择当前会话范围内的有效目标目录。');
  if (!name.trim() || name !== name.trim() || /[\\/\u0000-\u001f]/.test(name) || !fileCreateFormats.some(row => row[0] === media && row[2].test(name))) throw new Error('请填写与所选TXT、Markdown、Word、Excel或PowerPoint格式一致的文件名。');
  return { folder_id: folder.id, expected_folder_version: folder.version, name, media_type: media };
}

export function fileCreateReceipt(run: Schemas['Run'], input?: FileCreate): boolean {
  const value = run.file_create;
  if (!input) return !value;
  if (!value || value.recovery || run.file_update || value.run_id !== run.id || !hash.test(value.intent_sha256) || value.target.folder_id !== input.folder_id || value.target.folder_version !== input.expected_folder_version || value.target.name !== input.name || value.target.media_type !== input.media_type || !Array.isArray(value.drafts) || value.drafts.length > 3) return false;
  const officeKind = officeCreateKind(input.media_type);
  if (officeKind && value.drafts.some(draft => {
    const office = draft.diff?.office;
    return !office || office.adapter !== 'agent-office-create-v1' || office.kind !== officeKind || !hash.test(office.template_sha256) || !hash.test(office.workspace_sha256) || !Number.isSafeInteger(office.unit_count) || office.unit_count < 0 || office.unit_count > 2000 || !Array.isArray(office.warnings) || office.warnings.length > 20 || office.warnings.some(w => typeof w !== 'string' || w.length > 2000) || !/^\d+$/.test(draft.size_bytes) || BigInt(draft.size_bytes) < 1n || BigInt(draft.size_bytes) > 1048576n;
  })) return false;
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

export function fileAuthorizationMessage(state: string | undefined): string {
  if (state === 'current') return '本次文件授权绑定原登录；退出或登录到期后，后续处理会停止。已提交文件与历史回执继续保留。';
  if (state === 'changed') return '原授权登录已失效，后续文件处理已停止。请重新登录后明确发起新请求，或选择准确草稿恢复；历史提交不会撤销。';
  if (state === 'legacy_unbound') return '这份历史请求未保存原授权登录，禁止继续执行；历史回执仍可查阅，继续处理须明确建立新授权。';
  return '文件授权来源尚未核实，请重新读取实际结果；请勿据此继续执行。';
}
