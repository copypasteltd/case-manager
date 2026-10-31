import { Text, View } from '@tarojs/components';
import Taro, { useDidHide, useRouter } from '@tarojs/taro';
import { useEffect, useRef, useState } from 'react';
import { CommandNotice, DraftNotice, useCommand } from '../../platform/command';
import { formID } from '../../platform/business-input';
import { useCollection, useRecord } from '../../platform/collection';
import { useFormDraft } from '../../platform/form';
import { go, goVersion, identifier, usePage } from '../../platform/page';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { eventNames, messageText, runName, runReceipt, runtimeReason, skillNames } from '../../platform/secretary';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section, date } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';
import { useMessages } from './live';
import { ConversationSubscriptionPortfolio } from './subscription-portfolio';
import { CatalogSource } from './project-catalog';
import { ConversationPlans, ReferencedPlan } from './plans';
import { ConversationOrganization, ReferencedOrganizationPlan } from './organization';
import { FinanceReport } from './finance-reports';
import { ServiceReport } from './service-reports';
import { ContextChoice } from './sources';
import { ConversationMutation, MutationReceipt } from './mutations';
import { oaRequestAccepted, type OARequest } from '@copypaste/contracts/oa-request';
import { postWriteAccepted, type PostWrite } from '@copypaste/contracts/post-write';
import { ConversationPostWrite, PostWriteReceipt } from './post-writes';
import { ConversationOARequest, OARequestReceipt } from './oa-requests';
import { ConversationFileUpdate, FileUpdateReceipt } from './file-updates';
import { ConversationFileRecovery } from './file-recovery';
import { fileRecoveryReceipt, type FileRecovery } from '@copypaste/contracts/file-recovery';
import { contractBodyAccepted, type ContractBodyInput } from '@copypaste/contracts/contract-body';
import { ConversationContractBody, ContractBodyReview } from './contract-bodies';
import { ConversationFileCreate } from './file-creates';
import { fileUpdateReceipt, fileCreateReceipt, type FileUpdate, type FileCreate } from '@copypaste/contracts/file-update';
import { mutationReceipt, type Mutation } from '@copypaste/contracts/mutation';
import AgentRuntime from './runtime';
type Conversation = Schemas['Conversation'];
type SelectedSource=Pick<Schemas['ConversationContextView'],'id'|'title'|'resource_version'|'version_id'>&{type:Schemas['AgentSourceSnapshot']['type']};
const navigate = (kind: string, id?: string) => void Taro.redirectTo({ url: '/pages/secretary/index?kind=' + kind + (id ? '&id=' + identifier(id) : '') });

