import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { CodexAdapter, type ClientFactory, type NativeSettings, type RunBinding } from './codex-adapter.js';
import { HTTPRuntimeBoundary } from './http-boundary.js';
import { loadSkills, RuntimeError } from './skills.js';

export interface WorkerInput {
  repository: string; settings: NativeSettings;
  run: RunBinding & { profile: { id: string; skill_id: string; skill_version: string; skill_sha256: string; runtime_version: string } };
}

// The CLI exposes no test/factory option. Controlled clients can only be injected by
// repository tests calling this module directly; production always constructs Codex.
export async function executeWorker(input: WorkerInput, signal?: AbortSignal, factory?: ClientFactory) {
  if (!input || !path.isAbsolute(input.repository) || !input.settings || !input.run?.profile) throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID');
  const { run, settings } = input, profile = run.profile;
  const boundary = new HTTPRuntimeBoundary(run, settings.mcp_endpoint);
  try {
    if (profile.runtime_version !== 'codex-sdk-0.164.0' || settings.profile_id !== profile.id || run.skill_sha256 !== profile.skill_sha256) throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID');
    const skills = await loadSkills(input.repository), skill = skills.get(profile.skill_id);
    if (!skill || skill.manifest.version !== profile.skill_version || skill.sha256 !== profile.skill_sha256) throw new RuntimeError('SKILL_HASH_MISMATCH');
    return await new CodexAdapter(settings, boundary, factory).startRun(skill, run, signal);
  } catch {
    const stop = new AbortController(), timer = setTimeout(() => stop.abort(), 1000);
    try { await boundary.append(run, { type: 'run.interrupted', code: 'RUNTIME_UNAVAILABLE', reconciliation_required: false }, stop.signal); }
    catch { /* Go controller handles a missing terminal observation after process exit. */ }
    finally { clearTimeout(timer); }
    throw new RuntimeError('RUNTIME_UNAVAILABLE');
  }
}

async function main() {
  const chunks: Buffer[] = []; let length = 0;
  for await (const chunk of process.stdin) { const bytes = Buffer.from(chunk); length += bytes.length;
    if (length > 1048576) throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID'); chunks.push(bytes); }
  let input: WorkerInput; try { input = JSON.parse(Buffer.concat(chunks).toString('utf8')) as WorkerInput; }
  catch { throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID'); }
  const controller = new AbortController(), cancel = () => controller.abort();
  process.once('SIGTERM', cancel); process.once('SIGINT', cancel);
  try { const outcome = await executeWorker(input, controller.signal); if (outcome.state !== 'completed') process.exitCode = 1; }
  finally { process.removeListener('SIGTERM', cancel); process.removeListener('SIGINT', cancel); }
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  main().catch(() => { process.stderr.write('Agent worker stopped.\n'); process.exitCode = 1; });
}
