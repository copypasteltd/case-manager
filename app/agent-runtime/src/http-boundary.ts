import type { NativeProfileRequest, PublicEvent, RunBinding, RuntimeBoundary } from './codex-adapter.js';
import type { ExecutionReceipt } from './evidence.js';
import { RuntimeError } from './skills.js';

const GATEWAY_TOOLS = ['list_subscriptions', 'read_subscription_review', 'read_finance_record', 'read_finance_summary', 'read_service_summary', 'read_resource', 'search_resources', 'inspect_operation', 'propose_project', 'create_project',
  'create_risk', 'update_risk', 'create_issue', 'update_issue', 'create_deliverable', 'update_deliverable', 'create_milestone', 'update_project', 'update_task', 'transition_task', 'transition_project', 'update_milestone', 'transition_milestone', 'create_file_draft', 'propose_file_update', 'commit_file_update', 'stage_file_create', 'propose_file_create', 'commit_file_create',
  'list_directory', 'propose_organization', 'commit_organization', 'draft_request', 'submit_request', 'draft_post', 'publish_post'];
const SAFE_CODES = new Set(['RUNTIME_AUTHORITY_CHANGED', 'RUNTIME_CANCEL_REQUESTED', 'RUNTIME_DEADLINE_EXCEEDED',
  'RUNTIME_CONFINEMENT_UNVERIFIED', 'RUNTIME_TOOL_BUDGET_EXCEEDED', 'RUNTIME_USAGE_BUDGET_EXCEEDED']);

export class HTTPRuntimeBoundary implements RuntimeBoundary {
  readonly availableTools: ReadonlySet<string> = new Set(GATEWAY_TOOLS);
  private readonly base: URL;
  constructor(private readonly run: Readonly<RunBinding>, mcpEndpoint: string) {
    let url: URL; try { url = new URL(mcpEndpoint); } catch { throw new RuntimeError('RUNTIME_ENDPOINT_INVALID'); }
    if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.username || url.password || url.search || url.hash
      || url.pathname !== '/internal/v1/agent-mcp') throw new RuntimeError('RUNTIME_ENDPOINT_INVALID');
    this.base = new URL('/internal/v1/agent-control/', url);
  }
  private binding(run: Readonly<RunBinding>) {
    if (run.id !== this.run.id || run.authority_stamp !== this.run.authority_stamp || run.task_credential !== this.run.task_credential) throw new RuntimeError('RUNTIME_AUTHORITY_CHANGED');
  }
  private async post(action: string, value: unknown, signal: AbortSignal): Promise<unknown> {
    try {
      const response = await fetch(new URL(action, this.base), { method: 'POST', redirect: 'error', signal,
        headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + this.run.task_credential }, body: JSON.stringify(value) });
      if (Number(response.headers.get('content-length')) > 262144) throw new RuntimeError('RUNTIME_PROTOCOL_INVALID');
      const reader = response.body?.getReader(); if (!reader) throw new RuntimeError('RUNTIME_PROTOCOL_INVALID');
      let length = 0; const chunks: Uint8Array[] = [];
      try { for (;;) { const chunk = await reader.read(); if (chunk.done) break; length += chunk.value.byteLength;
        if (length > 262144) throw new RuntimeError('RUNTIME_PROTOCOL_INVALID'); chunks.push(chunk.value); }
      } finally { await reader.cancel().catch(() => undefined); }
      let result: unknown; try { result = JSON.parse(Buffer.concat(chunks).toString('utf8')); } catch { throw new RuntimeError('RUNTIME_PROTOCOL_INVALID'); }
      if (!response.ok) {
        const code = (result as { error?: { code?: string } }).error?.code;
        if (code === 'RUNTIME_SOURCE_CHANGED' || code === 'RUNTIME_PROFILE_RETIRED' || code === 'RUNTIME_CREDENTIAL_INVALID'
          || code === 'JOB_LEASE_LOST' || code === 'RUNTIME_TERMINAL') throw new RuntimeError('RUNTIME_AUTHORITY_CHANGED');
        throw new RuntimeError(code && SAFE_CODES.has(code) ? code : 'RUNTIME_UNAVAILABLE');
      }
      return result;
    } catch (error) { if (error instanceof RuntimeError) throw error; throw new RuntimeError('RUNTIME_UNAVAILABLE'); }
  }
  async assertCurrent(run: Readonly<RunBinding>, signal: AbortSignal): Promise<void> { this.binding(run); await this.post('current', {}, signal); }
  async assertNativeConfinement(profile: NativeProfileRequest, signal: AbortSignal): Promise<void> { await this.post('confinement', profile, signal); }
  async receipts(run: Readonly<RunBinding>, signal: AbortSignal): Promise<readonly ExecutionReceipt[]> {
    this.binding(run); const body = await this.post('receipts', {}, signal) as { receipts?: unknown };
    if (!Array.isArray(body.receipts) || body.receipts.length > 100) throw new RuntimeError('RUNTIME_PROTOCOL_INVALID');
    // Transport is authenticated to the Go-owned local service; verifyResult separately
    // checks scope and every proposed reference against these trusted ledger receipts.
    return body.receipts as ExecutionReceipt[];
  }
  async append(run: Readonly<RunBinding>, event: PublicEvent, signal: AbortSignal): Promise<void> { this.binding(run); await this.post('events', event, signal); }
}
