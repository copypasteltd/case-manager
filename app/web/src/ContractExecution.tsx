import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Schemas } from './api';
import { date, Dialog, ErrorNotice, Field } from './components';
import { bytes, FileVersionPicker, useCommandKey, type FileReference } from './BusinessSelectors';
import { DownloadDialog, type DownloadVersion } from './DownloadDialog';
import { MFAConfirmation } from './MFAConfirmation';

type Contract = Schemas['Contract'];
async function refresh(cache: ReturnType<typeof useQueryClient>, id: string) {
  await Promise.all([cache.invalidateQueries({ queryKey: ['contract', id] }), cache.invalidateQueries({ queryKey: ['contracts'] }), cache.invalidateQueries({ queryKey: ['file-retention'] })]);
}
export function ContractExecution({ contract }: { contract: Contract }) {
  const [action, setAction] = useState<'signature' | 'activate' | null>(null), [download, setDownload] = useState<DownloadVersion | null>(null);
  const signature = contract.signature;
  if (contract.lifecycle === 'draft') return null;
  return <section className="panel party-detail approval-panel"><div className="section-heading"><h2>签署与生效依据</h2><span className="muted">按已批准修订登记</span></div>
    {signature ? <><dl className="file-facts"><div><dt>实际签署日期</dt><dd>{date(signature.signed_on)}</dd></div><div><dt>批准的生效日期</dt><dd>{date(signature.effective_on)}</dd></div><div><dt>登记日期</dt><dd>{date(signature.created_at)}</dd></div></dl>
      <div className="party-notes"><h3>签署条款核对说明</h3><p>{signature.attestation}</p><h3>登记依据</h3><p>{signature.reason}</p></div>
      {signature.signed_file ? <><div className="reference-summary"><div><strong>{signature.signed_file.name}</strong><p>固定签署件版本 {signature.signed_file.seq} · {bytes(signature.signed_file.size_bytes)}</p></div></div>{signature.signed_file.scan_state === 'waived' && <p className="notice warning">此签署件依据扫描例外放行，请按公司制度查阅。</p>}<div className="post-actions"><button className="button secondary" onClick={() => { const v = signature.signed_file!; setDownload({ id: v.version_id, seq: v.seq, size_bytes: v.size_bytes, sha256: v.sha256, original_name: v.name, scan_state: v.scan_state }); }}>下载固定签署件</button><Link className="text-button" to={'/files/' + signature.signed_file.file_id}>查看签署文件</Link></div></> : <p className="muted">已固定签署件，当前不可用或无权查阅。</p>}
      {contract.lifecycle === 'pending_signature' && <p className="notice neutral">签署已登记。生效仍须核对批准日期、主合同状态和当前证据，再由有权人员确认。</p>}</> : <p className="muted">批准内容已固定。登记真实签署日期与签署件后，再核对生效条件。</p>}
    <div className="post-actions">{contract.can_register_signature && <button className="button primary" onClick={() => setAction('signature')}>登记已签署合同</button>}{contract.can_activate && <button className="button primary" onClick={() => setAction('activate')}>核对并确认生效</button>}</div>
    {action === 'signature' && <SignatureDialog contract={contract} onClose={() => setAction(null)} />}{action === 'activate' && <ActivationDialog contract={contract} onClose={() => setAction(null)} />}{download && <DownloadDialog version={download} onClose={() => setDownload(null)} />}
  </section>;
}
function SignatureDialog({ contract, onClose }: { contract: Contract; onClose: () => void }) {
  const [base] = useState(contract), [signedOn, setSignedOn] = useState(''), [file, setFile] = useState<FileReference | null>(null), [picker, setPicker] = useState(false), [attestation, setAttestation] = useState(''), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({ mutationFn: () => { if (!file) throw new Error('请先选择签署件的准确正式版本。'); const body: Schemas['ContractSignature'] = { expected_version: base.version, revision_id: base.current_revision_id, content_hash: base.content_hash, signed_on: signedOn, signed_file_version_id: file.version_id, attestation, reason }; return api<Contract>('/contracts/' + base.id + '/signature', { method: 'POST', body, key: key(body) }); }, onSuccess: async () => { await refresh(cache, base.id); onClose(); } });
  const conflict = mutation.error instanceof ApiError && ['VERSION_CONFLICT', 'CONTRACT_SIGNATURE_EXISTS', 'CONTRACT_APPROVED_REVISION_REQUIRED', 'AUTHORIZATION_CHANGED'].includes(mutation.error.code);
  return <Dialog title="登记真实签署依据" onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><h3>{base.title}</h3><p className="form-note">登记对应已批准条款的签署事实。批准的生效日期为 {date(base.start_date)}；此操作不会提前生效。</p><ErrorNotice error={mutation.error} />{conflict && <p className="notice neutral">合同或签署记录已变化。已保留填写内容，请关闭后核对最新依据。</p>}<Field label="实际签署日期"><input type="date" required value={signedOn} onChange={event => setSignedOn(event.target.value)} /></Field>
    <section className="contract-attachment"><h3>签署件版本</h3>{file && <div className="reference-summary"><div><strong>{file.name}</strong><p>固定版本 {file.seq} · {bytes(file.size_bytes)}</p>{file.scan_state === 'waived' && <p className="warning-text">扫描例外放行</p>}</div></div>}<button className="button secondary" type="button" onClick={() => setPicker(true)}>{file ? '重新选择签署件' : '选择签署件版本'}</button></section>
    <Field label="签署条款核对说明" hint="说明签署件与本次准确批准条款的核对结果。"><textarea rows={4} required maxLength={20000} value={attestation} onChange={event => setAttestation(event.target.value)} /></Field><Field label="签署登记依据"><textarea rows={3} required maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={!file || !fresh || conflict || mutation.isPending}>确认保存签署登记</button></div></form>{picker && <FileVersionPicker title="选择签署件正式版本" project={base.project_id} classification={base.classification} onSelect={value => { setFile(value); setPicker(false); }} onClose={() => setPicker(false)} />}</Dialog>;
}
function ActivationDialog({ contract, onClose }: { contract: Contract; onClose: () => void }) {
  const [base] = useState(contract), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false); const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({ mutationFn: () => { if (!base.signature) throw new Error('请先核对已登记签署依据。'); const body: Schemas['ContractActivation'] = { expected_version: base.version, action: 'activate', revision_id: base.current_revision_id, content_hash: base.content_hash, signature_id: base.signature.id, reason }; return api<Contract>('/contracts/' + base.id + '/transition', { method: 'POST', body, key: key(body) }); }, onSuccess: async () => { await refresh(cache, base.id); onClose(); } });
  return <Dialog title="确认合同生效依据" onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><h3>{base.title}</h3><p>实际签署日期 {date(base.signature?.signed_on)} · 批准生效日期 {date(base.signature?.effective_on)}</p><p className="form-note">核对固定签署件和批准条款后，确认该修订进入履行阶段。费用与收付款仍按各自真实凭证独立登记。</p><ErrorNotice error={mutation.error} /><Field label="确认生效依据"><textarea rows={3} required maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button className="button secondary" type="button" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={!fresh || mutation.isPending || !!mutation.error}>确认此批准修订生效</button></div></form></Dialog>;
}
