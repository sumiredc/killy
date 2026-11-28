import { useEffect, useState } from 'react';
import { StyleSheet, View } from 'react-native';
import Animated, {
  Easing,
  ReduceMotion,
  type SharedValue,
  useAnimatedProps,
  useAnimatedStyle,
  useDerivedValue,
  useSharedValue,
  withTiming,
} from 'react-native-reanimated';
import Svg, { Circle, Defs, G, Line, LinearGradient, RadialGradient, Rect, Stop } from 'react-native-svg';

import { HoodIcon } from '@/art/HoodIcon';
import { Killy } from '@/art/Killy';
import { Necromancer } from '@/art/Necromancer';
import { Soul } from '@/art/Soul';
import { Zombie } from '@/art/Zombie';
import type { Bubble } from '@/killy/battle-context';
import {
  type Counter,
  GROUND_HEIGHT,
  KILLY_WIDTH,
  MOON_SIZE,
  moonAt,
  NECROMANCER_WIDTH,
  necromancerAt,
  onGround,
  type Size,
  type Line as ArenaLine,
  slots,
  ZOMBIE_HEIGHT,
  ZOMBIE_WIDTH,
  zombieBox,
} from '@/killy/arena';
import { killyBob, necroFloat, necroWork, puppet, stringsAt } from '@/killy/idle';
import {
  BUBBLE_POP_MS,
  bubblePop,
  DEFEAT_FROM,
  defeatRows,
  ENTER_FROM,
  flashOpacity,
  killyDash,
  killyFidget,
  killyHop,
  necromancer as necromancerPose,
  NEXT_IN_MS,
  outcomeOf,
  REST,
  sigkill,
  stringsOpacity,
  zombie as zombiePose,
} from '@/killy/strike';
import type { StrikeMotion } from '@/killy/strike-motion';
import { KText } from '@/theme/KText';
import { colors } from '@/theme/tokens';

// CSS `ease-out`, the mock's `enter`: Reanimated's Easing.out is a different curve.
const cssEaseOut = Easing.bezier(0, 0, 0.58, 1);

// The mock's `.stars`: position as a fraction of the arena, radius in px, opacity.
const stars = [
  { x: 0.18, y: 0.42, r: 0.6, o: 0.53 },
  { x: 0.42, y: 0.3, r: 0.5, o: 0.4 },
  { x: 0.58, y: 0.48, r: 0.7, o: 0.47 },
  { x: 0.3, y: 0.58, r: 0.5, o: 0.33 },
  { x: 0.88, y: 0.26, r: 0.6, o: 0.4 },
  { x: 0.12, y: 0.7, r: 0.5, o: 0.27 },
];

// erfc(d / 30√2) / 2 for the halo stops, d in px past the shadow's spread edge.
const shadowFalloff = [
  [-10, 0.631],
  [0, 0.5],
  [15, 0.309],
  [30, 0.159],
  [45, 0.067],
  [60, 0.023],
] as const;

