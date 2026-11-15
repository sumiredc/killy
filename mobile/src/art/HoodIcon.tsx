import Svg, { Circle, Ellipse, Path } from 'react-native-svg';

import { colors } from '@/theme/tokens';

/**
 * One parent in the counter, from hoodIconSVG in mock/art.js. The colours are the mock's
 * `.counter .hood` rules: the current parent is purple with glowing eyes, the others dark grey.
 */
export function HoodIcon({ current }: { current: boolean }) {
  const eye = current ? colors.purple300 : '#5d5668';
  return (
    <Svg width={18} height={(18 * 22) / 20} viewBox="0 0 20 22">
      <Path d="M11 1 C5 1 2 6 2 11 L1 21 H19 L18 11 C18 6 16 2 13 1.4 Q16 0.6 17 2 Q15 0 11 1 Z" fill={current ? '#6b5a8e' : '#4a4157'} />
      <Ellipse cx="8.6" cy="10" rx="4.2" ry="4.8" fill="#000" />
      <Circle testID="hood-eye" cx="7" cy="10.2" r="1.1" fill={eye} />
      <Circle testID="hood-eye" cx="10" cy="10" r="0.9" fill={eye} />
    </Svg>
  );
}
