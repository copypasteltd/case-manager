import { useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowUpRight, FileText } from 'lucide-react';
import { type Schemas } from './api';
import { PaymentScheduleFacts } from './ContractPayments';
import { date } from './components';
import { bytes } from './BusinessSelectors';
import { DownloadDialog, type DownloadVersion } from './DownloadDialog';
type Contract = Schemas['Contract'];
export type Content = Pick<Contract, 'title' | 'direction' | 'type' | 'category' | 'counterparty_id' | 'counterparty_revision_id' | 'owner_id' | 'currency' | 'amount' | 'tax_amount' | 'start_date' | 'end_date' | 'renewal_terms' | 'termination_terms' | 'notes' | 'draft_file_version_id' | 'payment_schedule'>;
export type DisplayContent = Content & Pick<Contract, 'counterparty' | 'draft_file'>;
export const lifecycleNames: Record<string, string> = { draft: '草稿', pending_signature: '待签署', effective: '履行中', completed: '已完成', terminated: '已终止' };
export const approvalNames: Record<string, string> = { draft: '申请草稿', not_submitted: '未送审', in_review: '审批中', approved: '已批准', returned: '已退回', rejected: '已拒绝', withdrawn: '已撤回' };
export const directionNames = { receivable: '收款合同', payable: '付款合同', none: '无收付义务' };
export const money = (value: string | null | undefined, currency: string | null | undefined) => value == null ? '未填写' : (currency ?? '') + ' ' + (value.includes('.') ? value.replace(/0+$/, '').replace(/\.$/, '') : value);

const display = (value?: string | null) => value || '未填写';
export function ApprovalBadge({ value }: { value: string }) { return <span className={'badge ' + (value === 'approved' ? 'success' : value === 'in_review' ? 'info' : value === 'rejected' || value === 'returned' ? 'warning' : 'neutral')}>{approvalNames[value] ?? value}</span>; }
export function ContractFacts({ value }: { value: DisplayContent }) {
  const [download, setDownload] = useState<DownloadVersion | null>(null);
  return <><dl className="file-facts"><div><dt>收付方向</dt><dd>{value.direction ? directionNames[value.direction] : '未填写'}</dd></div><div><dt>合同类型</dt><dd>{display(value.type)}</dd></div><div><dt>业务类别</dt><dd>{display(value.category)}</dd></div><div><dt>合同总额</dt><dd>{money(value.amount, value.currency)}</dd></div><div><dt>其中税额</dt><dd>{money(value.tax_amount, value.currency)}</dd></div><div><dt>负责人</dt><dd>{value.owner_id ? '已指定负责人' : '未填写'}</dd></div><div><dt>起始日期</dt><dd>{date(value.start_date)}</dd></div><div><dt>结束日期</dt><dd>{date(value.end_date)}</dd></div><div><dt>往来方依据</dt><dd>{value.counterparty ? <><span>{value.counterparty.name}</span><small>已固定资料修订</small></> : value.counterparty_id ? '已引用资料，当前不可查阅' : '未选择'}</dd></div></dl>
    <section className="contract-attachment"><h3>正文依据</h3>{value.draft_file ? <><div className="reference-summary"><FileText size={20} /><div><strong>{value.draft_file.name}</strong><p>固定版本 {value.draft_file.seq} · {bytes(value.draft_file.size_bytes)}</p></div></div>{value.draft_file.scan_state === 'waived' && <p className="notice warning">此正文依扫描例外放行，请按公司约定查阅。</p>}<div className="post-actions"><button className="button secondary" type="button" onClick={() => { const file = value.draft_file!; setDownload({ id: file.version_id, seq: file.seq, sha256: file.sha256, size_bytes: file.size_bytes, original_name: file.name, scan_state: file.scan_state }); }}>下载固定正文</button><Link className="text-button" to={'/files/' + value.draft_file.file_id}>查看文件记录<ArrowUpRight size={16} /></Link></div></> : <p className="muted">{value.draft_file_version_id ? '已引用正文版本，当前不可用或无权查阅。' : '尚未选择合同正文。'}</p>}</section>
    <PaymentScheduleFacts schedule={value.payment_schedule} currency={value.currency} />
    {(['renewal_terms', 'termination_terms', 'notes'] as const).map((field, index) => value[field] && <section className="party-notes" key={field}><h3>{['续约条款', '终止条款', '补充说明'][index]}</h3><p>{value[field]}</p></section>)}{download && <DownloadDialog version={download} onClose={() => setDownload(null)} />}</>;
}
