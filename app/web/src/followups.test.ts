import { describe, it } from 'vitest';
import { followupCases } from '../../packages/contracts/test/followup-cases';
describe('current-authority application and deliverable followups', () => { for (const [name, run] of followupCases) it(name, run); });
