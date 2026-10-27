import { createHash, timingSafeEqual } from 'node:crypto';
import { readFile, realpath, readdir, stat } from 'node:fs/promises';
import path from 'node:path';
import { Ajv2020, type ValidateFunction } from 'ajv/dist/2020.js';
import { parseDocument } from 'yaml';

export class RuntimeError extends Error {
  constructor(public readonly code: string) { super(code); }
}
export interface Manifest {
  skill_id: string; version: string; state: 'draft' | 'tested' | 'pilot' | 'published' | 'disabled';
  owner_role: string; runtime_compatibility: string; authorization_level: 'L0' | 'L1' | 'L2';
  allowed_tools: string[]; completion_tools: string[]; input_schema_ref: string; output_schema_ref: string;
  instructions_ref: string; reference_docs: string[]; max_tool_calls: number; timeout_seconds: number; test_case_refs: string[];
}
export const KNOWN_TOOLS = new Set(['search_resources', 'list_directory', 'read_resource', 'propose_project', 'create_project',
  'update_task_dependency', 'create_task', 'create_risk', 'update_risk', 'create_issue', 'update_issue', 'create_deliverable', 'update_deliverable', 'create_milestone', 'update_project', 'update_task', 'transition_task', 'transition_project', 'update_milestone', 'transition_milestone', 'create_file_draft', 'propose_file_update', 'commit_file_update', 'stage_file_create', 'propose_file_create', 'commit_file_create', 'propose_organization',
  'commit_organization', 'draft_post', 'publish_post', 'draft_request', 'submit_request', 'list_subscriptions', 'read_subscription_review', 'read_finance_record', 'read_finance_summary',
  'read_service_summary', 'propose_notification', 'send_authorized_notification', 'inspect_operation', 'cancel_run']);
const READ_TOOLS = new Set(['search_resources', 'list_directory', 'read_resource', 'list_subscriptions', 'read_subscription_review', 'read_finance_record', 'read_finance_summary', 'read_service_summary', 'inspect_operation']);
const PLAN_COMMIT_TOOLS = new Set(['commit_organization', 'send_authorized_notification']);
const COMMON = 'https://copypaste.hk/schemas/company-skills/common.v1.json';

