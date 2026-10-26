import { Text, View } from '@tarojs/components';
import Taro, { useRouter } from '@tarojs/taro';
import { useState } from 'react';
import { client } from '../../platform/taro';
import { MobileError } from '../../platform/session';
import { go, identifier, usePage } from '../../platform/page';
import { businessNames, businessRoutes, currentBusinessPayload, type Business, type BusinessKind } from '../../platform/business-input';
import { Action, Busy, Empty, Fact, Notice, Page, Section, State, date } from '../../ui';
import { PayloadSnapshot } from '../detail/snapshot';
import { BusinessEditor } from './editor';
import { PrepareRequest } from './request-actions';

export default function BusinessPage() {
  const params = useRouter().params, [mode, setMode] = useState<'view' | 'edit' | 'prepare'>('view');
  const page = usePage(async scope => {
    if (!['contract', 'purchase', 'claim'].includes(params.kind ?? '')) throw new MobileError('INVALID_RESOURCE', '业务地址无效，请从业务列表重新打开。');
    const kind = params.kind as BusinessKind;
    const value = params.id ? await client.request<Business>(businessRoutes[kind] + '/' + identifier(params.id), { scope }) : undefined;
    if (value && (kind === 'contract' ? !('contract_no' in value) : !('kind' in value) || value.kind !== kind)) throw new MobileError('INVALID_RESPONSE', '业务类型与资料地址不一致，请重新查阅。');
    return { kind, value };
  });
  const result = page.result, kind = result?.value.kind, value = result?.value.value;
  const editing = mode === 'edit' || !value, name = kind ? businessNames[kind] : '业务';
  function committed(item: Business) {
    setMode('view');
    if (!value) void Taro.redirectTo({ url: '/pages/business/index?kind=' + kind + '&id=' + identifier(item.id) });
    else void page.reload();
  }
  return <Page title={editing ? (value ? '编辑' : '起草') + name : value.title} eyebrow='BUSINESS / EVERY DETAIL COUNTS' home>
    <Notice error={page.error} />{page.loading ? <Busy /> : result && kind && <>
      {editing ? (value ? value.can_edit : result.me.actions.includes(kind + '.create')) ? <BusinessEditor key={(value?.id ?? kind) + ':' + (value?.version ?? 'new') + ':' + result.me.authorization_version} kind={kind} value={value} me={result.me} onCommitted={committed} onCancel={() => { if (value) setMode('view'); else void Taro.navigateBack().catch(() => go(kind === 'contract' ? 'contracts' : kind === 'purchase' ? 'purchases' : 'claims')); }} /> : <Empty>当前权限或业务阶段不允许起草或编辑。</Empty>
        : value && <>
          <View className='cp-choice'><State value={value.approval_state} /><Text className='cp-muted cp-small'>业务修订 {value.version} · {date(value.updated_at)}</Text></View>
          <View className='cp-notice'><Text>以下为当前业务修订。业务资料、固定申请、审批、履行及实际付款分别记录。</Text></View>
          <PayloadSnapshot snapshot={currentBusinessPayload(kind, value)} digest={value.content_hash} me={result.me} heading='当前业务修订内容' />
          {mode === 'prepare' ? <PrepareRequest key={value.id + ':' + value.version} kind={kind} business={value} me={result.me} onCancel={() => setMode('view')} /> : <Section title='后续操作'><View className='cp-pad'>
            {value.can_edit && <Action secondary onClick={() => setMode('edit')}>编辑{name}草稿</Action>}
            {value.can_prepare_request && <Action onClick={() => setMode('prepare')}>{value.request_id ? '核对并准备申请草稿' : '建立申请草稿'}</Action>}
            {value.request_id && <Action secondary onClick={() => go('request', value.request_id!)}>查看关联申请与进展</Action>}
            {!value.can_edit && !value.can_prepare_request && <Text className='cp-muted cp-small'>当前业务阶段或权限不提供编辑与准备申请操作。</Text>}
          </View></Section>}
          <Section title='记录标识'><Fact label='当前内容摘要'><Text className='cp-code'>{value.content_hash}</Text></Fact><Fact label='当前业务阶段'><Text>{'contract_no' in value ? ({ draft: '草稿', pending_signature: '待签署', effective: '已生效', completed: '已完成', terminated: '已终止' }[value.lifecycle]) : ({ draft: '草稿', requested: '已进入申请', ordering: '下单中', receiving: '收货中', cancelled: '已取消', completed: '已完成' }[value.lifecycle])}</Text></Fact></Section>
        </>}
      <Action secondary onClick={() => go(kind === 'contract' ? 'contracts' : kind === 'purchase' ? 'purchases' : 'claims')}>返回{name}列表</Action>
    </>}
    <Action secondary loading={page.loading} onClick={() => { setMode('view'); void page.reload(); }}>重新读取业务资料</Action>
  </Page>;
}
