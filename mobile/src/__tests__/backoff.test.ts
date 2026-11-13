import { backoff } from '../killy/backoff';

describe('backoff', () => {
  test('the base wait doubles from 1 second and stops at 30 seconds', () => {
    const bases = [0, 1, 2, 3, 4, 5, 6, 7, 20].map((n) => backoff(n, () => 0));
    expect(bases).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000, 30000]);
  });

  test('the jitter adds up to a quarter of the base, never a full quarter', () => {
    for (const n of [0, 3, 6]) {
      const base = backoff(n, () => 0);
      expect(backoff(n, () => 0.5)).toBe(base * 1.125);
      const most = backoff(n, () => 0.999999);
      expect(most).toBeGreaterThan(base);
      expect(most).toBeLessThan(base * 1.25);
    }
  });
});
