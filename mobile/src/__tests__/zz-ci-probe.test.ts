// Deliberately failing: proves the app job fails on a failing test (#14). This branch is never merged.
test('ci probe fails', () => {
  expect(1).toBe(2);
});
