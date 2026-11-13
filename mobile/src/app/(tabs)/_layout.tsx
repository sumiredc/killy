import { BlurView } from 'expo-blur';
import { Tabs, usePathname } from 'expo-router';
import { StyleSheet } from 'react-native';

import { useI18n } from '@/i18n/context';
import { useBattle } from '@/killy/battle-context';
import { colors, fontFor } from '@/theme/tokens';

export default function TabsLayout() {
  const { t } = useI18n();
  const battle = useBattle();
  const onSettings = usePathname() === '/settings';
  const battleLabel = t('tabBattle');
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
        options={{
          title: battleLabel,
          tabBarAccessibilityLabel: battleLabel,
          tabBarLabelStyle: { fontFamily: fontFor(battleLabel, 'mono'), fontSize: 12 },
          // While Settings is open, how many zombie processes are waiting (DESIGN: Settings tab).
          tabBarBadge: onSettings && battle.waiting > 0 ? battle.waiting : undefined,
          tabBarBadgeStyle: { backgroundColor: colors.purple, color: '#FFFFFF' },
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{ title: settings, tabBarAccessibilityLabel: settings, tabBarLabelStyle: { fontFamily: fontFor(settings, 'mono'), fontSize: 12 } }}
        listeners={{
          // Not while a kill is in flight: its outcome belongs on the battle screen.
          tabPress: (e) => {
            if (battle.busy) {
              e.preventDefault();
            }
          },
        }}
      />
    </Tabs>
  );
}
