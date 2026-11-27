import { ghostMatrix, groupTransform, rotateAbout, scytheMatrix, scytheOrigin } from '../killy/pose';

// react-native-svg's group takes a `matrix` on a phone; on the Web it is an SVG <g> that only knows
// the `transform` attribute and ignores `matrix`, so there Killy's scythe and lean did not move.

describe('the props that move one of Killy\'s groups', () => {
  test('on a phone it is the matrix itself', () => {
    const m = rotateAbout(-42, scytheOrigin);
    expect(groupTransform(m, false)).toEqual({ matrix: m });
  });

  test('on the Web it is the same matrix as an SVG transform, so the part turns the same way', () => {
    const m = rotateAbout(-42, scytheOrigin);
    expect(groupTransform(m, true)).toEqual({ transform: `matrix(${m.join(' ')})` });
  });

  test.each([
    ['the scythe wound up', scytheMatrix(1)],
    ['the scythe mid-swing', scytheMatrix(0, 35)],
    ['the ghost leaning', ghostMatrix(1)],
    ['the ghost bowing', ghostMatrix(0, 1)],
    ['at rest', ghostMatrix(0)],
  ])('%s: six numbers, in the order SVG reads them', (_name, m) => {
    const { transform } = groupTransform(m, true) as { transform: string };
    const numbers = /^matrix\(([^)]*)\)$/.exec(transform)?.[1].split(' ').map(Number);
    // `join` writes -0 as 0, which is the same number to SVG.
    expect(numbers).toEqual(m.map((v) => v + 0));
    expect(numbers).toHaveLength(6);
    expect(numbers?.every(Number.isFinite)).toBe(true);
  });
});
