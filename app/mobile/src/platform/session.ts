import type { components } from '@copypaste/contracts';
export type Schemas = components['schemas'];
export type Page<T> = { items: T[]; next_cursor: string | null };
type Login = Schemas['LoginResult'];
type Device = 'wechat' | 'douyin' | 'android';
type Method = 'GET' | 'POST' | 'PATCH';
export type WireResponse = { status: number; data: unknown };
export interface WireRequest { method: Method; path: string; headers: Record<string, string>; body?: unknown }
export interface WireCall { promise: Promise<WireResponse>; abort(): void }
export type Wire = (request: WireRequest) => WireCall;
export type MobileErrorDetail = { field: string; safe_message: string };
export class MobileError extends Error {
  constructor(readonly code: string, message: string, readonly status = 0, readonly requestID = '', readonly details: readonly MobileErrorDetail[] = []) { super(message); }
}
export class Scope {
  closed = false;
  private calls = new Set<() => void>();
  own(abort: () => void) { if (this.closed) { abort(); return () => {}; } this.calls.add(abort); return () => { this.calls.delete(abort); }; }
  close() { this.closed = true; for (const abort of this.calls) abort(); this.calls.clear(); }
}
type Envelope = { data?: unknown; meta?: { request_id?: string }; error?: { code?: string; message?: string; details?: unknown } };
type Credentials = { access: string; refresh: string; expires: number; nonce: string };

