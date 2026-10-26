import { useCallback, useEffect, useRef, useState } from 'react';
import Taro, { useDidHide, useDidShow } from '@tarojs/taro';
import { MobileError, Scope, type Schemas } from './session';
import { client } from './taro';
import { financialURL } from './project-overview';
export function usePage<T>(loader: (scope: Scope, me: Schemas['Me']) => Promise<T>) {
  const ref = useRef(loader); ref.current = loader;
  const active = useRef(false), redirecting = useRef(false), scope = useRef<Scope | null>(null);
  const [result, setResult] = useState<{ me: Schemas['Me']; value: T }>();
  const [error, setError] = useState<unknown>(), [loading, setLoading] = useState(true);
  const close = useCallback(() => { scope.current?.close(); scope.current = null; setResult(undefined); setError(undefined); }, []);
  const requireLogin = useCallback(() => {
    if (!active.current || redirecting.current) return;
    redirecting.current = true;
    void Taro.reLaunch({ url: '/pages/login/index' }).catch(() => {
      redirecting.current = false;
      if (active.current) { setLoading(false); setError(new MobileError('LOGIN_REQUIRED', '登录状态已失效，请重新打开登录页面。')); }
    });
  }, []);
  const reload = useCallback(async () => {
    close(); if (!active.current) return; setLoading(true);
    if (!client.authenticated) { requireLogin(); return; }
    const current = new Scope(); scope.current = current;
    try {
      const me = await client.request<Schemas['Me']>('/me', { scope: current });
      const value = await ref.current(current, me);
      if (!current.closed && active.current) setResult({ me, value });
    } catch (cause) {
      if (active.current && !client.authenticated) requireLogin();
      else if (!current.closed && active.current) setError(cause);
    } finally { if (!current.closed && active.current) setLoading(false); }
  }, [close, requireLogin]);
  useDidShow(() => { active.current = true; redirecting.current = false; void reload(); });
  useDidHide(() => { active.current = false; close(); });
  useEffect(() => { const unsubscribe = client.subscribe(() => { if (!client.authenticated) { close(); requireLogin(); } }); return () => { active.current = false; scope.current?.close(); unsubscribe(); }; }, [close, requireLogin]);
  return { result, error, loading, reload, scope: scope.current };
}
export function go(kind: string, id?: string) {
  if (kind === 'new-project') { void Taro.navigateTo({ url: '/pages/project/index?section=setup' }); return; }
  if (id && ['cost', 'payment', 'subscription', 'change'].includes(kind)) { void Taro.navigateTo({ url: financialURL(kind as 'cost' | 'payment' | 'subscription' | 'change', identifier(id)) }); return; }
  if (kind === 'notifications') { void Taro.navigateTo({ url: '/pages/notifications/index' }); return; }
  if (kind === 'conversations' || kind === 'conversation') { void Taro.navigateTo({ url: '/pages/secretary/index?kind=' + (id ? 'conversation&id=' + identifier(id) : 'list') }); return; }
  if (['services', 'service', 'incidents', 'incident', 'new-incident'].includes(kind)) { void Taro.navigateTo({ url: '/pages/status/index?kind=' + kind + (id ? '&id=' + identifier(id) : '') }); return; }
  if (['contract', 'purchase', 'claim'].includes(kind)) { void Taro.navigateTo({ url: '/pages/business/index?kind=' + kind + (id ? '&id=' + identifier(id) : '') }); return; }
  if (kind === 'deliverable' && id) { void Taro.navigateTo({ url: '/pages/deliverable/index?id=' + identifier(id) }); return; }
  void Taro.navigateTo({ url: '/pages/' + (id ? 'detail' : 'list') + '/index?kind=' + encodeURIComponent(kind) + (id ? '&id=' + encodeURIComponent(id) : '') });
}
export function projectSection(id: string, section: 'setup' | 'overview' | 'milestones' | 'risks' | 'issues' | 'members' | 'deliverables' | 'changes' | 'budgets') {
  void Taro.navigateTo({ url: '/pages/project/index?id=' + identifier(id) + '&section=' + section });
}
export function projectResourceForm(kind: 'milestone' | 'risk' | 'issue', projectID: string, resourceID?: string) {
  void Taro.navigateTo({ url: '/pages/project/index?section=resource-form&kind=' + kind + '&id=' + identifier(projectID) + (resourceID ? '&resource_id=' + identifier(resourceID) : '') });
}
export function goVersion(fileID: string, versionID: string) {
  void Taro.navigateTo({ url: '/pages/detail/index?kind=file&id=' + identifier(fileID) + '&version=' + identifier(versionID) });
}
export function readVersion(fileID: string, versionID: string) {
  void Taro.navigateTo({ url: '/pages/content/index?id=' + identifier(fileID) + '&version=' + identifier(versionID) });
}
export function browse(kind: string, scope?: { project_id?: string; folder_id?: string }) {
  const suffix = Object.entries(scope ?? {}).filter(([, value]) => value).map(([key, value]) => '&' + key + '=' + encodeURIComponent(value!)).join('');
  void Taro.navigateTo({ url: '/pages/list/index?kind=' + encodeURIComponent(kind) + suffix });
}
export function identifier(input: unknown): string {
  if (typeof input !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(input)) throw new MobileError('INVALID_RESOURCE', '资料地址无效，请从列表重新打开。');
  return input.toLowerCase();
}
