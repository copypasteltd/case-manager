import test from 'node:test';
import { comparisonCases } from '../../../packages/contracts/test/file-comparison-cases';
for (const [name, run] of comparisonCases) test(name, run);
