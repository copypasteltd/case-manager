import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { createHash, randomUUID } from 'node:crypto';
import { cp, mkdir, mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:http';
import { once } from 'node:events';
import type { CodexOptions, ThreadEvent, ThreadOptions, Usage } from '@openai/codex-sdk';
import { CodexAdapter, nativeConfiguration, type ClientFactory, type NativeSettings, type PublicEvent, type RunBinding, type RuntimeBoundary } from './codex-adapter.js';
import { loadSkills, KNOWN_TOOLS, RuntimeError, type SkillPackage } from './skills.js';
import type { ExecutionReceipt, ProposedResult } from './evidence.js';
import { executeWorker } from './worker.js';
import { describeProfile } from './describe-profile.js';

const repository = fileURLToPath(new URL('../../../', import.meta.url)), local = path.join(repository, 'app/.local');
let temporary = '', catalog: ReadonlyMap<string, SkillPackage>;
const tokens: Usage = { input_tokens: 110, cached_input_tokens: 0, cache_write_input_tokens: 0, output_tokens: 40, reasoning_output_tokens: 0 };
const reference = { type: 'project', id: randomUUID(), version_id: randomUUID() };
before(async () => {
  await mkdir(local, { recursive: true }); temporary = await mkdtemp(path.join(local, 'adapter-release-'));
  await cp(path.join(repository, 'app/skills'), path.join(temporary, 'app/skills'), { recursive: true });
  await cp(path.join(repository, 'docs/skills'), path.join(temporary, 'docs/skills'), { recursive: true });
  for (const id of ['project-briefing', 'file-update']) {
    const file = path.join(temporary, 'app/skills', id, 'manifest.json'), value = JSON.parse(await readFile(file, 'utf8'));
    value.state = 'published'; await writeFile(file, JSON.stringify(value));
  }
  catalog = await loadSkills(temporary);
});
after(async () => {
  if (!temporary) return;
  const target = await realpath(temporary), parent = await realpath(local);
  if (path.dirname(target) !== parent || !path.basename(target).startsWith('adapter-release-')) throw new Error('Refusing cleanup outside owned fixture');
  await rm(target, { recursive: true, force: true });
});
function setup(id = 'project-briefing') {
  const skill = catalog.get(id)!, run: RunBinding = { id: randomUUID(), authority_stamp: 'current_authority_stamp', skill_sha256: skill.sha256,
    expires_at: new Date(Date.now() + 60000).toISOString(), task_credential: 'private_' + randomUUID(), prepared_context: JSON.stringify({source_note:'已筛选的当前授权资料。'}),
    input: id === 'file-update' ? { file_id: randomUUID(), base_version_id: randomUUID(), format: 'text/markdown', instructions: '更正日期。' }
      : { project_id: reference.id, window: { from: '2026-10-01T00:00:00+08:00', to: '2026-11-01T00:00:00+08:00' } },
    max_tool_calls: 10, max_input_tokens: 1000, max_output_tokens: 1000 };
  const result: ProposedResult = { status: 'completed', summary: '已依据来源生成摘要。', resource_refs: [], source_refs: [reference], operation_ids: [], missing_fields: [], next_action: '' };
  const events: PublicEvent[] = [], receipts: ExecutionReceipt[] = [{ run_id: run.id, authority_stamp: run.authority_stamp,
    tool: 'read_resource', status: 'succeeded', operation_id: null, committed: false, source_refs: [reference], resource_refs: [] }];
  const settings: NativeSettings = { enabled: true, model: 'deployment-selected-model', working_directory: temporary,
    profile_id: 'controlled-offline-test', mcp_endpoint: 'http://127.0.0.1:19991/internal/v1/agent-mcp' };
  let revoked = false, nativeAllowed = true, created = 0, options: CodexOptions | undefined, threadOptions: ThreadOptions | undefined, prompt = '';
  const boundary: RuntimeBoundary = { availableTools: KNOWN_TOOLS,
    async assertNativeConfinement() { if (!nativeAllowed) throw new RuntimeError('RUNTIME_CONFINEMENT_UNVERIFIED'); },
    async assertCurrent() { if (revoked) throw new RuntimeError('RUNTIME_AUTHORITY_CHANGED'); },
    async receipts() { return receipts; }, async append(_run, event) { events.push(event); } };
  const finished = (): ThreadEvent[] => [{ type: 'item.completed', item: { type: 'agent_message', id: 'answer', text: JSON.stringify(result) } }, { type: 'turn.completed', usage: tokens }];
  let stream: (signal: AbortSignal) => AsyncGenerator<ThreadEvent> = async function* () { yield* finished(); };
  const factory: ClientFactory = supplied => { created++; options = supplied; return { startThread(thread) { threadOptions = thread;
    return { async runStreamed(input, turn) { prompt = input; return { events: stream(turn.signal!) }; } }; } }; };
  return { skill, run, result, receipts, events, settings, boundary, finished,
    get created() { return created; }, get options() { return options; }, get threadOptions() { return threadOptions; }, get prompt() { return prompt; },
    revoke() { revoked = true; }, rejectNative() { nativeAllowed = false; },
    setStream(value: typeof stream) { stream = value; },
    adapter() { return new CodexAdapter(settings, boundary, factory); } };
}
const call = (id: string, phase: 'item.started' | 'item.updated' | 'item.completed' = 'item.completed', tool = 'read_resource'): ThreadEvent => ({ type: phase,
  item: { id, type: 'mcp_tool_call', server: 'company', tool, arguments: { secret_source: 'not-for-events' }, status: phase === 'item.completed' ? 'completed' : 'in_progress' } });

test('worker entry drives HTTP authority checks actual MCP reads and a receipt verified reply without a provider', async () => {
  const value = setup(), events: PublicEvent[] = [], paths: string[] = []; let read = false, confined = false;
  value.run.input = { request: '汇总目前共同授权的项目资料。' };
  const profileID = randomUUID();
  const server = createServer(async (request, response) => {
    assert.equal(request.headers.authorization, 'Bearer ' + value.run.task_credential);
    let raw = ''; for await (const chunk of request) raw += chunk;
    paths.push(request.url!); const body = JSON.parse(raw); response.setHeader('Content-Type', 'application/json');
    if (request.url === '/internal/v1/agent-mcp') {
      assert.equal(body.method, 'tools/call'); assert.deepEqual(body.params.arguments, { type: reference.type, id: reference.id }); read = true;
      response.end(JSON.stringify({ jsonrpc: '2.0', id: body.id, result: { isError: false, content: [{ type: 'text', text: '受控来源正文' }] } }));
    } else if (request.url?.endsWith('/confinement')) { assert.equal(body.profile_id, profileID); assert.match(body.configuration_sha256, /^[a-f0-9]{64}$/); confined = true; response.end('{"accepted":true}'); }
    else if (request.url?.endsWith('/receipts')) { assert.ok(read); response.end(JSON.stringify({ receipts: value.receipts })); }
    else if (request.url?.endsWith('/events')) { if (body.type === 'run.started') assert.ok(confined); events.push(body); response.end('{"accepted":true}'); }
    else { assert.ok(request.url?.endsWith('/current')); assert.deepEqual(body, {}); response.end('{"accepted":true}'); }
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening'); const address = server.address(); assert.ok(address && typeof address === 'object');
  const settings: NativeSettings = { ...value.settings, profile_id: profileID, mcp_endpoint: `http://127.0.0.1:${address.port}/internal/v1/agent-mcp` };
  const factory: ClientFactory = () => { assert.ok(confined); return { startThread() { return { async runStreamed(input) {
    assert.ok(!input.includes(value.run.task_credential)); return { events: (async function* (): AsyncGenerator<ThreadEvent> {
      yield call('read', 'item.started');
      const result = await fetch(settings.mcp_endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + value.run.task_credential },
        body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'read_resource', arguments: { type: reference.type, id: reference.id } } }) });
      assert.equal(result.status, 200); assert.ok((await result.text()).includes('受控来源正文'));
      yield call('read'); yield* value.finished();
    })() };
  } }; } }; };
  try {
    const outcome = await executeWorker({ repository: temporary, settings, run: { ...value.run, profile: { id: profileID, skill_id: value.skill.manifest.skill_id,
      skill_version: value.skill.manifest.version, skill_sha256: value.skill.sha256, runtime_version: 'codex-sdk-0.164.0' } } }, undefined, factory);
    assert.equal(outcome.state, 'completed'); assert.equal(events.at(-1)?.type, 'run.completed'); assert.ok(paths.some(item => item.endsWith('/receipts')));
    assert.ok(!JSON.stringify(events).includes(value.run.task_credential));
  } finally { server.closeAllConnections(); await new Promise<void>(resolve => server.close(() => resolve())); }
});

