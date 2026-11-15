import { useEffect, useRef, useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Switch, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { AddServerSheet } from '@/components/AddServerSheet';
import { ServerSheet } from '@/components/ServerSheet';
import type { Lang } from '@/i18n';
import { useI18n } from '@/i18n/context';
import { useKilly } from '@/killy/context';
import { useNotifications } from '@/killy/notifications-context';
import { usePairLinks } from '@/killy/pair-link';
import { serverRow } from '@/killy/server-row';
import { KText } from '@/theme/KText';
import { colors, gutter, radii } from '@/theme/tokens';

export default function SettingsScreen() {
  const { t } = useI18n();
  const { state } = useKilly();
  const [adding, setAdding] = useState(false);
  const [openId, setOpenId] = useState<string | null>(null);
  // A killy://pair link, already checked by its route, opens the Add server sheet. Settings has no
  // URL parameters of its own for this, so no other link can put a code in the field.
  const { link, clear } = usePairLinks();
  const closeAdd = () => {
    setAdding(false);
    clear();
  };
  // A sheet is a Modal and stays above the tabs. A tap on a notification opens the Battle tab, so
  // it closes the sheets that would hide it. Not on every blur: the killy://pair route also covers
  // Settings, and its link must reach the sheet.
  const { taps: tapCount } = useNotifications();
  const seenTaps = useRef(tapCount);
  useEffect(() => {
    if (tapCount === seenTaps.current) return;
    seenTaps.current = tapCount;
    setAdding(false);
    setOpenId(null);
    clear();
  }, [tapCount, clear]);
  const servers = state?.servers ?? [];
  // Looked up in every state, so the sheet shows the latest values and closes once the server is gone.
  const open = servers.find((s) => s.id === openId);
  // A server that left the state closed its sheet; forget it, so the sheet does not open again by
  // itself if the server is linked again.
  if (openId !== null && state !== null && open === undefined) {
    setOpenId(null);
  }
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
        <KText style={styles.groupTitle}>{t('app')}</KText>
        <View style={styles.group}>
          <LanguageRow />
          <View style={styles.rowDivider}>
            <NotificationsRow />
          </View>
        </View>
        <NotificationsError />
      </ScrollView>
      {/* Not over the Server sheet: iOS presents one sheet at a time, so a link waits until it closes. */}
      {(adding || link !== null) && open === undefined && <AddServerSheet onClose={closeAdd} link={link} />}
      {open !== undefined && <ServerSheet server={open} onClose={() => setOpenId(null)} />}
    </SafeAreaView>
  );
}

// Each language is named in itself, so it can be found whatever the app is in now.
const languages: { lang: Lang; name: string }[] = [
  { lang: 'ja', name: '日本語' },
  { lang: 'en', name: 'English' },
];

function LanguageRow() {
  const { t, lang, setLang } = useI18n();
  return (
    <View style={styles.row}>
      <KText style={[styles.rowTitle, styles.rowMain]}>{t('language')}</KText>
      {/* Buttons with a selected state, as the mock's aria-pressed buttons; no radio roles. */}
      <View style={styles.segmented}>
        {languages.map((l) => {
          const selected = l.lang === lang;
          return (
            <Pressable
              key={l.lang}
              accessibilityRole="button"
              accessibilityLabel={l.name}
              accessibilityState={{ selected }}
              onPress={() => setLang(l.lang)}
              style={[styles.segment, selected && styles.segmentSelected]}>
              <KText style={[styles.segmentText, selected && styles.segmentTextSelected]}>{l.name}</KText>
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

function NotificationsRow() {
  const { t } = useI18n();
  const { permission, notify, sending, setNotify, openSettings } = useNotifications();
  const blocked = permission === 'denied';
  const disabled = blocked || sending;
  return (
    <View style={styles.row}>
      <View style={styles.rowMain}>
        <KText style={styles.rowTitle}>{t('notifications')}</KText>
        <KText style={styles.rowSub}>{t('notificationsHint')}</KText>
        {blocked && (
          <>
            <KText style={styles.blocked}>{t('notifBlocked')}</KText>
            <Pressable accessibilityRole="link" onPress={openSettings} hitSlop={8}>
              <KText style={styles.link}>{t('notifOpenSettings')}</KText>
            </Pressable>
          </>
        )}
      </View>
      <Switch
        accessibilityLabel={t('notifications')}
        value={notify}
        onValueChange={setNotify}
        disabled={disabled}
        accessibilityState={{ disabled }}
        trackColor={{ false: colors.elev3, true: colors.purple }}
        thumbColor="#FFFFFF"
        ios_backgroundColor={colors.elev3}
      />
    </View>
  );
}

function NotificationsError() {
  const { t } = useI18n();
  const { error } = useNotifications();
  if (error === null) return null;
  return (
    <KText style={styles.errorLine} testID="notify-app-error" accessibilityLiveRegion="polite">
      {t(error)}
    </KText>
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
  rowTitle: { color: colors.text, fontSize: 15 },
  blocked: { color: colors.text, fontSize: 12.5, marginTop: 4 },
  link: { color: colors.purple300, fontSize: 12.5, marginTop: 2 },
  // DESIGN Settings tab: segmented 日本語 / English on elev-2, the chosen one in purple (mock/style.css).
  segmented: { flexDirection: 'row', padding: 3, borderRadius: 10, backgroundColor: colors.elev2 },
  segment: { paddingVertical: 6, paddingHorizontal: 10, borderRadius: 8 },
  segmentSelected: { backgroundColor: colors.purple },
  segmentText: { color: colors.muted, fontSize: 13 },
  segmentTextSelected: { color: '#FFFFFF' },
  errorLine: { color: colors.red, fontSize: 13, lineHeight: 20, marginTop: 6, marginHorizontal: 4 },
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
