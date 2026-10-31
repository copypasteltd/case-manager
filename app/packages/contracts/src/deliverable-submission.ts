import type { components } from './api';
import type { Mutation, MutationView } from './mutation';
import { trimMutationText, validMutationText } from './mutation-validation';

const uuid = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v);
const hash = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v);
type Head = components['schemas']['Deliverable'];
export function makeDeliverableSubmission(target: Head, versions: readonly string[], description: string, reason: string, reviewed: boolean): Mutation {
  if (!target.can_submit || target.protocol_version !== 1 || !uuid(target.id) || !Number.isSafeInteger(target.version) || target.version < 1 || !['draft','changes_requested','accepted'].includes(target.state)
    || target.current_revision_id !== null && !uuid(target.current_revision_id) || !reviewed) throw new Error('请核对可提交交付、原修订及本次固定文件；提交后仍须独立人员验收。');
  if (versions.length < 1 || versions.length > 20 || versions.some(v => !uuid(v)) || new Set(versions).size !== versions.length) throw new Error('请选择1至20个不同的正式文件版本。');
  if (!validMutationText(description,20000,true) || !validMutationText(reason,200,true)) throw new Error('请填写有限的交付说明和提交原因。');
  return { tool:'submit_deliverable', arguments:{ id:target.id, expected_version:target.version, expected_revision_id:target.current_revision_id, file_version_ids:[...versions], description:trimMutationText(description), reason:trimMutationText(reason) } };
}

export type SubmissionFile = {file_id:string;version_id:string;name:string;seq:number;sha256:string;size_bytes:string;media_type:string;scan_state:'clean'|'waived';scan_waiver_ref:string|null};
export function submissionFiles(m: MutationView): SubmissionFile[] {
  const rows: unknown = m.before.submission_files, ids = m.arguments.file_version_ids;
  if (m.tool !== 'submit_deliverable' || !Array.isArray(rows) || !Array.isArray(ids) || ids.length < 1 || ids.length > 20 || new Set(ids).size !== ids.length || rows.length !== ids.length) return [];
  const out: SubmissionFile[] = [];
  for (let i=0;i<rows.length;i++) {
    const v = rows[i] as Partial<SubmissionFile> | null;
    if (!v || !uuid(v.file_id) || !uuid(v.version_id) || v.version_id !== ids[i] || !hash(v.sha256) || typeof v.name !== 'string' || typeof v.media_type !== 'string'
      || !Number.isSafeInteger(v.seq) || (v.seq ?? 0)<1 || typeof v.size_bytes !== 'string' || !/^[0-9]+$/.test(v.size_bytes) || !['clean','waived'].includes(v.scan_state ?? '')
      || v.scan_state==='waived' && !uuid(v.scan_waiver_ref) || v.scan_state==='clean' && v.scan_waiver_ref!==null && !uuid(v.scan_waiver_ref)) return [];
    out.push(v as SubmissionFile);
  }
  return out;
}

export function deliverableSubmissionBasis(m: MutationView): boolean {
  const a=m.arguments,b=m.before;
  return m.resource_type==='deliverable' && uuid(b.project_id) && b.protocol_version===1 && ['draft','changes_requested','accepted'].includes(String(b.state))
    && b.current_revision_id===a.expected_revision_id && (a.expected_revision_id===null||uuid(a.expected_revision_id))
    && Object.keys(a).length===6 && validMutationText(a.description,20000,true) && validMutationText(a.reason,200,true)
    && submissionFiles(m).length === a.file_version_ids?.length;
}
export function deliverableSubmissionReceipt(m: MutationView): boolean {
  return deliverableSubmissionBasis(m) && uuid(m.result?.revision_id) && m.result?.revision_id !== m.arguments.expected_revision_id && Object.keys(m.result ?? {}).length===3;
}
