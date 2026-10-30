import { ApiError } from './api';

// A missing or unreadable response can follow a committed cash operation.
// Do not let a new source identity or edited body bypass that uncertainty.
export function costPaymentOutcomeUncertain(error: unknown, responseReceived: boolean) {
  return responseReceived || error instanceof ApiError && (error.status === 0 || error.status >= 500 || ['RESULT_UNCONFIRMED', 'INVALID_RESPONSE'].includes(error.code));
}
