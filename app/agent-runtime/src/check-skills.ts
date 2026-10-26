import { fileURLToPath } from 'node:url';
import { loadSkills } from './skills.js';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const catalog = await loadSkills(root);
console.log(JSON.stringify({ count: catalog.size, skills: [...catalog.values()].map(skill => ({ id: skill.manifest.skill_id, version: skill.manifest.version, state: skill.manifest.state, sha256: skill.sha256, tools: skill.manifest.allowed_tools.length })), actual_model_invoked: false }));
if (catalog.size !== 8) process.exitCode = 1;