test('deployment inspection uses the exact SDK configuration digest and never publishes a draft skill', async () => {
  const value = setup(), profileID = randomUUID(); value.settings.profile_id = profileID;
  const description = await describeProfile({ skill_id: value.skill.manifest.skill_id, profile_id: profileID, model: value.settings.model,
    working_directory: value.settings.working_directory, mcp_endpoint: value.settings.mcp_endpoint }, temporary);
  value.boundary.assertNativeConfinement = async profile => { assert.equal(profile.configuration_sha256, description.profile.configuration_sha256); };
  assert.equal((await value.adapter().startRun(value.skill, value.run)).state, 'completed');
  assert.equal(description.profile.configuration_sha256, createHash('sha256').update(JSON.stringify({ config: value.options!.config, threadOptions: value.threadOptions })).digest('hex'));
  const draft = await describeProfile({ skill_id: 'project-briefing', profile_id: profileID, model: value.settings.model,
    working_directory: temporary, mcp_endpoint: value.settings.mcp_endpoint }, repository);
  assert.equal(draft.skill_state, 'draft'); assert.equal(draft.ready_for_controlled_installation, false); assert.equal(draft.actual_model_invoked, false); assert.equal(draft.database_modified, false);
});

test('native model relay binds the exact SDK baseUrl and supplies only the broker placeholder', async () => {
  const value = setup(); value.settings.profile_id = randomUUID();
  const direct = nativeConfiguration(value.skill, value.settings).profile.configuration_sha256;
  value.settings.api_base_url = 'http://127.0.0.1:18082/v1'; value.settings.api_key = 'native-broker-managed';
  const description = await describeProfile({ skill_id: value.skill.manifest.skill_id, profile_id: value.settings.profile_id, model: value.settings.model,
    working_directory: value.settings.working_directory, mcp_endpoint: value.settings.mcp_endpoint, api_base_url: value.settings.api_base_url }, temporary);
  value.boundary.assertNativeConfinement = async profile => { assert.equal(profile.configuration_sha256, description.profile.configuration_sha256); };
  assert.equal((await value.adapter().startRun(value.skill,value.run)).state,'completed');
  assert.equal(value.options!.apiKey,'native-broker-managed'); assert.equal(value.options!.baseUrl,value.settings.api_base_url);
  const features=value.options!.config!.features as Record<string,unknown>;
  for(const key of ['enable_request_compression','responses_websockets','responses_websockets_v2']) assert.equal(features[key],false);
  assert.notEqual(description.profile.configuration_sha256,direct);
  assert.equal(description.profile.configuration_sha256,createHash('sha256').update(JSON.stringify({config:value.options!.config,threadOptions:value.threadOptions,baseUrl:value.settings.api_base_url})).digest('hex'));
  assert.ok(!value.prompt.includes('native-broker-managed')); assert.equal(description.actual_model_invoked,false);
});

