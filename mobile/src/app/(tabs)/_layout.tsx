import { BlurView } from 'expo-blur';
import { Tabs } from 'expo-router';
import { StyleSheet } from 'react-native';

import { useI18n } from '@/i18n/context';
import { colors, fontFor } from '@/theme/tokens';

export default function TabsLayout() {
  const { t } = useI18n();
  const battle = t('tabBattle');
  const settings = t('settings');
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        sceneStyle: { backgroundColor: colors.bg },
        // DESIGN Tab bar: 84px with the home-indicator area, translucent black with blur, top border line.
        tabBarStyle: { position: 'absolute', backgroundColor: 'transparent', borderTopColor: colors.line, height: 84 },
        tabBarBackground: () => (
          <BlurView tint="dark" intensity={40} style={[StyleSheet.absoluteFill, { backgroundColor: 'rgba(0,0,0,0.6)' }]} />
        ),
        tabBarActiveTintColor: colors.purple300,
        tabBarInactiveTintColor: colors.muted,
        tabBarIcon: () => null,
      }}>
      <Tabs.Screen
        name="index"
        options={{ title: battle, tabBarAccessibilityLabel: battle, tabBarLabelStyle: { fontFamily: fontFor(battle, 'mono'), fontSize: 12 } }}
      />
      <Tabs.Screen
        name="settings"
        options={{ title: settings, tabBarAccessibilityLabel: settings, tabBarLabelStyle: { fontFamily: fontFor(settings, 'mono'), fontSize: 12 } }}
      />
    </Tabs>
  );
}
