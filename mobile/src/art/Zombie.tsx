import { Platform } from 'react-native';
import Animated, { type SharedValue, useAnimatedProps } from 'react-native-reanimated';
import Svg, { Circle, G, type GProps, Path, Rect } from 'react-native-svg';

import { groupTransform, rotateAbout } from '@/killy/pose';
import type { ZombiePose } from '@/killy/strike';

import { hand, head } from './points';

const AnimatedG = Animated.createAnimatedComponent(G);

/**
 * Where each arm turns when it drops: the right end of its own box, half way down (the mock's
 * transform-box fill-box with transform-origin 100% 50%). The front arm's box takes in the hand.
 */
export const armOrigins = { back: { x: 26, y: 40.5 }, front: { x: 26, y: 46.5 } } as const;

function FaceDead() {
  return (
    <Path
      d="M16.5 15.5 L23 22 M23 15.5 L16.5 22 M28.5 15 L33.5 20 M33.5 15 L28.5 20"
      stroke="#1c1922"
      strokeWidth="2"
      strokeLinecap="round"
    />
  );
}

function FaceNormal() {
  return (
    <>
      <Circle cx="20" cy="19" r="4.2" fill="#fff" />
      <Circle cx="19" cy="19.6" r="1.9" fill="#1c1922" />
      <Circle cx="31" cy="18" r="3" fill="#fff" />
      <Circle cx="30.2" cy="18.5" r="1.4" fill="#1c1922" />
    </>
  );
}

/**
 * A zombie process, from zombieSVG in mock/art.js, facing left. `dead` swaps face-normal for
 * face-dead (the mock hides face-dead until the strings are cut). Given a `pose` instead, both
 * faces are drawn and the pose shows one of them and turns the arms, so the kill sequence can
 * move them on the UI thread.
 */
export function Zombie({ width, dead = false, pose }: { width: number; dead?: boolean; pose?: SharedValue<ZombiePose> }) {
  // `matrix` as in Killy: the transform react-native-svg's native group takes, `transform` on the Web.
  const web = Platform.OS === 'web';
  const armBack = useAnimatedProps(() => groupTransform(rotateAbout(pose ? pose.value.arm : 0, armOrigins.back), web) as unknown as GProps);
  const armFront = useAnimatedProps(() => groupTransform(rotateAbout(pose ? pose.value.arm : 0, armOrigins.front), web) as unknown as GProps);
  const normal = useAnimatedProps(() => ({ opacity: pose && !pose.value.alive ? 0 : 1 }));
  const deadFace = useAnimatedProps(() => ({ opacity: pose && !pose.value.alive ? 1 : 0 }));
  return (
    <Svg width={width} height={(width * 80) / 56} viewBox="0 0 56 80" testID="zombie-art">
      <Rect x="20" y="58" width="8" height="16" rx="3" fill="#3b4360" />
      <Rect x="32" y="58" width="8" height="16" rx="3" fill="#3b4360" />
      <Rect x="15" y="72" width="13" height="6" rx="3" fill="#262b3d" />
      <Rect x="28" y="72" width="13" height="6" rx="3" fill="#262b3d" />
      <AnimatedG testID="zombie-arm-back" animatedProps={armBack}>
        <Rect x="4" y="37" width="22" height="7" rx="3.5" fill="#6f9a5c" />
      </AnimatedG>
      <Path d="M16 33 H44 V60 L40 56 L36 61 L31 56 L26 61 L21 56 L16 60 Z" fill="#6b5a8e" />
      <Rect x="12" y="4" width="32" height="30" rx="13" fill="#8fbf74" />
      <Path d="M15 11 Q21 2 29 6 Q36 0 43 9 Q35 8 30 11 Q23 7 15 11 Z" fill="#3d6b35" />
      <Path d="M36 23 L40 27 M40 23 L36 27" stroke="#3d6b35" strokeWidth="1.4" strokeLinecap="round" />
      <Path d="M17 27.5 H28 M20 25.5 V29.5 M24 25.5 V29.5" stroke="#2f5228" strokeWidth="1.5" strokeLinecap="round" />
      {pose ? (
        <>
          <AnimatedG testID="zombie-face-dead" animatedProps={deadFace}>
            <FaceDead />
          </AnimatedG>
          <AnimatedG testID="zombie-face-normal" animatedProps={normal}>
            <FaceNormal />
          </AnimatedG>
        </>
      ) : dead ? (
        <G testID="zombie-face-dead">
          <FaceDead />
        </G>
      ) : (
        <G testID="zombie-face-normal">
          <FaceNormal />
        </G>
      )}
      <AnimatedG testID="zombie-arm-front" animatedProps={armFront}>
        <Rect x="1" y="43" width="25" height="7" rx="3.5" fill="#8fbf74" />
        <Circle testID="zombie-hand" cx={hand.x} cy={hand.y} r="3.6" fill="#8fbf74" />
      </AnimatedG>
      <Circle testID="zombie-head-pt" cx={head.x} cy={head.y} r="1" fill="none" />
    </Svg>
  );
}
