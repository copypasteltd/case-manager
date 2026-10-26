import { Text, View } from '@tarojs/components';
import { Input } from '../../platform/components';
import { useDidHide, useRouter } from '@tarojs/taro';
import { useRef, useState } from 'react';
import { client } from '../../platform/taro';
import { MobileError, type Page as ResultPage, type Schemas } from '../../platform/session';
import { go, identifier, usePage } from '../../platform/page';
import { Action, Busy, Empty, Field, Notice, Page, Row } from '../../ui';
import { ProjectTaskList } from '../project/task-forms';
import { KnowledgeList } from '../knowledge';

type Item = Schemas['Project'] | Schemas['Task'] | Schemas['Folder'] | Schemas['File'] | Schemas['Post'] | Schemas['Request'] | Schemas['ApprovalTask'] | Schemas['Conversation'] | Schemas['Contract'] | Schemas['Spend'] | Schemas['Cost'] | Schemas['Payment'] | Schemas['Subscription'];
const lists = { projects: ['项目', '/projects', 'project'], tasks: ['项目任务', '', 'task'], folders: ['文件目录', '/folders', 'folder'], files: ['文件', '/files', 'file'], posts: ['知识与记录', '/posts', 'post'], requests: ['业务申请', '/requests', 'request'], approvals: ['待我审批', '/approval-tasks', 'request'], conversations: ['公司秘书', '/conversations', 'conversation'], contracts: ['合同', '/contracts', 'contract'], purchases: ['采购', '/purchases', 'purchase'], claims: ['报销', '/claims', 'claim'], costs: ['实际费用', '/costs', 'cost'], payments: ['收付款', '/payments', 'payment'], subscriptions: ['订阅', '/subscriptions', 'subscription'] } as const;
type Kind = keyof typeof lists;
export default function ListPage() {
  const params = useRouter().params;
  return params.kind === 'tasks' ? <ProjectTaskList projectID={params.project_id} /> : params.kind === 'posts' ? <KnowledgeList projectID={params.project_id} /> : <RegularListPage />;
}
function RegularListPage() {
  const params = useRouter().params, kind = params.kind as Kind, entry = Object.prototype.hasOwnProperty.call(lists, kind) ? lists[kind] : undefined;
  const [search, setSearch] = useState(''), query = useRef('');
  const [extra, setExtra] = useState<Item[]>([]), [cursor, setCursor] = useState<string | null>(), [moreBusy, setMoreBusy] = useState(false), [moreError, setMoreError] = useState<unknown>();
  function path(next?: string) {
    if (!entry) throw new MobileError('INVALID_RESOURCE', '列表地址无效，请返回工作台。');
    const project = params.project_id ? identifier(params.project_id) : null, folder = params.folder_id ? identifier(params.folder_id) : null;
    if (kind === 'tasks' && !project) throw new MobileError('INVALID_RESOURCE', '请先选择项目。');
    return (kind === 'tasks' ? '/projects/' + project + '/tasks' : entry[1]) + '?limit=20' + (query.current ? '&q=' + encodeURIComponent(query.current) : '') + (next ? '&cursor=' + encodeURIComponent(next) : '') + (project && kind !== 'tasks' ? '&project_id=' + project : '') + (folder && kind === 'folders' ? '&parent_id=' + folder : folder && kind === 'files' ? '&folder_id=' + folder : '');
  }
  const page = usePage(async scope => { setExtra([]); setCursor(undefined); setMoreError(undefined); setMoreBusy(false); return client.request<ResultPage<Item>>(path(), { scope }); });
  useDidHide(() => { setExtra([]); setCursor(undefined); setMoreError(undefined); });
  const first = !page.error && !moreError ? page.result?.value : undefined, items = first ? [...first.items, ...extra] : [], next = cursor === undefined ? first?.next_cursor : cursor;
  async function more() {
    const scope = page.scope; if (!next || !scope || scope.closed || moreBusy) return; setMoreBusy(true); setMoreError(undefined);
    try { const value = await client.request<ResultPage<Item>>(path(next), { scope }); if (!scope.closed) { setExtra(old => [...old, ...value.items]); setCursor(value.next_cursor); } }
    catch (cause) { if (!scope.closed) setMoreError(cause); }
    finally { if (!scope.closed) setMoreBusy(false); }
  }
  return <Page title={entry?.[0] ?? '资料列表'} home><Field label='查找资料'><Input ariaLabel='查找资料' className='cp-input' value={search} maxlength={200} placeholder='按标题或名称查找' placeholderClass='cp-placeholder' onInput={event => setSearch(event.detail.value)} onConfirm={() => { query.current = search.trim(); void page.reload(); }} /><Action secondary loading={page.loading} onClick={() => { query.current = search.trim(); void page.reload(); }}>查找 / 刷新</Action></Field><Notice error={page.error ?? moreError} />{page.result && ['contracts', 'purchases', 'claims'].includes(kind) && page.result.me.actions.includes(entry![2] + '.create') && <Action onClick={() => go(entry![2])}>{'起草' + entry![0]}</Action>}{page.result && kind === 'projects' && page.result.me.actions.includes('project.create') && <Action onClick={() => go('new-project')}>新建项目</Action>}{page.loading ? <Busy /> : first && <><Text className='cp-muted cp-small'>仅显示你当前有权访问的资料。</Text><View className='cp-panel cp-section'>{items.length ? items.map(item => {
    const title = 'request_title' in item ? item.request_title : 'name' in item ? item.name : 'product' in item ? item.product : item.title;
    const subtitle = 'request_title' in item ? '步骤 ' + item.seq + ' · ' + item.title : 'project_no' in item ? item.project_no : 'classification' in item ? ({ internal: '公司内部', project_restricted: '项目受限', confidential: '机密' }[item.classification ?? 'internal']) : undefined;
    return <Row key={item.id} title={title} subtitle={subtitle} state={'state' in item ? item.state : item.approval_state} onClick={() => go(entry![2], 'request_title' in item ? item.request_id : item.id)} />;
  }) : <Empty>这里暂时没有符合条件的资料。</Empty>}</View>{next && <Action secondary onClick={() => void more()} loading={moreBusy}>加载更多</Action>}</>}</Page>;
}
