import type { components } from './api';
type S = components['schemas'];
export type FollowupSummary = S['FollowupSummary'];
export type FollowupItem = S['FollowupItem'];
export const followupKinds = ['requests_to_revise', 'deliverables_to_submit', 'deliverables_to_review'] as const;
export type FollowupKind = typeof followupKinds[number];
export const followupLabels: Record<FollowupKind, string> = {
  requests_to_revise: '我发起的待推进申请',
  deliverables_to_submit: '我负责的待提交交付',
  deliverables_to_review: '我可核对的已提交交付',
};
export const followupHints: Record<FollowupKind, string> = {
  requests_to_revise: '本人发起的草稿或已退回申请。打开详情核对业务依据，再修订或重新送审。',
  deliverables_to_submit: '本人负责、当前可提交的草稿或需修改交付。提交前仍须核对正式附件和验收标准。',
  deliverables_to_review: '本次由其他人提交，且当前具有读取和验收权限的交付。核对固定修订与附件后独立办理。',
};
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
const clock = (v: unknown): v is string => typeof v === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(v) && Number.isFinite(Date.parse(v));
const integer = (v: unknown, min = 0): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= min;
const fields = (v: unknown, names: readonly string[]): boolean => !!v && typeof v === 'object' && !Array.isArray(v) && Object.keys(v).length === names.length && names.every(name => Object.prototype.hasOwnProperty.call(v, name));
const invalid = () => new Error('业务跟进尚未核实，请刷新后重试。');

export function checkedFollowups(value: FollowupSummary): FollowupSummary {
  if (!fields(value, ['as_of', 'scope', 'groups']) || value.scope !== 'my_readable_followups' || !clock(value.as_of) || !Array.isArray(value.groups) || value.groups.length !== followupKinds.length) throw invalid();
  const kinds = new Set(value.groups.map(g => g?.kind));
  if (kinds.size !== followupKinds.length || followupKinds.some(kind => !kinds.has(kind))) throw invalid();
  const deliverables = new Set<string>();
  for (const group of value.groups) {
    if (!fields(group, ['kind', 'count', 'items', 'has_more']) || !integer(group.count) || !Array.isArray(group.items) || group.items.length !== Math.min(group.count, 5) || group.has_more !== (group.count > group.items.length) || new Set(group.items.map(item => typeof item?.id === 'string' ? item.id.toLowerCase() : item?.id)).size !== group.items.length) throw invalid();
    for (const item of group.items) {
      if (!fields(item, ['id', 'version', 'revision_id', 'project_id', 'title', 'state', 'updated_at']) || !uuid(item.id) || !integer(item.version, 1) || item.revision_id !== null && !uuid(item.revision_id) || item.project_id !== null && !uuid(item.project_id) || typeof item.title !== 'string' || !item.title.trim() || [...item.title].length > 200 || !clock(item.updated_at)) throw invalid();
      if (group.kind === 'requests_to_revise') {
        if (!['draft', 'returned'].includes(item.state) || !uuid(item.revision_id)) throw invalid();
      } else {
        if (!uuid(item.project_id) || deliverables.has(item.id.toLowerCase())) throw invalid();
        deliverables.add(item.id.toLowerCase());
        if (group.kind === 'deliverables_to_submit' && !['draft', 'changes_requested'].includes(item.state)) throw invalid();
        if (group.kind === 'deliverables_to_review' && (item.state !== 'submitted' || !uuid(item.revision_id))) throw invalid();
      }
    }
    for (let i = 1; i < group.items.length; i++) {
      const before = group.items[i - 1], after = group.items[i];
      if (Date.parse(before.updated_at) < Date.parse(after.updated_at) || Date.parse(before.updated_at) === Date.parse(after.updated_at) && before.id.toLowerCase() < after.id.toLowerCase()) throw invalid();
    }
  }
  return value;
}

export function followupURL(kind: FollowupKind, item: FollowupItem, client: 'web' | 'mobile'): string {
  if (!followupKinds.includes(kind) || !uuid(item.id) || !['web', 'mobile'].includes(client)) throw invalid();
  if (kind !== 'requests_to_revise' && !uuid(item.project_id)) throw invalid();
  if (client === 'web') return kind === 'requests_to_revise' ? '/requests/' + item.id : '/projects/' + item.project_id + '?tab=deliverables&deliverable=' + item.id;
  return (kind === 'requests_to_revise' ? '/pages/detail/index?kind=request&id=' : '/pages/deliverable/index?id=') + item.id;
}
export function followupState(item: FollowupItem): string {
  return ({ draft: '草稿待推进', returned: '已退回，待修订', changes_requested: '需要修改后重新提交', submitted: '已提交，待独立核对' } as Record<string, string>)[item.state] ?? '请核对';
}