/** The sky behind everything: purple glow, stars and the moon (the mock's `.arena` background, `.stars`, `.moon`). */
export function Backdrop({ size }: { size: Size }) {
  const moon = moonAt(size);
  const r = MOON_SIZE / 2;
  const mc = { x: moon.x + r, y: moon.y + r };
  // The mock's box-shadow (blur 60, spread 10, 25% purple-300): a Gaussian with sigma 30 around
  // the spread edge, 10px outside the rim. Its strength at d px past that edge is erfc(d / 30√2) / 2;
  // the stops below sample it, times the moon's 85% opacity.
  const halo = r + 10 + 60;
  return (
    <Svg style={StyleSheet.absoluteFill} width={size.width} height={size.height} pointerEvents="none">
      <Defs>
        <RadialGradient
          id="arenaGlow"
          gradientUnits="userSpaceOnUse"
          cx={size.width * 0.7}
          cy={size.height * 0.7}
          rx={size.width * 0.9}
          ry={size.height * 0.45}
          fx={size.width * 0.7}
          fy={size.height * 0.7}
        >
          <Stop offset="0" stopColor={colors.purple} stopOpacity="0.22" />
          <Stop offset="0.7" stopColor={colors.purple} stopOpacity="0" />
        </RadialGradient>
        <RadialGradient id="moonHalo" gradientUnits="userSpaceOnUse" cx={mc.x} cy={mc.y} r={halo} fx={mc.x} fy={mc.y}>
          {[
            // Nothing under the moon: a CSS box-shadow is not drawn beneath its box.
            <Stop key="center" offset="0" stopColor={colors.purple300} stopOpacity="0" />,
            <Stop key="rim" offset={(r - 0.5) / halo} stopColor={colors.purple300} stopOpacity="0" />,
            ...shadowFalloff.map(([d, k]) => (
              <Stop key={d} offset={(r + 10 + d) / halo} stopColor={colors.purple300} stopOpacity={0.25 * 0.85 * k} />
            )),
          ]}
        </RadialGradient>
        {/* CSS `radial-gradient(circle at 38% 36%, …)` reaches the farthest corner of the 150px box. */}
        <RadialGradient
          id="moonFace"
          gradientUnits="userSpaceOnUse"
          cx={moon.x + MOON_SIZE * 0.38}
          cy={moon.y + MOON_SIZE * 0.36}
          r={Math.hypot(MOON_SIZE * 0.62, MOON_SIZE * 0.64)}
          fx={moon.x + MOON_SIZE * 0.38}
          fy={moon.y + MOON_SIZE * 0.36}
        >
          <Stop offset="0" stopColor="#f3edff" />
          <Stop offset="0.55" stopColor="#cdb8ff" />
          <Stop offset="1" stopColor="#9f7ae6" />
        </RadialGradient>
      </Defs>
      <Rect x="0" y="0" width={size.width} height={size.height} fill="url(#arenaGlow)" />
      {stars.map((s) => (
        <Circle key={`${s.x},${s.y}`} cx={size.width * s.x} cy={size.height * s.y} r={s.r} fill="#fff" opacity={s.o} />
      ))}
      <Circle cx={mc.x} cy={mc.y} r={halo} fill="url(#moonHalo)" />
      {/* The moon and its craters fade together, as the mock's `.moon` opacity covers its ::after. */}
      <G opacity="0.85">
        <Circle testID="moon" cx={mc.x} cy={mc.y} r={r} fill="url(#moonFace)" />
        <Circle cx={moon.x + MOON_SIZE * 0.7} cy={moon.y + MOON_SIZE * 0.64} r="9" fill="rgb(120,90,190)" opacity="0.35" />
        <Circle cx={moon.x + MOON_SIZE * 0.34} cy={moon.y + MOON_SIZE * 0.7} r="6" fill="rgb(120,90,190)" opacity="0.25" />
      </G>
    </Svg>
  );
}

/** The darker band the characters stand on. */
export function Ground({ size }: { size: Size }) {
  return (
    <Svg style={styles.ground} width={size.width} height={GROUND_HEIGHT} pointerEvents="none">
      <Rect x="0" y="0" width={size.width} height={GROUND_HEIGHT} fill="url(#groundFill)" />
      <Defs>
        <LinearGradient id="groundFill" x1="0" y1="0" x2="0" y2="1">
          <Stop offset="0" stopColor="#211c29" />
          <Stop offset="1" stopColor="#15121a" />
        </LinearGradient>
      </Defs>
      <Line x1="0" y1="0.5" x2={size.width} y2="0.5" stroke={colors.line} strokeWidth="1" />
    </Svg>
  );
}

