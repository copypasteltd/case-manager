import { CheckboxGroup, Image, Label, Text, View } from '@tarojs/components';
import { Button, Checkbox, semantic } from './platform/components';
import Taro from '@tarojs/taro';
import { useEffect, type PropsWithChildren, type ReactNode } from 'react';
import tokens from '@copypaste/design/tokens.json';
import { MobileError } from './platform/session';
import { useAppearance } from './platform/theme';
import light from './assets/logo-light.png';
import dark from './assets/logo-dark.png';

export function Page({ children, title, eyebrow = 'COPYPASTE / WORKSPACE', home = false }: PropsWithChildren<{ title: string; eyebrow?: string; home?: boolean }>) {
  const { theme } = useAppearance();
  useEffect(() => {
    try {
      void Taro.setNavigationBarColor({ frontColor: theme === 'dark' ? '#ffffff' : '#000000', backgroundColor: tokens.themes[theme]['bg.canvas'] }).catch(() => {});
      if (process.env.TARO_ENV !== 'h5') void Taro.setBackgroundColor({ backgroundColor: tokens.themes[theme]['bg.canvas'] }).catch(() => {});
    } catch { /* Appearance controls remain available on platforms without a native color API. */ }
  }, [theme]);
  return <View className={'cp-page cp-theme-' + theme}><View className='cp-brand'><Image className='cp-logo' ariaHidden src={theme === 'dark' ? dark : light} mode='aspectFit' /><Text className='cp-brand-name'>copypaste<Text className='cp-accent'>.</Text></Text>{home && <Button className='cp-link cp-settings' onClick={() => void Taro.navigateTo({ url: '/pages/settings/index' })}>我的</Button>}</View><View className='cp-heading'><Text className='cp-eyebrow'>{eyebrow}</Text><Text className='cp-title' {...semantic('heading')}>{title}<Text className='cp-accent'>.</Text></Text></View>{children}<Text className='cp-footer'>COPYPASTE / 一起创造，更好的下一步。</Text></View>;
}
export function Notice({ error }: { error: unknown }) { return error ? <View className='cp-notice cp-danger'><Text>{error instanceof MobileError ? error.message : '暂时无法读取，请稍后重试。'}</Text>{error instanceof MobileError && error.requestID && <Text className='cp-muted cp-small'>查询编号 {error.requestID}</Text>}</View> : null; }
export function Busy() { return <View className='cp-empty'><Text>正在读取资料…</Text></View>; }
export function Empty({ children }: PropsWithChildren) { return <View className='cp-empty'><Text>{children}</Text></View>; }
export function Check({ children, checked, disabled = false, onChange }: PropsWithChildren<{ checked: boolean; disabled?: boolean; onChange(value: boolean): void }>) {
  const { theme } = useAppearance();
  return <CheckboxGroup onChange={event => onChange(event.detail.value.includes('confirmed'))}><Label className='cp-check'><Checkbox ariaLabel={typeof children === 'string' ? children : undefined} color={tokens.themes[theme]['accent.fg']} value='confirmed' checked={checked} disabled={disabled} /><Text>{children}</Text></Label></CheckboxGroup>;
}
export function Action({ children, onClick, disabled = false, secondary = false, loading = false }: { children: ReactNode; onClick(): void; disabled?: boolean; secondary?: boolean; loading?: boolean }) { return <Button className={'cp-button ' + (secondary ? 'cp-secondary' : 'cp-primary')} disabled={disabled || loading} loading={loading} onClick={onClick}>{children}</Button>; }
export function Row({ title, subtitle, state, onClick }: { title: string; subtitle?: string; state?: string; onClick(): void }) { return <Button className='cp-row' onClick={onClick}><View className='cp-row-copy'><Text className='cp-row-title'>{title}</Text>{subtitle && <Text className='cp-muted cp-small'>{subtitle}</Text>}</View>{state && <State value={state} />}<Text className='cp-row-arrow'>↗</Text></Button>; }
const names: Record<string, string> = { resource_attention: '资源待核对', resource_record: '资源进展记录', not_submitted: '尚未送审', submitted: '待人工验收', accepted: '已验收', changes_requested: '已退回，待修订', open: '待处理', achieved: '已达成', missed: '未按期达成', mitigating: '缓解中', closed: '已关闭', realized: '已发生', investigating: '调查中', resolved: '已解决', quarantined: '已隔离', retiring: '原件清理中', purged: '原件已清理', draft: '草稿', active: '进行中', on_hold: '暂缓', closing: '结项中', archived: '已归档', cancelled: '已取消', todo: '待开始', in_progress: '进行中', in_review: '审核中', done: '已完成', blocked: '受阻', published: '已发布', approved: '已批准', rejected: '已拒绝', returned: '已退回', withdrawn: '已撤回', pending: '待前序', verified: '已核实', available: '可用', unavailable: '不可用', clean: '扫描通过', waived: '已批准例外', unsupported: '不支持扫描', unscannable: '无法扫描', queued: '排队中', copying: '复制中', verifying: '核对中', retry_wait: '等待重试', failed: '失败', missing: '缺失' };
export function State({ value }: { value: string }) { const tone = ['done', 'approved', 'verified', 'clean', 'accepted', 'achieved', 'resolved'].includes(value) ? 'success' : ['rejected', 'failed', 'missing', 'unavailable', 'missed', 'quarantined'].includes(value) ? 'danger' : ['blocked', 'returned', 'unsupported', 'unscannable', 'changes_requested', 'realized', 'submitted'].includes(value) ? 'warning' : ['active', 'in_progress', 'in_review', 'published'].includes(value) ? 'info' : 'neutral'; return <Text className={'cp-state cp-state-' + tone}>{names[value] ?? value}</Text>; }
export function Section({ title, children }: PropsWithChildren<{ title: string }>) { return <View className='cp-section'><Text className='cp-section-title'>{title}</Text><View className='cp-panel'>{children}</View></View>; }
export function Fact({ label, children }: PropsWithChildren<{ label: string }>) { return <View className='cp-fact'><Text className='cp-muted cp-small'>{label}</Text><View className='cp-fact-value'>{children}</View></View>; }
export function Field({ label, children, hint }: PropsWithChildren<{ label: string; hint?: string }>) { return <View className='cp-field'><Text className='cp-label'>{label}</Text>{children}{hint && <Text className='cp-muted cp-small'>{hint}</Text>}</View>; }
export function date(input: string | null | undefined) { if (!input) return '未设置'; const value = new Date(input); if (!Number.isFinite(value.getTime())) return '日期无效'; const shifted = new Date(value.getTime() + 8 * 3600000); return shifted.toISOString().slice(0, 16).replace('T', ' ') + ' · 香港'; }
export function bytes(input: string) { const n = Number(input); if (!/^\d+$/.test(input) || !Number.isSafeInteger(n)) return input + ' B'; return n >= 1073741824 ? (n / 1073741824).toFixed(2) + ' GiB' : n >= 1048576 ? (n / 1048576).toFixed(1) + ' MiB' : input + ' B'; }

export function dateOnly(input: string | null | undefined) { return !input ? '未设置' : /^\d{4}-\d{2}-\d{2}$/.test(input) ? input : '日期无效'; }
