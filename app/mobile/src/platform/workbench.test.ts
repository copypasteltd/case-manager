import test from 'node:test';
import { workbenchCases } from '../../../packages/contracts/test/workbench-cases';
for (const [name, run] of workbenchCases) test(name, run);
