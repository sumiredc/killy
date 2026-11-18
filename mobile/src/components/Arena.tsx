import { StyleSheet, View } from 'react-native';
import Animated, { type SharedValue, useAnimatedProps, useAnimatedStyle, useDerivedValue } from 'react-native-reanimated';
import Svg, { Circle, Defs, G, Line, LinearGradient, RadialGradient, Rect, Stop } from 'react-native-svg';

import { HoodIcon } from '@/art/HoodIcon';
import { Killy } from '@/art/Killy';
import { Necromancer } from '@/art/Necromancer';
import { Zombie } from '@/art/Zombie';
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
import { KText } from '@/theme/KText';
import { colors } from '@/theme/tokens';

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

/** One marionette string, redrawn each frame from the idle poses (the mock's drawThreads). */
function StringLine({ lines, index }: { lines: SharedValue<ArenaLine[]>; index: number }) {
  // When zombies are added, the new strings render before the derived lines are recomputed for
  // them; until then such a string is a point, which draws nothing.
  const props = useAnimatedProps(() => lines.value[index] ?? { x1: 0, y1: 0, x2: 0, y2: 0 });
  return <AnimatedLine animatedProps={props} stroke={colors.purple300} strokeWidth="1" opacity="0.6" testID="string" />;
}

/** A zombie in slot i, doing its puppet jerk around its feet. */
function ZombieOnGround({ pid, index, size, clock }: { pid: number; index: number; size: Size; clock: SharedValue<number> }) {
  const box = zombieBox(index, size);
  const jerk = useAnimatedStyle(() => {
    const p = puppet(clock.value, index);
    return { transform: [{ scale: box.scale }, { translateY: p.y }, { rotate: `${p.rotate}deg` }] };
  });
  return (
    <Animated.View testID="zombie" style={[styles.zombie, { left: box.left, top: box.top, zIndex: index < 3 ? 9 : 5 }, jerk]}>
      <Zombie width={ZOMBIE_WIDTH} />
      <KText kind="mono" style={styles.pid} testID="zombie-pid">
        {pid}
      </KText>
    </Animated.View>
  );
}

/** The necromancer, its zombies on the ground and the strings between them, all moving with the idle clock. */
export function Enemies({ size, zombies, clock }: { size: Size; zombies: number[]; clock: SharedValue<number> }) {
  const ground = onGround(zombies);
  const necro = necromancerAt(size);
  const last = zombieBox(slots.length - 1, size);
  const count = ground.shown.length;
  const lines = useDerivedValue(() => stringsAt(clock.value, count, size));
  const float = useAnimatedStyle(() => ({ transform: [{ translateY: necroFloat(clock.value) }] }));
  const work = useAnimatedStyle(() => ({ transform: [{ rotate: `${necroWork(clock.value)}deg` }] }));
  return (
    // Above the info block, as the mock's `.party` (z-index 2) is above `.info` (1).
    <View style={styles.enemies} pointerEvents="none" testID="enemies">
      <Svg style={StyleSheet.absoluteFill} width={size.width} height={size.height} testID="strings">
        {Array.from({ length: 2 * count }, (_, i) => (
          <StringLine key={i} lines={lines} index={i} />
        ))}
      </Svg>
      <Animated.View style={[styles.necromancer, { left: necro.x, top: necro.y }, float]} testID="necromancer">
        <Animated.View style={[styles.work, work]} testID="necromancer-work">
          <Necromancer width={NECROMANCER_WIDTH} clock={clock} />
        </Animated.View>
      </Animated.View>
      {ground.shown.map((pid, i) => (
        <ZombieOnGround key={pid} pid={pid} index={i} size={size} clock={clock} />
      ))}
      {ground.more > 0 && (
        <View style={[styles.more, { left: last.left, bottom: size.height - last.top + 4 }]} testID="zombies-more">
          <KText kind="mono" style={styles.moreText}>{`+${ground.more}`}</KText>
        </View>
      )}
    </View>
  );
}

/** Killy, bottom-left, facing right, bobbing and blinking with the idle clock. */
export function KillyOnStage({ clock, charge, lean }: { clock: SharedValue<number>; charge?: SharedValue<number>; lean?: SharedValue<number> }) {
  const bob = useAnimatedStyle(() => ({ transform: [{ translateY: killyBob(clock.value) }] }));
  return (
    <Animated.View style={[styles.killy, bob]} pointerEvents="none" testID="killy">
      <Killy width={KILLY_WIDTH} clock={clock} charge={charge} lean={lean} />
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
  // Under the feet, 10px wider than the zombie on each side; the mock puts its bottom edge 16px below them.
  pid: {
    position: 'absolute',
    top: ZOMBIE_HEIGHT + 2,
    left: -10,
    right: -10,
    textAlign: 'center',
    fontSize: 10,
    color: colors.muted,
  },
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
});
