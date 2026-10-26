import type { Schemas } from './session';

export type IncidentInput = {
  title: string; description: string; owner: string; severity: Schemas['Incident']['severity']; dedup: string;
  state: Schemas['Incident']['state']; note: string; resolution: string; postmortem: string;
};
const limits = { title: 200, description: 10000, owner: 36, severity: 8, dedup: 200, state: 13, note: 10000, resolution: 10000, postmortem: 10000 };
const uuid = /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
export function copyIncidentInput(input: unknown): IncidentInput | undefined {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return;
  const value = input as Record<string, unknown>, copy: Record<string, string> = {};
  for (const [key, max] of Object.entries(limits)) { const text = value[key]; if (typeof text !== 'string' || [...text].length > max) return; copy[key] = text; }
  if (!['low', 'medium', 'high', 'critical'].includes(copy.severity) || !['open', 'investigating', 'monitoring', 'resolved'].includes(copy.state)) return;
  return copy as IncidentInput;
}
export function incidentInputCharacters(input: IncidentInput) { return Object.values(input).reduce((sum, value) => sum + value.length, 0); }
export function incidentIssues(input: IncidentInput, reason: string, services: string[], current?: Schemas['Incident']) {
  const issues: { path: string; message: string }[] = [];
  const required = (path: string, value: string, max: number, label: string) => { if (!value.trim() || [...value].length > max) issues.push({ path, message: label + '必填且不能超过' + max + '字。' }); };
  required('note', input.note, 10000, '时间线说明'); required('reason', reason, 200, '操作依据');
  if (!copyIncidentInput(input)) issues.push({ path: 'input', message: '事件输入超出允许范围，请核对字段长度与选项。' });
  if (!current) {
    required('title', input.title, 200, '事件标题'); required('dedup', input.dedup, 200, '本次登记标识');
    if (!uuid.test(input.owner)) issues.push({ path: 'owner', message: '请选择当前有效的事件负责人。' });
    if (services.length < 1 || services.length > 20 || new Set(services).size !== services.length || services.some(id => !uuid.test(id))) issues.push({ path: 'services', message: '请从当前有权处理的服务中选择1—20项，不能重复。' });
  }
  if (input.state === 'resolved' && !input.resolution.trim()) issues.push({ path: 'resolution', message: '解决事件前请填写实际恢复与解决说明。' });
  if (current?.state === 'resolved' && input.state !== 'resolved') issues.push({ path: 'state', message: '已解决事件只能补充记录与复盘，不能重新打开。' });
  return issues.map(issue => ({ ...issue, label: issue.path }));
}
export function incidentCreate(input: IncidentInput, reason: string, serviceIDs: string[]): Schemas['IncidentCreate'] {
  return { title: input.title, description: input.description, owner_id: input.owner, severity: input.severity, dedup_key: input.dedup, service_ids: [...serviceIDs], timeline_note: input.note, reason };
}
export function incidentPatch(input: IncidentInput, reason: string, version: number): Schemas['IncidentPatch'] {
  return { expected_version: version, state: input.state, timeline_note: input.note, resolution: input.resolution, postmortem: input.postmortem, reason };
}
export function incidentReceipt(value: Schemas['Incident'], intent: Schemas['IncidentCreate'] | Schemas['IncidentPatch'], current?: Schemas['Incident']) {
  if (!value || !uuid.test(value.id) || !Number.isSafeInteger(value.version) || value.version < 1 || !['open', 'investigating', 'monitoring', 'resolved'].includes(value.state)) return false;
  if ('expected_version' in intent) return !!current && value.id === current.id && value.version === current.version + 1 && value.state === intent.state && value.resolution === intent.resolution && value.postmortem === intent.postmortem;
  return value.dedup_key === intent.dedup_key && value.title === intent.title && value.owner_id === intent.owner_id && Array.isArray(value.service_ids) && JSON.stringify([...value.service_ids].sort()) === JSON.stringify([...intent.service_ids].sort());
}
