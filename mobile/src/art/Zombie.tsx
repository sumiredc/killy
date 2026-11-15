import Svg, { Circle, G, Path, Rect } from 'react-native-svg';

/** Where the strings end, in the 56 × 80 viewBox: zombieSVG's `.head-pt` and `.hand`. */
export const head = { x: 28, y: 5 };
export const hand = { x: 3.5, y: 46.5 };

/**
 * A zombie process, from zombieSVG in mock/art.js, facing left. `dead` swaps face-normal for
 * face-dead (the mock hides face-dead until the strings are cut).
 */
export function Zombie({ width, dead = false }: { width: number; dead?: boolean }) {
  return (
    <Svg width={width} height={(width * 80) / 56} viewBox="0 0 56 80" testID="zombie-art">
      <Rect x="20" y="58" width="8" height="16" rx="3" fill="#3b4360" />
      <Rect x="32" y="58" width="8" height="16" rx="3" fill="#3b4360" />
      <Rect x="15" y="72" width="13" height="6" rx="3" fill="#262b3d" />
      <Rect x="28" y="72" width="13" height="6" rx="3" fill="#262b3d" />
      <G testID="zombie-arm-back">
        <Rect x="4" y="37" width="22" height="7" rx="3.5" fill="#6f9a5c" />
      </G>
      <Path d="M16 33 H44 V60 L40 56 L36 61 L31 56 L26 61 L21 56 L16 60 Z" fill="#6b5a8e" />
      <Rect x="12" y="4" width="32" height="30" rx="13" fill="#8fbf74" />
      <Path d="M15 11 Q21 2 29 6 Q36 0 43 9 Q35 8 30 11 Q23 7 15 11 Z" fill="#3d6b35" />
      <Path d="M36 23 L40 27 M40 23 L36 27" stroke="#3d6b35" strokeWidth="1.4" strokeLinecap="round" />
      <Path d="M17 27.5 H28 M20 25.5 V29.5 M24 25.5 V29.5" stroke="#2f5228" strokeWidth="1.5" strokeLinecap="round" />
      {dead ? (
        <G testID="zombie-face-dead">
          <Path
            d="M16.5 15.5 L23 22 M23 15.5 L16.5 22 M28.5 15 L33.5 20 M33.5 15 L28.5 20"
            stroke="#1c1922"
            strokeWidth="2"
            strokeLinecap="round"
          />
        </G>
      ) : (
        <G testID="zombie-face-normal">
          <Circle cx="20" cy="19" r="4.2" fill="#fff" />
          <Circle cx="19" cy="19.6" r="1.9" fill="#1c1922" />
          <Circle cx="31" cy="18" r="3" fill="#fff" />
          <Circle cx="30.2" cy="18.5" r="1.4" fill="#1c1922" />
        </G>
      )}
      <G testID="zombie-arm-front">
        <Rect x="1" y="43" width="25" height="7" rx="3.5" fill="#8fbf74" />
        <Circle testID="zombie-hand" cx={hand.x} cy={hand.y} r="3.6" fill="#8fbf74" />
      </G>
      <Circle testID="zombie-head-pt" cx={head.x} cy={head.y} r="1" fill="none" />
    </Svg>
  );
}
