import assert from 'node:assert/strict';
import test from 'node:test';
import { randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { loadSkills } from './skills.js';
const catalog = await loadSkills(fileURLToPath(new URL('../../../', import.meta.url)));

test('Office creation admits only explicit supported human-selected formats and original-bound existing Office updates', () => {
  const skill = catalog.get('file-update')!;
  for (const [extension, suffix] of [['docx', 'wordprocessingml.document'], ['xlsx', 'spreadsheetml.sheet'], ['pptx', 'presentationml.presentation']]) {
    const media = 'application/vnd.openxmlformats-officedocument.' + suffix;
    skill.validateInput({ folder_id: randomUUID(), expected_folder_version: 2, name: `新文件.${extension}`, media_type: media, instructions: 'Only create this selected file; inspect real completion.' });
    skill.validateInput({ file_id: randomUUID(), base_version_id: randomUUID(), format: media, instructions: 'Only edit exact original workspace units.' });
  }
  assert.equal(skill.manifest.version, '1.6.0'); assert.equal(skill.manifest.state, 'draft');
  assert.throws(() => skill.validateInput({ folder_id: randomUUID(), expected_folder_version: 2, name: 'unsafe.docm', media_type: 'application/vnd.ms-word.document.macroEnabled.12', instructions: 'No macros.' }), /SKILL_INPUT_INVALID/);
});

test('Office creation keeps the same scoped tool set and cannot acquire approval or contract-body adoption tools', () => {
  const skill = catalog.get('file-update')!;
  for (const tool of ['read_office_workspace', 'stage_file_create', 'propose_file_create', 'commit_file_create', 'recover_file_draft']) skill.authorizeTool(tool);
  for (const tool of ['approve_request', 'adopt_contract_body', 'execute_shell', 'write_file']) assert.throws(() => skill.authorizeTool(tool));
  const contractBody = { contract_id: randomUUID(), expected_version: 1, base_revision_id: randomUUID(), content_hash: 'a'.repeat(64), reason: 'Existing text-only adoption context' };
  assert.throws(() => skill.validateInput({ folder_id: randomUUID(), expected_folder_version: 2, name: 'body.docx', media_type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', instructions: 'No implicit Office body adoption.', contract_body: contractBody }), /SKILL_INPUT_INVALID/);
});
