import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import type { Schemas } from './api';
import { Busy, ErrorNotice } from './components';
import { money } from './ContractDisplay';
import { useClaimSettlement } from './PaymentDisplay';

export function ClaimSettlementPanel({ claim }: { claim: Schemas['Spend'] }) {
  const query = useQuery(useClaimSettlement(claim.id, !!claim.approved_revision_id));
  if (!claim.approved_revision_id) return null;
  const s = query.error ? undefined : query.data;
  return <section className="panel party-detail"><h2>员工垫付结算</h2><p className="form-note">公司已付的报销明细不再产生员工应付。结算由实际公司付款、退款及冲销推导。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : s && <><dl className="file-facts"><div><dt>批准员工垫付</dt><dd>{money(s.eligible_amount, s.currency)}</dd></div><div><dt>公司已实际结算</dt><dd>{s.visibility === 'complete' ? money(s.paid_amount, s.currency) : '结算资料当前不可完整查阅'}</dd></div><div><dt>员工待收金额</dt><dd>{s.visibility === 'complete' ? money(s.outstanding_amount, s.currency) : '未显示，需完整结算权限'}</dd></div><div><dt>结算状态</dt><dd>{s.state ? { unsettled: '尚未结算', partial: '部分结算', settled: '已结清' }[s.state] : '当前无法完整核定'}</dd></div></dl><Link className="text-button" to="/payments">核对实际付款与结算</Link></>}</section>;
}
