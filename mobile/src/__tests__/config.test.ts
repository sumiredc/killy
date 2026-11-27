import { buildDemoEnabled, buildServerURL, buildWebDemoEnabled, ConfigError, demoEnabled, serverURL, webDemoEnabled } from '../config';

describe('serverURL', () => {
  test('reads the URL set for the build', () => {
    expect(serverURL('https://killy.sumire.pink')).toBe('https://killy.sumire.pink');
  });

  test('drops a trailing slash', () => {
    expect(serverURL('https://stg-killy.sumire.pink/')).toBe('https://stg-killy.sumire.pink');
  });

  test('fails when the build has no URL', () => {
    expect(() => serverURL(undefined)).toThrow(ConfigError);
    expect(() => serverURL('  ')).toThrow(ConfigError);
  });

  test('fails on something that is not an http(s) URL', () => {
    expect(() => serverURL('killy.sumire.pink')).toThrow(ConfigError);
    expect(() => serverURL('ftp://killy.sumire.pink')).toThrow(ConfigError);
  });
});

describe('buildServerURL', () => {
  const original = process.env.EXPO_PUBLIC_KILLY_SERVER_URL;
  afterEach(() => {
    process.env.EXPO_PUBLIC_KILLY_SERVER_URL = original;
  });

  test('reads EXPO_PUBLIC_KILLY_SERVER_URL', () => {
    process.env.EXPO_PUBLIC_KILLY_SERVER_URL = 'https://stg-killy.sumire.pink';
    expect(buildServerURL()).toBe('https://stg-killy.sumire.pink');
  });

  test('fails when EXPO_PUBLIC_KILLY_SERVER_URL is not set', () => {
    delete process.env.EXPO_PUBLIC_KILLY_SERVER_URL;
    expect(() => buildServerURL()).toThrow(ConfigError);
  });
});

describe('demoEnabled', () => {
  test('only 1 turns the public demo on', () => {
    expect(demoEnabled('1')).toBe(true);
    expect(demoEnabled(' 1 ')).toBe(true);
    for (const v of [undefined, '', '0', 'true', 'yes']) {
      expect(demoEnabled(v)).toBe(false);
    }
  });
});

describe('buildDemoEnabled', () => {
  const original = process.env.EXPO_PUBLIC_KILLY_DEMO;
  afterEach(() => {
    process.env.EXPO_PUBLIC_KILLY_DEMO = original;
  });

  test('reads EXPO_PUBLIC_KILLY_DEMO', () => {
    process.env.EXPO_PUBLIC_KILLY_DEMO = '1';
    expect(buildDemoEnabled()).toBe(true);
    delete process.env.EXPO_PUBLIC_KILLY_DEMO;
    expect(buildDemoEnabled()).toBe(false);
  });
});

describe('webDemoEnabled', () => {
  test('only 1 makes the Web version demo-only', () => {
    expect(webDemoEnabled('1')).toBe(true);
    expect(webDemoEnabled(' 1 ')).toBe(true);
    for (const v of [undefined, '', '0', 'true', 'yes', '11']) {
      expect(webDemoEnabled(v)).toBe(false);
    }
  });
});

describe('buildWebDemoEnabled', () => {
  const original = process.env.EXPO_PUBLIC_KILLY_WEB_DEMO;
  afterEach(() => {
    if (original === undefined) delete process.env.EXPO_PUBLIC_KILLY_WEB_DEMO;
    else process.env.EXPO_PUBLIC_KILLY_WEB_DEMO = original;
  });

  test('reads EXPO_PUBLIC_KILLY_WEB_DEMO, not EXPO_PUBLIC_KILLY_DEMO', () => {
    process.env.EXPO_PUBLIC_KILLY_WEB_DEMO = '1';
    expect(buildWebDemoEnabled()).toBe(true);
    delete process.env.EXPO_PUBLIC_KILLY_WEB_DEMO;
    expect(buildWebDemoEnabled()).toBe(false);
    process.env.EXPO_PUBLIC_KILLY_DEMO = '1';
    try {
      expect(buildWebDemoEnabled()).toBe(false);
    } finally {
      delete process.env.EXPO_PUBLIC_KILLY_DEMO;
    }
  });
});
