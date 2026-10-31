import test from 'node:test';
import { followupCases } from '../../../packages/contracts/test/followup-cases';
for (const [name, run] of followupCases) test(name, run);