/** One hood per parent, or one hood and `× remaining` from five parents on. */
export function ParentCounter({ value, label }: { value: Counter; label: string }) {
  return (
    <View style={styles.counter} accessibilityRole="image" accessibilityLabel={label} testID="parent-counter">
      {value.kind === 'compact' ? (
        <>
          <View style={styles.current} testID="hood-current">
            <HoodIcon current />
          </View>
          <KText kind="mono" style={styles.times} testID="hood-times">
            {`× ${value.remaining}`}
          </KText>
        </>
      ) : (
        value.hoods.map((h, i) => (
          <View
            // The hoods are a row of identical icons; their order is all that tells them apart.
            key={i}
            testID={`hood-${h}`}
            style={h === 'current' ? styles.current : h === 'done' ? styles.done : undefined}
          >
            <HoodIcon current={h === 'current'} />
          </View>
        ))
      )}
    </View>
  );
}

const AnimatedLine = Animated.createAnimatedComponent(Line);

/**
 * The kill sequence as the enemies see it: its clock and outcome while a kill is on, at rest
 * otherwise. Given as a plain prop, `on` is right from the first frame of a parent that comes in
 * after a defeat, when the shared values may still hold the defeat's end.
 */
function seen(motion: StrikeMotion, on: boolean) {
  'worklet';
  return on
    ? { t: motion.elapsed.value, outcome: outcomeOf(motion.outcome.value), at: motion.at.value, n: motion.n.value }
    : { t: REST, outcome: null, at: DEFEAT_FROM, n: 0 };
}

/** One marionette string, redrawn each frame from the idle poses (the mock's drawThreads). */
function StringLine({ lines, index }: { lines: SharedValue<ArenaLine[]>; index: number }) {
  // When zombies are added, the new strings render before the derived lines are recomputed for
  // them; until then such a string is a point, which draws nothing.
  const props = useAnimatedProps(() => lines.value[index] ?? { x1: 0, y1: 0, x2: 0, y2: 0 });
  return <AnimatedLine animatedProps={props} stroke={colors.purple300} strokeWidth="1" opacity="0.6" testID="string" />;
}

/** The soul's width, and how far its bottom is above the zombie's feet (the mock's `.zombie .soul`). */
const SOUL_WIDTH = 26;
const SOUL_HEIGHT = (SOUL_WIDTH * 46) / 40;

/** A zombie in slot i, doing its puppet jerk around its feet until the kill sequence knocks it out and releases its soul. */
function ZombieOnGround({
  pid,
  index,
  size,
  clock,
  motion,
  on,
}: {
  pid: number;
  index: number;
  size: Size;
  clock: SharedValue<number>;
  motion: StrikeMotion;
  on: boolean;
}) {
  const box = zombieBox(index, size);
  const pose = useDerivedValue(() => {
    const k = seen(motion, on);
    return zombiePose(k.t, index, k.n, k.outcome, k.at);
  });
  const jerk = useAnimatedStyle(() => {
    // A knocked-out zombie stops jerking (the mock's `.zombie.dead { animation: none }`).
    const p = pose.value.alive ? puppet(clock.value, index) : { y: 0, rotate: 0 };
    return { transform: [{ scale: box.scale }, { translateY: p.y }, { rotate: `${p.rotate}deg` }] };
  });
  const body = useAnimatedStyle(() => {
    const b = pose.value.body;
    return { opacity: b.opacity, transform: [{ translateX: b.x }, { rotate: `${b.rotate}deg` }, { scaleY: b.scaleY }] };
  });
  const soul = useAnimatedStyle(() => {
    const s = pose.value.soul;
    return { opacity: s.opacity, transform: [{ translateX: s.x }, { translateY: s.y }, { scale: s.scale }] };
  });
  const pidStyle = useAnimatedStyle(() => ({ opacity: pose.value.pidOpacity }));
  return (
    <Animated.View testID="zombie" style={[styles.zombie, { left: box.left, top: box.top, zIndex: index < 3 ? 9 : 5 }, jerk]}>
      <Animated.View style={[styles.body, body]} testID="zombie-body">
        <Zombie width={ZOMBIE_WIDTH} pose={pose} />
      </Animated.View>
      <Animated.View style={[styles.soul, soul]} testID="zombie-soul">
        <Soul width={SOUL_WIDTH} />
      </Animated.View>
      <Animated.View style={[styles.pidBox, pidStyle]}>
        <KText kind="mono" style={styles.pid} testID="zombie-pid">
          {pid}
        </KText>
      </Animated.View>
    </Animated.View>
  );
}

