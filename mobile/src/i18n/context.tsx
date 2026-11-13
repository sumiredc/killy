import { getLocales } from 'expo-localization';
import { createContext, type ReactNode, useContext, useState } from 'react';

import { initialLanguage, type Key, type Lang, t } from '.';

type I18n = {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: Key, params?: Record<string, string | number>) => string;
};

const I18nContext = createContext<I18n | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLang] = useState<Lang>(() => initialLanguage(getLocales()));
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
