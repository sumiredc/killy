import { BlurView } from 'expo-blur';
import { Pressable, StyleSheet, View } from 'react-native';
import { useSharedValue } from 'react-native-reanimated';

import { Killy } from '@/art/Killy';
import { useI18n } from '@/i18n/context';
import { useBattle } from '@/killy/battle-context';
import { KText } from '@/theme/KText';
import { colors, radii } from '@/theme/tokens';

/**
 * The All clear overlay (DESIGN: All clear overlay): a dimmed, blurred backdrop over everything,
 * the tab bar included, with a card of Killy bowing without the scythe, the title, the two counts
 * and Close. It sits at the root of the app, outside the tabs, so the tab bar cannot be drawn
 * over it. When it appears is decided by the battle (the bow plus 600 ms, or at once with reduce
 * motion).
 */
export function AllClearOverlay() {
  const { t } = useI18n();
  const battle = useBattle();
  // Killy stays bowed on the card.
  const bow = useSharedValue(1);
  if (!battle.allClear) {
    return null;
  }
  return (
    <View style={styles.overlay} testID="all-clear" accessibilityViewIsModal>
      <BlurView tint="dark" intensity={30} style={StyleSheet.absoluteFill} testID="all-clear-blur" />
      <View style={styles.dim} />
      <View style={styles.card}>
        <View style={styles.killy} testID="all-clear-killy">
          <Killy width={150} bow={bow} scythe={false} />
          <KText style={styles.heart}>💜</KText>
        </View>
        <KText kind="monoSemiBold" style={styles.title}>
          {t('clearTitle')}
        </KText>
        <KText style={styles.text}>{t('clearText')}</KText>
        <View style={styles.stats}>
          <View style={styles.stat}>
            <KText kind="monoSemiBold" style={styles.statValue} testID="stat-parents">
              {battle.allClear.parents}
            </KText>
            <KText style={styles.statLabel}>{t('statParents')}</KText>
          </View>
          <View style={styles.stat}>
            <KText kind="monoSemiBold" style={styles.statValue} testID="stat-zombies">
              {battle.allClear.zombies}
            </KText>
            <KText style={styles.statLabel}>{t('statZombies')}</KText>
          </View>
        </View>
        <Pressable accessibilityRole="button" style={styles.close} onPress={battle.closeAllClear}>
          <KText kind="monoSemiBold" style={styles.closeLabel}>
            {t('close')}
          </KText>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  overlay: { ...StyleSheet.absoluteFill, justifyContent: 'center', padding: 24, zIndex: 100, elevation: 100 },
  // The mock's rgba(0,0,0,.72) over the blur.
  dim: { ...StyleSheet.absoluteFill, backgroundColor: 'rgba(0,0,0,0.72)' },
  card: { backgroundColor: colors.elev1, borderRadius: radii.card, borderWidth: 1, borderColor: colors.line, padding: 24, gap: 12, alignItems: 'stretch' },
  // The mock's `.clear-killy`: 150 wide, in the middle, the heart at its top right.
  killy: { width: 150, height: 112, alignSelf: 'center' },
  heart: { position: 'absolute', top: 0, right: 24, fontSize: 20 },
  title: { color: colors.text, fontSize: 24, textAlign: 'center' },
  text: { color: colors.muted, fontSize: 14, lineHeight: 22, textAlign: 'center' },
  stats: { flexDirection: 'row', gap: 12 },
  stat: { flex: 1, backgroundColor: colors.elev2, borderRadius: radii.input, padding: 14, gap: 4 },
  statValue: { color: colors.text, fontSize: 28 },
  statLabel: { color: colors.muted, fontSize: 12 },
  closeLabel: { color: '#FFFFFF', fontSize: 16 },
  close: { minHeight: 52, borderRadius: radii.button, backgroundColor: colors.purple, alignItems: 'center', justifyContent: 'center' },
});
