import { BlurView } from 'expo-blur';
import { Tabs, usePathname, useRouter } from 'expo-router';
import { StyleSheet, View } from 'react-native';

import { BattleIcon, SettingsIcon } from '@/art/TabIcons';
import { useI18n } from '@/i18n/context';
import { useBattle } from '@/killy/battle-context';
import { useOpenBattleOnTap } from '@/killy/notifications-context';
import { colors, fontFor } from '@/theme/tokens';

export default function TabsLayout() {
  // Drawn after the splash, so the tabs are there to move to.
  const router = useRouter();
  useOpenBattleOnTap(() => router.navigate('/'));
  const { t } = useI18n();
  const battle = useBattle();
  const onSettings = usePathname() === '/settings';
  const battleLabel = t('tabBattle');
  const settings = t('settings');
  const dim = battle.busy ? 0.4 : 1;
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
      }}>
      <Tabs.Screen
        name="index"
        options={{
          title: battleLabel,
          tabBarIcon: ({ color }) => <BattleIcon color={color} />,
          tabBarAccessibilityLabel: battleLabel,
          tabBarLabelStyle: { fontFamily: fontFor(battleLabel, 'mono'), fontSize: 12 },
          // While Settings is open, how many zombie processes are waiting (DESIGN: Settings tab).
          tabBarBadge: onSettings && battle.waiting > 0 ? battle.waiting : undefined,
          tabBarBadgeStyle: { backgroundColor: colors.purple, color: '#FFFFFF' },
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: settings,
          // DESIGN Tab bar: disabled while a kill animation is running, at 40% as the mock's `.tab:disabled`.
          // On the icon and the label: the tab's button sets its own opacity for presses.
          tabBarIcon: ({ color }) => (
            <View style={{ opacity: dim }} testID="tab-settings-icon">
              <SettingsIcon color={color} />
            </View>
          ),
          tabBarAccessibilityLabel: settings,
          tabBarLabelStyle: { fontFamily: fontFor(settings, 'mono'), fontSize: 12, opacity: dim },
        }}
        listeners={{
          // Not while a kill is under way: its sequence and outcome belong on the battle screen.
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
