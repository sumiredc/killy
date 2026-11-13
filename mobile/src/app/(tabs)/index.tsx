import { StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useI18n } from '@/i18n/context';
import { KText } from '@/theme/KText';
import { colors, gutter, radii } from '@/theme/tokens';

// The battle screen: the arena and the kill button come with #13.
export default function BattleScreen() {
  const { t } = useI18n();
  return (
    <SafeAreaView style={styles.screen} edges={['top']} testID="battle-screen">
      <View style={styles.arena}>
        <KText kind="monoSemiBold" style={styles.title}>
          {t('emptyNoServerTitle')}
        </KText>
        <KText style={styles.text}>{t('emptyNoServerText')}</KText>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  // The tab bar floats over the bottom 84px (it is translucent), so the arena stops above it.
  screen: { flex: 1, backgroundColor: colors.bg, paddingHorizontal: gutter, paddingBottom: 84 + gutter },
  arena: {
    flex: 1,
    backgroundColor: colors.elev1,
    borderColor: colors.line,
    borderWidth: 1,
    borderRadius: radii.card,
    padding: 20,
    justifyContent: 'center',
  },
  title: { color: colors.text, fontSize: 18, marginBottom: 8 },
  text: { color: colors.muted, fontSize: 14, lineHeight: 20 },
});
