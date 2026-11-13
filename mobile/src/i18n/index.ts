import en from './en.json';
import ja from './ja.json';

export type Lang = 'ja' | 'en';
export type Key = keyof typeof en;

export const translations: Record<Lang, Record<Key, string>> = { ja, en };

/** The language the app starts in: Japanese on a phone whose first language is Japanese, English otherwise. */
export function initialLanguage(locales: readonly { languageCode?: string | null }[]): Lang {
  return locales[0]?.languageCode === 'ja' ? 'ja' : 'en';
}

/** The text for key in lang, with {name} placeholders filled from params. */
export function t(lang: Lang, key: Key, params: Record<string, string | number> = {}): string {
  return translations[lang][key].replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? ''));
}
