import { useFocusEffect } from 'expo-router';
import { useCallback, useState } from 'react';
import { StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Backdrop, Enemies, Ground, KillyOnStage, ParentCounter, StrikeEffects } from '@/components/Arena';
import { KillButton } from '@/components/KillButton';
import type { Key } from '@/i18n';
import { useI18n } from '@/i18n/context';
import { counter, type Size } from '@/killy/arena';
import { parentKey, type Shown } from '@/killy/battle';
import { useBattle } from '@/killy/battle-context';
import type { KillErrorKind } from '@/killy/client';
import { useKilly } from '@/killy/context';
import { useFidget } from '@/killy/fidget';
import { useIdleClock, useReduceMotion } from '@/killy/motion';
import { useWebDemo } from '@/killy/web-demo';
import { useHoldToKill } from '@/killy/hold';
import { useStrikeMotion } from '@/killy/strike-motion';
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

function Result({ shown }: { shown: { lines: string[]; ok: boolean } }) {
  return (
    <View style={[styles.result, !shown.ok && styles.resultBox]} testID="kill-result">
      {shown.lines.map((line) => (
        <KText key={line} kind="mono" style={shown.ok ? styles.resultOk : styles.resultBad}>
          {line}
        </KText>
      ))}
    </View>
  );
}

