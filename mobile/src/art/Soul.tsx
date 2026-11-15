import Svg, { Path } from 'react-native-svg';

/** A zombie's soul, from soulSVG in mock/art.js. It rises when the zombie is released (#28). */
export function Soul({ width }: { width: number }) {
  return (
    <Svg width={width} height={(width * 46) / 40} viewBox="0 0 40 46" testID="soul-art">
      <Path
        d="M20 3 C10 3 5 11 5 20 V36 Q9 43 13 37 Q16.5 43 20 37 Q23.5 43 27 37 Q31 43 35 36 V20 C35 11 30 3 20 3 Z"
        fill="#e2d4ff"
        opacity=".9"
      />
      <Path d="M12.5 20 Q15 17.5 17.5 20 M22.5 20 Q25 17.5 27.5 20" fill="none" stroke="#6a4fb0" strokeWidth="1.6" strokeLinecap="round" />
    </Svg>
  );
}
