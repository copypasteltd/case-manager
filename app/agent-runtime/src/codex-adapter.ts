import { Codex, type CodexOptions, type ThreadEvent, type ThreadOptions, type TurnOptions, type Usage } from '@openai/codex-sdk';
import { createHash } from 'node:crypto';
import path from 'node:path';
import { RuntimeError, type SkillPackage } from './skills.js';
import { verifyResult, type ExecutionReceipt, type ProposedResult } from './evidence.js';

export const CODEX_SDK_VERSION = '0.164.0';
export interface RunBinding {
  id: string; authority_stamp: string; skill_sha256: string; expires_at: string; channel?: 'web' | 'android' | 'wechat' | 'douyin' | 'telegram';
  task_credential: string; input: unknown; prepared_context: string;
  max_tool_calls: number; max_input_tokens: number; max_output_tokens: number;
}
export interface NativeSettings {
  enabled: boolean; model: string; working_directory: string; profile_id: string;
  mcp_endpoint: string; api_key?: string; executable_path?: string; api_base_url?: string;
}
export interface NativeProfileRequest {
  sdk_version: string; profile_id: string; model: string; working_directory: string;
  configuration_sha256: string; expected_tools: readonly string[];
}
export type PublicEvent =
  | { type: 'run.started' }
  | { type: 'tool.started' | 'tool.finished'; tool: string }
  | { type: 'run.usage'; usage: Usage }
  | { type: 'run.completed'; result: ProposedResult; usage: Usage }
  | { type: 'run.interrupted'; code: string; reconciliation_required: boolean };
// Implementations must authenticate the run and use the Go authority/ledger, not model claims.
export interface RuntimeBoundary {
  availableTools: ReadonlySet<string>;
  assertNativeConfinement(profile: NativeProfileRequest, signal: AbortSignal): Promise<void>;
  assertCurrent(run: Readonly<RunBinding>, signal: AbortSignal): Promise<void>;
  receipts(run: Readonly<RunBinding>, signal: AbortSignal): Promise<readonly ExecutionReceipt[]>;
  append(run: Readonly<RunBinding>, event: PublicEvent, signal: AbortSignal): Promise<void>;
}
export interface NativeClient {
  startThread(options: ThreadOptions): {
    runStreamed(input: string, options: TurnOptions): Promise<{ events: AsyncGenerator<ThreadEvent> }>;
  };
}
export type ClientFactory = (options: CodexOptions) => NativeClient;
export interface AdapterOutcome {
  state: 'completed' | 'interrupted'; code: string | null; result: ProposedResult | null;
  usage: Usage | null; reconciliation_required: boolean;
}

