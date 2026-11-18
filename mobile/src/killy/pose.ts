// Killy's pose while the kill button is held (DESIGN: Hold to kill), as SVG matrices in Killy's
// 200 × 150 viewBox. They are passed to the `ghost` and `scythe` groups as the `matrix` prop, which
// is what react-native-svg sends to native for a transform; worklets, so the UI thread can call them.

/** The scythe's wind-up at a full charge, in degrees (the mock's `--charge * -42deg`). */
export const WIND_UP = -42;
/** Where the scythe turns: Killy's hand (the mock's `.killy .scythe` transform-origin). */
export const scytheOrigin = { x: 115, y: 82 };
/** Where the ghost leans back from (the mock's `.killy .ghost` transform-origin). */
export const ghostOrigin = { x: 70, y: 110 };
/** The lean at its fullest: the mock's `.killy.charging .ghost { transform: rotate(-6deg) translateX(-4px) }`. */
export const LEAN = { rotate: -6, x: -4 };
/** The bow: the mock's `.killy.bow .ghost { transform: rotate(14deg) translateY(4px) }`. */
export const BOW = { rotate: 14, y: 4 };

/** An SVG matrix [a, b, c, d, e, f]: rotate by `deg` around `o`, after moving the drawing by (tx, ty). */
export function rotateAbout(deg: number, o: { x: number; y: number }, tx = 0, ty = 0): number[] {
  'worklet';
  const r = (deg * Math.PI) / 180;
  const cos = Math.cos(r);
  const sin = Math.sin(r);
  // p' = o + R(p + (tx, ty) - o), as CSS applies `rotate() translate()` about a transform-origin.
  const dx = tx - o.x;
  const dy = ty - o.y;
  return [cos, sin, -sin, cos, o.x + cos * dx - sin * dy, o.y + sin * dx + cos * dy];
}

/**
 * The scythe's matrix at a charge from 0 to 1, or at `swing` degrees while the kill's swing runs
 * (null otherwise).
 */
export function scytheMatrix(charge: number, swing: number | null = null): number[] {
  'worklet';
  return rotateAbout(swing ?? WIND_UP * charge, scytheOrigin);
}

/** The ghost's matrix at a lean from 0 to 1 and a bow from 0 to 1 (Killy does not do both at once). */
export function ghostMatrix(lean: number, bow = 0): number[] {
  'worklet';
  return rotateAbout(LEAN.rotate * lean + BOW.rotate * bow, ghostOrigin, LEAN.x * lean, BOW.y * bow);
}

/** Applies an SVG matrix to a point, for tests and for anything that must follow a moved part. */
export function apply(m: number[], p: { x: number; y: number }): { x: number; y: number } {
  'worklet';
  return { x: m[0] * p.x + m[2] * p.y + m[4], y: m[1] * p.x + m[3] * p.y + m[5] };
}
