import { test } from 'node:test';
import { fileBatchCases } from '../../../packages/contracts/test/file-batch-cases';
for (const [name, run] of fileBatchCases) test('mobile batch: ' + name, run);
