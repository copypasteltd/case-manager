import { describe, it } from 'vitest';
import { scopeTransferCases } from '../../packages/contracts/test/scope-transfer-cases';
describe('human reviewed scope transfers', () => { for (const [name, run] of scopeTransferCases) it(name, run); });
