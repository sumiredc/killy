// Points of the artwork that motion and the strings need, in each figure's own viewBox. They are
// kept apart from the components so the motion code can use them without importing the art.

/** The necromancer's control-bar anchors in its 150 × 150 viewBox, left to right (parentSVG's `.anchor`). */
export const anchors = [
  { x: 12, y: 96 },
  { x: 44, y: 96 },
  { x: 75, y: 109 },
  { x: 106, y: 96 },
  { x: 138, y: 96 },
] as const;

/** Where the strings end on a zombie, in its 56 × 80 viewBox: zombieSVG's `.head-pt` and `.hand`. */
export const head = { x: 28, y: 5 };
export const hand = { x: 3.5, y: 46.5 };