/**
 * The necromancer, its zombies on the ground and the strings between them, all moving with the
 * idle clock and, while a kill is on (`striking`), with the kill sequence. A parent drawn with
 * `entering` comes in from above, as the next one does after a defeat.
 */
export function Enemies({
  size,
  zombies,
  clock,
  motion,
  striking = false,
  entering = false,
}: {
  size: Size;
  zombies: number[];
  clock: SharedValue<number>;
  motion: StrikeMotion;
  striking?: boolean;
  entering?: boolean;
}) {
  const ground = onGround(zombies);
  const necro = necromancerAt(size);
  const last = zombieBox(slots.length - 1, size);
  const count = ground.shown.length;
  // How far the parent has come in, 0–1: from its first frame, so it never shows before it starts coming.
  const enter = useSharedValue(entering ? 0 : 1);
  useEffect(() => {
    if (entering) {
      enter.set(withTiming(1, { duration: NEXT_IN_MS, easing: cssEaseOut, reduceMotion: ReduceMotion.Never }));
    }
  }, [entering, enter]);
  const lines = useDerivedValue(() => stringsAt(clock.value, count, size, ENTER_FROM * (1 - enter.value)));
  const pose = useDerivedValue(() => {
    const k = seen(motion, striking);
    return necromancerPose(k.t, k.outcome, k.at, k.n);
  });
  const float = useAnimatedStyle(() => {
    const p = pose.value;
    const e = enter.value;
    return {
      opacity: p.opacity * e,
      transform: [{ translateY: necroFloat(clock.value) + ENTER_FROM * (1 - e) + p.y }, { scale: p.scale }],
    };
  });
  const work = useAnimatedStyle(() => ({ transform: [{ translateX: pose.value.x }, { rotate: `${necroWork(clock.value)}deg` }] }));
  // The strings are cut and, after a recoil, come back together: each is drawn at 60%, so the
  // whole set goes from 1 down to 0 and back.
  const cut = useAnimatedStyle(() => ({
    opacity: (() => {
      const k = seen(motion, striking);
      return stringsOpacity(k.t, k.outcome, k.at) / 0.6;
    })(),
  }));
  // The +N chip goes with the last zombie drawn.
  const more = useAnimatedStyle(() => ({
    opacity: (() => {
      const k = seen(motion, striking);
      return zombiePose(k.t, count - 1, k.n, k.outcome, k.at).body.opacity;
    })(),
  }));
  return (
    // Above the info block, as the mock's `.party` (z-index 2) is above `.info` (1).
    <View style={styles.enemies} pointerEvents="none" testID="enemies">
      <Animated.View style={[StyleSheet.absoluteFill, cut]} testID="strings-cut">
        <Svg style={StyleSheet.absoluteFill} width={size.width} height={size.height} testID="strings">
          {Array.from({ length: 2 * count }, (_, i) => (
            <StringLine key={i} lines={lines} index={i} />
          ))}
        </Svg>
      </Animated.View>
      <Animated.View style={[styles.necromancer, { left: necro.x, top: necro.y }, float]} testID="necromancer">
        <Animated.View style={[styles.work, work]} testID="necromancer-work">
          <Necromancer width={NECROMANCER_WIDTH} clock={clock} />
        </Animated.View>
      </Animated.View>
      {ground.shown.map((pid, i) => (
        <ZombieOnGround key={pid} pid={pid} index={i} size={size} clock={clock} motion={motion} on={striking} />
      ))}
      {ground.more > 0 && (
        <Animated.View style={[styles.more, { left: last.left, bottom: size.height - last.top + 4 }, more]} testID="zombies-more">
          <KText kind="mono" style={styles.moreText}>{`+${ground.more}`}</KText>
        </Animated.View>
      )}
    </View>
  );
}

