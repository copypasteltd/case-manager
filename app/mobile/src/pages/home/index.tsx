import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { client } from '../../platform/taro';
import type { Schemas } from '../../platform/session';
import { go, usePage } from '../../platform/page';
import { Action, Busy, Empty, Notice, Page, Row, Section, date } from '../../ui';
import { notificationCount } from '@copypaste/contracts/notifications';
import { UploadEntry } from '../../platform/upload-entry';
import { checkedWorkbench, type WorkbenchSummary } from '@copypaste/contracts/workbench';
import { BusinessWorkbench } from './business';
export default function HomePage() {
  const page = usePage(async scope => {
    const [dashboard, notifications, business] = await Promise.all([client.request<Schemas['Dashboard']>('/dashboard', { scope }), client.request<Schemas['NotificationSummary']>('/notifications/summary', { scope }), client.request<WorkbenchSummary>('/dashboard/business', { scope }).then(value => ({ summary: checkedWorkbench(value), error: undefined as unknown })).catch((error: unknown) => ({ summary: undefined, error }))]);
    return { dashboard, notifications, business };
  });
  const value = page.result?.value.dashboard, me = page.result?.me, notifications = page.result?.value.notifications;
  return <Page title={me ? me.user.display_name + '，工作从这里开始' : '工作台'} eyebrow='A SPACE FOR GOOD WORK' home>
    <Notice error={page.error} />{page.loading ? <Busy /> : value && <><View className='cp-metrics'>{[['我的任务', value.my_open_tasks], ['已经逾期', value.my_overdue_tasks], ['进行中项目', value.active_projects]].map(([label, count]) => <View className='cp-metric' key={label}><Text className='cp-muted cp-small'>{label}</Text><Text className='cp-metric-number'>{count}</Text></View>)}</View><Text className='cp-muted cp-small'>截至 {date(value.as_of)}</Text><Section title='待我推进'>{value.tasks.length ? value.tasks.map(task => <Row key={task.id} title={task.title} subtitle={'截止 ' + date(task.due_at)} state={task.state} onClick={() => go('task', task.id)} />) : <Empty>当前没有分配给你的未完成任务。</Empty>}</Section></>}
    {page.result && <BusinessWorkbench value={page.result.value.business.summary} error={page.result.value.business.error} />}
    {me && <Section title='工作空间'><Row title='搜索工作资料' subtitle='名称、知识记录与当前文件正文' onClick={() => void Taro.navigateTo({ url: '/pages/search/index' })} /><Row title='Telegram 接入' subtitle='本人数字身份、群入口与请求回执' onClick={() => void Taro.navigateTo({ url: '/pages/telegram/index?section=access' })} /><Row title='项目' subtitle='查看项目范围、任务与进度' onClick={() => go('projects')} /><Row title='文件与资料' subtitle='目录、正式版本与副本状态' onClick={() => go('folders')} /><Row title='知识与记录' subtitle='项目日志、会议记录与内部知识' onClick={() => go('posts')} />{me.actions.includes('request.read') && <Row title='业务申请' subtitle='固定修订与审批进展' onClick={() => go('requests')} />}{me.actions.includes('approval.decide') && <Row title='待我审批' subtitle='核对资料后处理本人的步骤' onClick={() => go('approvals')} />}{me.actions.includes('conversation.read') && <Row title='公司秘书' subtitle='查看会话与明确加入的成员' onClick={() => go('conversations')} />}</Section>}
    {me?.actions.includes('report.read') && <Section title='财务核对'><Row title='财务报表' subtitle='发生、现金、订阅预测与限时导出' onClick={() => void Taro.navigateTo({ url: '/pages/reports/index' })} /></Section>}
    {me && <Section title='合同与费用申请'>{me.actions.includes('contract.read') && <Row title='合同' subtitle='起草、保存修订与核对送审' onClick={() => go('contracts')} />}{me.actions.includes('purchase.read') && <Row title='采购' subtitle='整理需求、明细、供应商与报价' onClick={() => go('purchases')} />}{me.actions.includes('claim.read') && <Row title='报销' subtitle='费用、垫付、凭证与申请进展' onClick={() => go('claims')} />}</Section>}
    {me && ['cost.read', 'payment.read', 'subscription.read'].some(action => me.actions.includes(action)) && <Section title='费用、资金与订阅'>{me.actions.includes('cost.read') && <Row title='实际费用' subtitle='草稿、确认、依据、分摊与冲销' onClick={() => go('costs')} />}{me.actions.includes('payment.read') && <Row title='收付款' subtitle='登记、分配、退款、释放与固定凭证' onClick={() => go('payments')} />}{me.actions.includes('subscription.read') && <Row title='订阅' subtitle='价格期间、停费核对与已生成账期' onClick={() => go('subscriptions')} />}</Section>}
    {me?.actions.includes('service.read') && <Section title='服务与运行事件'><Row title='服务状态' subtitle='当前观测、来源修订、签名与维护记录' onClick={() => go('services')} /><Row title='服务事件' subtitle='影响范围、人工处理、恢复与复盘' onClick={() => go('incidents')} /></Section>}
    {notifications && <Section title='工作通知'><Row title='通知中心' subtitle={'未读 ' + notificationCount(notifications.unread_count, notifications.count_complete) + ' · 仍需处理 ' + notificationCount(notifications.attention_count, notifications.count_complete)} onClick={() => go('notifications')} /></Section>}
    {me && <UploadEntry />}
    <Action secondary onClick={() => void page.reload()} loading={page.loading}>刷新工作台</Action>
  </Page>;
}
