import Animated, { type SharedValue, useAnimatedProps } from 'react-native-reanimated';
import Svg, { Circle, Ellipse, Path, Rect } from 'react-native-svg';

import { necroGlow } from '@/killy/idle';

import { anchors } from './points';

const AnimatedEllipse = Animated.createAnimatedComponent(Ellipse);

/** A parent process: the necromancer from parentSVG in mock/art.js, facing left. Given the idle clock, its eyes pulse. */
export function Necromancer({ width, clock }: { width: number; clock?: SharedValue<number> }) {
  const glow = useAnimatedProps(() => ({ opacity: clock ? necroGlow(clock.value) : 1 }));
  return (
    <Svg width={width} height={width} viewBox="0 0 150 150" testID="necromancer-art">
      <Path
        d="M86 6 C64 4 46 18 39 40 C28 50 17 66 12 82 Q26 76 38 78 C38 100 48 118 60 134 Q67 126 75 142 Q83 126 90 134 C102 118 112 100 112 78 Q124 76 138 82 C133 66 121 52 111 44 C113 30 110 18 102 10 Q112 6 118 12 Q110 2 86 6 Z"
        fill="#2a2433"
        stroke="#4a4157"
        strokeWidth="2"
        strokeLinejoin="round"
      />
      <Path d="M90 18 Q100 36 94 62" fill="none" stroke="#1e1a25" strokeWidth="2.4" strokeLinecap="round" />
      <Path d="M58 70 Q66 96 64 126 M92 70 Q84 96 86 126" fill="none" stroke="#1e1a25" strokeWidth="2" />
      <Ellipse cx="61" cy="42" rx="15" ry="18" fill="#000" transform="rotate(-8 61 42)" />
      <AnimatedEllipse testID="necromancer-glow" cx="54" cy="43" rx="3.4" ry="4.6" fill="#bf9eff" animatedProps={glow} />
      <AnimatedEllipse testID="necromancer-glow" cx="66" cy="42" rx="2.6" ry="4.2" fill="#bf9eff" animatedProps={glow} />

      <Rect x="8" y="91" width="134" height="6" rx="3" fill="#6b5a8e" />
      <Rect x="72" y="80" width="6" height="30" rx="3" fill="#6b5a8e" />
      <Circle cx="13" cy="86" r="6.5" fill="#b8a9cc" />
      <Circle cx="137" cy="86" r="6.5" fill="#b8a9cc" />
      {anchors.map((a) => (
        <Circle key={`${a.x},${a.y}`} testID="necromancer-anchor" cx={a.x} cy={a.y} r="2.2" fill="#bf9eff" />
      ))}
    </Svg>
  );
}