/** The arena's flash at the hit, and "SIGKILL" floating up above the necromancer (the mock's `.flash` and `.float`). */
export function StrikeEffects({ size, motion }: { size: Size; motion: StrikeMotion }) {
  const necro = necromancerAt(size);
  const flash = useAnimatedStyle(() => ({ opacity: flashOpacity(motion.elapsed.value) }));
  const text = useAnimatedStyle(() => {
    const s = sigkill(motion.elapsed.value);
    return { opacity: s.opacity, transform: [{ translateY: s.y }] };
  });
  return (
    <>
      <Animated.View style={[styles.flash, flash]} pointerEvents="none" testID="strike-flash" />
      <Animated.View
        style={[styles.sigkill, { left: necro.x - 10, top: necro.y - 10 }, text]}
        pointerEvents="none"
        testID="strike-sigkill">
        <KText kind="monoSemiBold" style={styles.sigkillText}>
          SIGKILL
        </KText>
      </Animated.View>
    </>
  );
}

/**
 * Killy's bubble: just above the head, reaching left, its tail dropping from the lower right onto the head (the mock's `.bubble`).
 * It pops in and out over 180 ms (scale from 0.4, opacity over the first 120 ms, around its lower right); with reduce motion it
 * comes and goes at once. While it pops out it still shows the symbol it had.
 */
function SpeechBubble({ bubble, instant }: { bubble: Bubble | null; instant: boolean }) {
  // The last bubble stays while it pops out. Set while rendering, as React allows, rather than in an effect.
  const [last, setLast] = useState<Bubble | null>(bubble);
  if (bubble !== null && bubble !== last) {
    setLast(bubble);
  }
  const pop = useSharedValue(instant && bubble !== null ? 1 : 0);
  const visible = bubble !== null;
  useEffect(() => {
    if (instant) {
      pop.set(visible ? 1 : 0);
      return;
    }
    pop.set(withTiming(visible ? 1 : 0, { duration: BUBBLE_POP_MS, easing: Easing.bezier(0.2, 1.6, 0.4, 1), reduceMotion: ReduceMotion.Never }));
    if (visible) {
      return;
    }
    const id = setTimeout(() => setLast(null), BUBBLE_POP_MS);
    return () => clearTimeout(id);
  }, [visible, instant, pop]);
  const shown = instant ? bubble : last;
  const style = useAnimatedStyle(() => {
    const { scale, opacity } = bubblePop(pop.value);
    return { opacity, transform: [{ scale: Math.max(0, scale) }] };
  });
  if (shown === null) {
    return null;
  }
  return (
    <Animated.View
      style={[styles.bubble, style]}
      testID="killy-bubble"
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants">
      <KText style={styles.bubbleText}>{shown.symbol}</KText>
      <View style={styles.bubbleTail} />
    </Animated.View>
  );
}

/**
 * Killy, bottom-left, facing right, bobbing and blinking with the idle clock. During a kill it
 * dashes and swings, then hops twice when the parent is put away; it bows for All clear.
 */
