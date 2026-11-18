import { Pressable, StyleSheet } from 'react-native';
import Animated, { type SharedValue, useAnimatedStyle } from 'react-native-reanimated';

import { KText } from '@/theme/KText';
import { colors, radii } from '@/theme/tokens';

/**
 * The hold-to-kill button (DESIGN: Battle tab, Hold to kill): it fills with red from the left as
 * `charge` goes from 0 to 1. The label stays above the fill.
 */
export function KillButton({
  label,
  sub,
  enabled,
  charge,
  onPressIn,
  onPressOut,
}: {
  label: string;
  sub?: string;
  enabled: boolean;
  charge: SharedValue<number>;
  onPressIn: () => void;
  onPressOut: () => void;
}) {
  const fill = useAnimatedStyle(() => ({ transform: [{ scaleX: charge.value }] }));
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      disabled={!enabled}
      onPressIn={onPressIn}
      onPressOut={onPressOut}
      // The mock's `.kill:active`: pressed in a little while held.
      style={({ pressed }) => [styles.kill, !enabled && styles.killDisabled, pressed && enabled && styles.pressed]}>
      <Animated.View style={[styles.fill, fill]} testID="kill-fill" pointerEvents="none" />
      <KText kind="monoSemiBold" style={styles.label}>
        {label}
      </KText>
      {sub !== undefined && (
        <KText kind="mono" style={styles.sub}>
          {sub}
        </KText>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  kill: {
    marginTop: 12,
    minHeight: 64,
    borderRadius: radii.button,
    backgroundColor: colors.purple,
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: 8,
    overflow: 'hidden',
  },
  killDisabled: { backgroundColor: colors.elev3 },
  pressed: { transform: [{ scale: 0.985 }] },
  // The mock's `.kill-fill`: the whole button, scaled from the left edge.
  fill: { ...StyleSheet.absoluteFill, backgroundColor: colors.red, transformOrigin: 'left' },
  label: { color: '#FFFFFF', fontSize: 16 },
  sub: { color: colors.purple100, fontSize: 11, marginTop: 2 },
});
