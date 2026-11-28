import { useEffect, useState } from 'react';
import { StyleSheet } from 'react-native';
import Animated, { cancelAnimation, Easing, ReduceMotion, useAnimatedStyle, useSharedValue, withTiming } from 'react-native-reanimated';

import { CHIP_POP_MS, chipPop } from '@/killy/strike';
import { KText } from '@/theme/KText';
import { colors, radii } from '@/theme/tokens';

/**
 * The server chip over the parent's count. When the parent on screen is from another server than
 * the one before it, the chip pops: it grows to 1.08 and brightens for 500 ms (DESIGN: Other
 * motion). With reduce motion it just changes.
 */
export function ServerChip({ serverId, name, instant }: { serverId: string; name: string; instant: boolean }) {
  const [shownId, setShownId] = useState(serverId);
  // How many times the server has changed: a new count starts the pop again.
  const [changes, setChanges] = useState(0);
  if (shownId !== serverId) {
    setShownId(serverId);
    setChanges(changes + 1);
  }
  // ms into the pop; CHIP_POP_MS when it is over.
  const t = useSharedValue(CHIP_POP_MS);
  useEffect(() => {
    if (changes === 0) {
      return;
    }
    cancelAnimation(t);
    if (instant) {
      t.set(CHIP_POP_MS);
      return;
    }
    t.set(0);
    t.set(withTiming(CHIP_POP_MS, { duration: CHIP_POP_MS, easing: Easing.linear, reduceMotion: ReduceMotion.Never }));
    return () => cancelAnimation(t);
  }, [changes, instant, t]);
  const style = useAnimatedStyle(() => {
    const p = chipPop(t.value);
    return { transform: [{ scale: p.scale }], backgroundColor: `rgba(144,70,255,${p.tint})` };
  });
  return (
    <Animated.View style={[styles.chip, style]} testID="server-chip">
      <KText kind="mono" style={styles.text} testID="battle-server">
        {name}
      </KText>
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  chip: {
    alignSelf: 'flex-start',
    marginBottom: 10,
    paddingVertical: 4,
    paddingHorizontal: 10,
    borderRadius: radii.pill,
    borderWidth: 1,
    borderColor: 'rgba(191,158,255,0.35)',
  },
  text: { color: colors.purple100, fontSize: 12.5 },
});