test('native relay refuses remote routes aliases credentials and unbound host model keys', () => {
  const value=setup();
  for (const url of ['https://api.openai.com/v1','http://localhost:18082/v1','http://0.0.0.0:18082/v1','http://127.0.0.1/v1','http://127.0.0.1:0/v1','http://127.0.0.1:18082/files','http://user@127.0.0.1:18082/v1','http://127.0.0.1:18082/v1?origin=x','http://127.0.0.1:18082/v1#x']) {
    assert.throws(()=>nativeConfiguration(value.skill,{...value.settings,api_base_url:url,api_key:'native-broker-managed'}),/RUNTIME_CONFIGURATION_INVALID/);
  }
  assert.throws(()=>nativeConfiguration(value.skill,{...value.settings,api_base_url:'http://127.0.0.1:18082/v1',api_key:'synthetic-host-only-key'}),/RUNTIME_CONFIGURATION_INVALID/);
});

test('inspection and execution share company tool coverage while draft Skills stay unavailable', async () => {
  for (const skill of (await loadSkills(repository)).values()) {
    const description=await describeProfile({skill_id:skill.manifest.skill_id,profile_id:randomUUID(),model:'operator-selected-model',working_directory:temporary,mcp_endpoint:'http://127.0.0.1:18081/internal/v1/agent-mcp'},repository);
    assert.deepEqual(description.missing_tools,[]); assert.equal(description.skill_state,'draft');assert.equal(description.ready_for_controlled_installation,false);
  }
});

