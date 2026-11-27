import type { Key } from '@/i18n';

import type { ClaimErrorKind } from './client';

/** What is said for each way adding a server (a claim or the public demo) can fail. */
export const claimMessages: Record<ClaimErrorKind, Key> = {
  code_unknown: 'pairError',
  code_used: 'pairError',
  code_expired: 'pairError',
  rate_limited: 'pairRateLimited',
  unreachable: 'pairUnreachable',
  not_registered: 'deviceNotRegistered',
  demo_not_configured: 'demoNotConfigured',
  other: 'pairFailed',
};
