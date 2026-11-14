import { Redirect, router, useLocalSearchParams, useNavigation } from 'expo-router';
import { useEffect } from 'react';

import { useServerURL } from '@/config-context';
import { checkPairParams } from '@/killy/pair-url';

/**
 * killy://pair?code=…&server=…, as when the phone's own camera reads the QR code of killy agent
 * pair. The link gets the same checks as a QR code read in the app, then Settings opens the Add
 * server sheet with the code in its field; nothing is sent until Add is pressed.
 */
export default function PairRoute() {
  const link = checkPairParams(useLocalSearchParams(), useServerURL());
  const params: { pairCode?: string; pairError?: string } = link.ok ? { pairCode: link.code } : { pairError: link.reason };
  // When the app was already open its tabs are in the stack: go back to them instead of stacking
  // another Settings screen, whose Add server sheet iOS would refuse to present over the first.
  const tabsOpen = useNavigation().getState()?.routes.some((r) => r.name === '(tabs)') ?? false;
  const { pairCode, pairError } = params;
  useEffect(() => {
    if (tabsOpen) {
      router.dismissTo({ pathname: '/settings', params: pairCode !== undefined ? { pairCode } : { pairError } });
    }
  }, [tabsOpen, pairCode, pairError]);
  return tabsOpen ? null : <Redirect href={{ pathname: '/settings', params }} />;
}