test('SDK configuration is explicit, credentials are excluded from prompts and public events contain only verified output', async () => {
  const value = setup(); value.setStream(async function* () {
    yield { type: 'thread.started', thread_id: 'private-native-thread' };
    yield { type: 'item.completed', item: { type: 'reasoning', id: 'reasoning', text: 'private reasoning secret' } };
    yield call('read', 'item.started'); yield call('read', 'item.updated'); yield call('read'); yield* value.finished();
  });
  const result = await value.adapter().startRun(value.skill, value.run);
  assert.equal(result.state, 'completed'); assert.equal(result.reconciliation_required, false); assert.deepEqual(result.usage, tokens);
  assert.equal(value.created, 1); assert.equal(value.threadOptions?.sandboxMode, 'read-only'); assert.equal(value.threadOptions?.webSearchMode, 'disabled');
  assert.equal(value.threadOptions?.approvalPolicy, 'never'); assert.deepEqual(value.threadOptions?.additionalDirectories, []);
  assert.equal(value.options?.env?.CP_AGENT_TASK_CREDENTIAL, value.run.task_credential);
  assert.equal(value.options?.env?.CP_DATABASE_URL, undefined); assert.equal(value.options?.env?.CODEX_HOME, undefined);
  assert.ok(!value.prompt.includes(value.run.task_credential)); assert.ok(!value.prompt.includes(value.run.authority_stamp));
  assert.equal((value.options?.config?.mcp_servers as { company: { enabled_tools: string[] } }).company.enabled_tools.length, value.skill.manifest.allowed_tools.length);
  assert.deepEqual(value.events.map(event => event.type), ['run.started', 'tool.started', 'tool.finished', 'run.usage', 'run.completed']);
  assert.ok(!JSON.stringify(value.events).includes('private reasoning')); assert.ok(!JSON.stringify(value.events).includes('not-for-events'));
  assert.ok(!JSON.stringify(value.events).includes('private-native-thread'));
});

test('disabled runtime, draft skill, missing native boundary and pre-cancellation never start a model process', async () => {
  const value = setup(); value.settings.enabled = false;
  await assert.rejects(value.adapter().startRun(value.skill, value.run), /RUNTIME_DISABLED/); assert.equal(value.created, 0);
  value.settings.enabled = true; const draft = catalog.get('incident-summary')!;
  await assert.rejects(value.adapter().startRun(draft, { ...value.run, skill_sha256: draft.sha256, input: { service_ids: [randomUUID()], window: { from: '2026-10-01T00:00:00Z', to: '2026-11-01T00:00:00Z' } } }), /SKILL_NOT_PUBLISHED/);
  value.rejectNative(); const rejected = await value.adapter().startRun(value.skill, value.run);
  assert.equal(rejected.code, 'RUNTIME_CONFINEMENT_UNVERIFIED'); assert.equal(rejected.reconciliation_required, false); assert.equal(value.created, 0);
  const cancelled = setup(), controller = new AbortController(); controller.abort();
  assert.equal((await cancelled.adapter().startRun(cancelled.skill, cancelled.run, controller.signal)).code, 'RUNTIME_CANCEL_REQUESTED'); assert.equal(cancelled.created, 0);
});