// The battle screen: the arena (DESIGN: Battle tab), the kill button.
// The kill sequence is #101; Killy's other reactions come with #83. The All clear overlay is AllClearOverlay, at the root.
export default function BattleScreen() {
  const { t } = useI18n();
  const { registration, state } = useKilly();
  const battle = useBattle();
  const webDemo = useWebDemo();
  const current = battle.current;
  const servers = state?.servers ?? [];

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
  const [size, setSize] = useState<Size | null>(null);
  const reduceMotion = useReduceMotion();
  const clock = useIdleClock(reduceMotion === false);
  const instant = reduceMotion !== false;
  const fidget = useFidget(battle.fidget, instant);
  // The Battle tab coming to the front (launch, a switch of tabs, a tap on a notification): Killy reacts.
  const { greet } = battle;
  useFocusEffect(
    useCallback(() => {
      greet();
      // Only when the tab comes to the front, not whenever the battle changes.
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []),
  );
  const motion = useStrikeMotion(battle.strike, battle.bowing, instant);
  const canKill = current !== undefined && !battle.busy && battle.allClear === null;
  // The sequence's clock starts as the hold ends, so the swing takes over from the wind-up at once.
  const fire = (key: string) => {
    const n = battle.kill(key, instant);
    if (n !== null && !instant) {
      motion.start(n);
    }
  };
  // A hold only runs while the button can be pressed: when it cannot, or another parent comes to the front, the hold ends.
  const press = useHoldToKill(canKill ? parentKey(current.server.id, current.parent) : undefined, fire, reduceMotion === true);

  return (
    <SafeAreaView style={styles.screen} edges={['top']} testID="battle-screen">
      {registration === 'rate_limited' && (
        <View style={styles.notice}>
          <KText style={styles.noticeText}>{t('deviceRateLimited')}</KText>
        </View>
      )}
      {webDemo.notice !== null && (
        <View style={styles.notice} testID="web-demo-notice">
          <KText style={styles.noticeText}>{t(webDemo.notice)}</KText>
        </View>
      )}
      <View style={styles.arena}>
        {/* Measured inside the border, where the absolutely placed scene is laid out. */}
        <View
          style={StyleSheet.absoluteFill}
          testID="arena"
          onLayout={(e) => setSize({ width: e.nativeEvent.layout.width, height: e.nativeEvent.layout.height })}>
        {size && <Backdrop size={size} />}
        {size && <Ground size={size} />}
        {current ? (
          // A reason in red reaches down over the necromancer: while one is shown, the info block
          // comes in front of the enemies so it can be read (not in the mock, where it is behind).
          <View style={[styles.info, shown && !shown.ok && styles.infoFront]} testID="battle-info">
            <View style={styles.chip}>
              <KText kind="mono" style={styles.chipText} testID="battle-server">
                {current.server.name}
              </KText>
            </View>
            <View style={styles.countRow}>
              <KText kind="monoSemiBold" style={styles.count} testID="battle-zombies">
                {/* Once the parent is put away its zombies are released, as the mock's count goes to 0. */}
                {battle.strike?.outcome === 'defeat' && battle.strike.revealed ? 0 : current.parent.zombies.length}
              </KText>
              <KText kind="mono" style={styles.countLabel}>
                {t('zLabel')}
              </KText>
            </View>
            {shown && shown.ok && battle.strike !== null ? (
              // The parent being put away: its result takes the place of its PID and command line, the
              // first line in green, as the mock's `.who.done`.
              <View testID="kill-result">
                {shown.lines.map((line, i) => (
                  <KText key={line} kind="mono" style={i === 0 ? [styles.pid, styles.resultOk] : styles.cmd}>
                    {line}
                  </KText>
                ))}
              </View>
            ) : (
              <>
                <KText kind="mono" style={styles.pid}>
                  {t('whoParent', { pid: current.parent.pid })}
                </KText>
                <KText kind="mono" style={styles.cmd} numberOfLines={2}>
                  {current.parent.cmd}
                </KText>
                {shown && <Result shown={shown} />}
              </>
            )}
          </View>
        ) : (
          <View style={styles.empty}>
            {/* The demo-only Web version has no Add server to point to: its demo server is on its way, or the notice says why not. */}
            {!(webDemo.enabled && servers.length === 0) && (
              <>
                <KText kind="monoSemiBold" style={styles.title}>
                  {t(servers.length === 0 ? 'emptyNoServerTitle' : 'emptyNoneTitle')}
                </KText>
                <KText style={styles.text}>{t(servers.length === 0 ? 'emptyNoServerText' : 'emptyNoneText')}</KText>
              </>
            )}
            {shown && <Result shown={shown} />}
          </View>
        )}
        {current && (
          <ParentCounter
            value={counter(battle.counter.cleared, battle.counter.queued, battle.counter.fighting)}
            label={t('counter', {
              // Once the last parent is put away, the count stays on it.
              i: Math.min(battle.counter.cleared + 1, battle.counter.cleared + battle.counter.queued),
              n: battle.counter.cleared + battle.counter.queued,
            })}
          />
        )}
        {current && size && (
          <Enemies
            // A new parent is drawn afresh, its zombies in their places.
            key={parentKey(current.server.id, current.parent)}
            size={size}
            zombies={current.parent.zombies}
            clock={clock}
            motion={motion}
            striking={battle.strike !== null}
            entering={battle.afterDefeat && !instant}
          />
        )}
        <KillyOnStage clock={clock} charge={press.windUp} lean={press.lean} motion={motion} bubble={battle.bubble} fidget={fidget} instant={instant} />
        {size && <StrikeEffects size={size} motion={motion} />}
        </View>
      </View>

      <KillButton
        label={t('hold')}
        sub={current && t('killSub', { pid: current.parent.pid, host: current.server.name })}
        enabled={canKill}
        charge={press.charge}
        onPressIn={press.onPressIn}
        onPressOut={press.onPressOut}
      />
      {battle.error !== null && (
        <KText style={styles.error} testID="kill-error">
          {t(errors[battle.error])}
        </KText>
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
  // The mock's `.arena`; everything in it is placed absolutely, as there.
  arena: { flex: 1, backgroundColor: colors.elev1, borderColor: colors.line, borderWidth: 1, borderRadius: radii.card, overflow: 'hidden' },
  info: { position: 'absolute', top: 20, left: 20, right: 70, zIndex: 1 },
  // Above the enemies (2), below Killy (11).
  infoFront: { zIndex: 3 },
  chip: {
    alignSelf: 'flex-start',
    marginBottom: 10,
    paddingVertical: 4,
    paddingHorizontal: 10,
    borderRadius: radii.pill,
    borderWidth: 1,
    borderColor: 'rgba(191,158,255,0.35)',
    backgroundColor: 'rgba(144,70,255,0.14)',
  },
  chipText: { color: colors.purple100, fontSize: 12.5 },
  countRow: { flexDirection: 'row', alignItems: 'baseline', gap: 8 },
  count: { color: colors.text, fontSize: 56, lineHeight: 56, letterSpacing: -2 },
  countLabel: { color: colors.muted, fontSize: 15 },
  pid: { color: colors.purple300, fontSize: 12.5, lineHeight: 20, marginTop: 10 },
  cmd: { color: colors.muted, fontSize: 12.5, lineHeight: 20 },
  empty: { position: 'absolute', top: 24, left: 20, right: 20, gap: 8, zIndex: 1 },
  title: { color: colors.text, fontSize: 22 },
  text: { color: colors.muted, fontSize: 14, lineHeight: 22 },
  result: { gap: 4, marginTop: 8 },
  resultOk: { color: colors.green, fontSize: 12.5, lineHeight: 20 },
  resultBad: { color: colors.red, fontSize: 12.5, lineHeight: 20 },
  // The arena's own colour behind a red reason, so it reads over the moon and the robe.
  resultBox: { alignSelf: 'flex-start', paddingVertical: 4, paddingHorizontal: 8, marginLeft: -8, borderRadius: radii.input, backgroundColor: 'rgba(25,22,31,0.85)' },
  error: { color: colors.red, fontSize: 13, marginTop: 8, textAlign: 'center' },
});
