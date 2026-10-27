import { lazy, Suspense, useEffect, useState, useRef, type KeyboardEvent } from 'react';
import { NavLink, Route, Routes, Link, useLocation, useNavigate } from 'react-router-dom';
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query';
import { LayoutDashboard, FolderKanban, ArrowUpRight, Moon, Sun, LogOut, Menu, X, ArrowRight, ShieldCheck, BookOpen, FolderOpen, MessageSquare, Activity, ShoppingCart, Receipt, Wallet, ChartNoAxesCombined } from 'lucide-react';
import { api, ApiError, type Me, type Page, type Project, type Schemas } from './api';
import Login from './Login';
import { NotificationIndicator } from './NotificationIndicator';
const NotificationsPage = lazy(() => import('./Notifications'));
const AccountSecurity = lazy(() => import('./AccountSecurity'));
const Telegram = lazy(() => import('./Telegram'));
import { Busy, ErrorNotice, Empty, State, date } from './components';




import { UploadProvider, UploadIndicator, UploadQueuePage } from './UploadQueue';







import { RouteBoundary } from './RouteBoundary';

const ConversationsPage = lazy(() => import('./Conversations').then(module => ({ default: module.ConversationsPage })));
const ServicesPage = lazy(() => import('./Services').then(module => ({ default: module.ServicesPage })));
const ServiceDetail = lazy(() => import('./Services').then(module => ({ default: module.ServiceDetail })));
const IncidentsPage = lazy(() => import('./StatusIncidents').then(module => ({ default: module.IncidentsPage })));
const IncidentDetail = lazy(() => import('./StatusIncidents').then(module => ({ default: module.IncidentDetail })));
const ConversationDetail = lazy(() => import('./Conversations').then(module => ({ default: module.ConversationDetail })));
const ProjectsPage = lazy(() => import('./Projects').then(module => ({ default: module.ProjectsPage })));
const ProjectDetail = lazy(() => import('./Projects').then(module => ({ default: module.ProjectDetail })));
const ProjectChangeDetail = lazy(() => import('./ProjectChanges').then(module => ({ default: module.ProjectChangeDetail })));
const ClassificationPolicies = lazy(() => import('./ClassificationPolicies'));
const AgentBudget = lazy(() => import('./AgentBudget'));
const Administration = lazy(() => import('./Administration').then(module => ({ default: module.default })));
const KnowledgePage = lazy(() => import('./Knowledge').then(module => ({ default: module.KnowledgePage })));
const KnowledgeDetail = lazy(() => import('./Knowledge').then(module => ({ default: module.KnowledgeDetail })));
const FilesPage = lazy(() => import('./Files').then(module => ({ default: module.FilesPage })));
const FileDetail = lazy(() => import('./Files').then(module => ({ default: module.FileDetail })));
const ScanWaiversPage = lazy(() => import('./ScanWaivers').then(module => ({ default: module.ScanWaiversPage })));
const ScanWaiverDetail = lazy(() => import('./ScanWaivers').then(module => ({ default: module.ScanWaiverDetail })));
const ScanPolicyPage = lazy(() => import('./ScanWaivers').then(module => ({ default: module.ScanPolicyPage })));
const RetentionPolicyPage = lazy(() => import('./Retention').then(module => ({ default: module.RetentionPolicyPage })));
const WorkflowDefinitionsPage = lazy(() => import('./Workflows').then(module => ({ default: module.WorkflowDefinitionsPage })));
const PartiesPage = lazy(() => import('./Parties').then(module => ({ default: module.PartiesPage })));
const PartyDetail = lazy(() => import('./Parties').then(module => ({ default: module.PartyDetail })));
const ContractsPage = lazy(() => import('./Contracts').then(module => ({ default: module.ContractsPage })));
const SpendsPage = lazy(() => import('./Spends').then(module => ({ default: module.SpendsPage })));
const SpendDetail = lazy(() => import('./Spends').then(module => ({ default: module.SpendDetail })));
const PurchaseVarianceDetail = lazy(() => import('./PurchaseVariances').then(module => ({ default: module.PurchaseVarianceDetail })));
const CostsPage = lazy(() => import('./Costs').then(module => ({ default: module.CostsPage })));
const CostDetail = lazy(() => import('./Costs').then(module => ({ default: module.CostDetail })));
const PaymentsPage = lazy(() => import('./Payments').then(module => ({ default: module.PaymentsPage })));
const PaymentDetail = lazy(() => import('./Payments').then(module => ({ default: module.PaymentDetail })));
const CashReport = lazy(() => import('./CashReport'));
const SubscriptionsPage = lazy(() => import('./Subscriptions').then(module => ({ default: module.SubscriptionsPage })));
const SubscriptionDetail = lazy(() => import('./Subscriptions').then(module => ({ default: module.SubscriptionDetail })));
const BillingPeriodsPage = lazy(() => import('./Subscriptions').then(module => ({ default: module.BillingPeriodsPage })));
const SubscriptionPeriodDetail = lazy(() => import('./SubscriptionPeriods'));
const ForecastReport = lazy(() => import('./ForecastReport'));
const CostReport = lazy(() => import('./CostReport'));
const ContractDetail = lazy(() => import('./Contracts').then(module => ({ default: module.ContractDetail })));
const RequestsPage = lazy(() => import('./Requests').then(module => ({ default: module.RequestsPage })));
const RequestDetail = lazy(() => import('./Requests').then(module => ({ default: module.RequestDetail })));
const ApprovalTasksPage = lazy(() => import('./Requests').then(module => ({ default: module.ApprovalTasksPage })));