test('invented formal completion is rejected even when the model emits valid JSON and an operation UUID', async () => {
  const value = setup('file-update'); value.result.operation_ids = [randomUUID()]; value.result.source_refs = [];
  const result = await value.adapter().startRun(value.skill, value.run);
  assert.equal(result.state, 'interrupted'); assert.equal(result.code, 'RESULT_OPERATION_UNVERIFIED'); assert.equal(result.result, null);
  assert.ok(!value.events.some(event => event.type === 'run.completed'));
});

test('native shell, file, web and other MCP tools abort the adapter without publishing their arguments', async () => {
  const forbidden: ThreadEvent[] = [
    { type: 'item.started', item: { type: 'command_execution', id: 'shell', command: 'private command', aggregated_output: 'private output', status: 'in_progress' } },
    { type: 'item.completed', item: { type: 'file_change', id: 'file', changes: [{ path: 'private.env', kind: 'update' }], status: 'completed' } },
    { type: 'item.started', item: { type: 'web_search', id: 'web', query: 'private query' } },
    { type: 'item.started', item: { type: 'mcp_tool_call', id: 'other', server: 'unknown', tool: 'read_resource', arguments: { secret: 'private argument' }, status: 'in_progress' } },
  ];
  for (const event of forbidden) { const value = setup(); value.setStream(async function* () { yield event; yield* value.finished(); });
    const result = await value.adapter().startRun(value.skill, value.run);
    assert.equal(result.code, 'RUNTIME_NATIVE_TOOL_FORBIDDEN'); assert.equal(result.reconciliation_required, true);
    assert.ok(!JSON.stringify(value.events).includes('private ')); }
});

test('changed authority suppresses final text and requires reconciliation rather than inventing cancellation rollback', async () => {
  const value = setup(); value.setStream(async function* () { yield call('read'); value.revoke(); yield* value.finished(); });
  const result = await value.adapter().startRun(value.skill, value.run);
  assert.equal(result.code, 'RUNTIME_AUTHORITY_CHANGED'); assert.equal(result.reconciliation_required, true);
  assert.ok(!value.events.some(event => event.type === 'run.completed'));
});

test('stable tool IDs count once while new calls and actual provider usage respect the reserved bounds', async () => {
  const allowed = setup(); allowed.run.max_tool_calls = 1;
  allowed.setStream(async function* () { yield call('same', 'item.started'); yield call('same', 'item.updated'); yield call('same'); yield* allowed.finished(); });
  assert.equal((await allowed.adapter().startRun(allowed.skill, allowed.run)).state, 'completed');
  const exceeded = setup(); exceeded.run.max_tool_calls = 1;
  exceeded.setStream(async function* () { yield call('one'); yield call('two'); yield* exceeded.finished(); });
  assert.equal((await exceeded.adapter().startRun(exceeded.skill, exceeded.run)).code, 'RUNTIME_TOOL_BUDGET_EXCEEDED');
  const usage = setup(); usage.run.max_output_tokens = 1;
  const outcome = await usage.adapter().startRun(usage.skill, usage.run);
  assert.equal(outcome.code, 'RUNTIME_USAGE_BUDGET_EXCEEDED'); assert.deepEqual(outcome.usage, tokens); assert.equal(outcome.reconciliation_required, true);
});

test('cancellation and a stalled native stream expire without claiming that a prior business write was undone', async () => {
  const cancelled = setup(), controller = new AbortController();
  cancelled.setStream(async function* (signal) { yield call('read'); controller.abort(); if (!signal.aborted) throw new Error('Native signal was not cancelled'); });
  const first = await cancelled.adapter().startRun(cancelled.skill, cancelled.run, controller.signal);
  assert.equal(first.code, 'RUNTIME_CANCEL_REQUESTED'); assert.equal(first.reconciliation_required, true);
  const stalled = setup(); stalled.run.expires_at = new Date(Date.now() + 1500).toISOString();
  stalled.setStream(async function* (signal) { await new Promise<void>(resolve => signal.addEventListener('abort', () => resolve(), { once: true })); });
  const second = await stalled.adapter().startRun(stalled.skill, stalled.run);
  assert.equal(second.code, 'RUNTIME_DEADLINE_EXCEEDED'); assert.equal(second.reconciliation_required, true);
});

