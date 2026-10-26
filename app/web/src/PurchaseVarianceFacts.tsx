import { Link } from 'react-router-dom';
import { type Schemas } from './api';
import { money } from './ContractDisplay';
import { SpendFacts, type SpendFactsValue } from './SpendDisplay';

export function PurchaseVarianceFacts({ value }: { value: SpendFactsValue & { basis: Schemas['PurchaseVarianceBasis'] } }) {
  const basis = value.basis;
  return <>
    <p className="notice neutral">此提案固定采购版本 {basis.purchase_version} 的执行依据与履行集合。差异获批后，仍须由获授权人员明确启用。</p>
    <dl className="file-facts"><div><dt>原采购</dt><dd><Link className="text-button" to={'/purchases/' + basis.purchase_id}>查阅原批准与履行记录 ↗</Link></dd></div><div><dt>本次依据</dt><dd>{basis.execution_plan_revision_id ? '已启用的差异执行基线' : '原采购商业批准'}</dd></div><div><dt>送审时执行总额</dt><dd>{money(basis.amount, basis.content.currency)}</dd></div><div><dt>拟执行总额</dt><dd>{money(value.amount, value.currency)}</dd></div></dl>
    <section className="contract-editor-section"><h3>送审时的履行集合</h3>{basis.commitments.map(line => <article className="spend-line" key={line.line_id}><h4>{basis.content.lines.find(item => item.id === line.line_id)?.description ?? '准确采购行'}</h4><dl className="file-facts"><div><dt>当时执行数量</dt><dd>{line.approved_quantity}</dd></div><div><dt>已占用数量</dt><dd>{line.reserved_quantity}</dd></div><div><dt>净验收数量</dt><dd>{line.net_accepted_quantity}</dd></div><div><dt>待收数量</dt><dd>{line.outstanding_quantity}</dd></div><div><dt>已占用金额</dt><dd>{money(line.reserved_amount, basis.content.currency)}</dd></div><div><dt>已占用税额／折扣</dt><dd>{money(line.reserved_tax, basis.content.currency)} ／ {money(line.reserved_discount, basis.content.currency)}</dd></div></dl></article>)}</section>
    <section className="contract-editor-section" aria-label="送审时完整执行依据"><h3>送审时完整执行依据</h3><SpendFacts kind="purchase" value={{ ...basis.content, amount: basis.amount, line_values: basis.line_values, suppliers: value.suppliers, attachments: value.attachments, employee_reimbursement_amount: null }} /></section>
    <section className="contract-editor-section" aria-label="完整拟执行内容"><h3>完整拟执行内容</h3><SpendFacts kind="purchase" value={value} /></section>
  </>;
}