function safeEnvironment(credential: string): Record<string, string> {
  const allowed = new Set(['path', 'systemroot', 'windir', 'comspec', 'pathext', 'userprofile', 'home', 'tmp', 'temp', 'lang', 'lc_all']);
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(process.env)) if (allowed.has(key.toLowerCase()) && value !== undefined) result[key] = value;
  result.CP_AGENT_TASK_CREDENTIAL = credential;
  return result;
}
function endpoint(value: string): string {
  let url: URL; try { url = new URL(value); } catch { throw new RuntimeError('RUNTIME_ENDPOINT_INVALID'); }
  const loopback = url.hostname === '127.0.0.1' || url.hostname === '[::1]';
  if ((url.protocol !== 'https:' && !(loopback && url.protocol === 'http:')) || url.username || url.password || url.search || url.hash || url.pathname !== '/internal/v1/agent-mcp') throw new RuntimeError('RUNTIME_ENDPOINT_INVALID');
  return url.href;
}
function validateBinding(run: RunBinding) {
  if (run.channel !== undefined && !['web', 'android', 'wechat', 'douyin', 'telegram'].includes(run.channel)) throw new RuntimeError('RUNTIME_BINDING_INVALID');
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(run.id) || !/^[A-Za-z0-9_-]{16,200}$/.test(run.authority_stamp)
    || !/^[A-Za-z0-9._~-]{32,4096}$/.test(run.task_credential) || typeof run.prepared_context !== 'string'
    || Buffer.byteLength(run.prepared_context) > 131072 || !Number.isFinite(Date.parse(run.expires_at))) throw new RuntimeError('RUNTIME_BINDING_INVALID');
  for (const value of [run.max_tool_calls, run.max_input_tokens, run.max_output_tokens]) if (!Number.isSafeInteger(value) || value < 1) throw new RuntimeError('RUNTIME_BUDGET_INVALID');
}
function validateUsage(usage: Usage): Usage {
  const keys = ['input_tokens', 'cached_input_tokens', 'cache_write_input_tokens', 'output_tokens', 'reasoning_output_tokens'] as const;
  if (keys.some(key => !Number.isSafeInteger(usage[key]) || usage[key] < 0)) throw new RuntimeError('RUNTIME_USAGE_INVALID');
  return Object.fromEntries(keys.map(key => [key, usage[key]])) as unknown as Usage;
}
function abortError(signal: AbortSignal): Error { return signal.reason instanceof RuntimeError ? signal.reason : new RuntimeError('RUNTIME_CANCEL_REQUESTED'); }
const SAFE_CODES = new Set(['RUNTIME_CANCEL_REQUESTED', 'RUNTIME_DEADLINE_EXCEEDED', 'RUNTIME_AUTHORITY_CHANGED', 'RUNTIME_CONFINEMENT_UNVERIFIED',
  'RUNTIME_PROVIDER_FAILED', 'RUNTIME_PROTOCOL_INVALID', 'RUNTIME_USAGE_INVALID', 'RUNTIME_USAGE_BUDGET_EXCEEDED', 'RUNTIME_OUTPUT_LIMIT',
  'RUNTIME_NATIVE_TOOL_FORBIDDEN', 'RUNTIME_TOOL_BUDGET_EXCEEDED', 'RUNTIME_STREAM_INCOMPLETE', 'RUNTIME_OUTPUT_INVALID', 'RUNTIME_UNAVAILABLE',
  'SKILL_TOOL_DENIED', 'SKILL_OUTPUT_INVALID', 'SKILL_PAYLOAD_INVALID', 'RESULT_REFERENCE_UNVERIFIED', 'RESULT_OPERATION_UNVERIFIED',
  'RESULT_COMPLETION_INCOMPLETE', 'RESULT_COMPLETION_UNVERIFIED', 'RESULT_READ_UNVERIFIED']);
function withAbort<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(abortError(signal));
  return new Promise<T>((resolve, reject) => {
    const cancel = () => { signal.removeEventListener('abort', cancel); reject(abortError(signal)); };
    signal.addEventListener('abort', cancel, { once: true });
    promise.then(value => { signal.removeEventListener('abort', cancel); resolve(value); }, error => { signal.removeEventListener('abort', cancel); reject(error); });
  });
}