export function KillyOnStage({
  clock,
  charge,
  lean,
  motion,
  bubble = null,
  fidget,
  instant = false,
}: {
  clock: SharedValue<number>;
  charge?: SharedValue<number>;
  lean?: SharedValue<number>;
  motion?: StrikeMotion;
  bubble?: Bubble | null;
  /** Killy's fidget clock (useFidget): ms since it began, REST when Killy stands still. */
  fidget?: SharedValue<number>;
  /** Reduce motion: the bubble comes and goes at once. */
  instant?: boolean;
}) {
  const bob = useAnimatedStyle(() => {
    // The hops take the place of the bob while they last (the mock's `.killy.hop` animation).
    const hop =
      motion && outcomeOf(motion.outcome.value) === 'defeat'
        ? killyHop(motion.elapsed.value, defeatRows(motion.at.value, motion.n.value).result)
        : null;
    return { transform: [{ translateY: hop ?? killyBob(clock.value) }] };
  });
  // The dash moves the drawing, not the bubble (the mock's `.killy.dash .art`).
  const dash = useAnimatedStyle(() => ({ transform: [{ translateX: (motion ? killyDash(motion.elapsed.value) : 0) + (fidget ? killyFidget(fidget.value) : 0) }] }));
  return (
    <Animated.View style={[styles.killy, bob]} pointerEvents="none" testID="killy">
      <Animated.View style={dash} testID="killy-dash">
        <Killy width={KILLY_WIDTH} clock={clock} charge={charge} lean={lean} strike={motion?.elapsed} bow={motion?.bow} />
      </Animated.View>
      <SpeechBubble bubble={bubble} instant={instant} />
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  ground: { position: 'absolute', left: 0, right: 0, bottom: 0 },
  counter: {
    position: 'absolute',
    top: 20,
    right: 18,
    flexDirection: 'row',
    alignItems: 'flex-end',
    gap: 5,
    zIndex: 1,
  },
  current: { transform: [{ translateY: -2 }] },
  done: { opacity: 0.22 },
  times: { alignSelf: 'center', color: colors.text, fontSize: 14 },
  enemies: { ...StyleSheet.absoluteFill, zIndex: 2 },
  necromancer: { position: 'absolute', width: NECROMANCER_WIDTH, zIndex: 10 },
  // The mock turns the figure around 50% across and 20% down.
  work: { transformOrigin: '50% 20%' },
  // Scaled, lifted and tilted around its feet: the mock's transform-origin 50% 100%.
  zombie: { position: 'absolute', width: ZOMBIE_WIDTH, alignItems: 'center', transformOrigin: '50% 100%' },
  // The drawing, which sags back around 72% across and its feet (the mock's `.zombie .body`).
  body: { transformOrigin: '72% 100%' },
  // Centred over the feet, 6px up (the mock's `.zombie .soul`).
  soul: { position: 'absolute', left: ZOMBIE_WIDTH / 2 - SOUL_WIDTH / 2, top: ZOMBIE_HEIGHT - 6 - SOUL_HEIGHT, width: SOUL_WIDTH },
  // Under the feet, 10px wider than the zombie on each side; the mock puts its bottom edge 16px below them.
  pidBox: { position: 'absolute', top: ZOMBIE_HEIGHT + 2, left: -10, right: -10 },
  pid: { textAlign: 'center', fontSize: 10, color: colors.muted },
  more: {
    position: 'absolute',
    paddingVertical: 2,
    paddingHorizontal: 8,
    borderRadius: 999,
    borderWidth: 1,
    borderColor: 'rgba(191,158,255,0.35)',
    backgroundColor: 'rgba(144,70,255,0.14)',
    zIndex: 9,
  },
  moreText: { fontSize: 11, color: colors.purple100 },
  killy: {
    position: 'absolute',
    left: 0,
    bottom: 36,
    width: KILLY_WIDTH,
    zIndex: 11,
  },
  // Over everything in the arena, Killy too (the mock's z-index 4 and 5 above `.killy`'s 3).
  flash: { ...StyleSheet.absoluteFill, backgroundColor: colors.purple100, zIndex: 12 },
  sigkill: { position: 'absolute', zIndex: 13 },
  sigkillText: { fontSize: 20, color: colors.red },
  bubble: {
    position: 'absolute',
    top: -24,
    right: 78,
    paddingVertical: 4,
    paddingHorizontal: 9,
    borderRadius: 12,
    backgroundColor: colors.text,
    // The mock's transform-origin 85% 130%: the bubble grows from near its tail.
    transformOrigin: '85% 130%',
  },
  bubbleText: { color: '#000', fontSize: 13 },
  // A right-angled triangle under the bubble's right end, as the mock's border trick draws it.
  bubbleTail: {
    position: 'absolute',
    right: 12,
    bottom: -8,
    width: 0,
    height: 0,
    borderLeftWidth: 9,
    borderLeftColor: 'transparent',
    borderTopWidth: 9,
    borderTopColor: colors.text,
  },
});
