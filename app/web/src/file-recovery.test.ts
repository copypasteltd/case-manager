import { describe, it } from 'vitest';
import { recoveryCases } from '../../packages/contracts/test/file-recovery-cases';
describe('explicit saved file draft recovery', () => { for (const [name, run] of recoveryCases) it(name, run); });
