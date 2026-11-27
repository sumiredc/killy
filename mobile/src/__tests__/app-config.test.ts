// The Expo config is app.json plus app.config.js, which changes it for the Web demo only.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const makeConfig = require('../../app.config.js') as (ctx: { config: Record<string, unknown> }) => {
  experiments?: { baseUrl?: string; reactCompiler?: boolean };
  name?: string;
};

const base = { name: 'Killy', experiments: { reactCompiler: true } };
const original = process.env.EXPO_PUBLIC_KILLY_WEB_DEMO;

afterEach(() => {
  if (original === undefined) delete process.env.EXPO_PUBLIC_KILLY_WEB_DEMO;
  else process.env.EXPO_PUBLIC_KILLY_WEB_DEMO = original;
});

test('the Web demo build is put under /demo/, where the introduction site serves it', () => {
  process.env.EXPO_PUBLIC_KILLY_WEB_DEMO = '1';
  const config = makeConfig({ config: base });
  expect(config.experiments).toEqual({ reactCompiler: true, baseUrl: '/demo' });
  expect(config.name).toBe('Killy');
});

test.each([undefined, '', '0', 'true'])('a build without the Web demo variable (%s) has no base URL, as the phone builds', (value) => {
  if (value === undefined) delete process.env.EXPO_PUBLIC_KILLY_WEB_DEMO;
  else process.env.EXPO_PUBLIC_KILLY_WEB_DEMO = value;
  const config = makeConfig({ config: base });
  expect(config.experiments).toEqual({ reactCompiler: true });
});
