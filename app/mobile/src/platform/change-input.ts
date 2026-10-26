import { sha256 } from '@noble/hashes/sha2.js';
import { utf8Bytes } from './encoding';
import { MobileError, type Schemas } from './session';
import { requestReceipt, uuidPattern } from './business-input';

type Basis = Schemas['ChangeBasis'];
type Change = Schemas['Change'];
type Revision = Schemas['ChangeRevision'];
type Line = Schemas['ProjectBudgetLine'];
export type ChangeFile = Pick<Schemas['BusinessFileReference'], 'file_id' | 'version_id' | 'name' | 'seq' | 'size_bytes' | 'sha256'>;
export type ChangeLineInput = { id: string; category: string; start: string; end: string; amount: string };
// Only editable text belongs in the short-lived draft. Never basis snapshots,
// fixed files, request/implementation receipts, MFA or human attestations.
const limits = { title: 200, category: 200, currency: 3, scope: 20000, scopeDelta: 20000, scheduleDelta: 20000, start: 10, due: 10, budgetMode: 10 };
export type ChangeInput = Record<keyof typeof limits, string> & { lines: ChangeLineInput[] };
const object = (v: unknown): v is Record<string, unknown> => Boolean(v && typeof v === 'object' && !Array.isArray(v));
const uuid = (v: unknown): v is string => typeof v === 'string' && uuidPattern.test(v);
const version = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v > 0;
const digest = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v);
const trim = (v: string) => v.replace(/^[\s\u0085]+|[\s\u0085]+$/g, '');
const text = (v: unknown, limit: number) => typeof v === 'string' && trim(v).length > 0 && v.length <= limit;
const amount = (v: unknown): v is string => typeof v === 'string' && /^(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?$/.test(v);
const normalizedAmount = (v: string) => { const [whole, fraction = ''] = v.split('.'); return whole + '.' + fraction.padEnd(6, '0'); };
const signedAmount = (v: unknown) => typeof v === 'string' && /^-?(0|[1-9][0-9]{0,13})\.[0-9]{6}$/.test(v);
const instant = (v: unknown) => typeof v === 'string' && Number.isFinite(Date.parse(v));
export function changeDate(v: unknown): v is string {
  if (typeof v !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(v) || Number(v.slice(0, 4)) < 1) return false;
  const date = new Date(v + 'T00:00:00Z');
  return Number.isFinite(date.valueOf()) && date.toISOString().slice(0, 10) === v;
}
const nullableDate = (v: unknown) => v === null || changeDate(v);
const linesValid = (v: unknown): v is Line[] => Array.isArray(v) && v.length <= 100 && v.every(l => object(l)
  && text(l.category, 200) && changeDate(l.period_start) && changeDate(l.period_end) && l.period_start <= l.period_end && amount(l.amount));
function canonical(v: unknown): string {
  if (Array.isArray(v)) return '[' + v.map(canonical).join(',') + ']';
  if (object(v)) return '{' + Object.keys(v).sort().map(k => JSON.stringify(k) + ':' + canonical(v[k])).join(',') + '}';
  return JSON.stringify(v);
}
const same = (a: unknown, b: unknown) => canonical(a) === canonical(b);
const lineValues = (lines: Line[]) => lines.map(l => ({ category: l.category, period_start: l.period_start, period_end: l.period_end, amount: normalizedAmount(l.amount) }));
function basisValid(v: unknown): v is Basis {
  return object(v) && uuid(v.project_id) && version(v.project_version) && text(v.name, 200) && typeof v.scope === 'string'
    && nullableDate(v.start_date) && nullableDate(v.due_date) && !(v.start_date && v.due_date && v.start_date > v.due_date)
    && ['internal', 'project_restricted', 'confidential'].includes(v.classification as string)
    && typeof v.currency === 'string' && /^[A-Z]{3}$/.test(v.currency) && (v.budget_id === null || uuid(v.budget_id))
    && linesValid(v.budget_lines) && amount(v.budget_total)
    && (v.budget_id !== null || v.budget_lines.length === 0 && normalizedAmount(v.budget_total) === '0.000000');
}
export function currentChangeBasis(v: unknown, project: Schemas['Project'], currency: string): Basis {
  if (!basisValid(v) || v.project_id !== project.id || v.project_version !== project.version || v.name !== project.name
    || v.scope !== project.scope || v.start_date !== (project.start_date ?? null) || v.due_date !== (project.due_date ?? null)
    || v.classification !== project.classification || v.currency !== currency) {
    throw new MobileError('INVALID_RESPONSE', '项目与预算基线不完整或已变化，请重新读取后明确采用。');
  }
  return v;
}
export function changeBasisKey(basis: Basis) { return Array.from(sha256(utf8Bytes(canonical(basis))), b => b.toString(16).padStart(2, '0')).join(''); }
export function sameChangeBasis(a: Basis, b: Basis) { return same(a, b); }
function contentValid(v: unknown): v is Schemas['ChangeContent'] {
  return object(v) && text(v.title, 200) && text(v.category, 200) && typeof v.currency === 'string' && /^[A-Z]{3}$/.test(v.currency)
    && text(v.scope, 20000) && text(v.scope_delta, 20000) && text(v.schedule_delta, 20000)
    && nullableDate(v.start_date) && nullableDate(v.due_date) && !(v.start_date && v.due_date && v.start_date > v.due_date)
    && (v.budget_lines === null || linesValid(v.budget_lines)) && Array.isArray(v.file_version_ids) && v.file_version_ids.length <= 20
    && v.file_version_ids.every(uuid) && new Set(v.file_version_ids).size === v.file_version_ids.length;
}
function filesValid(files: unknown, ids: string[]): files is Schemas['BusinessFileReference'][] {
  return Array.isArray(files) && files.length === ids.length && files.every((f, i) => object(f) && f.version_id === ids[i]
    && uuid(f.file_id) && version(f.seq) && text(f.name, 1000) && digest(f.sha256) && typeof f.size_bytes === 'string'
    && /^(0|[1-9][0-9]*)$/.test(f.size_bytes) && typeof f.media_type === 'string' && ['clean', 'waived'].includes(f.scan_state as string)
    && (f.scan_waiver_ref === null || typeof f.scan_waiver_ref === 'string'));
}
export function currentChangeRevision(v: unknown, changeID: string): Revision {
  if (!object(v) || !uuid(v.id) || v.change_id !== changeID || !version(v.seq) || v.payload_schema_version !== 1
    || !(v.base_revision_id === null || uuid(v.base_revision_id)) || !contentValid(v.content) || !basisValid(v.basis)
    || v.basis.currency !== v.content.currency || !digest(v.content_hash) || !amount(v.approval_amount) || !signedAmount(v.budget_delta)
    || !(v.target_budget_id === null || uuid(v.target_budget_id)) || (v.content.budget_lines === null) !== (v.target_budget_id === null)
    || !uuid(v.author_id) || !text(v.reason, 200) || !instant(v.created_at) || !filesValid(v.attachments, v.content.file_version_ids)) {
    throw new MobileError('INVALID_RESPONSE', '变更固定修订或文件依据不完整，请重新读取准确资料。');
  }
  return v as Revision;
}
export function currentChange(v: unknown, id: string): Change {
  if (!object(v) || v.id !== id || !uuid(v.id) || !version(v.version) || !uuid(v.project_id) || !uuid(v.creator_id)
    || !uuid(v.current_revision_id) || !(v.approved_revision_id === null || uuid(v.approved_revision_id)) || !(v.request_id === null || uuid(v.request_id))
    || !['draft', 'in_review', 'approved', 'rejected', 'withdrawn', 'implemented'].includes(v.state as string)
    || !['draft', 'requested', 'effective'].includes(v.lifecycle as string)
    || !['not_submitted', 'in_review', 'approved', 'returned', 'rejected', 'withdrawn'].includes(v.approval_state as string)
    || !['can_edit', 'can_prepare_request', 'can_implement'].every(k => typeof v[k] === 'boolean')) {
    throw new MobileError('INVALID_RESPONSE', '变更记录与当前选择不一致或缺少准确状态，请重新读取。');
  }
  const r = currentChangeRevision(v.revision, id);
  if (r.id !== v.current_revision_id || r.basis.project_id !== v.project_id || r.basis.classification !== v.classification
    || (v.state === 'implemented' ? !object(v.implementation) : v.implementation !== null)) throw new MobileError('INVALID_RESPONSE', '当前变更的项目、固定修订或实施事实不一致。');
  return v as Change;
}
export function currentChangePayload(change: Change, revision = change.revision): Schemas['ChangeRequestPayload'] {
  currentChangeRevision(revision, change.id);
  return { business_payload_schema_version: 1, kind: 'change', business_id: change.id, business_revision_id: revision.id,
    business_content_hash: revision.content_hash, project_id: revision.basis.project_id, classification: revision.basis.classification,
    content: revision.content, project_change_basis: revision.basis, project_change_budget_delta: revision.budget_delta,
    amount: revision.approval_amount, attachments: revision.attachments };
}
export function changeRequestProof(v: unknown, change: Change, state: Schemas['Request']['state'], before?: Schemas['Request']): v is Schemas['Request'] {
  return requestReceipt(v, { kind: 'change', businessID: change.id, state, ...(before ? { id: before.id, minimumVersion: before.version + 1, previousRevisionID: before.current_revision_id } : {}), businessRevisionID: change.current_revision_id })
    && same(v.payload, currentChangePayload(change));
}
export function copyChangeInput(v: unknown): ChangeInput | undefined {
  if (!object(v)) return;
  const out = {} as ChangeInput;
  for (const k of Object.keys(limits) as (keyof typeof limits)[]) { const value = v[k]; if (typeof value !== 'string' || value.length > limits[k]) return; out[k] = value; }
  if (!['keep', 'replace'].includes(out.budgetMode) || !Array.isArray(v.lines) || v.lines.length > 100) return;
  out.lines = [];
  for (const l of v.lines) {
    if (!object(l) || typeof l.id !== 'string' || !text(l.id, 36) || typeof l.category !== 'string' || l.category.length > 200
      || typeof l.start !== 'string' || l.start.length > 10 || typeof l.end !== 'string' || l.end.length > 10
      || typeof l.amount !== 'string' || l.amount.length > 30) return;
    out.lines.push({ id: l.id, category: l.category, start: l.start, end: l.end, amount: l.amount });
  }
  if (new Set(out.lines.map(l => l.id)).size !== out.lines.length) return;
  return out;
}
export const changeInputCharacters = (v: ChangeInput) => Object.entries(v).reduce((sum, [k, value]) => sum + (k === 'lines' ? (value as ChangeLineInput[]).reduce((n, l) => n + Object.values(l).join('').length, 0) : (value as string).length), 0);
export function currentChangeInput(basis: Basis, revision?: Revision): ChangeInput {
  const c = revision?.content;
  return { title: c?.title ?? '', category: c?.category ?? '', currency: basis.currency, scope: c?.scope ?? basis.scope,
    scopeDelta: c?.scope_delta ?? '', scheduleDelta: c?.schedule_delta ?? '', start: c?.start_date ?? (revision ? '' : basis.start_date ?? ''),
    due: c?.due_date ?? (revision ? '' : basis.due_date ?? ''), budgetMode: c?.budget_lines === null || !revision ? 'keep' : 'replace',
    lines: (c?.budget_lines ?? basis.budget_lines).map((l, i) => ({ id: 'line-' + i, category: l.category, start: l.period_start, end: l.period_end, amount: l.amount })) };
}
export function changeWriter(project: Schemas['Project'], members: Schemas['ProjectMembers'], me: Schemas['Me'], action: string) {
  return ['draft', 'active', 'on_hold'].includes(project.state) && members.memberships?.find(m => m.user_id === me.user.id)?.project_role !== 'viewer'
    && me.actions.includes(action) && me.actions.includes('budget.read');
}
export function changePayload(input: ChangeInput, basis: Basis, project: Schemas['Project'], files: ChangeFile[], reason: string, accepted: boolean, before?: Change): Schemas['ChangeCreate'] | Schemas['ChangePatch'] {
  currentChangeBasis(basis, project, input.currency);
  if (!copyChangeInput(input) || !accepted || !['draft', 'active', 'on_hold'].includes(project.state)) throw new MobileError('INVALID_INPUT', '请先明确采用当前完整项目与预算基线。');
  if (!text(reason, 200) || !text(input.title, 200) || !text(input.category, 200) || !text(input.scope, 20000)
    || !text(input.scopeDelta, 20000) || !text(input.scheduleDelta, 20000)) throw new MobileError('INVALID_INPUT', '请完整填写变更名称、类别、范围、两项影响说明和200字以内原因。');
  if (!(input.start === '' || changeDate(input.start)) || !(input.due === '' || changeDate(input.due)) || input.start && input.due && input.start > input.due) throw new MobileError('INVALID_INPUT', '请填写有效起止日期，或明确留空；截止日期不能早于开始日期。');
  if (before && (!before.can_edit || before.project_id !== project.id || before.revision.content.currency !== input.currency)) throw new MobileError('INVALID_INPUT', '当前修订不可编辑；币种固定，其他币种请另建提案。');
  if (files.length > 20 || files.some(f => !uuid(f.version_id) || !uuid(f.file_id)) || new Set(files.map(f => f.version_id)).size !== files.length) throw new MobileError('INVALID_INPUT', '请选择最多20个不同的实际正式文件版本。');
  const lines: Line[] = input.lines.map(l => ({ category: trim(l.category), period_start: l.start, period_end: l.end, amount: l.amount }));
  if (input.budgetMode === 'replace') {
    if (!linesValid(lines)) throw new MobileError('INVALID_INPUT', '预算明细须有有效类别、日期区间及非负十进制金额，最多100项、14位整数和6位小数；币种精度由服务器复核。');
    for (let i = 0; i < lines.length; i++) for (let j = i + 1; j < lines.length; j++) {
      const a = lines[i], b = lines[j];
      if (a.category === b.category && a.period_start <= b.period_end && b.period_start <= a.period_end) throw new MobileError('INVALID_INPUT', '同类别预算期间不能重叠，首尾同日也视为重叠。');
    }
  }
  return { content: { title: trim(input.title), category: trim(input.category), currency: input.currency, scope: trim(input.scope),
    scope_delta: trim(input.scopeDelta), schedule_delta: trim(input.scheduleDelta), start_date: input.start || null, due_date: input.due || null,
    budget_lines: input.budgetMode === 'keep' ? null : lines, file_version_ids: files.map(f => f.version_id) },
    base_project_version: basis.project_version, base_budget_id: basis.budget_id, reason: trim(reason),
    ...(before ? { expected_version: before.version, base_revision_id: before.current_revision_id } : {}) };
}
export function changeReceipt(v: unknown, before?: Change): v is Schemas['ChangeReceipt'] {
  return object(v) && uuid(v.id) && uuid(v.revision_id) && v.version === (before ? before.version + 1 : 1)
    && (!before || v.id === before.id && v.revision_id !== before.current_revision_id);
}
export function savedChangeProof(actual: Change, receipt: Schemas['ChangeReceipt'], payload: Schemas['ChangeCreate'] | Schemas['ChangePatch'], basis: Basis, files: ChangeFile[], actor: string, before?: Change) {
  currentChange(actual, receipt.id);
  const r = actual.revision, expected = { ...payload.content, budget_lines: payload.content.budget_lines === null ? null : lineValues(payload.content.budget_lines) };
  return changeReceipt(receipt, before) && actual.version === receipt.version && actual.current_revision_id === receipt.revision_id
    && actual.project_id === basis.project_id && actual.classification === basis.classification && actual.lifecycle === 'draft'
    && r.seq === (before ? before.revision.seq + 1 : 1) && r.base_revision_id === (before?.current_revision_id ?? null)
    && r.author_id === actor && r.reason === payload.reason && same(r.content, expected) && sameChangeBasis(r.basis, basis)
    && files.every((file, index) => Object.keys(file).every(key => r.attachments[index]?.[key as keyof ChangeFile] === file[key as keyof ChangeFile]));
}
export function implementPayload(change: Change, project: Schemas['Project'], basis: Basis, request: Schemas['Request'], reason: string, scopeChecked: boolean, budgetChecked: boolean, fresh: boolean): Schemas['ChangeImplement'] {
  currentChange(change, change.id); currentChangeBasis(basis, project, change.revision.content.currency);
  if (!change.can_implement || change.state !== 'approved' || change.lifecycle !== 'requested' || change.approved_revision_id !== change.current_revision_id
    || change.request_id !== request.id || !changeRequestProof(request, change, 'approved') || !sameChangeBasis(change.revision.basis, basis)) throw new MobileError('INVALID_INPUT', '批准修订、原申请或当前项目与预算依据已变化，请重新核对并按原流程处理。');
  if (!scopeChecked || !budgetChecked || !fresh || !text(reason, 200)) throw new MobileError('INVALID_INPUT', '请分别核实范围日期与预算影响、填写200字以内原因，并完成本次两步验证。');
  return { expected_version: change.version, revision_id: change.current_revision_id, content_hash: change.revision.content_hash,
    base_project_version: basis.project_version, request_revision_id: request.current_revision_id,
    scope_verified: true, budget_verified: true, reason: trim(reason) };
}
export function changeBudgetProof(v: unknown, change: Change, state: 'draft' | 'approved'): v is Schemas['ProjectBudget'] {
  const r = change.revision;
  return object(v) && r.target_budget_id !== null && v.id === r.target_budget_id && v.change_id === change.id && v.change_revision_id === r.id
    && v.project_id === change.project_id && v.currency === r.content.currency && v.state === state && version(v.revision)
    && v.predecessor_id === r.basis.budget_id && linesValid(v.lines) && same(lineValues(v.lines), lineValues(r.content.budget_lines!)) && amount(v.total);
}
export function implementationProof(receipt: unknown, head: Change, project: Schemas['Project'], basis: Basis, before: Change, oldProject: Schemas['Project'], input: Schemas['ChangeImplement'], actor: string, budget?: Schemas['ProjectBudget'], predecessor?: Schemas['ProjectBudget']) {
  currentChange(head, before.id); currentChangeBasis(basis, project, before.revision.content.currency);
  const r = before.revision, fact = head.implementation;
  return object(receipt) && receipt.id === before.id && receipt.revision_id === r.id && receipt.version === before.version + 1
    && head.version === receipt.version && head.current_revision_id === r.id && same(head.revision, r)
    && head.state === 'implemented' && head.lifecycle === 'effective' && head.approval_state === 'approved' && Boolean(fact)
    && uuid(fact!.id) && fact!.change_id === before.id && fact!.revision_id === r.id && fact!.request_revision_id === input.request_revision_id
    && fact!.project_id === oldProject.id && fact!.actor_id === actor && fact!.change_version === head.version
    && fact!.before_project_version === oldProject.version && fact!.after_project_version === project.version
    && fact!.scope_verified === true && fact!.budget_verified === true && fact!.reason === input.reason
    && instant(fact!.created_at) && instant(fact!.mfa_verified_at)
    && project.id === oldProject.id && project.version === oldProject.version + 1 && project.state === oldProject.state
    && project.name === oldProject.name && project.owner_id === oldProject.owner_id && project.classification === oldProject.classification
    && project.scope === r.content.scope && (project.start_date ?? null) === r.content.start_date && (project.due_date ?? null) === r.content.due_date
    && basis.budget_id === (r.target_budget_id ?? r.basis.budget_id)
    && same(lineValues(basis.budget_lines), lineValues(r.content.budget_lines ?? r.basis.budget_lines))
    && (r.content.budget_lines === null ? normalizedAmount(basis.budget_total) === normalizedAmount(r.basis.budget_total)
      : changeBudgetProof(budget, before, 'approved') && normalizedAmount(basis.budget_total) === normalizedAmount(budget.total)
        && (r.basis.budget_id === null || Boolean(predecessor && predecessor.id === r.basis.budget_id && predecessor.state === 'superseded'
          && predecessor.project_id === project.id && predecessor.currency === basis.currency && linesValid(predecessor.lines)
          && same(lineValues(predecessor.lines), lineValues(r.basis.budget_lines)) && normalizedAmount(predecessor.total) === normalizedAmount(r.basis.budget_total))));
}
