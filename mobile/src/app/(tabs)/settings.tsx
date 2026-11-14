import { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { AddServerSheet } from '@/components/AddServerSheet';
import { ServerSheet } from '@/components/ServerSheet';
import { useI18n } from '@/i18n/context';
import { useKilly } from '@/killy/context';
import { serverRow } from '@/killy/server-row';
import { KText } from '@/theme/KText';
import { colors, gutter, radii } from '@/theme/tokens';

// The settings screen. The app group comes with #20 and #21.
export default function SettingsScreen() {
  const { t } = useI18n();
  const { state } = useKilly();
  const [adding, setAdding] = useState(false);
  const [openId, setOpenId] = useState<string | null>(null);
  const servers = state?.servers ?? [];
  // Looked up in every state, so the sheet shows the latest values and closes once the server is gone.
  const open = servers.find((s) => s.id === openId);
  return (
    <SafeAreaView style={styles.screen} edges={['top']} testID="settings-screen">
      <KText kind="mono" style={styles.title}>
        {t('settings')}
      </KText>
      <ScrollView contentContainerStyle={styles.body}>
        <KText style={styles.groupTitle}>{t('servers')}</KText>
        {servers.length === 0 ? (
          <View style={styles.groupEmpty}>
            <KText style={styles.groupEmptyText}>{t('noServers')}</KText>
          </View>
        ) : (
          <View style={styles.group}>
            {servers.map((s, i) => {
              const row = serverRow(s);
              return (
                <Pressable
                  key={s.id}
                  accessibilityRole="button"
                  onPress={() => setOpenId(s.id)}
                  style={[styles.row, i > 0 && styles.rowDivider]}
                  testID="server-row">
                  <View style={styles.rowMain}>
                    <KText kind="mono" style={styles.serverName} numberOfLines={1}>
                      {row.name}
                    </KText>
                    <KText style={styles.rowSub}>{t(row.sub.key, row.sub.params)}</KText>
                  </View>
                  {row.badge !== null && (
                    <View style={styles.badge}>
                      <KText kind="mono" style={styles.badgeText} testID="server-badge">
                        {row.badge}
                      </KText>
                    </View>
                  )}
                  <KText style={styles.chevron}>›</KText>
                </Pressable>
              );
            })}
          </View>
        )}
        <Pressable accessibilityRole="button" style={styles.addButton} onPress={() => setAdding(true)}>
          <KText style={styles.addButtonText}>＋</KText>
          <KText style={styles.addButtonText}>{t('addServer')}</KText>
        </Pressable>
      </ScrollView>
      {adding && <AddServerSheet onClose={() => setAdding(false)} />}
      {open !== undefined && <ServerSheet server={open} onClose={() => setOpenId(null)} />}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg, paddingHorizontal: gutter },
  title: { color: colors.text, fontSize: 22, marginTop: 12 },
  // The tab bar floats over the bottom 84px.
  body: { paddingTop: 8, paddingBottom: 84 + 32 },
  groupTitle: { color: colors.muted, fontSize: 13, marginTop: 22, marginBottom: 8, marginHorizontal: 4 },
  // borderStyle is set even when solid: Android keeps the empty state's dashed border when this View replaces it.
  group: { borderRadius: radii.group, borderWidth: 1, borderStyle: 'solid', borderColor: colors.line, backgroundColor: colors.elev1, overflow: 'hidden' },
  groupEmpty: { padding: 18, borderRadius: radii.group, borderWidth: 1, borderStyle: 'dashed', borderColor: colors.line },
  groupEmptyText: { color: colors.muted, fontSize: 14, lineHeight: 22 },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 12, paddingHorizontal: 14, minHeight: 56 },
  rowDivider: { borderTopWidth: 1, borderTopColor: colors.line },
  rowMain: { flex: 1, minWidth: 0, gap: 2 },
  serverName: { color: colors.text, fontSize: 15 },
  rowSub: { color: colors.muted, fontSize: 12.5 },
  badge: { paddingVertical: 3, paddingHorizontal: 9, borderRadius: radii.pill, backgroundColor: 'rgba(144,70,255,0.2)' },
  badgeText: { color: colors.purple100, fontSize: 13 },
  chevron: { color: colors.muted, fontSize: 18 },
  addButton: {
    marginTop: 10,
    minHeight: 52,
    borderRadius: radii.button,
    borderWidth: 1,
    borderStyle: 'dashed',
    borderColor: 'rgba(191,158,255,0.5)',
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  addButtonText: { color: colors.purple300, fontSize: 15 },
});