test('durable event failure stops processing and provider diagnostics never enter public events', async () => {
  const failed = setup(); failed.setStream(async function* () { yield { type: 'error', message: 'Bearer private-api-secret' }; });
  assert.equal((await failed.adapter().startRun(failed.skill, failed.run)).code, 'RUNTIME_PROVIDER_FAILED');
  assert.ok(!JSON.stringify(failed.events).includes('private-api-secret'));
  const stopped = setup(); let traversed = false;
  stopped.boundary.append = async (_run, event) => { if (event.type === 'tool.started') throw new Error('private storage connection'); stopped.events.push(event); };
  stopped.setStream(async function* () { yield call('read'); traversed = true; yield* stopped.finished(); });
  const result = await stopped.adapter().startRun(stopped.skill, stopped.run);
  assert.equal(result.code, 'RUNTIME_UNAVAILABLE'); assert.equal(traversed, false); assert.equal(result.reconciliation_required, true);
  assert.ok(!JSON.stringify(stopped.events).includes('private storage'));
});

test('verified business commits are independent of native MCP success text and incomplete streams cannot publish completion', async () => {
  const value = setup('file-update'), operation = randomUUID(), file = { type: 'file', id: randomUUID(), version_id: randomUUID() };
  value.result.operation_ids = [operation]; value.result.resource_refs = [file]; value.result.source_refs = [];
  value.receipts.splice(0, 1, { run_id: value.run.id, authority_stamp: value.run.authority_stamp, tool: 'commit_file_update',
    status: 'succeeded', operation_id: operation, committed: true, resource_refs: [file], source_refs: [] });
  const committed = await value.adapter().startRun(value.skill, value.run);
  assert.equal(committed.state, 'completed'); assert.deepEqual(committed.result?.resource_refs, [file]);
  const incomplete = setup(); incomplete.setStream(async function* () { yield incomplete.finished()[0]; });
  assert.equal((await incomplete.adapter().startRun(incomplete.skill, incomplete.run)).code, 'RUNTIME_STREAM_INCOMPLETE');
  const changed = setup(); changed.setStream(async function* () { yield call('one', 'item.started'); yield call('one', 'item.completed', 'inspect_operation'); });
  assert.equal((await changed.adapter().startRun(changed.skill, changed.run)).code, 'RUNTIME_PROTOCOL_INVALID');
});


test('Telegram profile exposes only reads and binds a separate SDK configuration digest', async () => {
  const value=setup(); value.run.channel='telegram';
  let expected:readonly string[]=[];
  value.boundary.assertNativeConfinement=async profile=>{expected=profile.expected_tools;};
  value.setStream(async function*(){yield call('read');yield* value.finished();});
  const result=await value.adapter().startRun(value.skill,value.run);
  assert.equal(result.state,'completed');
  assert.deepEqual(expected,['company/read_resource','company/search_resources','company/inspect_operation']);
  const tools=(value.options?.config?.mcp_servers as {company:{enabled_tools:string[]}}).company.enabled_tools;
  assert.deepEqual(tools,['read_resource','search_resources','inspect_operation']);
  assert.notEqual(nativeConfiguration(value.skill,value.settings).profile.configuration_sha256,nativeConfiguration(value.skill,value.settings,'telegram').profile.configuration_sha256);
  const id=randomUUID();
  const description=await describeProfile({skill_id:'project-briefing',profile_id:id,model:value.settings.model,working_directory:value.settings.working_directory,mcp_endpoint:value.settings.mcp_endpoint},temporary);
  assert.equal(description.profile.telegram_configuration_sha256,description.telegram_native?.configuration_sha256);
  assert.deepEqual(description.telegram_native?.expected_tools,expected);
});

