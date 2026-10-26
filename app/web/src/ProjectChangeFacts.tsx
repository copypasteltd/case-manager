import './project-changes.css';
import { useState } from 'react';
import { Link } from 'react-router-dom';
import { type Schemas } from './api';
import { bytes } from './BusinessSelectors';
import { DownloadDialog, type DownloadVersion } from './DownloadDialog';

type Content = Schemas['ChangeContent'];
type Basis = Schemas['ChangeBasis'];
export function BudgetLines({ lines, currency, empty = '没有预算明细。' }: { lines: Schemas['ProjectBudgetLine'][]; currency: string; empty?: string }) {
  return lines.length ? <div className="change-budget-lines">{lines.map((line, index) => <dl className="file-facts change-budget-line" key={index}><div><dt>类别</dt><dd>{line.category}</dd></div><div><dt>期间</dt><dd>{line.period_start} — {line.period_end}</dd></div><div><dt>原币预算</dt><dd className="change-money">{currency} {line.amount}</dd></div></dl>)}</div> : <p className="muted">{empty}</p>;
}
export function ProjectChangeFacts({ content, basis, amount, files, delta }: { content: Content; basis: Basis; amount: string; files: Schemas['BusinessFileReference'][]; delta: string }) {
  const [download, setDownload] = useState<DownloadVersion | null>(null);
  return <div className="project-change-facts"><h3>固定项目基线</h3><dl className="file-facts"><div><dt>项目</dt><dd>{basis.name}</dd></div><div><dt>项目版本</dt><dd>{basis.project_version}</dd></div><div><dt>业务类别</dt><dd>{content.category}</dd></div><div><dt>原预算</dt><dd>{basis.currency} {basis.budget_total} · {basis.budget_id ?? '尚无该币种的已实施预算'}</dd></div></dl>
    <div className="change-comparison"><section><h4>原项目基线</h4><p className="completion-prose">{basis.scope || '未填写项目范围'}</p><p>开始 {basis.start_date ?? '未设置'} · 截止 {basis.due_date ?? '未设置'}</p></section><section><h4>本次提案</h4><p className="completion-prose">{content.scope}</p><p>开始 {content.start_date ?? '未设置'} · 截止 {content.due_date ?? '未设置'}</p></section></div>
    <h4>范围影响说明</h4><p className="completion-prose">{content.scope_delta}</p><h4>时间影响说明</h4><p className="completion-prose">{content.schedule_delta}</p>
    <h3>原币预算对照</h3><div className="change-comparison"><section><h4>原预算明细 · {basis.currency}</h4><BudgetLines lines={basis.budget_lines} currency={basis.currency} /></section><section><h4>本次建议预算 · {content.currency}</h4>{content.budget_lines === null ? <p className="notice neutral">保持该币种当前预算。</p> : <BudgetLines lines={content.budget_lines} currency={content.currency} empty="本次将该币种预算明细清空。" />}</section></div>
    <dl className="file-facts"><div><dt>预算净变化</dt><dd className="change-money">{content.currency} {delta}</dd></div><div><dt>用于选择审批规则的增加合计</dt><dd className="change-money">{content.currency} {amount}</dd></div></dl><p className="muted">各明细的减少不会抵消其他明细的增加。预算批准与实施不会产生实际费用或付款。</p>
    <h3>固定依据文件</h3>{!files.length ? <p className="notice neutral">尚未固定依据文件；正式送审需至少一个正式文件版本。</p> : <div className="reference-options">{files.map(file => <div className="reference-option" key={file.version_id}><strong>{file.name}</strong><span>固定版本 {file.seq} · {bytes(file.size_bytes)} · {file.scan_state === 'waived' ? '含扫描豁免风险' : '扫描通过'}</span><code>{file.sha256}</code><div className="inline-actions"><button className="text-button" type="button" onClick={() => setDownload({ id: file.version_id, seq: file.seq, size_bytes: file.size_bytes, sha256: file.sha256, original_name: file.name, scan_state: file.scan_state })}>下载固定版本</button><Link className="text-button" to={'/files?file=' + file.file_id}>查看文件记录</Link></div></div>)}</div>}
    {download && <DownloadDialog version={download} onClose={() => setDownload(null)} />}
  </div>;
}