function freeze<T>(value: T): T {
  if (value && typeof value === 'object') { for (const item of Object.values(value)) freeze(item); Object.freeze(value); }
  return value;
}
async function boundedFile(root: string, relative: string, maximum = 65536): Promise<Buffer> {
  const target = path.resolve(root, relative), inside = path.relative(root, target);
  if (!inside || inside.startsWith('..' + path.sep) || inside === '..' || path.isAbsolute(inside)) throw new RuntimeError('SKILL_PATH_OUTSIDE_ROOT');
  const canonical = await realpath(target), resolved = path.relative(root, canonical);
  if (resolved.startsWith('..' + path.sep) || resolved === '..' || path.isAbsolute(resolved)) throw new RuntimeError('SKILL_PATH_OUTSIDE_ROOT');
  const info = await stat(canonical); if (!info.isFile() || info.size > maximum) throw new RuntimeError('SKILL_FILE_LIMIT');
  const bytes = await readFile(canonical); if (bytes.length > maximum) throw new RuntimeError('SKILL_FILE_LIMIT'); return bytes;
}
function safeJSON(bytes: Buffer): unknown { try { return JSON.parse(bytes.toString('utf8')); } catch { throw new RuntimeError('SKILL_JSON_INVALID'); } }
function checkSize(value: unknown) {
  try { if (Buffer.byteLength(JSON.stringify(value)) > 1048576) throw new Error(); } catch { throw new RuntimeError('SKILL_PAYLOAD_INVALID'); }
}
function makeValidator() {
  const ajv = new Ajv2020({ strict: true, allErrors: false, coerceTypes: false });
  ajv.addFormat('uuid', /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
  ajv.addFormat('date', value => {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value) || value.startsWith('0000-')) return false;
    const day = new Date(value + 'T00:00:00Z');
    return Number.isFinite(day.getTime()) && day.toISOString().slice(0, 10) === value;
  });
  ajv.addFormat('date-time', value => {
    if (!/^\d{4}-\d{2}-\d{2}T(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(\.\d+)?(Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.test(value) || !Number.isFinite(Date.parse(value))) return false;
    const day = new Date(value.slice(0, 10) + 'T00:00:00Z');
    return value.slice(0, 4) !== '0000' && Number.isFinite(day.getTime()) && day.toISOString().slice(0, 10) === value.slice(0, 10);
  });
  return ajv;
}

function selfContainedSchema(schema: unknown, common: unknown, seen: readonly string[] = [], depth = 0): unknown {
  if (depth > 64) throw new RuntimeError('SKILL_SCHEMA_DEPTH');
  if (!schema || typeof schema !== 'object') return schema;
  if (Array.isArray(schema)) return schema.map(item => selfContainedSchema(item, common, seen, depth + 1));
  const node = schema as Record<string, unknown>, result: Record<string, unknown> = {};
  if ('$ref' in node) {
    const reference = node.$ref;
    if (typeof reference !== 'string' || !reference.startsWith(COMMON + '#/') || seen.includes(reference)) throw new RuntimeError('SKILL_SCHEMA_REFERENCE_INVALID');
    let target = common;
    for (const part of reference.slice(COMMON.length + 2).split('/').map(value => value.replaceAll('~1', '/').replaceAll('~0', '~'))) {
      if (!target || typeof target !== 'object' || !Object.hasOwn(target, part)) throw new RuntimeError('SKILL_SCHEMA_REFERENCE_INVALID');
      target = (target as Record<string, unknown>)[part];
    }
    if (!target || typeof target !== 'object' || Array.isArray(target)) throw new RuntimeError('SKILL_SCHEMA_REFERENCE_INVALID');
    Object.assign(result, selfContainedSchema(target, common, [...seen, reference], depth + 1));
  }
  for (const [name, value] of Object.entries(node)) if (name !== '$ref') result[name] = selfContainedSchema(value, common, seen, depth + 1);
  return result;
}

export class SkillPackage {
  readonly manifest: Readonly<Manifest>;
  constructor(manifest: Manifest, public readonly sha256: string, public readonly instructions: string,
    public readonly inputSchema: Readonly<Record<string, unknown>>, public readonly outputSchema: Readonly<Record<string, unknown>>,
    private readonly input: ValidateFunction, private readonly output: ValidateFunction,
    public readonly modelOutputSchema: Readonly<Record<string, unknown>>) { this.manifest = freeze(manifest); freeze(inputSchema); freeze(outputSchema); freeze(modelOutputSchema); }
  validateInput(value: unknown): void {
    checkSize(value); if (!this.input(value)) throw new RuntimeError('SKILL_INPUT_INVALID');
    const window = value && typeof value === 'object' ? (value as { window?: { from: string; to: string } }).window : undefined;
    if (window && Date.parse(window.from) >= Date.parse(window.to)) throw new RuntimeError('SKILL_WINDOW_INVALID');
  }
  validateOutput(value: unknown): void { checkSize(value); if (!this.output(value)) throw new RuntimeError('SKILL_OUTPUT_INVALID'); }
  authorizeTool(name: string): void { if (!this.manifest.allowed_tools.includes(name)) throw new RuntimeError('SKILL_TOOL_DENIED'); }
  assertRunnable(registryHash: string, availableTools: ReadonlySet<string>, pilot = false): void {
    if (this.manifest.state !== 'published' && !(pilot && this.manifest.state === 'pilot')) throw new RuntimeError('SKILL_NOT_PUBLISHED');
    if (!/^[0-9a-f]{64}$/.test(registryHash) || !timingSafeEqual(Buffer.from(registryHash), Buffer.from(this.sha256))) throw new RuntimeError('SKILL_RELEASE_CHANGED');
    if (this.manifest.allowed_tools.some(name => !availableTools.has(name))) throw new RuntimeError('SKILL_TOOLS_UNAVAILABLE');
  }
}

export async function loadSkills(repository: string): Promise<ReadonlyMap<string, SkillPackage>> {
  const sourceRoot = await realpath(path.join(repository, 'app/skills')), docsRoot = await realpath(path.join(repository, 'docs/skills'));
  const commonBytes = await boundedFile(sourceRoot, 'common.schema.json'), manifestBytes = await boundedFile(sourceRoot, 'manifest.schema.json');
  const common = safeJSON(commonBytes), ajv = makeValidator(); ajv.addSchema(common as object, COMMON);
  const validateManifest = ajv.compile(safeJSON(manifestBytes) as object), catalog = new Map<string, SkillPackage>();
  const entries = (await readdir(sourceRoot, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name));
  for (const entry of entries) {
    if (entry.isSymbolicLink()) throw new RuntimeError('SKILL_SYMLINK_FOLDER'); if (!entry.isDirectory()) continue;
    if (!/^[a-z][a-z0-9-]{1,63}$/.test(entry.name)) throw new RuntimeError('SKILL_NAME_INVALID');
    const rawManifest = await boundedFile(sourceRoot, entry.name + '/manifest.json'), parsed = safeJSON(rawManifest);
    if (!validateManifest(parsed)) throw new RuntimeError('SKILL_MANIFEST_INVALID'); const manifest = parsed as Manifest;
    if (manifest.skill_id !== entry.name || manifest.instructions_ref !== entry.name + '/SKILL.md' || !manifest.reference_docs.includes('execution-boundaries.md')) throw new RuntimeError('SKILL_IDENTITY_MISMATCH');
    if (manifest.allowed_tools.some(name => !KNOWN_TOOLS.has(name)) || manifest.completion_tools.some(name => !manifest.allowed_tools.includes(name)) || (manifest.authorization_level !== 'L0' && !manifest.completion_tools.length)) throw new RuntimeError('SKILL_TOOL_POLICY_INVALID');
    if ((manifest.authorization_level === 'L0' && (manifest.completion_tools.length || manifest.allowed_tools.some(name => !READ_TOOLS.has(name))))
      || (manifest.authorization_level !== 'L2' && manifest.allowed_tools.some(name => PLAN_COMMIT_TOOLS.has(name)))) throw new RuntimeError('SKILL_TOOL_POLICY_INVALID');
    const instructionBytes = await boundedFile(docsRoot, manifest.instructions_ref), instructions = instructionBytes.toString('utf8');
    const front = /^---\r?\n([\s\S]+?)\r?\n---\r?\n/.exec(instructions); if (!front) throw new RuntimeError('SKILL_FRONTMATTER_INVALID');
    const document = parseDocument(front[1]); if (document.errors.length) throw new RuntimeError('SKILL_FRONTMATTER_INVALID');
    const identity = document.toJS(); if (identity?.name !== manifest.skill_id || typeof identity.description !== 'string' || !identity.description.trim()) throw new RuntimeError('SKILL_FRONTMATTER_INVALID');
    const inputBytes = await boundedFile(sourceRoot, entry.name + '/' + manifest.input_schema_ref), outputBytes = await boundedFile(sourceRoot, entry.name + '/' + manifest.output_schema_ref);
    const inputSchema = safeJSON(inputBytes) as Record<string, unknown>, outputSchema = safeJSON(outputBytes) as Record<string, unknown>;
    const hash = createHash('sha256'), instructionsParts: string[] = [];
    for (const [name, bytes] of [['manifest', rawManifest], ['common.schema', commonBytes], ['manifest.schema', manifestBytes], ['input.schema', inputBytes], ['output.schema', outputBytes], ['instructions', instructionBytes]] as const) hash.update(name + '\0').update(bytes).update('\0');
    for (const ref of [...manifest.reference_docs].sort()) { const bytes = await boundedFile(docsRoot, ref); hash.update(ref + '\0').update(bytes).update('\0'); instructionsParts.push(bytes.toString('utf8')); }
    instructionsParts.push(instructions.replace(/^---[\s\S]+?---\r?\n/, ''));
    const modelOutputSchema = selfContainedSchema(outputSchema, common) as Record<string, unknown>;
    catalog.set(entry.name, new SkillPackage(manifest, hash.digest('hex'), instructionsParts.join('\n\n'), inputSchema, outputSchema, ajv.compile(inputSchema), ajv.compile(outputSchema), modelOutputSchema));
  }
  return catalog;
}