// Credentials are memory only. Native secure persistence requires a separate
// reviewed platform adapter; generic mini-program storage is never used here.
export class MobileClient {
  private credentials: Credentials | null = null;
  private generation = 0;
  private serial = 0;
  private listeners = new Set<() => void>();
  private refreshFlight: Promise<void> | null = null;
  constructor(private wire: Wire, readonly device: Device, private now = () => Date.now()) {}
  get authenticated() { return this.credentials !== null; }
  // Public lifetime marker, independent of credentials and refresh rotation.
  get epoch() { return this.generation; }
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private notify() { for (const listener of this.listeners) listener(); }
  clear() { this.generation++; this.credentials = null; this.refreshFlight = null; this.notify(); }
  operationKey() {
    if (!this.credentials) throw new MobileError('LOGIN_REQUIRED', '请先登录工作空间。');
    // The server request nonce is public, cryptographically generated and
    // independent of access/refresh secrets. A counter makes each new intent
    // unique; the form keeps this key for uncertain-result reconciliation.
    return 'mobile:' + this.device + ':' + this.credentials.nonce + ':' + (++this.serial);
  }
  private async send(request: WireRequest, scope?: Scope): Promise<Envelope> {
    if (scope?.closed) throw new MobileError('REQUEST_CANCELLED', '此页面的读取已结束。');
    const call = this.wire(request), release = scope?.own(() => call.abort());
    try {
      let response: WireResponse;
      try { response = await call.promise; } catch {
        if (scope?.closed) throw new MobileError('REQUEST_CANCELLED', '此页面的读取已结束。');
        throw new MobileError('RESULT_UNCONFIRMED', request.method === 'GET' ? '暂时无法连接，请稍后重试。' : '连接中断，操作结果尚未确认。请先刷新资料核对。');
      }
      if (scope?.closed) throw new MobileError('REQUEST_CANCELLED', '此页面的读取已结束。');
      if (!response.data || typeof response.data !== 'object' || Array.isArray(response.data)) throw new MobileError('INVALID_RESPONSE', '服务响应异常，请稍后重试。', response.status);
      const value = response.data as Envelope;
      if (response.status < 200 || response.status >= 300 || value.error) {
        const details = Array.isArray(value.error?.details) ? value.error.details.slice(0, 100).filter((detail): detail is MobileErrorDetail => !!detail && typeof detail === 'object' && typeof detail.field === 'string' && detail.field.length <= 200 && typeof detail.safe_message === 'string' && detail.safe_message.length <= 2000).map(detail => ({ field: detail.field, safe_message: detail.safe_message })) : [];
        throw new MobileError(value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? '操作未能完成。', response.status, value.meta?.request_id ?? '', details);
      }
      if (!('data' in value)) throw new MobileError('INVALID_RESPONSE', '服务响应缺少必要资料。', response.status);
      return value;
    } finally { release?.(); }
  }
  private accept(value: Envelope, generation: number) {
    if (generation !== this.generation) throw new MobileError('SESSION_CHANGED', '登录状态已变化，请重新核对。');
    if (!value.data || typeof value.data !== 'object' || Array.isArray(value.data)) throw new MobileError('INVALID_RESPONSE', '登录响应异常，请重新登录。');
    const data = value.data as Login;
    if (!['authenticated', 'mfa', 'mfa_enroll'].includes(data.step) || (data.step !== 'authenticated' && !data.challenge)) throw new MobileError('INVALID_RESPONSE', '登录响应缺少必要步骤。');
    if (data.step === 'authenticated') {
      const expires = Date.parse(data.expires_at ?? '');
      if (!data.access_token || !data.refresh_token || !Number.isFinite(expires) || !value.meta?.request_id) throw new MobileError('INVALID_RESPONSE', '登录凭证不完整，请重新登录。');
      this.credentials = { access: data.access_token, refresh: data.refresh_token, expires, nonce: value.meta.request_id }; this.notify();
    }
    return data;
  }
  async login(login: string, password: string) {
    this.clear(); const generation = this.generation;
    return this.accept(await this.send({ method: 'POST', path: '/auth/login', headers: {}, body: { login, password, device_type: this.device } }), generation);
  }
  async miniappProfiles() {
    const value = await this.send({ method: 'GET', path: '/auth/miniapps', headers: {} });
    const data = value.data as Schemas['MiniappProfiles'];
    if (!data || !Array.isArray(data.items) || data.items.length > 2 || data.items.some(item => !['wechat', 'douyin'].includes(item.platform) || typeof item.app_id !== 'string')) throw new MobileError('INVALID_RESPONSE', '小程序登录配置不完整。');
    return data;
  }
  async loginMiniapp(app_id: string, code: string) {
    if (this.device === 'android') throw new MobileError('MINIAPP_RUNTIME_REQUIRED', '请使用公司小程序进行平台登录。');
    this.clear(); const generation = this.generation;
    return this.accept(await this.send({ method: 'POST', path: '/auth/miniapps/login', headers: {}, body: { platform: this.device, app_id, code } }), generation);
  }
  async enroll(challenge: string) {
    const generation = this.generation;
    const value = await this.send({ method: 'POST', path: '/auth/mfa/enroll', headers: {}, body: { challenge } });
    if (generation !== this.generation) throw new MobileError('SESSION_CHANGED', '登录状态已变化，请重新核对。');
    const data = value.data as Login | null;
    if (!data || data.step !== 'mfa_enroll' || typeof data.mfa_secret !== 'string' || !/^[A-Z2-7]+$/.test(data.mfa_secret)) throw new MobileError('INVALID_RESPONSE', '验证器设置资料不完整。');
    return data;
  }
  async activate(token: string, password: string) { await this.send({ method: 'POST', path: '/auth/activate', headers: {}, body: { token, password } }); }
  async resetPassword(token: string, password: string) {
    const value = await this.send({ method: 'POST', path: '/auth/password/reset', headers: {}, body: { token, password } });
    if ((value.data as Schemas['PasswordChanged'] | null)?.changed !== true) throw new MobileError('INVALID_RESPONSE', '重置结果尚未核对，请使用新密码登录确认。');
  }
  async changePassword(currentPassword: string, password: string, key: string) {
    const value = await this.request<Schemas['PasswordChanged']>('/me/password', { method: 'POST', key, body: { current_password: currentPassword, password } });
    if (value?.changed !== true) throw new MobileError('INVALID_RESPONSE', '密码修改结果尚未核对，请重新登录确认。');
    this.clear();
  }
  async revokeDevice(id: string, current: boolean, key: string) {
    if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id)) throw new MobileError('INVALID_DEVICE', '设备地址无效，请刷新列表。');
    const value = await this.request<Schemas['SessionRevocation']>('/me/sessions/' + id + '/revoke', { method: 'POST', key, body: { reason: current ? '本人退出当前设备' : '本人撤销不再使用的设备' } });
    if (value?.id !== id || !Number.isFinite(Date.parse(value.revoked_at))) throw new MobileError('INVALID_RESPONSE', '设备撤销结果尚未核对，请刷新设备列表。');
    if (current) this.clear();
  }
  async verify(challenge: string, code: string, recovery = false) {
    const generation = this.generation;
    return this.accept(await this.send({ method: 'POST', path: recovery ? '/auth/mfa/recover' : '/auth/mfa/verify', headers: {}, body: { challenge, code } }), generation);
  }
  private async refresh() {
    if (this.refreshFlight) return this.refreshFlight;
    const generation = this.generation, original = this.credentials;
    if (!original) throw new MobileError('LOGIN_REQUIRED', '请先登录工作空间。');
    const promise = (async () => {
      try {
        const value = await this.send({ method: 'POST', path: '/auth/refresh', headers: {}, body: { refresh_token: original.refresh } });
        if (this.credentials !== original || generation !== this.generation) throw new MobileError('SESSION_CHANGED', '登录状态已变化，请重新核对。');
        this.accept(value, generation);
      } catch (error) {
        if (error instanceof MobileError && error.status === 401 && generation === this.generation && this.credentials === original) this.clear();
        throw error;
      }
    })();
    this.refreshFlight = promise;
    try { await promise; } finally { if (this.refreshFlight === promise) this.refreshFlight = null; }
  }
  async request<T>(path: string, options: { method?: Method; body?: unknown; key?: string; scope?: Scope; ifRange?: string } = {}, retried = false): Promise<T> {
    if (!/^\/[a-z][a-z0-9_/-]*(?:\?[^#\r\n]*)?$/.test(path) || path.split('?')[0].includes('//')) throw new MobileError('INVALID_PATH', '请求路径无效。');
    if (options.scope?.closed) throw new MobileError('REQUEST_CANCELLED', '此页面的读取已结束。');
    const generation = this.generation, method = options.method ?? 'GET';
    if (!this.credentials) throw new MobileError('LOGIN_REQUIRED', '请先登录工作空间。');
    if (this.credentials.expires - this.now() < 30000) await this.refresh();
    if (generation !== this.generation || !this.credentials) throw new MobileError('SESSION_CHANGED', '登录状态已变化，请重新核对。');
    const sent = this.credentials;
    const headers: Record<string, string> = { Authorization: 'Bearer ' + sent.access };
    if (options.ifRange !== undefined) {
      if (method !== 'GET' || !/^"[a-f0-9]{64}"$/.test(options.ifRange) || !/^\/downloads\/[a-f0-9-]{36}\/chunks\?offset=\d+&length=\d+$/.test(path)) throw new MobileError('INVALID_RANGE', '文件读取条件无效。');
      headers['If-Range'] = options.ifRange;
    }
    if (method !== 'GET' && !path.startsWith('/auth/')) {
      if (!options.key || options.key.length < 16) throw new MobileError('COMMAND_KEY_REQUIRED', '请重新打开表单，核对本次操作。');
      headers['Idempotency-Key'] = options.key;
    }
    try {
      const response = await this.send({ method, path, headers, ...(method === 'GET' ? {} : { body: options.body ?? {} }) }, options.scope);
      if (generation !== this.generation) throw new MobileError('SESSION_CHANGED', '登录状态已变化，请重新核对。');
      return response.data as T;
    } catch (error) {
      if (error instanceof MobileError && error.status === 401 && method === 'GET' && !retried && generation === this.generation) {
        if (this.credentials === sent) await this.refresh();
        return this.request<T>(path, options, true);
      }
      throw error;
    }
  }
  async logout() {
    const credentials = this.credentials; this.clear();
    if (credentials) await this.send({ method: 'POST', path: '/auth/logout', headers: { Authorization: 'Bearer ' + credentials.access }, body: {} });
  }
}
