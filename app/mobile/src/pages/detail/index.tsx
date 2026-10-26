import { Text, View } from '@tarojs/components';
import { useRouter } from '@tarojs/taro';
import { client } from '../../platform/taro';
import { MobileError, type Page as ResultPage, type Schemas } from '../../platform/session';
import { browse, go, identifier, projectSection, readVersion, usePage } from '../../platform/page';
import { Action, Busy, Empty, Fact, Notice, Page, Row, Section, State, bytes, date, dateOnly } from '../../ui';
import { DecisionForm } from './mutations';
import { ProjectResource, TaskCollaboration } from '../project/resources';
import { RequestSnapshot } from './snapshot';
import { RequestActions } from '../business/request-actions';
import { ProjectSetupEntry } from '../project/setup';
import { UploadEntry } from '../../platform/upload-entry';

type Detail = { kind: 'project'; item: Schemas['Project'] } | { kind: 'milestone'; item: Schemas['Milestone'] } | { kind: 'risk'; item: Schemas['Risk'] } | { kind: 'issue'; item: Schemas['Issue'] } | { kind: 'task'; item: Schemas['Task'] } | { kind: 'folder'; item: Schemas['Folder'] } | { kind: 'file'; item: Schemas['File']; versions: ResultPage<Schemas['FileVersion']>; pinned: boolean } | { kind: 'post'; item: Schemas['Post'] } | { kind: 'request'; item: Schemas['Request'] } | { kind: 'conversation'; item: Schemas['Conversation'] };
export default function DetailPage() {
  const params = useRouter().params;
  const page = usePage<Detail>(async scope => {
    const id = identifier(params.id), options = { scope };
    switch (params.kind) {
      case 'project': return { kind: 'project', item: await client.request<Schemas['Project']>('/projects/' + id, options) };
      case 'milestone': return { kind: 'milestone', item: await client.request<Schemas['Milestone']>('/milestones/' + id, options) };
      case 'risk': return { kind: 'risk', item: await client.request<Schemas['Risk']>('/risks/' + id, options) };
      case 'issue': return { kind: 'issue', item: await client.request<Schemas['Issue']>('/issues/' + id, options) };
      case 'task': return { kind: 'task', item: await client.request<Schemas['Task']>('/tasks/' + id, options) };
      case 'folder': return { kind: 'folder', item: await client.request<Schemas['Folder']>('/folders/' + id, options) };
      case 'file': {
        const item = await client.request<Schemas['File']>('/files/' + id, options);
        if (params.version) {
          const version = await client.request<Schemas['FileVersion']>('/files/' + id + '/versions/' + identifier(params.version), options);
          return { kind: 'file', item, versions: { items: [version], next_cursor: null }, pinned: true };
        }
        const versions = await client.request<ResultPage<Schemas['FileVersion']>>('/files/' + id + '/versions?limit=20', options);
        return { kind: 'file', item, versions, pinned: false };
      }
      case 'post': return { kind: 'post', item: await client.request<Schemas['Post']>('/posts/' + id, options) };
      case 'request': return { kind: 'request', item: await client.request<Schemas['Request']>('/requests/' + id, options) };
      case 'conversation': return { kind: 'conversation', item: await client.request<Schemas['Conversation']>('/conversations/' + id, options) };
      default: throw new MobileError('INVALID_RESOURCE', '资料地址无效，请从列表重新打开。');
    }
  });
  const value = page.result?.value, item = value?.item;
  return <Page title={item ? 'name' in item ? item.name : item.title : '资料详情'} home><Notice error={page.error} />{page.loading ? <Busy /> : value && <><View className='cp-choice'><State value={value.item.state} /><Text className='cp-muted cp-small'>修订 {value.item.version} · {date(value.item.updated_at)}</Text></View>
    {value.kind === 'project' && <><ProjectSetupEntry project={value.item} me={page.result!.me} /><Section title='项目范围'><View className='cp-pad'><Text className='cp-paragraph'>{value.item.scope || '暂未填写范围。'}</Text><Text className='cp-muted cp-small'>{value.item.project_no}</Text></View><Fact label='项目开始日期'><Text>{dateOnly(value.item.start_date)}</Text></Fact><Fact label='计划交付日期'><Text>{dateOnly(value.item.due_date)}</Text></Fact></Section><Section title='项目资料'><Row title='项目业务概览' subtitle='当前可读业务、发生支出与预算' onClick={() => projectSection(value.item.id, 'overview')} /><Row title='项目变更' onClick={() => projectSection(value.item.id, 'changes')} /><Row title='预算修订' onClick={() => projectSection(value.item.id, 'budgets')} /><Row title='项目任务' onClick={() => browse('tasks', { project_id: value.item.id })} /><Row title='里程碑' onClick={() => projectSection(value.item.id, 'milestones')} /><Row title='风险' onClick={() => projectSection(value.item.id, 'risks')} /><Row title='问题' onClick={() => projectSection(value.item.id, 'issues')} /><Row title='项目成员' onClick={() => projectSection(value.item.id, 'members')} /><Row title='项目交付与验收' onClick={() => projectSection(value.item.id, 'deliverables')} /><Row title='项目文件' onClick={() => browse('files', { project_id: value.item.id })} /><Row title='项目记录' onClick={() => browse('posts', { project_id: value.item.id })} /></Section></>}
    {value.kind === 'task' && <><Section title='任务内容'><View className='cp-pad'><Text className='cp-paragraph'>{value.item.body || '暂未填写说明。'}</Text></View><Fact label='开始时间'><Text>{date(value.item.start_at)}</Text></Fact><Fact label='截止时间'><Text>{date(value.item.due_at)}</Text></Fact><Fact label='完成要求'><Text>{value.item.review_required ? '需经过审核' : '按项目要求完成'}</Text></Fact></Section><TaskCollaboration key={value.item.id + ':' + value.item.version} task={value.item} me={page.result!.me} onCommitted={() => void page.reload()} /><Action secondary onClick={() => go('project', value.item.project_id)}>查看所属项目</Action></>}
    {(value.kind === 'milestone' || value.kind === 'risk' || value.kind === 'issue') && <ProjectResource key={value.item.id + ':' + value.item.version + ':' + page.result!.me.authorization_version} kind={value.kind} item={value.item} me={page.result!.me} onCommitted={() => void page.reload()} />}
    {value.kind === 'folder' && <Section title='目录内容'><Row title='查看子目录' onClick={() => browse('folders', { folder_id: value.item.id })} /><Row title='查看目录文件' onClick={() => browse('files', { folder_id: value.item.id })} /><UploadEntry folder={value.item} /></Section>}
    {value.kind === 'file' && !value.pinned && <UploadEntry file={value.item} />}
    {value.kind === 'file' && <><Section title={value.pinned ? '本次引用的固定版本' : '正式版本'}>{value.versions.items.length ? value.versions.items.map(version => <View key={version.id}><Fact label={'版本 ' + version.seq + (version.id === value.item.current_version_id ? ' · 当前' : ' · 历史')}><Text>{version.original_name ?? '原文件名未提供'}</Text><Text className='cp-muted cp-small'>{bytes(version.size_bytes)} · {date(version.created_at)}</Text><View className='cp-choice'><State value={version.availability} /><State value={version.scan_state} /></View><Text className='cp-muted cp-small'>SHA-256</Text><Text className='cp-code' selectable>{version.sha256}</Text><Action secondary onClick={() => readVersion(value.item.id, version.id)}>查阅这个版本的内容</Action>{version.replicas.map(replica => <View key={replica.id} className='cp-paragraph'><Text>{replica.provider === 'oss' ? 'OSS 原件' : 'R2 副本'} · </Text><State value={replica.state} /></View>)}</Fact></View>) : <Empty>此文件还没有正式版本。</Empty>}</Section>{value.versions.next_cursor && <Text className='cp-notice'>当前显示最近20个版本。</Text>}{value.pinned && <Action secondary onClick={() => go('file', value.item.id)}>查看文件记录与历史</Action>}</>}
    {value.kind === 'post' && <><View className='cp-notice'><Text>{value.item.has_unpublished_changes ? '当前有未发布修改，请按你看到的修订核对。' : value.item.state === 'published' ? '当前为已发布内容。' : '当前为未发布草稿。'}</Text></View><Section title='内容'><View className='cp-pad'><Text className='cp-paragraph' selectable>{value.item.body || '暂无正文。'}</Text></View></Section></>}
    {value.kind === 'request' && <><RequestSnapshot request={value.item} me={page.result!.me} /><RequestActions key={value.item.id + ":" + value.item.version} request={value.item} me={page.result!.me} onCommitted={() => void page.reload()} /><Section title='审批顺序'>{value.item.steps.length ? value.item.steps.map(step => <View key={step.id} className='cp-pad'><Text className='cp-row-title'>{step.seq} · {step.title}</Text><Text className='cp-muted cp-small'>{step.assigned_user_name}{!step.assigned_user_active && ' · 账号已停用'}</Text><State value={step.state} />{step.decision_reason && <Text className='cp-paragraph'>{step.decision_reason}</Text>}{step.can_decide && <DecisionForm key={step.id + ':' + step.version} request={value.item} step={step} me={page.result!.me} onCommitted={() => void page.reload()} />}</View>) : <Empty>此申请尚未进入审批。</Empty>}</Section></>}
    {value.kind === 'conversation' && <><Section title='会话成员'>{value.item.members.map(member => <Fact key={member.user_id} label={member.is_creator ? '发起人' : '已明确加入'}><Text>{member.display_name}{!member.available && ' · 当前不可用'}</Text></Fact>)}</Section><Action onClick={() => go('conversation', value.item.id)}>打开会话消息与任务</Action></>}
  </>}<Action secondary onClick={() => void page.reload()} loading={page.loading}>刷新资料</Action></Page>;
}