/** Bounded adapter; a verified native profile and an authenticated Go boundary are mandatory. */
// Used both by deployment inspection and execution, so the stored configuration
// digest is computed from the exact options later sent to the pinned SDK.
export function executionTools(skill: SkillPackage, channel?: RunBinding['channel']): readonly string[] {
  if (channel !== 'telegram') return skill.manifest.allowed_tools;
  if (!['project-briefing', 'incident-summary'].includes(skill.manifest.skill_id)) throw new RuntimeError('SKILL_TOOLS_UNAVAILABLE');
  return skill.manifest.allowed_tools.filter(tool => ['read_resource', 'search_resources', 'inspect_operation', 'read_service_summary'].includes(tool));
}
export function nativeConfiguration(skill: SkillPackage, settings: Readonly<NativeSettings>, channel?: RunBinding['channel']) {
  if (settings.api_base_url !== undefined) {
    let gateway: URL;
    try { gateway = new URL(settings.api_base_url); } catch { throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID'); }
    if (gateway.protocol !== 'http:' || gateway.hostname !== '127.0.0.1' || !gateway.port || Number(gateway.port) < 1024 || gateway.pathname !== '/v1'
      || gateway.username || gateway.password || gateway.search || gateway.hash || settings.api_key !== 'native-broker-managed') throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID');
  }
  const tools = executionTools(skill, channel);
  const config = {
    developer_instructions: skill.instructions,
    hide_agent_reasoning: true, history: { persistence: 'none' },
    features: { shell_tool: false, apps: false, code_mode: { enabled: false },
      ...(settings.api_base_url ? { enable_request_compression: false, responses_websockets: false, responses_websockets_v2: false } : {}) },
    mcp_servers: { company: { url: endpoint(settings.mcp_endpoint), bearer_token_env_var: 'CP_AGENT_TASK_CREDENTIAL',
      enabled_tools: [...tools], startup_timeout_sec: 10, tool_timeout_sec: 30 } },
  };
  const threadOptions: ThreadOptions = { model: settings.model, workingDirectory: settings.working_directory,
    skipGitRepoCheck: true, sandboxMode: 'read-only', approvalPolicy: 'never', networkAccessEnabled: false,
    webSearchMode: 'disabled', additionalDirectories: [] };
  const profile: NativeProfileRequest = { sdk_version: CODEX_SDK_VERSION, profile_id: settings.profile_id, model: settings.model,
    working_directory: settings.working_directory, configuration_sha256: createHash('sha256').update(JSON.stringify({ config, threadOptions, ...(settings.api_base_url ? { baseUrl: settings.api_base_url } : {}) })).digest('hex'),
    expected_tools: tools.map(name => 'company/' + name) };
  return { config, threadOptions, profile };
}

export class CodexAdapter {
  constructor(private readonly settings: Readonly<NativeSettings>, private readonly boundary: RuntimeBoundary,
    private readonly factory: ClientFactory = options => new Codex(options)) { this.settings = Object.freeze({ ...settings }); }

  async startRun(skill: SkillPackage, binding: RunBinding, requestedCancel?: AbortSignal): Promise<AdapterOutcome> {
    const run = structuredClone(binding); validateBinding(run); skill.validateInput(run.input);
    if (!this.settings.enabled) throw new RuntimeError('RUNTIME_DISABLED');
    skill.assertRunnable(run.skill_sha256, this.boundary.availableTools);
    if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,100}$/.test(this.settings.model) || !path.isAbsolute(this.settings.working_directory)
      || !/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,100}$/.test(this.settings.profile_id)) throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID');
    endpoint(this.settings.mcp_endpoint); const controller = new AbortController();
    const deadline = Math.min(Date.parse(run.expires_at), Date.now() + skill.manifest.timeout_seconds * 1000);
    if (deadline <= Date.now()) throw new RuntimeError('RUNTIME_EXPIRED');
    const cancel = () => controller.abort(new RuntimeError('RUNTIME_CANCEL_REQUESTED'));
    if (requestedCancel?.aborted) cancel(); requestedCancel?.addEventListener('abort', cancel, { once: true });
    const timeout = setTimeout(() => controller.abort(new RuntimeError('RUNTIME_DEADLINE_EXCEEDED')), Math.max(1, deadline - Date.now()));
    let started = false, usage: Usage | null = null, iterator: AsyncGenerator<ThreadEvent> | undefined;
    const current = async () => {
      if (controller.signal.aborted) throw abortError(controller.signal);
      if (Date.now() >= deadline) throw new RuntimeError('RUNTIME_DEADLINE_EXCEEDED');
      await withAbort(this.boundary.assertCurrent(run, controller.signal), controller.signal);
    };
    const emit = async (event: PublicEvent) => { await current(); await withAbort(this.boundary.append(run, event, controller.signal), controller.signal); };
    try {
      const tools = new Set(executionTools(skill, run.channel));
      const { config, threadOptions, profile } = nativeConfiguration(skill, this.settings, run.channel);
      await current();
      await withAbort(this.boundary.assertNativeConfinement(profile, controller.signal), controller.signal);
      await current(); await emit({ type: 'run.started' });
      if (controller.signal.aborted) throw abortError(controller.signal);
      const client = this.factory({ config, env: safeEnvironment(run.task_credential), apiKey: this.settings.api_key,
        codexPathOverride: this.settings.executable_path, baseUrl: this.settings.api_base_url });
      // Never resume native state: authorized context is rebuilt for each company run.
      const thread = client.startThread(threadOptions);
      started = true;
      const turn = await withAbort(thread.runStreamed(JSON.stringify({ input: run.input, authorized_context: run.prepared_context }),
        { outputSchema: skill.modelOutputSchema, signal: controller.signal }), controller.signal);
      iterator = turn.events;
      let response: string | null = null, completed = false;
      const calls = new Map<string, { tool: string; finished: boolean }>();
      for (;;) {
        await current(); const next = await withAbort(iterator.next(), controller.signal); if (next.done) break;
        const event = next.value;
        if (event.type === 'thread.started' || event.type === 'turn.started') continue;
        if (event.type === 'error' || event.type === 'turn.failed') throw new RuntimeError('RUNTIME_PROVIDER_FAILED');
        if (event.type === 'turn.completed') {
          if (completed) throw new RuntimeError('RUNTIME_PROTOCOL_INVALID');
          usage = validateUsage(event.usage); completed = true;
          const accounting = new AbortController(); const accountingTimer = setTimeout(() => accounting.abort(), 3000);
          try { await withAbort(this.boundary.append(run, { type: 'run.usage', usage }, accounting.signal), accounting.signal); } finally { clearTimeout(accountingTimer); }
          if (usage.input_tokens > run.max_input_tokens || usage.output_tokens > run.max_output_tokens) throw new RuntimeError('RUNTIME_USAGE_BUDGET_EXCEEDED');
          continue;
        }
        if (event.type !== 'item.started' && event.type !== 'item.updated' && event.type !== 'item.completed') throw new RuntimeError('RUNTIME_PROTOCOL_INVALID');
        if (completed) throw new RuntimeError('RUNTIME_PROTOCOL_INVALID');
        const item = event.item;
        if (item.type === 'reasoning' || item.type === 'todo_list') continue;
        if (item.type === 'error') throw new RuntimeError('RUNTIME_PROVIDER_FAILED');
        if (item.type === 'agent_message') {
          if (Buffer.byteLength(item.text) > 1048576) throw new RuntimeError('RUNTIME_OUTPUT_LIMIT');
          if (event.type === 'item.completed') response = item.text;
          continue;
        }
        if (item.type !== 'mcp_tool_call' || item.server !== 'company') throw new RuntimeError('RUNTIME_NATIVE_TOOL_FORBIDDEN');
        skill.authorizeTool(item.tool);
        if (!tools.has(item.tool)) throw new RuntimeError('SKILL_TOOL_DENIED');
        let call = calls.get(item.id);
        if (!call) {
          if (calls.size >= Math.min(run.max_tool_calls, skill.manifest.max_tool_calls)) throw new RuntimeError('RUNTIME_TOOL_BUDGET_EXCEEDED');
          call = { tool: item.tool, finished: false }; calls.set(item.id, call); await emit({ type: 'tool.started', tool: item.tool });
        } else if (call.tool !== item.tool) throw new RuntimeError('RUNTIME_PROTOCOL_INVALID');
        if (event.type === 'item.completed' && !call.finished) { call.finished = true; await emit({ type: 'tool.finished', tool: item.tool }); }
      }
      if (!completed || !response || [...calls.values()].some(call => !call.finished)) throw new RuntimeError('RUNTIME_STREAM_INCOMPLETE');
      let proposed: unknown; try { proposed = JSON.parse(response); } catch { throw new RuntimeError('RUNTIME_OUTPUT_INVALID'); }
      await current(); const receipts = await withAbort(this.boundary.receipts(run, controller.signal), controller.signal); await current();
      const result = verifyResult(skill, run, proposed, receipts); await emit({ type: 'run.completed', result, usage: usage! });
      return { state: 'completed', code: null, result, usage, reconciliation_required: false };
    } catch (error) {
      controller.abort(error); const code = error instanceof RuntimeError && SAFE_CODES.has(error.code) ? error.code : 'RUNTIME_UNAVAILABLE';
      // A cancellation or lost stream is not proof that dispatched business writes were rolled back.
      const terminal = new AbortController(), terminalTimer = setTimeout(() => terminal.abort(), 1000);
      try { await withAbort(this.boundary.append(run, { type: 'run.interrupted', code, reconciliation_required: started }, terminal.signal), terminal.signal); }
      catch { /* Durable controller reconciles absent terminal events. */ } finally { clearTimeout(terminalTimer); }
      return { state: 'interrupted', code, result: null, usage, reconciliation_required: started };
    } finally {
      clearTimeout(timeout); requestedCancel?.removeEventListener('abort', cancel); controller.abort();
      if (iterator) { const stopped = iterator.return(undefined).catch(() => undefined); let cleanupTimer: ReturnType<typeof setTimeout> | undefined;
        await Promise.race([stopped, new Promise<void>(resolve => { cleanupTimer = setTimeout(resolve, 1000); })]); if (cleanupTimer) clearTimeout(cleanupTimer); }
    }
  }
}
