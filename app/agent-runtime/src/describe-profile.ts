import { fileURLToPath, pathToFileURL } from 'node:url';
import path from 'node:path';
import { nativeConfiguration } from './codex-adapter.js';
import { loadSkills, RuntimeError } from './skills.js';
import { GATEWAY_TOOLS } from './http-boundary.js';

export async function describeProfile(input: { skill_id: string; profile_id: string; model: string; working_directory: string; mcp_endpoint: string; api_base_url?: string }, repository: string) {
  if (!input || !/^[a-f0-9-]{36}$/i.test(input.profile_id) || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,100}$/.test(input.model) || !path.isAbsolute(input.working_directory)) throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID');
  const skill = (await loadSkills(repository)).get(input.skill_id); if (!skill) throw new RuntimeError('SKILL_NOT_FOUND');
  const settings = { ...input, enabled: false, ...(input.api_base_url ? { api_key: 'native-broker-managed' } : {}) };
  const { profile } = nativeConfiguration(skill, settings);
  const telegram = ['project-briefing', 'incident-summary'].includes(input.skill_id) ? nativeConfiguration(skill, settings, 'telegram').profile : null;
  const available = new Set(GATEWAY_TOOLS);
  const missing = skill.manifest.allowed_tools.filter(tool => !available.has(tool));
  return { profile: { id: input.profile_id, skill_id: skill.manifest.skill_id, skill_version: skill.manifest.version, skill_sha256: skill.sha256,
    runtime_version: 'codex-sdk-0.164.0', configuration_sha256: profile.configuration_sha256, telegram_configuration_sha256: telegram?.configuration_sha256 ?? null }, native: profile, telegram_native: telegram, skill_state: skill.manifest.state,
    missing_tools: missing, ready_for_controlled_installation: skill.manifest.state === 'published' && missing.length === 0,
    actual_model_invoked: false, database_modified: false };
}
async function main() {
  const chunks: Buffer[] = []; let length = 0;
  for await (const chunk of process.stdin) { const raw = Buffer.from(chunk); length += raw.length; if (length > 16384) throw new RuntimeError('RUNTIME_CONFIGURATION_INVALID'); chunks.push(raw); }
  const input = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  const result = await describeProfile(input, fileURLToPath(new URL('../../../', import.meta.url)));
  process.stdout.write(JSON.stringify(result) + '\n');
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) main().catch(() => { process.stderr.write('Runtime profile inspection failed.\n'); process.exitCode = 1; });
