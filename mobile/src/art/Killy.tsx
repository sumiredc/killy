import { Platform, View } from 'react-native';
import Animated, { type SharedValue, useAnimatedProps } from 'react-native-reanimated';
import Svg, { Circle, Defs, Ellipse, G, type GProps, LinearGradient, Path, Rect, Stop } from 'react-native-svg';

import { killyBlink } from '@/killy/idle';
import { ghostMatrix, groupTransform, scytheMatrix } from '@/killy/pose';
import { scytheSwing } from '@/killy/strike';

/** Where Killy is drawn, in the art's units: the 200 × 150 box and room for the scythe's whole swing. */
export const DRAW = { x: 0, y: -32, width: 230, height: 204 } as const;

const AnimatedEllipse = Animated.createAnimatedComponent(Ellipse);
const AnimatedG = Animated.createAnimatedComponent(G);

/**
 * Killy, ported from killySVG in mock/art.js without redrawing. Faces right, has no mouth.
 * `ghost`, `eye`, `scythe` and `arm` stay separate elements so later motion can move them. Given
 * the idle clock, the eyes blink; each is squashed towards its own centre, as the mock's scaleY
 * with transform-box fill-box.
 */
export function Killy({
  width,
  clock,
  charge,
  lean,
  strike,
  bow,
  scythe: withScythe = true,
}: {
  width: number;
  clock?: SharedValue<number>;
  /** How full the kill button is, 0–1: the scythe winds up with it. */
  charge?: SharedValue<number>;
  /** 1 while the kill button is held: the ghost leans back. */
  lean?: SharedValue<number>;
  /** The kill sequence's clock (ms since the hold completed): the scythe swings at its start. */
  strike?: SharedValue<number>;
  /** How far Killy has bowed, 0–1. */
  bow?: SharedValue<number>;
  /** Whether Killy carries the scythe; the All clear card shows Killy without it (the mock's `.clear-killy .scythe`). */
  scythe?: boolean;
}) {
  // `matrix` is the transform react-native-svg's native group takes; its public props do not list
  // it, because transforms usually go through `transform` and are turned into a matrix in JS,
  // which animated props skip. On the Web the group is an SVG <g>, which takes `transform` and
  // not `matrix` (groupTransform).
  const web = Platform.OS === 'web';
  const k = width / 200;
  const ghost = useAnimatedProps(() => groupTransform(ghostMatrix(lean ? lean.value : 0, bow ? bow.value : 0), web) as unknown as GProps);
  const scythe = useAnimatedProps(
    () => groupTransform(scytheMatrix(charge ? charge.value : 0, strike ? scytheSwing(strike.value) : null), web) as unknown as GProps,
  );
  const near = useAnimatedProps(() => ({ ry: 7.2 * (clock ? killyBlink(clock.value) : 1) }));
  const far = useAnimatedProps(() => ({ ry: 6.6 * (clock ? killyBlink(clock.value) : 1) }));
  return (
    // Laid out as the mock's 200 × 150 box, but drawn on a larger one: wound up and swinging, the
    // scythe reaches about 27 above, 24 right of and 19 below it.
    <View style={{ width, height: (width * 150) / 200 }}>
      <Svg
        style={{ position: 'absolute', left: k * DRAW.x, top: k * DRAW.y }}
        width={k * DRAW.width}
        height={k * DRAW.height}
        viewBox={`${DRAW.x} ${DRAW.y} ${DRAW.width} ${DRAW.height}`}
        testID="killy-art">
        <Defs>
          <LinearGradient id="killyBody" x1="1" y1="0" x2="0" y2="1">
            <Stop offset="0.45" stopColor="#ffffff" />
            <Stop offset="1" stopColor="#d9d0f0" />
          </LinearGradient>
          <LinearGradient id="killyBlade" x1="0" y1="0" x2="0" y2="1">
            <Stop offset="0" stopColor="#b6a9da" />
            <Stop offset="0.55" stopColor="#e3ddf5" />
            <Stop offset="1" stopColor="#f8f6ff" />
          </LinearGradient>
        </Defs>

        <AnimatedG testID="killy-ghost" animatedProps={ghost}>
          <Path
            d="M80 16 C102 16 116 34 116 56 C116 74 112 88 108 100 Q100 112 92 102 Q84 114 74 104 Q64 116 54 106 Q40 113 26 106 Q14 101 8 88 Q28 95 40 84 C46 77 46 66 46 56 C46 34 58 16 80 16 Z"
            fill="url(#killyBody)"
          />
          <AnimatedEllipse testID="killy-eye" cx="88" cy="56" rx="4.8" ry="7.2" fill="#000" animatedProps={near} />
          <AnimatedEllipse testID="killy-eye" cx="105" cy="55" rx="3.8" ry="6.6" fill="#000" animatedProps={far} />
        </AnimatedG>

        {withScythe && (
        <AnimatedG testID="killy-scythe" animatedProps={scythe}>
          <Path d="M100 146 Q119 82 127 16" fill="none" stroke="#2f2740" strokeWidth="7" strokeLinecap="round" />
          <Path d="M98.6 143 Q117 82 125 18" fill="none" stroke="#5c4f78" strokeWidth="2" strokeLinecap="round" />
          <Circle cx="100" cy="146" r="4.5" fill="#9046ff" />

          <Path d="M124 6 C148 -8 184 -2 199 46 C186 24 160 16 130 24 Z" fill="url(#killyBlade)" />
          <Path d="M131 21 C160 13 185 21 197 41" fill="none" stroke="#ffffff" strokeWidth="2.4" strokeLinecap="round" />
          <Path d="M126 5.5 C150 -6.5 183 -1.5 198 43" fill="none" stroke="#8a7db3" strokeWidth="1.6" strokeLinecap="round" />
          <Path d="M150 6 C164 5 176 10 184 20" fill="none" stroke="#ffffff" strokeWidth="1.6" strokeLinecap="round" opacity=".7" />

          <G transform="translate(127 15) rotate(8)">
            <Rect x="-13" y="-10" width="26" height="23" rx="5" fill="#5b22b8" />
            <Rect x="-13" y="-12" width="26" height="21" rx="5" fill="#9046ff" />
            <Rect x="-10" y="-10" width="20" height="15" rx="3.5" fill="#a46dff" />
            <Path
              d="M5 -6.5 V-1 H-5 M-2.4 -3.6 L-5 -1 L-2.4 1.6"
              fill="none"
              stroke="#fff"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </G>
        </AnimatedG>
        )}

        {/* Without the scythe the arm hangs a little lower and nearer the body, as the mock's `.clear-killy .arm`. */}
        <Circle testID="killy-arm" cx={withScythe ? 115 : 109} cy={withScythe ? 82 : 88} r="7.5" fill="#ffffff" />
      </Svg>
    </View>
  );
}
