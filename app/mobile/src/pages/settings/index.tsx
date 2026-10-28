import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useState } from 'react';
import { client } from '../../platform/taro';
import { usePage } from '../../platform/page';
import { chooseTheme, useAppearance, type Preference } from '../../platform/theme';
import { Action, Busy, Fact, Notice, Page, Section, date } from '../../ui';
import type { Schemas } from '../../platform/session';
import { AccountSettings } from './security';
export default function SettingsPage() {
  const page = usePage(async scope => client.request<Schemas['DeviceSessionListResponse']['data']>('/me/sessions?limit=20', { scope })), me = page.result?.me;
  const { preference } = useAppearance(), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false);
  async function logout() {
    if (busy) return;
    const answer = await Taro.showModal({ title: '退出工作空间', content: '本机登录资料会被清除。是否退出？', confirmText: '退出', cancelText: '继续工作' }); if (!answer.confirm) return;
    setBusy(true); try { await client.logout(); } catch (cause) { setError(cause); try { await Taro.showModal({ title: '已退出本机', content: '服务器会话撤销结果尚未确认。你可以重新登录并在会话管理中核对。', showCancel: false }); } catch { /* Local credentials are already cleared. */ } } finally { setBusy(false); void Taro.reLaunch({ url: '/pages/login/index' }); }
  }
  return <Page title='我的工作空间' eyebrow='YOUR SPACE'><Notice error={page.error ?? error} />{page.loading ? <Busy /> : me && <Section title='登录资料'><Fact label='当前成员'><Text>{me.user.display_name}</Text></Fact><Fact label='登录有效期'><Text>{date(me.session_expires_at)}</Text></Fact><Fact label='最近两步验证'><Text>{date(me.mfa_verified_at)}</Text></Fact></Section>}<Section title='秘书用量历史'><View className='cp-pad'><Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/budget/index' })}>查看只读历史账本</Action></View></Section><Section title='TG请求与群通知'><View className='cp-pad'><Action secondary onClick={() => void Taro.navigateTo({ url: '/pages/telegram/index' })}>查看本人TG请求与投递回执</Action></View></Section><Section title='外观'><View className='cp-pad'><Text className='cp-muted cp-small'>让工作空间适应你的使用习惯。</Text><View className='cp-choice'>{([['system', '跟随系统'], ['light', '浅色'], ['dark', '深色']] as [Preference, string][]).map(([value, label]) => <View key={value} className={preference === value ? 'cp-selected' : ''}><Action secondary onClick={() => chooseTheme(value)}>{label}</Action></View>)}</View></View></Section>{me && page.result && <AccountSettings initial={page.result.value} me={me} onReload={page.reload} />}<Action secondary onClick={() => { void Taro.navigateBack().catch(() => Taro.reLaunch({ url: '/pages/home/index' })); }}>返回上一页</Action><Action secondary onClick={() => void logout()} loading={busy}>退出登录</Action></Page>;
}
