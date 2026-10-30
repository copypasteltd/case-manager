import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { businessFilters, businessSelection, checkedBusinessContext, projectBusinessActions, projectBusinessHome, type ProjectBusinessContext, type ProjectBusinessKind } from '@copypaste/contracts/project-business';
import { api, type Me, type Project } from './api';
import { Busy, ErrorNotice } from './components';

export function useProjectBusinessScope(kind: ProjectBusinessKind, me: Me) {
  const [params, setParams] = useSearchParams();
  let selection = { projectID: null as string | null, companyOnly: false }, invalid: unknown;
  try {
    if (params.getAll('project_id').length > 1 || params.getAll('company_only').length > 1) throw new Error('业务地址包含重复范围，请从项目重新打开。');
    selection = businessSelection(params.get('project_id'), params.get('company_only'));
  } catch (error) { invalid = error; }
  const context = useQuery({ queryKey: ['project-business-context', selection.projectID, me.user.id, me.authorization_version], enabled: !invalid && !!selection.projectID, retry: false, gcTime: 0, refetchOnWindowFocus: false, queryFn: async ({ signal }) => checkedBusinessContext(await api<ProjectBusinessContext>('/projects/' + selection.projectID + '/business-context', { signal }), selection.projectID!) });
  const current = invalid || context.error ? undefined : context.data;
  const canCreate = !invalid && (selection.projectID ? current?.capabilities.find(item => item.kind === kind)?.can_create === true : me.actions.includes(projectBusinessActions[kind]));
  const [draft, setDraft] = useState<{ project?: Project }>();
  function open() { if (canCreate) setDraft({ project: current?.project }); }
  useEffect(() => {
    if (params.get('create') === '1' && canCreate) {
      open();
      setParams(old => { const next = new URLSearchParams(old); next.delete('create'); return next; }, { replace: true });
    }
  }, [params.toString(), canCreate, current?.project.id, current?.project.version]);
  return { selection, filters: businessFilters(selection, kind), project: current?.project, canCreate, draft, open, close: () => setDraft(undefined), error: invalid ?? context.error, loading: !!selection.projectID && context.isPending, ready: !invalid && (!selection.projectID || !!current), kind };
}
export type ProjectBusinessScopeState = ReturnType<typeof useProjectBusinessScope>;
export function ProjectBusinessReturn({ projectID, kind }: { projectID: string | null | undefined; kind: ProjectBusinessKind }) {
  if (!projectID) return null;
  return <div className="post-actions"><Link className="text-button" to={projectBusinessHome(projectID, 'web')}>返回项目业务概览</Link><Link className="text-button" to={'/' + kind + '?project_id=' + projectID}>查看本项目同类业务</Link></div>;
}
export function ProjectBusinessScopeNotice({ scope }: { scope: ProjectBusinessScopeState }) {
  return <><ErrorNotice error={scope.error} />{scope.loading && <Busy label="正在核对项目业务范围" />}{scope.project && <div className="notice neutral" aria-label="当前项目业务范围"><strong>{scope.project.name}</strong><p>当前列表按此项目查询，新建草稿将保留此项目范围。费用及资金列表包含本项目资料及实际分摊／归属，金额汇总仍按实际份额计算。</p><Link className="text-button" to={projectBusinessHome(scope.project.id, 'web')}>返回项目业务概览</Link><Link className="text-button" to={'/' + scope.kind}>退出项目范围，查看全部可读记录</Link>{!scope.canCreate && <p>当前项目阶段、分类或授权不允许从这里新建此类业务。</p>}</div>}{scope.selection.companyOnly && <p className="notice neutral">当前仅查看公司范围的业务资料。<Link className="text-button" to={'/' + scope.kind}>查看全部可读记录</Link></p>}</>;
}
