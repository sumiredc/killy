import { Redirect, router, useLocalSearchParams, useNavigation } from 'expo-router';
import { useEffect } from 'react';

import { useServerURL } from '@/config-context';
import { usePairLinks } from '@/killy/pair-link';
import { checkPairParams, type PairCheck } from '@/killy/pair-url';

/**
 * killy://pair?code=…&server=…, as when the phone's own camera reads the QR code of killy agent
 * pair. The link gets the same checks as a QR code read in the app, then Settings opens the Add
 * server sheet with the code in its field; nothing is sent until Add is pressed.
 */
export default function PairRoute() {
  const params = useLocalSearchParams();
  const appServer = useServerURL();
  const { offer } = usePairLinks();
  // When the app was already open its tabs are in the stack: go back to them instead of stacking
  // another Settings screen, whose Add server sheet iOS would refuse to present over the first.
  const tabsOpen = useNavigation().getState()?.routes.some((r) => r.name === '(tabs)') ?? false;
  // A string, so the effect runs once per link: params is a new object on every render.
  const checked = JSON.stringify(checkPairParams(params, appServer));
  useEffect(() => {
    offer(JSON.parse(checked) as PairCheck);
    if (tabsOpen) {
      router.dismissTo('/settings');
    }
  }, [checked, offer, tabsOpen]);
  return tabsOpen ? null : <Redirect href="/settings" />;
}