const ProjectTemplates = lazy(() => import('./ProjectTemplates'));
const ProjectTemplateDetail = lazy(() => import('./ProjectTemplates').then(module => ({ default: module.ProjectTemplateDetail })));

type ThemePreference = 'system' | 'light' | 'dark';
function useTheme() {
  const [preference, setPreference] = useState<ThemePreference>(() => {
    try { const value = localStorage.getItem('cp:theme'); if (value === 'light' || value === 'dark') return value; } catch {}
    return 'system';
  });
  const [systemDark, setSystemDark] = useState(() => matchMedia('(prefers-color-scheme: dark)').matches);
  const theme = preference === 'system' ? (systemDark ? 'dark' : 'light') : preference;
  useEffect(() => {
    const media = matchMedia('(prefers-color-scheme: dark)'); const changed = () => setSystemDark(media.matches);
    media.addEventListener('change', changed); return () => media.removeEventListener('change', changed);
  }, []);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try { if (preference === 'system') localStorage.removeItem('cp:theme'); else localStorage.setItem('cp:theme', preference); } catch {}
  }, [theme, preference]);
  return { theme, preference, setPreference, toggle: () => setPreference(theme === 'dark' ? 'light' : 'dark') };
}
export default function App() {
  const cache = useQueryClient(); const navigate = useNavigate(); const [epoch, setEpoch] = useState(0); const { theme, preference, setPreference, toggle } = useTheme();
  const me = useQuery({ queryKey: ['me', epoch], queryFn: () => api<Me>('/me') });
  useEffect(() => {
    if (!me.data) return;
    const expire = () => { cache.clear(); setEpoch(value => value + 1); };
    window.addEventListener('cp:session-expired', expire); return () => window.removeEventListener('cp:session-expired', expire);
  }, [cache, me.data]);
  const signedIn = () => { cache.clear(); setEpoch(value => value + 1); };
  const signedOut = () => { navigate('/', { replace: true }); signedIn(); };
  if (me.isPending) return <Busy />;
  if (!me.data && me.error instanceof ApiError && me.error.status === 401) return <Login onSignedIn={signedIn} />;
  if (!me.data) return <div className="startup-error"><img src={'/brand/logo-' + theme + '.png'} alt="CopyPaste" /><h1>暂时无法载入工作空间</h1><ErrorNotice error={me.error} /><button className="button primary" onClick={() => void me.refetch()}>重新连接</button></div>;
  return <UploadProvider key={me.data.user.id} me={me.data}><Shell me={me.data} theme={theme} preference={preference} setPreference={setPreference} toggleTheme={toggle} onSignedOut={signedOut} /></UploadProvider>;
}
function Shell({ me, theme, preference, setPreference, toggleTheme, onSignedOut }: { me: Me; theme: string; preference: ThemePreference; setPreference: (value: ThemePreference) => void; toggleTheme: () => void; onSignedOut: () => void }) {
  const [menu, setMenu] = useState(false); const location = useLocation();
  const [narrow, setNarrow] = useState(() => matchMedia('(max-width: 899px)').matches);
  const sidebar = useRef<HTMLElement>(null), workspace = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const media = matchMedia('(max-width: 899px)');
    const changed = () => { setNarrow(media.matches); setMenu(false); };
    media.addEventListener('change', changed); return () => media.removeEventListener('change', changed);
  }, []);
  useEffect(() => {
    if (sidebar.current) sidebar.current.inert = narrow && !menu;
    if (!narrow || !menu) return;
    const previous = document.activeElement as HTMLElement | null;
    const oldOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    if (workspace.current) workspace.current.inert = true;
    sidebar.current?.querySelector<HTMLButtonElement>('.sidebar-close')?.focus();
    return () => {
      document.body.style.overflow = oldOverflow;
      if (workspace.current) workspace.current.inert = false;
      if (previous?.isConnected) previous.focus();
    };
  }, [narrow, menu]);
  function navigationKeys(event: KeyboardEvent<HTMLElement>) {
    if (!narrow || !menu) return;
    if (event.key === 'Escape') { event.preventDefault(); setMenu(false); return; }
    if (event.key !== 'Tab') return;
    const controls = [...(sidebar.current?.querySelectorAll<HTMLElement>('a[href],button:not([disabled]),input,select,[tabindex="0"]') ?? [])].filter(node => node.getClientRects().length);
    const first = controls[0], last = controls.at(-1);
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
  }
  useEffect(() => { setMenu(false); }, [location.pathname]);
  const logout = useMutation({ mutationFn: () => api('/auth/logout', { method: 'POST' }), onSuccess: onSignedOut });
  const projectPage = location.pathname.startsWith('/projects');
  return <div className="app-shell">
    <a className="skip-link" href="#main-content">跳转至主要内容</a>
    {menu && <button className="nav-scrim" aria-label="关闭导航" aria-hidden="true" tabIndex={-1} onClick={() => setMenu(false)} />}
    <aside ref={sidebar} className={'sidebar' + (menu ? ' is-open' : '')} aria-label="工作区导航" role={narrow && menu ? 'dialog' : undefined} aria-modal={narrow && menu ? true : undefined} aria-hidden={narrow && !menu ? true : undefined} onKeyDown={navigationKeys}>
      <button type="button" className="icon-button sidebar-close" aria-label="关闭导航" onClick={() => setMenu(false)}><X size={20} /></button>
      <Link to="/" className="brand" aria-label="CopyPaste 工作空间"><img src={'/brand/logo-' + theme + '.png'} alt="" /><span className="brand-name">copypaste<span className="accent">.</span></span></Link>
      <div className="workspace-label"><span>公司工作空间</span><span className="live-dot" /></div>
      <nav aria-label="主导航" onClick={() => setMenu(false)}><NavLink to="/" end><LayoutDashboard size={19} /><span>工作台</span></NavLink><NotificationIndicator me={me} /><NavLink to="/telegram"><MessageSquare size={19} /><span>Telegram 接入</span></NavLink><NavLink to="/agent-budget"><ShieldCheck size={19} /><span>秘书额度</span></NavLink><NavLink to="/account/security"><ShieldCheck size={19} /><span>账号安全</span></NavLink><NavLink to="/projects"><FolderKanban size={19} /><span>项目空间</span></NavLink>{me.actions.includes('file.read') && <NavLink to="/files"><FolderOpen size={19} /><span>文件与资料</span></NavLink>}{me.actions.includes('request.read') && <NavLink to="/requests"><BookOpen size={19} /><span>业务申请</span></NavLink>}{me.actions.includes('approval.decide') && <NavLink to="/approvals"><ShieldCheck size={19} /><span>待我处理</span></NavLink>}{me.actions.includes('contract.read') && <NavLink to="/contracts"><BookOpen size={19} /><span>合同</span></NavLink>}{me.actions.includes('purchase.read') && <NavLink to="/purchases" className={location.pathname.startsWith('/purchases') || location.pathname.startsWith('/purchase-variances') ? 'active' : undefined}><ShoppingCart size={19} /><span>采购管理</span></NavLink>}{me.actions.includes('claim.read') && <NavLink to="/claims"><Receipt size={19} /><span>报销管理</span></NavLink>}{me.actions.includes('cost.read') && <NavLink to="/costs"><Wallet size={19} /><span>实际费用</span></NavLink>}{me.actions.includes('payment.read') && <NavLink to="/payments"><Wallet size={19} /><span>付款与结算</span></NavLink>}{me.actions.includes('payment.read') && me.actions.includes('report.read') && <NavLink to="/reports/cash"><ChartNoAxesCombined size={19} /><span>月度现金支出</span></NavLink>}{me.actions.includes('report.read') && <NavLink to="/reports/expenses"><ChartNoAxesCombined size={19} /><span>月度发生支出</span></NavLink>}{me.actions.includes('subscription.read') && <NavLink to="/subscriptions" className={location.pathname.startsWith('/subscriptions') || location.pathname.startsWith('/subscription-periods') ? 'active' : undefined}><Receipt size={19} /><span>订阅与续费</span></NavLink>}{me.actions.includes('subscription.read') && me.actions.includes('report.read') && <NavLink to="/reports/forecast"><ChartNoAxesCombined size={19} /><span>月度订阅预测</span></NavLink>}{me.actions.includes('party.read') && <NavLink to="/parties"><BookOpen size={19} /><span>往来资料</span></NavLink>}{me.actions.includes('post.read') && <NavLink to="/knowledge"><BookOpen size={19} /><span>知识与记录</span></NavLink>}{me.actions.includes('file.scan.waive') && <NavLink to="/scan-waivers"><ShieldCheck size={19} /><span>扫描例外审核</span></NavLink>}{me.actions.includes('config.manage') && <NavLink to="/storage/scan-policies" className={location.pathname.startsWith('/storage') ? 'active' : undefined}><ShieldCheck size={19} /><span>存储制度</span></NavLink>}{me.actions.includes('config.manage') && <NavLink to="/project-templates"><FolderKanban size={19} /><span>项目模板</span></NavLink>}{me.actions.includes('config.manage') && <NavLink to="/workflows"><ShieldCheck size={19} /><span>审批制度</span></NavLink>}{me.actions.includes('service.read') && <NavLink to="/services" className={location.pathname.startsWith('/services') || location.pathname.startsWith('/incidents') ? 'active' : undefined}><Activity size={19} /><span>服务与事件</span></NavLink>}{me.actions.includes('conversation.read') && <NavLink to="/conversations"><MessageSquare size={19} /><span>公司秘书</span></NavLink>}{me.actions.includes('policy.manage') && <NavLink to="/classification-policies"><ShieldCheck size={19} /><span>资料分类</span></NavLink>}{me.actions.includes('user.read') && <NavLink to="/administration"><ShieldCheck size={19} /><span>成员与授权</span></NavLink>}</nav>
      <div className="sidebar-bottom"><span className="sidebar-caption">MAKE GOOD THINGS,<br />TOGETHER.</span><button className="profile" onClick={() => void logout.mutate()} disabled={logout.isPending} title="退出登录"><span className="avatar">{me.user.display_name.slice(0, 1)}</span><span className="profile-name"><strong>{me.user.display_name}</strong><small>退出当前设备</small></span><LogOut size={17} /></button></div>
    </aside>
    <div ref={workspace} className="main-shell"><header className="topbar"><div className="breadcrumb"><button className="icon-button mobile-menu" aria-label={menu ? '关闭导航' : '打开导航'} aria-expanded={menu} onClick={() => setMenu(value => !value)}>{menu ? <X size={20} /> : <Menu size={20} />}</button><span>工作空间</span><span className="breadcrumb-divider">/</span><strong>{location.pathname.startsWith('/subscriptions') || location.pathname.startsWith('/subscription-periods') ? '订阅与账期' : location.pathname.startsWith('/reports/forecast') ? '订阅预测' : location.pathname.startsWith('/claims') ? '报销管理' : location.pathname.startsWith('/purchases') || location.pathname.startsWith('/purchase-variances') ? '采购管理' : location.pathname.startsWith('/payments') ? '付款与结算' : location.pathname.startsWith('/reports/cash') ? '现金报表' : location.pathname.startsWith('/costs') ? '实际费用' : location.pathname.startsWith('/reports/expenses') ? '费用报表' : projectPage ? '项目' : location.pathname.startsWith('/uploads') ? '上传队列' : location.pathname.startsWith('/scan-waivers') ? '扫描例外申请' : location.pathname.startsWith('/storage') ? '存储制度' : location.pathname.startsWith('/files') ? '文件与资料' : location.pathname.startsWith('/knowledge') ? '知识与记录' : location.pathname.startsWith('/requests') ? '业务申请' : location.pathname.startsWith('/approvals') ? '待我处理' : location.pathname.startsWith('/contracts') ? '合同' : location.pathname.startsWith('/parties') ? '往来资料' : location.pathname.startsWith('/services') ? '服务状态' : location.pathname.startsWith('/incidents') ? '事件记录' : location.pathname.startsWith('/agent-budget') ? '秘书额度与用量' : location.pathname.startsWith('/conversations') ? '公司秘书' : location.pathname.startsWith('/workflows') ? '审批制度' : location.pathname.startsWith('/classification-policies') ? '资料分类与渠道' : location.pathname === '/administration' ? '成员与授权' : '工作台'}</strong></div><div className="topbar-tools"><UploadIndicator /><span className="timezone">香港时间 · UTC+8</span><select className="theme-preference" aria-label="外观偏好" value={preference} onChange={event => setPreference(event.target.value as ThemePreference)}><option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option></select><button className="icon-button" aria-label={theme === 'dark' ? '切换至浅色主题' : '切换至深色主题'} onClick={toggleTheme}>{theme === 'dark' ? <Sun size={19} /> : <Moon size={19} />}</button><span className="avatar small" title={me.user.display_name}>{me.user.display_name.slice(0, 1)}</span></div></header>
      <main id="main-content" className="page" tabIndex={-1}><ErrorNotice error={logout.error} /><RouteBoundary key={location.pathname}><Suspense fallback={<Busy label="正在载入页面" />}><Routes><Route path="/agent-budget" element={<AgentBudget me={me} />} /><Route path="/account/security" element={<AccountSecurity me={me} />} /><Route path="/telegram" element={<Telegram me={me} />} /><Route path="/" element={<Dashboard me={me} />} /><Route path="/notifications" element={<NotificationsPage me={me} />} /><Route path="/services" element={<ServicesPage me={me} />} /><Route path="/services/:id" element={<ServiceDetail me={me} />} /><Route path="/incidents" element={<IncidentsPage me={me} />} /><Route path="/incidents/:id" element={<IncidentDetail />} /><Route path="/conversations" element={<ConversationsPage me={me} />} /><Route path="/conversations/:id" element={<ConversationDetail me={me} />} /><Route path="/project-templates" element={<ProjectTemplates me={me} />} /><Route path="/project-templates/:id" element={<ProjectTemplateDetail me={me} />} /><Route path="/projects" element={<ProjectsPage me={me} />} /><Route path="/projects/:id" element={<ProjectDetail me={me} />} /><Route path="/changes/:id" element={<ProjectChangeDetail me={me} />} /><Route path="/uploads" element={<UploadQueuePage />} /><Route path="/scan-waivers" element={<ScanWaiversPage me={me} />} /><Route path="/scan-waivers/:id" element={<ScanWaiverDetail me={me} />} /><Route path="/storage/scan-policies" element={<ScanPolicyPage me={me} />} /><Route path="/storage/retention-policies" element={<RetentionPolicyPage me={me} />} /><Route path="/workflows" element={<WorkflowDefinitionsPage me={me} />} /><Route path="/requests" element={<RequestsPage me={me} />} /><Route path="/requests/:id" element={<RequestDetail me={me} />} /><Route path="/approvals" element={<ApprovalTasksPage me={me} />} /><Route path="/purchases" element={<SpendsPage me={me} kind="purchase" />} /><Route path="/purchases/:id" element={<SpendDetail me={me} kind="purchase" />} /><Route path="/purchase-variances/:id" element={<PurchaseVarianceDetail me={me} />} /><Route path="/claims" element={<SpendsPage me={me} kind="claim" />} /><Route path="/claims/:id" element={<SpendDetail me={me} kind="claim" />} /><Route path="/payments" element={<PaymentsPage me={me} />} /><Route path="/payments/:id" element={<PaymentDetail me={me} />} /><Route path="/reports/cash" element={<CashReport me={me} />} /><Route path="/costs" element={<CostsPage me={me} />} /><Route path="/costs/:id" element={<CostDetail me={me} />} /><Route path="/reports/expenses" element={<CostReport me={me} />} /><Route path="/contracts" element={<ContractsPage me={me} />} /><Route path="/contracts/:id" element={<ContractDetail me={me} />} /><Route path="/subscriptions" element={<SubscriptionsPage me={me} />} /><Route path="/subscriptions/:id" element={<SubscriptionDetail me={me} />} /><Route path="/subscription-periods" element={<BillingPeriodsPage me={me} />} /><Route path="/subscription-periods/:id" element={<SubscriptionPeriodDetail me={me} />} /><Route path="/reports/forecast" element={<ForecastReport me={me} />} /><Route path="/parties" element={<PartiesPage me={me} />} /><Route path="/parties/:id" element={<PartyDetail me={me} />} /><Route path="/files" element={<FilesPage me={me} />} /><Route path="/files/folders/:folderID" element={<FilesPage me={me} />} /><Route path="/files/:id" element={<FileDetail />} /><Route path="/knowledge" element={<KnowledgePage me={me} />} /><Route path="/knowledge/:id" element={<KnowledgeDetail me={me} />} /><Route path="/classification-policies" element={<ClassificationPolicies me={me} />} /><Route path="/administration" element={<Administration me={me} />} /><Route path="*" element={<Empty title="这个页面不存在" detail="你可以回到工作台，继续查看有权访问的项目。" action={<Link className="button primary" to="/">返回工作台</Link>} />} /></Routes></Suspense></RouteBoundary></main>
      <footer className="app-footer"><span>COPYPASTE / INTERNAL WORKSPACE</span><span>创造有序，协作无界。 ↗</span></footer>
    </div>
  </div>;
}
function Dashboard({ me }: { me: Me }) {
  const dashboard = useQuery({ queryKey: ['dashboard'], queryFn: ({ signal }) => api<Schemas['Dashboard']>('/dashboard', { signal }) });
  const projects = useQuery({ queryKey: ['dashboard-projects'], queryFn: ({ signal }) => api<Page<Project>>('/projects?limit=6', { signal }) });
  const visible = projects.data?.items ?? [];
  return <><div className="page-heading"><div><span className="eyebrow">A SPACE FOR GOOD WORK</span><h1>{me.user.display_name}，工作从这里开始<span className="accent">.</span></h1><p>让每个想法有归属，让每一步进展被看见。</p></div><span className="today">{new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', month: 'long', day: 'numeric', weekday: 'long' }).format(new Date())}</span></div>
    <ErrorNotice error={dashboard.error} />
    {dashboard.isPending ? <Busy label="正在读取待办" /> : dashboard.data && <><div className="metrics-row"><div className="metric"><span>我的未完成任务</span><strong>{dashboard.data.my_open_tasks.toLocaleString()}</strong><small>已分配给你的任务</small></div><div className="metric"><span>我的逾期任务</span><strong className={dashboard.data.my_overdue_tasks ? 'warning-text' : ''}>{dashboard.data.my_overdue_tasks.toLocaleString()}</strong><small>截止时间已过的未完成任务</small></div><div className="metric"><span>进行中的项目</span><strong>{dashboard.data.active_projects.toLocaleString()}</strong><small>你当前有权访问的项目</small></div></div><section className="section-block"><div className="section-heading"><div><span className="eyebrow">YOUR NEXT STEPS</span><h2>待我推进</h2></div><small className="muted">截至 {new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', hour: '2-digit', minute: '2-digit' }).format(new Date(dashboard.data.as_of))}</small></div><div className="panel">{dashboard.data.tasks.length ? dashboard.data.tasks.map(task => <Link key={task.id} to={'/projects/' + task.project_id + '?task=' + task.id} className="todo-row"><div><strong>{task.title}</strong><small>截止 {date(task.due_at)}</small></div><State state={task.state} /><ArrowUpRight size={17} /></Link>) : <Empty title="当前没有分配给你的未完成任务" detail="安排好下一步，让团队的工作有序推进。" />}</div></section></>}
    <section className="section-block"><div className="section-heading"><div><span className="eyebrow">PICK UP WHERE YOU LEFT OFF</span><h2>最近更新的项目</h2></div><Link className="text-button" to="/projects">查看项目<ArrowRight size={16} /></Link></div><ErrorNotice error={projects.error} />{projects.isPending ? <Busy label="正在读取项目" /> : !visible.length ? <div className="panel"><Empty title="工作空间准备好了" detail={me.actions.includes('project.create') ? '从第一个项目开始，把目标和接下来的工作整理起来。' : '获授权的项目会在这里显示。'} action={me.actions.includes('project.create') ? <Link className="button primary" to="/projects">新建第一个项目 <ArrowRight size={16} /></Link> : undefined} /></div> : <div className="project-grid">{visible.map(project => <Link className="project-card" key={project.id} to={'/projects/' + project.id}><div className="project-card-top"><span className="project-number">{project.project_no}</span><ArrowUpRight size={18} /></div><h3>{project.name}</h3><p>{project.scope || '尚未填写项目范围'}</p><div className="project-card-bottom"><State state={project.state} /><span>更新于 {date(project.updated_at)}</span></div></Link>)}</div>}</section>
    <div className="workspace-footnote"><ShieldCheck size={16} /><span>此处仅展示你有权访问的项目与资料。</span></div>
  </>;
}
