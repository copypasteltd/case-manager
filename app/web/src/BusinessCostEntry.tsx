import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { businessCostPath, checkedBusinessCostBasis, type BusinessCostKind } from '@copypaste/contracts/cost-business';
import { api, type Me } from './api';
import { Busy, Dialog, ErrorNotice } from './components';
import { CostEditor } from './CostEditor';

export function BusinessCostEntry({ kind, id, eligible, me }: { kind: BusinessCostKind; id: string; eligible: boolean; me: Me }) {
  const [open, setOpen] = useState(false);
  if (!eligible || !['cost.read', 'cost.record', 'cost.confirm'].every(action => me.actions.includes(action))) return null;
  return <><button className="button secondary" onClick={() => setOpen(true)}>登记实际费用并关联</button>
    {open && <BusinessCostDialog key={id + ':' + me.user.id + ':' + me.authorization_version} kind={kind} id={id} me={me} onClose={() => setOpen(false)} />}</>;
}

function BusinessCostDialog({ kind, id, me, onClose }: { kind: BusinessCostKind; id: string; me: Me; onClose(): void }) {
  const query = useQuery({ queryKey: ['cost-business-basis', kind, id, me.user.id, me.authorization_version], gcTime: 0, staleTime: 0, retry: false, refetchOnWindowFocus: false,
    queryFn: async ({ signal }) => checkedBusinessCostBasis(await api<unknown>(businessCostPath(kind, id), { signal }), kind, id) });
  const basis = query.error ? undefined : query.data;
  if (!basis) return <Dialog title="核对批准业务依据" onClose={onClose}>{query.isPending ? <Busy /> : <ErrorNotice error={query.error} />}</Dialog>;
  return <CostEditor key={basis.source.version + ':' + basis.source.revision_id + ':' + basis.source.content_hash} businessBasis={basis} onClose={onClose} />;
}
