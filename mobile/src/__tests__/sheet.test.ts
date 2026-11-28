import { SHEET_BACKDROP_MS, SHEET_EASING, SHEET_FROM, SHEET_UP_MS } from '../killy/sheet';

// DESIGN: Other motion. "Sheets: slide up 300 ms; backdrop fades 200 ms." The mock's `sheet-up`
// is `.3s cubic-bezier(.2, .8, .2, 1)` from translateY(40%). The motion itself is BottomSheet's
// (motion.test.tsx); these are the numbers it is given.
describe('how a sheet comes in', () => {
  test('the sheet rises over 300 ms and the backdrop fades over 200 ms', () => {
    expect(SHEET_UP_MS).toBe(300);
    expect(SHEET_BACKDROP_MS).toBe(200);
  });

  test('the sheet starts 40% of its height below its place', () => {
    expect(SHEET_FROM).toBe(0.4);
  });

  test('the easing is the mock\'s cubic-bezier(.2, .8, .2, 1)', () => {
    expect(SHEET_EASING).toEqual([0.2, 0.8, 0.2, 1]);
  });
});
