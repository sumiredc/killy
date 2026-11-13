import { Pressable, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import type { Key } from '@/i18n';
import { useI18n } from '@/i18n/context';
import { parentKey, type Shown } from '@/killy/battle';
import { useBattle } from '@/killy/battle-context';
import type { KillErrorKind } from '@/killy/client';
import { useKilly } from '@/killy/context';
import { useHoldToKill } from '@/killy/hold';
import { KText } from '@/theme/KText';
import { colors, gutter, radii } from '@/theme/tokens';

const reasons = new Map<string, Key>(Object.entries({
  pid_reused: 'reasonPidReused',
  not_a_zombie_parent: 'reasonNotZombieParent',
  protected: 'reasonProtected',
  not_demo: 'reasonNotDemo',
  pidfd_unavailable: 'reasonPidfdUnavailable',
  timeout: 'reasonTimeout',
  agent_disconnected: 'reasonAgentDisconnected',
}) as [string, Key][]);

const errors: Record<KillErrorKind, Key> = {
  unreachable: 'killUnreachable',
  rate_limited: 'killRateLimited',
  other: 'killFailed',
};

// The battle screen. The arena's art and the kill sequence come with #26–#28; this is the text.
export default function BattleScreen() {
  const { t } = useI18n();
  const { registration, state } = useKilly();
  const battle = useBattle();
  const current = battle.queue[0];
  const press = useHoldToKill(current && parentKey(current.server.id, current.parent), battle.kill);
  const servers = state?.servers ?? [];
  const canKill = current !== undefined && !battle.busy && battle.allClear === null;

  // A reason the app knows is said in words; any other is shown as the agent sent it.
  const reason = (r: string | undefined) => {
    const key = r === undefined ? 'reasonUnknown' : reasons.get(r);
    return key ? t(key) : (r as string);
  };
  const outcome = (s: Shown): { lines: string[]; ok: boolean } => {
    switch (s.kind) {
      case 'killed':
        return { lines: [t('whoKilled', { pid: s.pid }), t('whoReaped', { n: s.reaped })], ok: true };
      case 'already_gone':
        return { lines: [t('whoGone', { pid: s.pid })], ok: true };
      case 'refused':
        return { lines: [t('outcomeRefused', { reason: reason(s.reason) })], ok: false };
      case 'signal_failed':
        return { lines: [t('outcomeFailed', { reason: t('reasonSignal', { errno: s.reason ?? '?' }) })], ok: false };
      case 'zombies_remain':
        return { lines: [t('outcomeFailed', { reason: t('reasonZombiesRemain') })], ok: false };
      case 'unknown':
        return { lines: [t('outcomeUnknown', { reason: reason(s.reason) })], ok: false };
    }
  };
  const shown = battle.shown && outcome(battle.shown);

  return (
    <SafeAreaView style={styles.screen} edges={['top']} testID="battle-screen">
      {registration === 'rate_limited' && (
        <View style={styles.notice}>
          <KText style={styles.noticeText}>{t('deviceRateLimited')}</KText>
        </View>
      )}
      <View style={styles.arena}>
        {current ? (
          <View>
            <View style={styles.chip}>
              <KText kind="mono" style={styles.chipText} testID="battle-server">
                {current.server.name}
              </KText>
            </View>
            <View style={styles.countRow}>
              <KText kind="monoSemiBold" style={styles.count} testID="battle-zombies">
                {current.parent.zombies.length}
              </KText>
              <KText style={styles.countLabel}>{t('zLabel')}</KText>
            </View>
            <KText kind="mono" style={styles.pid}>
              {t('whoParent', { pid: current.parent.pid })}
            </KText>
            <KText kind="mono" style={styles.cmd} numberOfLines={2}>
              {current.parent.cmd}
            </KText>
          </View>
        ) : (
          <View>
            <KText kind="monoSemiBold" style={styles.title}>
              {t(servers.length === 0 ? 'emptyNoServerTitle' : 'emptyNoneTitle')}
            </KText>
            <KText style={styles.text}>{t(servers.length === 0 ? 'emptyNoServerText' : 'emptyNoneText')}</KText>
          </View>
        )}
        {shown && (
          <View style={styles.result} testID="kill-result">
            {shown.lines.map((line) => (
              <KText key={line} kind="mono" style={shown.ok ? styles.resultOk : styles.resultBad}>
                {line}
              </KText>
            ))}
          </View>
        )}
      </View>

      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t('hold')}
        disabled={!canKill}
        onPressIn={press.onPressIn}
        onPressOut={press.onPressOut}
        style={[styles.kill, !canKill && styles.killDisabled]}>
        <KText kind="monoSemiBold" style={styles.killLabel}>
          {t('hold')}
        </KText>
        {current && (
          <KText kind="mono" style={styles.killSub}>
            {t('killSub', { pid: current.parent.pid, host: current.server.name })}
          </KText>
        )}
      </Pressable>
      {battle.error !== null && (
        <KText style={styles.error} testID="kill-error">
          {t(errors[battle.error])}
        </KText>
      )}

      {battle.allClear && (
        <View style={styles.overlay} testID="all-clear">
          <View style={styles.card}>
            <KText kind="monoSemiBold" style={styles.clearTitle}>
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
              <KText kind="monoSemiBold" style={styles.killLabel}>
                {t('close')}
              </KText>
            </Pressable>
          </View>
        </View>
      )}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  // The tab bar floats over the bottom 84px (it is translucent), so the content stops above it.
  screen: { flex: 1, backgroundColor: colors.bg, paddingHorizontal: gutter, paddingBottom: 84 + gutter },
  // Registration failed with 429: said where the app opens, until the next start tries again.
  notice: { padding: 12, borderRadius: radii.input, backgroundColor: 'rgba(247,80,106,0.12)', marginBottom: 12 },
  noticeText: { color: '#FF9AAA', fontSize: 13, lineHeight: 20 },
  arena: {
    flex: 1,
    backgroundColor: colors.elev1,
    borderColor: colors.line,
    borderWidth: 1,
    borderRadius: radii.card,
    padding: 20,
    justifyContent: 'space-between',
  },
  chip: { alignSelf: 'flex-start', paddingVertical: 4, paddingHorizontal: 12, borderRadius: radii.pill, backgroundColor: 'rgba(144,70,255,0.25)' },
  chipText: { color: colors.purple100, fontSize: 13 },
  countRow: { flexDirection: 'row', alignItems: 'baseline', gap: 8, marginTop: 10 },
  count: { color: colors.text, fontSize: 56 },
  countLabel: { color: colors.muted, fontSize: 14 },
  pid: { color: colors.purple300, fontSize: 14, marginTop: 4 },
  cmd: { color: colors.muted, fontSize: 13, marginTop: 4 },
  title: { color: colors.text, fontSize: 18, marginBottom: 8 },
  text: { color: colors.muted, fontSize: 14, lineHeight: 20 },
  result: { gap: 4 },
  resultOk: { color: colors.green, fontSize: 13 },
  resultBad: { color: colors.red, fontSize: 13, lineHeight: 19 },
  kill: { marginTop: 12, minHeight: 64, borderRadius: radii.button, backgroundColor: colors.purple, alignItems: 'center', justifyContent: 'center', paddingVertical: 8 },
  killDisabled: { backgroundColor: colors.elev3 },
  killLabel: { color: '#FFFFFF', fontSize: 16 },
  killSub: { color: colors.purple100, fontSize: 11, marginTop: 2 },
  error: { color: colors.red, fontSize: 13, marginTop: 8, textAlign: 'center' },
  overlay: { ...StyleSheet.absoluteFill, backgroundColor: 'rgba(0,0,0,0.7)', justifyContent: 'center', padding: 24 },
  card: { backgroundColor: colors.elev1, borderRadius: radii.card, borderWidth: 1, borderColor: colors.line, padding: 24, gap: 12 },
  clearTitle: { color: colors.text, fontSize: 24 },
  stats: { flexDirection: 'row', gap: 12 },
  stat: { flex: 1, backgroundColor: colors.elev2, borderRadius: radii.input, padding: 14, gap: 4 },
  statValue: { color: colors.text, fontSize: 28 },
  statLabel: { color: colors.muted, fontSize: 12 },
  close: { minHeight: 52, borderRadius: radii.button, backgroundColor: colors.purple, alignItems: 'center', justifyContent: 'center' },
});