export default function SecretaryPage() {
  const route = useRouter();
  return route.params.kind === 'runtime' ? <AgentRuntime /> : <SecretaryWorkspace />;
}
function SecretaryWorkspace() {
  const params = useRouter().params, [editing, setEditing] = useState<'title' | 'state' | 'members'>();
  useDidHide(() => setEditing(undefined));
  const page = usePage(async scope => {
    const kind = params.kind ?? 'list'; if (!['list', 'conversation', 'new'].includes(kind)) throw new MobileError('INVALID_RESOURCE', '会话地址无效，请从公司秘书重新打开。');
    const conversation = kind === 'conversation' ? await client.request<Conversation>('/conversations/' + identifier(params.id), { scope }) : undefined;
    if (conversation && conversation.id !== identifier(params.id)) throw new MobileError('INVALID_RESPONSE', '会话与所选地址不一致。');
    if (params.run) { const run = await client.request<Schemas['Run']>('/runs/' + identifier(params.run), { scope }); if (!conversation || run.id !== identifier(params.run) || run.conversation_id !== conversation.id) throw new MobileError('INVALID_RESOURCE', '任务与当前会话不一致，请从通知重新打开。'); }
    return { kind, conversation };
  });
  const result = page.result, value = result?.value, c = value?.conversation;
  const reload = () => { setEditing(undefined); void page.reload(); };
  return <Page title={c?.title ?? (value?.kind === 'new' ? '新建秘书会话' : '公司秘书')} eyebrow='COMPANY / CONVERSATIONS' home>
    <Notice error={page.error} />{page.loading ? <Busy /> : result && value && <>
      {value.kind === 'list' && <Conversations me={result.me} />}
      {value.kind === 'new' && (result.me.actions.includes('conversation.create') ? <CreateConversation me={result.me} /> : <Empty>当前权限不允许新建会话。</Empty>)}
      {c && <ConversationView key={c.id + ':' + c.version + ':' + result.me.authorization_version} conversation={c} me={result.me} editing={editing} onEdit={setEditing} onReload={reload} />}
    </>}
    <Action secondary loading={page.loading} onClick={reload}>重新读取会话与当前权限</Action>
    {value?.kind !== 'list' && <Action secondary onClick={() => navigate('list')}>返回公司秘书</Action>}
  </Page>;
}
function Conversations({ me }: { me: Schemas['Me'] }) {
  const [search, setSearch] = useState(''), [q, setQ] = useState(''), [state, setState] = useState('active'), [type, setType] = useState('');
  const query = useCollection<Conversation>('/conversations?limit=20&state=' + state + '&type=' + type + '&q=' + encodeURIComponent(q));
  return <>{me.actions.includes('agent.runtime.read') && <Action secondary onClick={() => navigate('runtime')}>秘书运行配置</Action>}{me.actions.includes('conversation.create') && <Action onClick={() => navigate('new')}>新建会话</Action>}
    <TextField name='conversation-search' label='查找会话标题' value={search} onChange={setSearch} disabled={false} /><Action secondary onClick={() => { setQ(search.trim()); query.reload(); }}>查找会话</Action>
    <SelectField label='会话阶段' value={state} options={[[ 'active', '使用中' ], [ 'archived', '已归档' ], [ '', '全部可见会话' ]]} onChange={setState} disabled={false} />
    <SelectField label='会话范围' value={type} options={[[ '', '全部可见范围' ], [ 'private', '个人会话' ], [ 'project', '项目共享会话' ]]} onChange={setType} disabled={false} />
    <Notice error={query.error} />{query.loading ? <Busy /> : query.items.length ? <Section title='当前可见会话'>{query.items.map(c => <Row key={c.id} title={c.title} subtitle={(c.type === 'telegram' ? 'TG渠道' : c.type === 'private' ? '个人' : '项目共享') + ' · ' + c.members.length + ' 位成员 · ' + (c.state === 'active' ? '使用中' : '已归档')} onClick={() => navigate('conversation', c.id)} />)}</Section> : !query.error && <Empty>没有符合条件的会话。</Empty>}
    {query.next && <Action secondary loading={query.moreBusy} onClick={() => void query.more()}>加载更多会话</Action>}
  </>;
}
function CreateConversation({ me }: { me: Schemas['Me'] }) {
  const draft = useFormDraft(me, { kind: 'conversation-create', id: 'new-conversation', version: 1 }, 'private');
  const [project, setProject] = useState<Schemas['Project']>(), [picking, setPicking] = useState(false), [error, setError] = useState<unknown>();
  const intent = useRef<Schemas['ConversationCreate']>();
  const command = useCommand<Conversation>(value => { draft.forget(); navigate('conversation', value.id); }, draft.binding, value => !!intent.current && value.creator_id === me.user.id && value.type === intent.current.type && value.title === intent.current.title && value.project_id === intent.current.project_id && value.members.length === 1 && value.members[0].user_id === me.user.id);
  const disabled = command.busy || command.blocked;
  if (picking) return <ChoiceList<Schemas['Project']> title='选择会话项目' path='/projects?limit=20' label={value => value.name} eligible={value => value.classification !== 'confidential'} onSelect={value => { setProject(value); setPicking(false); }} onClose={() => setPicking(false)} />;
  function save() {
    if (!draft.fields.name?.trim() || draft.fields.action === 'project' && !project) { setError(new MobileError('INPUT_REQUIRED', '请填写会话标题，并明确选择共享会话所属项目。')); return; }
    const body: Schemas['ConversationCreate'] = { title: draft.fields.name, type: draft.fields.action === 'project' ? 'project' : 'private', project_id: project?.id ?? null, member_ids: [] }; intent.current = body; void command.submit('/conversations', body);
  }
  return <Section title='明确本次会话范围'><View className='cp-pad'><DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <TextField name='conversation-title' label='会话标题' value={draft.fields.name ?? ''} onChange={name => draft.update({ name })} disabled={disabled} />
    <SelectField label='新会话范围' value={draft.fields.action} options={[[ 'private', '仅本人可见' ], [ 'project', '指定项目的共享会话' ]]} onChange={action => { draft.update({ action }); setProject(undefined); }} disabled={disabled} />
    <Text className='cp-muted cp-small cp-paragraph'>{project?.name ?? '尚未关联项目。共享会话必须选择项目；个人会话可以关联项目或公司范围。'}</Text>
    <Action secondary disabled={disabled} onClick={() => setPicking(true)}>选择会话项目</Action>{project && <Action secondary disabled={disabled} onClick={() => setProject(undefined)}>清除项目选择</Action>}
    <Text className='cp-notice cp-paragraph'>新会话最初只有你本人。项目成员不会自动加入；创建后再明确调整成员。</Text>
    <Action loading={command.busy} disabled={disabled} onClick={save}>确认创建会话</Action>
  </View></Section>;
}
function ConversationView({ conversation: c, me, editing, onEdit, onReload }: { conversation: Conversation; me: Schemas['Me']; editing?: 'title' | 'state' | 'members'; onEdit(value?: 'title' | 'state' | 'members'): void; onReload(): void }) {
  const [valid, setValid] = useState(true), [error, setError] = useState<unknown>();
  useEffect(() => {
    const scope = new Scope(); let flight = false;
    const refresh = async () => {
      if (scope.closed || flight) return; flight = true;
      try { const current = await client.request<Conversation>('/conversations/' + c.id, { scope }); if (!scope.closed && (current.version !== c.version || current.context_security_version !== c.context_security_version)) { setValid(false); onReload(); } }
      catch (cause) { if (!scope.closed) { setValid(false); setError(cause); } } finally { flight = false; }
    };
    const timer = setInterval(() => void refresh(), 5000); return () => { scope.close(); clearInterval(timer); };
  }, [c.id, c.version]);
  if (!valid) return <><Notice error={error} /><Text className='cp-notice'>会话范围或读取状态已变化，请重新读取后继续。</Text></>;
  if (editing) return editing === 'members' ? <Members conversation={c} me={me} onCommitted={onReload} onCancel={() => onEdit(undefined)} /> : <EditConversation conversation={c} me={me} changingState={editing === 'state'} onCommitted={onReload} onCancel={() => onEdit(undefined)} />;
  return <><Section title='会话范围与成员'><View className='cp-pad'><Text className='cp-paragraph'>{c.type === 'telegram' ? 'TG渠道会话 · 本人请求与结果' : c.type === 'private' ? '个人会话 · 仅本人' : '项目共享会话 · 只向明确加入且当前有权的成员开放'} · {c.state === 'active' ? '使用中' : '已归档'}</Text>
    {c.members.map(member => <Fact key={member.user_id} label={member.is_creator ? '发起人' : '已明确加入'}><Text>{member.display_name}{!member.available && ' · 当前不可用'}</Text></Fact>)}
    {c.can_edit && <><Action secondary onClick={() => onEdit('title')}>编辑会话标题</Action><Action secondary onClick={() => onEdit('state')}>{c.state === 'active' ? '归档会话' : '恢复会话'}</Action></>}{c.can_manage_members && <Action secondary onClick={() => onEdit('members')}>调整明确成员</Action>}
    {c.project_id && <Action secondary onClick={() => go('project', c.project_id!)}>查看所属项目</Action>}
  </View></Section>{me.actions.includes('report.read') && <ConversationSubscriptionPortfolio key={c.id+':'+c.current_revision_id+':'+me.authorization_version} conversation={c} />}<MessageHistory conversation={c} me={me} /></>;
}
function EditConversation({ conversation: c, me, changingState, onCommitted, onCancel }: { conversation: Conversation; me: Schemas['Me']; changingState: boolean; onCommitted(): void; onCancel(): void }) {
  const draft = useFormDraft(me, { kind: 'conversation-edit', id: c.id, version: c.version }, changingState ? (c.state === 'active' ? 'archived' : 'active') : c.state, { name: c.title });
  const intent = useRef<Schemas['ConversationPatch']>(), command = useCommand<Conversation>(() => { draft.forget(); onCommitted(); }, draft.binding, value => !!intent.current && value.id === c.id && value.version === c.version + 1 && value.title === intent.current.title && value.state === intent.current.state);
  return <Section title={changingState ? c.state === 'active' ? '归档会话' : '恢复会话' : '编辑会话标题'}><View className='cp-pad'><DraftNotice restored={draft.restored} /><Notice error={command.error} /><CommandNotice blocked={command.blocked} />
    {!changingState && <TextField name='title' label='新的会话标题' value={draft.fields.name ?? ''} onChange={name => draft.update({ name })} disabled={command.busy || command.blocked} />}
    <TextField name='reason' label='会话变更原因' value={draft.fields.reason} onChange={reason => draft.update({ reason })} disabled={command.busy || command.blocked} />
    <Action loading={command.busy} disabled={command.blocked || !draft.fields.reason.trim() || !draft.fields.name?.trim()} onClick={() => { const body: Schemas['ConversationPatch'] = { expected_version: c.version, title: draft.fields.name!, state: changingState ? c.state === 'active' ? 'archived' : 'active' : c.state, reason: draft.fields.reason }; intent.current = body; void command.submit('/conversations/' + c.id, body, 'PATCH'); }}>确认会话变更</Action>
    <Action secondary disabled={command.busy} onClick={onCancel}>返回会话，保留输入</Action>
  </View></Section>;
}
function Members({ conversation: c, me, onCommitted, onCancel }: { conversation: Conversation; me: Schemas['Me']; onCommitted(): void; onCancel(): void }) {
  const candidates = useRecord<Schemas['ConversationCandidates']>('/conversations/' + c.id + '/member-candidates'), [selected, setSelected] = useState('');
  const draft = useFormDraft(me, { kind: 'conversation-member', id: c.id, version: c.version }, 'add');
  const intent = useRef<Schemas['ConversationMemberChange']>(), command = useCommand<Schemas['ConversationAccessResult']>(value => { draft.forget(); if (value.access_lost) navigate('list'); else onCommitted(); }, draft.binding, value => value.conversation_id === c.id && value.version === c.version + 1 && value.context_security_version === c.context_security_version + 1 && typeof value.access_lost === 'boolean');
  const options = draft.fields.action === 'remove' ? c.members.filter(member => !member.is_creator) : candidates.value?.items ?? [];
  return <Section title='调整明确会话成员'><View className='cp-pad'><Notice error={candidates.error ?? command.error} /><CommandNotice blocked={command.blocked} /><DraftNotice restored={draft.restored} />
    <SelectField label='成员变更动作' value={draft.fields.action} options={[[ 'add', '明确加入一位成员' ], [ 'remove', '移出一位现有成员' ]]} disabled={command.busy || command.blocked} onChange={action => { draft.update({ action }); setSelected(''); }} />
    <Text className='cp-notice cp-paragraph'>加入成员不会自动取得项目或资料权限，也不会向其披露加入前的消息。</Text>
    {candidates.loading ? <Busy /> : options.map(member => <Row key={member.user_id} title={member.display_name + (member.user_id === me.user.id ? ' · 本人' : '')} subtitle={selected === member.user_id ? '本次已明确选择' : member.available ? '当前可用' : '当前不可用'} onClick={() => { if (!command.busy && !command.blocked) setSelected(member.user_id); }} />)}
    <TextField name='reason' label='成员变更原因' value={draft.fields.reason} onChange={reason => draft.update({ reason })} disabled={command.busy || command.blocked} />
    <Action loading={command.busy} disabled={command.blocked || !selected || !draft.fields.reason.trim()} onClick={() => { const body: Schemas['ConversationMemberChange'] = { expected_version: c.version, user_id: selected, action: draft.fields.action === 'remove' ? 'remove' : 'add', reason: draft.fields.reason }; intent.current = body; void command.submit('/conversations/' + c.id + '/members', body); }}>确认成员变更</Action>
    <Action secondary disabled={command.busy} onClick={onCancel}>返回会话，保留文字</Action>
  </View></Section>;
}
function MessageHistory({ conversation: c, me }: { conversation: Conversation; me: Schemas['Me'] }) {
  const history = useMessages<Schemas['Message']>('/conversations/' + c.id + '/messages?limit=20'), [run, setRun] = useState<string | undefined>(() => { const value = Taro.getCurrentInstance().router?.params.run; return value ? identifier(value) : undefined; }), [sent, setSent] = useState(0), [planRef,setPlanRef]=useState<Schemas['AgentSourceSnapshot']>(),[financeRef,setFinanceRef]=useState<Schemas['AgentSourceSnapshot']>(),[serviceRef,setServiceRef]=useState<Schemas['AgentSourceSnapshot']>();
  return <><Composer key={sent} conversation={c} me={me} onCommitted={value => { setRun(value.id); setSent(previous => previous + 1); history.reload(); }} />
    {run && <RunProgress id={run} me={me} onClose={() => setRun(undefined)} onCommitted={history.reload} />}{planRef?.type==='agent_organization_plan'&&<ReferencedOrganizationPlan reference={planRef} conversation={c} me={me} onClose={()=>setPlanRef(undefined)}/>}{planRef&&planRef.type!=='agent_organization_plan'&&<ReferencedPlan reference={planRef} conversation={c} me={me} onClose={()=>setPlanRef(undefined)}/>}
    {financeRef&&<FinanceReport conversation={c} source={financeRef} onClose={()=>setFinanceRef(undefined)}/>}
{serviceRef&&<ServiceReport conversation={c} source={serviceRef} onClose={()=>setServiceRef(undefined)}/>}
    <Section title='实际消息记录'><View className='cp-pad'><Text className='cp-muted cp-small cp-paragraph'>按接收顺序显示最近消息；只展示发送时向你开放、且当前依赖资料仍可查阅的内容。</Text><Notice error={history.error} />
      {history.loading ? <Busy /> : history.items.length ? history.items.map(message => <View className='cp-editor-row' key={message.id}><Text className='cp-row-title'>{message.role === 'user' ? '成员消息' : message.role === 'assistant' ? '秘书回复' : '系统记录'} · {message.seq}</Text><Text className='cp-muted cp-small'>{date(message.created_at)}</Text><Text className='cp-paragraph' selectable>{messageText(message)}</Text>
        {message.source_refs.map(ref => <Fact key={ref.type + ref.id + (ref.version_id??'')} label={'固定资料 · 修订 ' + ref.resource_version}><Text className='cp-code' selectable>{ref.sha256}</Text>{(ref.type==='agent_contract_body'||ref.type==='agent_post_write'||ref.type==='agent_oa_request'||ref.type==='agent_mutation'||(ref.type==='agent_file_update'||ref.type==='agent_file_create'))?<Action secondary onClick={()=>setRun(message.run_id)}>查看本次准确更新</Action>:(ref.type==='agent_plan'||ref.type==='agent_organization_plan')?<Action secondary onClick={()=>setPlanRef(ref)}>查看当前计划</Action>:ref.type==='agent_directory'?<Action secondary onClick={()=>setRun(message.run_id)}>查看盘点任务</Action>:(ref.type==='agent_project_person'||ref.type==='agent_project_template'||ref.type==='agent_project_catalog')?<CatalogSource conversation={c} source={ref}/>:ref.type==='agent_finance_report'?<Action secondary onClick={()=>setFinanceRef(ref)}>查看固定财务快照</Action>:ref.type==='service'&&ref.version_id?<Action secondary onClick={()=>setServiceRef(ref)}>查看固定状态快照</Action>:ref.type==='file'&&ref.version_id?<Action secondary onClick={()=>goVersion(ref.id,ref.version_id!)}>查看准确文件版本</Action>:<Action secondary onClick={() => go(ref.type, ref.id)}>查看当前有权资料</Action>}</Fact>)}
        {message.visibility === 'available' && <Action secondary onClick={() => setRun(message.run_id)}>查看任务进度</Action>}
      </View>) : !history.error && <Empty>尚无已保存消息。只有真正受理的请求才进入记录。</Empty>}
      {history.next && <Action secondary loading={history.busy} onClick={() => void history.more()}>加载更早消息</Action>}<Action secondary onClick={history.reload}>重新读取消息</Action>
    </View></Section></>;
}
function Composer({ conversation: c, me, onCommitted }: { conversation: Conversation; me: Schemas['Me']; onCommitted(value: Schemas['Run']): void }) {
  const options = useRecord<Schemas['RuntimeOptions']>('/conversations/' + c.id + '/runtime-options'), [messageID] = useState(() => formID(client.operationKey()));
  const draft = useFormDraft(me, { kind: 'run-create', id: c.id, version: c.version }, '', { body: '', owner: messageID });
  const [refs, setRefs] = useState<SelectedSource[]>([]), [picking, setPicking] = useState(false), [error, setError] = useState<unknown>();
  const [mutation,setMutation]=useState<Mutation>();
  const [fileUpdate,setFileUpdate]=useState<FileUpdate>();
  const [fileRecovery,setFileRecovery]=useState<FileRecovery>();
  const [oaRequest,setOARequest]=useState<OARequest>();
  const [postWrite,setPostWrite]=useState<PostWrite>();
  const [fileCreate,setFileCreate]=useState<FileCreate>();
  const [contractBody,setContractBody]=useState<ContractBodyInput>();
  const intent = useRef<Schemas['RunCreate']>(), command = useCommand<Schemas['Run']>(value => { draft.forget(); draft.update({ body: '', owner: formID(client.operationKey()) }); setRefs([]); setMutation(undefined);setOARequest(undefined);setPostWrite(undefined); setFileUpdate(undefined);setFileCreate(undefined);setContractBody(undefined);setFileRecovery(undefined); onCommitted(value); }, draft.binding, value => !!intent.current && runReceipt(value, { conversation: c.id, actor: me.user.id, profile: intent.current.profile_id, message: intent.current.client_message_id })&&mutationReceipt(value,intent.current.mutation)&&(!intent.current.file_recovery&&fileUpdateReceipt(value,intent.current.file_update)&&fileCreateReceipt(value,intent.current.file_create)||!!intent.current.file_recovery)&&fileRecoveryReceipt(value,intent.current.file_recovery)&&oaRequestAccepted(value,intent.current.oa_request)&&postWriteAccepted(value,intent.current.post_write)&&contractBodyAccepted(value,intent.current.contract_body));
  if (picking) return <ContextChoice conversation={c} onSelect={value => { setRefs(old => [...old.filter(item => item.type !== value.type || item.id !== value.id || value.type === 'file' && item.version_id !== value.version_id), value]); setPicking(false); }} onClose={() => setPicking(false)} />;
  return <><ConversationOrganization conversation={c} me={me} referenceDisabled={command.busy||command.blocked||refs.length>=20} onReference={options.value?.profiles.some(v=>v.skill_id==='information-organize')?plan=>{if(command.busy||command.blocked||refs.length>=20)return;setRefs(old=>[...old.filter(r=>r.type!=='agent_plan'&&(r.type!=='agent_organization_plan'||r.id!==plan.id)),{type:'agent_organization_plan',id:plan.id,title:'资料整理计划 · '+plan.preview.length+'项',version_id:null,resource_version:1}]);setMutation(undefined);setOARequest(undefined);setPostWrite(undefined);setFileUpdate(undefined);setFileCreate(undefined);setContractBody(undefined);setFileRecovery(undefined);draft.update({action:options.value!.profiles.find(v=>v.skill_id==='information-organize')!.id});}:undefined}/><ConversationPlans conversation={c} me={me} referenceDisabled={command.busy||command.blocked||refs.length>=20} onReference={options.value?.profiles.some(profile=>profile.skill_id==='project-create')?plan=>{if(command.busy||command.blocked||refs.length>=20)return;setRefs(old=>[...old.filter(item=>item.type!=='agent_organization_plan'&&(item.type!=='agent_plan'||item.id!==plan.id)),{type:'agent_plan',id:plan.id,title:plan.project.name,version_id:null,resource_version:1}]);setMutation(undefined);setOARequest(undefined);setPostWrite(undefined);setFileUpdate(undefined);setFileCreate(undefined);setContractBody(undefined);setFileRecovery(undefined);draft.update({action:options.value!.profiles.find(profile=>profile.skill_id==='project-create')!.id});}:undefined}/><Section title='给公司秘书的请求'><View className='cp-pad'><Notice error={options.error} />{options.loading ? <Busy /> : options.value && !options.value.available ? <Text className='cp-notice cp-paragraph'>{runtimeReason(options.value.reason)}</Text> : options.value && <>
    <DraftNotice restored={draft.restored} /><Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <SelectField label='本次秘书任务' value={draft.fields.action} options={[[ '', '请明确选择已配置任务' ], ...options.value.profiles.filter(profile=>(!refs.some(ref=>ref.type==='agent_organization_plan')||profile.skill_id==='information-organize')&&(!refs.some(ref=>ref.type==='agent_plan')||profile.skill_id==='project-create')&&(!mutation||profile.skill_id==='project-update')&&(!(fileUpdate||fileCreate||fileRecovery)||profile.skill_id==='file-update')&&(!oaRequest||profile.skill_id==='oa-assist')&&(!postWrite||profile.skill_id==='project-briefing')).map(profile => [profile.id, (skillNames[profile.skill_id] ?? profile.skill_id) + ' · ' + profile.skill_version] as [string, string])]} onChange={action => draft.update({ action })} disabled={command.busy || command.blocked} />
    <TextField name='message' label='本次消息内容' value={draft.fields.body ?? ''} onChange={body => draft.update({ body })} disabled={command.busy || command.blocked} multiline max={20000} />
    {options.value.profiles.some(profile=>profile.id===draft.fields.action&&profile.skill_id==='project-update')&&<ConversationMutation conversation={c} disabled={command.busy||command.blocked} value={mutation} onChange={setMutation}/>}
    {options.value.profiles.some(v=>v.id===draft.fields.action&&v.skill_id==='project-briefing')&&<ConversationPostWrite conversation={c} disabled={command.busy||command.blocked} value={postWrite} references={refs.map(r=>({type:r.type,id:r.id,...(r.version_id?{version_id:r.version_id}:{})}))} onChange={setPostWrite}/>}
    {options.value.profiles.some(profile=>profile.id===draft.fields.action&&profile.skill_id==='oa-assist')&&<ConversationOARequest conversation={c} me={me} disabled={command.busy||command.blocked} value={oaRequest} onChange={setOARequest}/>}
    {options.value.profiles.some(profile=>profile.id===draft.fields.action&&profile.skill_id==='file-update')&&<><ConversationFileUpdate conversation={c} disabled={command.busy||command.blocked||!!fileCreate||!!fileRecovery} value={fileUpdate} onChange={setFileUpdate}/><ConversationFileCreate conversation={c} disabled={command.busy||command.blocked||!!fileUpdate||!!fileRecovery} value={fileCreate} onChange={value=>{setFileCreate(value);setContractBody(undefined);}}/>{fileCreate&&<ConversationContractBody conversation={c} file={fileCreate} disabled={command.busy||command.blocked} value={contractBody} onChange={setContractBody}/>}<ConversationFileRecovery conversation={c} actor={me.user.id} disabled={command.busy||command.blocked||!!fileUpdate||!!fileCreate} value={fileRecovery} onChange={setFileRecovery}/></>}
    {refs.map(ref => <Row key={ref.type + ref.id + (ref.version_id??'')} title={ref.title} subtitle={((ref.type==='agent_plan'||ref.type==='agent_organization_plan')?'固定计划意图':'已核对修订 '+ref.resource_version)+' · 点击移除此项'} onClick={() => { if (!command.busy && !command.blocked) setRefs(old => old.filter(item => item !== ref)); }} />)}
    <Action secondary disabled={command.busy || command.blocked || refs.length >= 20} onClick={() => setPicking(true)}>核对并选择上下文资料</Action>
    <Text className='cp-muted cp-small cp-paragraph'>资料只在本次明确选择后加入请求；文字恢复不会恢复引用。引用计划不等于批准或授权执行，正式写入和批量计划继续由服务器逐项核对。</Text>
    <Action loading={command.busy} disabled={command.blocked} onClick={() => { const profile = options.value?.profiles.find(value => value.id === draft.fields.action); if (!profile || (refs.some(ref=>ref.type==='agent_organization_plan')&&profile.skill_id!=='information-organize') || (refs.some(ref=>ref.type==='agent_plan')&&profile.skill_id!=='project-create') || (mutation&&profile.skill_id!=='project-update') || ((fileUpdate||fileCreate||fileRecovery)&&profile.skill_id!=='file-update') || (oaRequest&&profile.skill_id!=='oa-assist') || (postWrite&&profile.skill_id!=='project-briefing') || !draft.fields.body?.trim() || !draft.fields.owner) { setError(new MobileError('INPUT_REQUIRED', '请明确选择当前执行任务，并填写消息。')); return; } const body: Schemas['RunCreate'] = { expected_version: c.version, context_security_version: c.context_security_version, profile_id: profile.id, client_message_id: identifier(draft.fields.owner), message: draft.fields.body, context_refs: refs.map(ref => ({ type: ref.type, id: ref.id, version_id: ref.version_id })),...(mutation?{mutation}:{}),...(fileUpdate?{file_update:fileUpdate}:{}),...(fileCreate?{file_create:fileCreate}:{}),...(contractBody?{contract_body:contractBody}:{}),...(fileRecovery?{file_recovery:fileRecovery}:{}),...(oaRequest?{oa_request:oaRequest}:{}),...(postWrite?{post_write:postWrite}:{}) }; intent.current = body; void command.submit('/conversations/' + c.id + '/runs', body); }}>确认发送本次请求</Action>
  </>}</View></Section></>;
}
function RunProgress({ id, me, onClose, onCommitted }: { id: string; me: Schemas['Me']; onClose(): void; onCommitted(): void }) {
  const [value, setValue] = useState<Schemas['RunEventPage']>(), [error, setError] = useState<unknown>(), [revision, setRevision] = useState(0), [cancelling, setCancelling] = useState(false);
  const [extra, setExtra] = useState<Schemas['RunEvent'][]>([]), [moreBusy, setMoreBusy] = useState(false), currentScope = useRef<Scope>();
  useEffect(() => {
    const scope = new Scope(); let flight = false;
    currentScope.current = scope; setValue(undefined); setError(undefined); setExtra([]); setMoreBusy(false);
    async function refresh() { if (scope.closed || flight) return; flight = true; try { const result = await client.request<Schemas['RunEventPage']>('/runs/' + identifier(id) + '/event-log?after_seq=0&limit=100', { scope }); if (!scope.closed) { setValue(result); setError(undefined); } } catch (cause) { if (!scope.closed) { setValue(undefined); setExtra([]); setError(cause); } } finally { flight = false; } }
    void refresh(); const timer = setInterval(() => void refresh(), 3000); return () => { scope.close(); clearInterval(timer); };
  }, [id, revision]);
  const run = value?.run;
  const events = [...new Map([...(value?.items ?? []), ...extra].map(event => [event.id, event])).values()].sort((a, b) => a.seq - b.seq), after = events.at(-1)?.seq ?? 0;
  async function more() { const scope = currentScope.current; if (!scope || scope.closed || moreBusy) return; setMoreBusy(true); try { const next = await client.request<Schemas['RunEventPage']>('/runs/' + identifier(id) + '/event-log?after_seq=' + after + '&limit=100', { scope }); if (!scope.closed) setExtra(old => [...old, ...next.items]); } catch (cause) { if (!scope.closed) { setValue(undefined); setExtra([]); setError(cause); } } finally { if (!scope.closed) setMoreBusy(false); } }
  return <Section title='任务当前进展'><View className='cp-pad'><Notice error={error} />{!run ? !error && <Busy /> : <><Fact label='当前阶段'><Text>{runName(run)}</Text></Fact><Fact label='本次最长处理时刻'><Text>{date(run.expires_at)}</Text></Fact>
    {run.post_write&&<PostWriteReceipt run={run}/>}{run.oa_request&&<OARequestReceipt run={run}/>}{run.mutation&&<MutationReceipt value={run.mutation}/>}{run.file_update&&<FileUpdateReceipt value={run.file_update} stopped={!!run.cancel_requested_at||!['queued','running','awaiting_approval','committing'].includes(run.state)}/>}{run.file_create&&<FileUpdateReceipt value={run.file_create} stopped={!!run.cancel_requested_at||!['queued','running','awaiting_approval','committing'].includes(run.state)}/>}{run.contract_body&&<ContractBodyReview run={run}/>}
    {run.cancel_requested_at && <Text className='cp-notice cp-paragraph'>{run.state === 'cancelled' ? '已确认取消；已完成的历史业务事实仍保留。' : '已请求取消，尚未确认执行停止；请以实际任务和业务结果为准。'}</Text>}
    {run.state === 'committing' && <Text className='cp-notice cp-paragraph'>正在核对正式提交结果，不能将此阶段宣称为已撤销。</Text>}
    {events.map(event => <Fact key={event.id} label={event.seq + ' · ' + eventNames[event.type]}><Text>{date(event.created_at)}</Text>{event.safe_code && <Text className='cp-code'>{event.safe_code}</Text>}</Fact>)}
    {after < run.last_event_seq && <Action secondary loading={moreBusy} onClick={() => void more()}>继续读取后续事件</Action>}
    {run.can_cancel && (cancelling ? <CancelRun key={run.id + ':' + run.version} run={run} me={me} onCommitted={() => { setCancelling(false); setRevision(value => value + 1); onCommitted(); }} /> : <Action secondary onClick={() => setCancelling(true)}>明确取消这个任务</Action>)}
  </>}<Action secondary onClick={() => setRevision(value => value + 1)}>重新读取任务结果</Action><Action secondary onClick={onClose}>收起任务进度</Action></View></Section>;
}
function CancelRun({ run, me, onCommitted }: { run: Schemas['Run']; me: Schemas['Me']; onCommitted(): void }) {
  const draft = useFormDraft(me, { kind: 'run-cancel', id: run.id, version: run.version }, 'cancel'), command = useCommand<Schemas['Run']>(() => { draft.forget(); onCommitted(); }, draft.binding, value => value.id === run.id && value.version === run.version + 1 && !!value.cancel_requested_at && value.state === (run.state === 'running' ? 'running' : 'cancelled'));
  return <><Notice error={command.error} /><CommandNotice blocked={command.blocked} /><TextField name='reason' label='取消任务的原因' value={draft.fields.reason} onChange={reason => draft.update({ reason })} disabled={command.busy || command.blocked} /><Action loading={command.busy} disabled={command.blocked || !draft.fields.reason.trim()} onClick={() => void command.submit('/runs/' + run.id + '/cancel', { expected_version: run.version, action: 'cancel', reason: draft.fields.reason })}>确认请求取消</Action></>;
}
