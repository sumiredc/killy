import type { ReactNode } from 'react';
import { Platform, StyleSheet, useWindowDimensions, View } from 'react-native';

import { colors } from '@/theme/tokens';

/** How wide the app is on the Web: a phone screen (DESIGN is drawn for one). */
export const frameWidth = 430;

/**
 * On the Web, the app in a frame at most `frameWidth` wide, in the middle of the page; on a phone
 * browser, which is narrower, it fills the screen. A native build is not framed.
 */
export function PhoneFrame({ children }: { children: ReactNode }) {
  const { width } = useWindowDimensions();
  if (Platform.OS !== 'web') {
    return <>{children}</>;
  }
  return (
    <View style={styles.outer} testID="phone-frame-outer">
      <View style={[styles.frame, width > frameWidth && styles.framed]} testID="phone-frame">
        {children}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  outer: { flex: 1, alignItems: 'center', backgroundColor: colors.bg },
  // The tab bar is placed absolutely: it is the frame's, which is positioned and clips what overflows it.
  frame: {
    flex: 1,
    width: '100%',
    maxWidth: frameWidth,
    position: 'relative',
    overflow: 'hidden',
  },
  // A line on each side, only where the page shows around the frame.
  framed: { borderColor: colors.line, borderLeftWidth: 1, borderRightWidth: 1 },
});
