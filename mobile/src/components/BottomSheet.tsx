import { type ReactNode, useEffect, useState } from 'react';
import { Animated, Easing, KeyboardAvoidingView, Modal, StyleSheet } from 'react-native';

import { useReduceMotion } from '@/killy/motion';
import { SHEET_BACKDROP_MS, SHEET_EASING, SHEET_FROM, SHEET_UP_MS } from '@/killy/sheet';

/** How long to wait for the Modal to say it is shown before starting the motion anyway. */
const SHOW_FALLBACK_MS = 500;

/**
 * What the Add server sheet and the server sheet stand in (DESIGN: Other motion): a dimmed backdrop
 * that fades in over 200 ms and a sheet that rises from 40% of its height over 300 ms. It leaves at
 * once, as the mock's sheet does when it is hidden. With reduce motion both are simply there.
 *
 * The Modal is not animated by the OS (`animationType="none"`), because that would slide the
 * backdrop with the sheet. The motion starts when the Modal is on screen (`onShow`): on the phone
 * the Modal appears some tenths of a second after it is mounted, and a motion started at the mount
 * is over before anyone sees it. If `onShow` never comes, it starts anyway after half a second.
 */
export function BottomSheet({
  onRequestClose,
  modalTestID,
  children,
}: {
  onRequestClose: () => void;
  modalTestID?: string;
  children: ReactNode;
}) {
  const reduce = useReduceMotion();
  // The sheet's own height, to move it by pixels.
  const [height, setHeight] = useState<number | null>(null);
  // The Modal is on screen.
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const id = setTimeout(() => setShown(true), SHOW_FALLBACK_MS);
    return () => clearTimeout(id);
  }, []);
  // 0 hidden, 1 in place.
  const [up] = useState(() => new Animated.Value(0));
  const [fade] = useState(() => new Animated.Value(0));
  useEffect(() => {
    // Not started before the OS has said whether to move, the sheet has been measured and the Modal is there.
    if (reduce === null || height === null || !shown) {
      return;
    }
    if (reduce) {
      up.setValue(1);
      fade.setValue(1);
      return;
    }
    up.setValue(0);
    fade.setValue(0);
    const run = Animated.parallel([
      Animated.timing(up, { toValue: 1, duration: SHEET_UP_MS, easing: Easing.bezier(...SHEET_EASING), useNativeDriver: true }),
      Animated.timing(fade, { toValue: 1, duration: SHEET_BACKDROP_MS, easing: Easing.linear, useNativeDriver: true }),
    ]);
    run.start();
    return () => run.stop();
  }, [reduce, height, shown, up, fade]);
  const lift = up.interpolate({ inputRange: [0, 1], outputRange: [SHEET_FROM * (height ?? 0), 0] });
  return (
    <Modal transparent animationType="none" onRequestClose={onRequestClose} onShow={() => setShown(true)} visible testID={modalTestID}>
      <Animated.View style={[StyleSheet.absoluteFill, styles.backdrop, { opacity: fade }]} testID="sheet-backdrop" />
      {/* padding on both: the Modal's window is not resized on Android, so the keyboard would cover the buttons. */}
      <KeyboardAvoidingView style={styles.wrap} behavior="padding" pointerEvents="box-none">
        <Animated.View
          style={[styles.rise, { transform: [{ translateY: lift }] }]}
          testID="sheet-rise"
          onLayout={(e) => setHeight(e.nativeEvent.layout.height)}>
          {children}
        </Animated.View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { backgroundColor: 'rgba(0,0,0,0.6)' },
  wrap: { flex: 1, justifyContent: 'flex-end' },
  // Full width (on the Web, the sheet inside sets its own width); the sheet is as high as its content.
  rise: { alignSelf: 'stretch' },
});
