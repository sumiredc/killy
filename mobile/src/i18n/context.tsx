import { createContext, type ReactNode, useContext, useState } from 'react';

import { type Key, type Lang, t } from '.';

type I18n = {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: Key, params?: Record<string, string | number>) => string;
};

const I18nContext = createContext<I18n | null>(null);

/**
 * The app language. It starts as initial, which RootLayout read before drawing anything, and a
 * change is shown at once and then saved with save. A save that fails is not shown: the chosen
 * language stays for this run.
 */
export function I18nProvider({ children, initial, save }: { children: ReactNode; initial: Lang; save: (lang: Lang) => Promise<void> }) {
  const [lang, setLangState] = useState<Lang>(initial);
  const setLang = (l: Lang) => {
    setLangState(l);
    save(l).catch(() => {});
  };
  return (
    <I18nContext.Provider value={{ lang, setLang, t: (key, params) => t(lang, key, params) }}>{children}</I18nContext.Provider>
  );
}

export function useI18n(): I18n {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error('useI18n outside I18nProvider');
  }
  return ctx;
}
