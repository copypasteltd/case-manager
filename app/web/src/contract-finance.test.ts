import { describe, it } from 'vitest';
import { contractFinanceCases } from '../../packages/contracts/test/contract-finance-cases';
describe('contract principal and actual refund cash', () => { for (const [name, run] of contractFinanceCases) it(name, run); });
