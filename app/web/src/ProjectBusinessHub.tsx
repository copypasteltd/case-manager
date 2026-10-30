import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { checkedBusinessContext, projectBusinessLabels, projectBusinessURL, type ProjectBusinessContext } from '@copypaste/contracts/project-business';
import { api } from './api';
import { Busy, ErrorNotice } from './components';

export function ProjectBusinessHub({ projectID }: { projectID: string }) {
  const query = useQuery({ queryKey: ['project-business-hub', projectID], gcTime: 0, retry: false, queryFn: async ({ signal }) => checkedBusinessContext(await api<ProjectBusinessContext>('/projects/' + projectID + '/business-context', { signal }), projectID) });
  const context = query.error ? undefined : query.data;
  return <section className="panel party-detail" aria-label="项目业务办理"><h2>项目业务办理</h2><p className="muted">从这里查阅或起草六类业务，保留当前项目。保存、送审、批准与实际付款分别办理。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在核对业务权限" /> : context && <div className="overview-grid">{context.capabilities.map(cap => <div className="overview-card" key={cap.kind}><h3>{projectBusinessLabels[cap.kind]}</h3><Link className="text-button" to={projectBusinessURL(cap.kind, projectID, 'web')}>查看项目{projectBusinessLabels[cap.kind]}</Link>{cap.can_create ? <Link className="button secondary" to={projectBusinessURL(cap.kind, projectID, 'web', true)}>起草项目{projectBusinessLabels[cap.kind]}</Link> : <p className="form-note">当前项目阶段、分类或授权不允许从此入口新建。</p>}</div>)}</div>}</section>;
}