test('Telegram runtime rejects write observations and unsupported skill scopes', async()=>{
  const value=setup();value.run.channel='telegram';
  value.setStream(async function*(){yield call('write','item.completed','draft_post');yield* value.finished();});
  const result=await value.adapter().startRun(value.skill,value.run);
  assert.equal(result.state,'interrupted');assert.equal(result.code,'SKILL_TOOL_DENIED');
  assert.ok(!value.events.some(e=>e.type==='tool.started'||e.type==='run.completed'));
  assert.throws(()=>nativeConfiguration(catalog.get('file-update')!,value.settings,'telegram'),/SKILL_TOOLS_UNAVAILABLE/);
});


test('incident summary exposes four implemented reads in both channels and keeps its release draft', async()=>{
  const skill=catalog.get('incident-summary')!, value=setup();
  const expected=['company/read_resource','company/search_resources','company/inspect_operation','company/read_service_summary'];
  for(const channel of ['web','telegram'] as const){
    const configuration=nativeConfiguration(skill,value.settings,channel);
    assert.deepEqual(configuration.profile.expected_tools,expected);
  }
  const description=await describeProfile({skill_id:'incident-summary',profile_id:randomUUID(),model:value.settings.model,working_directory:value.settings.working_directory,mcp_endpoint:value.settings.mcp_endpoint},temporary);
  assert.deepEqual(description.missing_tools,[]);
  assert.equal(description.ready_for_controlled_installation,false);
  assert.deepEqual(description.telegram_native?.expected_tools,expected);
});

test('monthly finance exposes its implemented reads only on the business channel and stays draft',async()=>{
 const skill=catalog.get('subscription-review')!,value=setup();
 const config=nativeConfiguration(skill,value.settings,'web');
 assert.deepEqual(config.profile.expected_tools,['company/read_resource','company/read_finance_summary','company/inspect_operation','company/list_subscriptions','company/read_subscription_review','company/read_finance_record','company/read_subscription_portfolio']);
 assert.throws(()=>nativeConfiguration(skill,value.settings,'telegram'),/SKILL_TOOLS_UNAVAILABLE/);
 const description=await describeProfile({skill_id:'subscription-review',profile_id:randomUUID(),model:value.settings.model,working_directory:value.settings.working_directory,mcp_endpoint:value.settings.mcp_endpoint},temporary);
 assert.deepEqual(description.missing_tools,[]);assert.equal(description.telegram_native,null);assert.equal(description.skill_state,'draft');assert.equal(description.ready_for_controlled_installation,false);
});


test('metering is durable before an oversized turn or invalid answer can be rejected', async () => {
  const over = setup(); over.run.max_output_tokens = 1;
  const stopped = await over.adapter().startRun(over.skill, over.run);
  assert.equal(stopped.code, 'RUNTIME_USAGE_BUDGET_EXCEEDED');
  assert.deepEqual(over.events.filter(e => e.type === 'run.usage'), [{ type: 'run.usage', usage: tokens }]);
  assert.equal(over.events.at(-1)?.type, 'run.interrupted');
  const malformed = setup(); malformed.setStream(async function* () {
    yield { type: 'item.completed', item: { type: 'agent_message', id: 'invalid', text: 'invalid JSON answer' } };
    yield { type: 'turn.completed', usage: tokens };
  });
  assert.equal((await malformed.adapter().startRun(malformed.skill, malformed.run)).state, 'interrupted');
  assert.ok(malformed.events.some(e => e.type === 'run.usage'));
  assert.ok(!malformed.events.some(e => e.type === 'run.completed'));
});

test('an accounting transport failure cannot be presented as a completed business reply', async () => {
  const value = setup(); const append = value.boundary.append.bind(value.boundary);
  value.boundary.append = async (run, event, signal) => { if (event.type === 'run.usage') throw new RuntimeError('RUNTIME_UNAVAILABLE'); await append(run, event, signal); };
  const outcome = await value.adapter().startRun(value.skill, value.run);
  assert.equal(outcome.state, 'interrupted'); assert.equal(outcome.reconciliation_required, true);
  assert.deepEqual(outcome.usage, tokens); assert.ok(!value.events.some(e => e.type === 'run.completed'));
});
