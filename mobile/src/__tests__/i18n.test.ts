import fs from 'node:fs';
import path from 'node:path';

import { initialLanguage, t, translations } from '../i18n';

// The keys of the I18N table in mock/app.js, the starting copy for both languages (docs/DESIGN.md).
function mockKeys(lang: 'ja' | 'en'): string[] {
  const src = fs.readFileSync(path.join(__dirname, '../../../mock/app.js'), 'utf8');
  const start = src.indexOf(`  ${lang}: {`);
  const block = src.slice(start, src.indexOf('\n  },', start));
  return [...block.matchAll(/^\s{4}(\w+): /gm)].map((m) => m[1]);
}

// Lock-screen preview and mock controls: not part of the app.
const excluded = (key: string) => ['lockDate', 'now', 'pushText', 'tapToOpen'].includes(key) || key.startsWith('mock');

describe('translations', () => {
  test.each(['ja', 'en'] as const)('%s has every key of the mock I18N table except the mock-only ones', (lang) => {
    const want = mockKeys(lang).filter((k) => !excluded(k));
    expect(want.length).toBeGreaterThan(40);
    expect(Object.keys(translations[lang]).sort()).toEqual(expect.arrayContaining(want));
  });

  test.each(['ja', 'en'] as const)('%s has none of the mock-only keys', (lang) => {
    expect(Object.keys(translations[lang]).filter(excluded)).toEqual([]);
  });

  test('both languages have the same keys', () => {
    expect(Object.keys(translations.ja).sort()).toEqual(Object.keys(translations.en).sort());
  });

  test('every text is filled in', () => {
    for (const lang of ['ja', 'en'] as const) {
      for (const [key, text] of Object.entries(translations[lang])) {
        expect([key, text.trim().length > 0]).toEqual([key, true]);
      }
    }
  });
});

describe('initialLanguage', () => {
  test('a phone set to Japanese starts in Japanese', () => {
    expect(initialLanguage([{ languageCode: 'ja' }, { languageCode: 'en' }])).toBe('ja');
  });

  test('a phone set to English starts in English', () => {
    expect(initialLanguage([{ languageCode: 'en' }, { languageCode: 'ja' }])).toBe('en');
  });

  test('a phone set to any other language starts in English', () => {
    expect(initialLanguage([{ languageCode: 'fr' }])).toBe('en');
    expect(initialLanguage([{ languageCode: 'zh' }, { languageCode: 'ja' }])).toBe('en');
  });

  test('without a known language it starts in English', () => {
    expect(initialLanguage([])).toBe('en');
    expect(initialLanguage([{ languageCode: null }])).toBe('en');
  });
});

describe('t', () => {
  test('returns the text of the language', () => {
    expect(t('ja', 'hold')).toBe('長押しで kill');
    expect(t('en', 'hold')).toBe('Hold to kill');
  });

  test('fills in the placeholders', () => {
    expect(t('en', 'killSub', { pid: 3801, host: 'web-1' })).toBe('SIGKILL → PID 3801 on web-1');
    expect(t('ja', 'rowZombies', { n: 5, p: 2 })).toBe('ゾンビプロセス 5 / 親 2');
  });
});
