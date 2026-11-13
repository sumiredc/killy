import { StyleSheet } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useI18n } from '@/i18n/context';
import { KText } from '@/theme/KText';
import { colors, gutter } from '@/theme/tokens';

// The settings screen: servers come with #12, the app group with #20 and #21.
export default function SettingsScreen() {
  const { t } = useI18n();
  return (
    <SafeAreaView style={styles.screen} edges={['top']} testID="settings-screen">
      <KText kind="mono" style={styles.title}>
        {t('settings')}
      </KText>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg, paddingHorizontal: gutter },
  title: { color: colors.text, fontSize: 22, marginTop: 12 },
});
