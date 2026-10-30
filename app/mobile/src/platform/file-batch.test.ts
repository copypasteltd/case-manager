import { test } from 'node:test';
import { fileBatchCases } from '../../../packages/contracts/test/file-batch-cases';
import { officeAuthorCases } from '../../../packages/contracts/test/office-author-cases';
for (const [name, run] of officeAuthorCases) test('mobile Office: ' + name, run);
for (const [name, run] of fileBatchCases) test('mobile batch: ' + name, run);
