import test from 'node:test';
import { recoveryCases } from '../../../packages/contracts/test/file-recovery-cases';
for (const [name, run] of recoveryCases) test(name, run);
