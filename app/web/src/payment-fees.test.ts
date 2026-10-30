import { describe, it } from 'vitest';
import { paymentFeesCases } from '../../packages/contracts/test/payment-fees-cases';
describe('actual payment fee evidence', () => { for (const [name, run] of paymentFeesCases) it(name, run); });
