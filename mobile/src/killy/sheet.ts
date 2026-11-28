// How the sheets come in (DESIGN: Other motion): the sheet slides up over 300 ms and the backdrop
// fades over 200 ms. The mock's `sheet-up` is `.3s cubic-bezier(.2, .8, .2, 1)` from translateY(40%).

export const SHEET_UP_MS = 300;
export const SHEET_BACKDROP_MS = 200;

/** How far below its place the sheet starts, as a fraction of its height. */
export const SHEET_FROM = 0.4;

/** The sheet's easing, the mock's cubic-bezier(.2, .8, .2, 1). */
export const SHEET_EASING = [0.2, 0.8, 0.2, 1] as const;
