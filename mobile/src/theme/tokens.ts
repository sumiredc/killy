// Design tokens from docs/DESIGN.md (Design tokens).
export const colors = {
  bg: '#000000',
  elev1: '#19161F',
  elev2: '#26222D',
  elev3: '#322C3A',
  line: '#3A3443',
  text: '#FAFAFA',
  muted: '#958F9E',
  purple: '#9046FF',
  purple300: '#BF9EFF',
  purple100: '#E2D4FF',
  red: '#F7506A',
  green: '#4FDB8F',
} as const;

export const radii = {
  card: 24,
  group: 20,
  button: 16,
  input: 12,
  pill: 999,
} as const;

export const gutter = 16;

// Font families as loaded in the root layout.
export const fonts = {
  body: 'Geist_400Regular',
  bodyMedium: 'Geist_500Medium',
  mono: 'GeistMono_400Regular',
  monoSemiBold: 'GeistMono_600SemiBold',
  ja: 'ZenKakuGothicNew_400Regular',
  jaBold: 'ZenKakuGothicNew_700Bold',
} as const;

export type FontKind = 'body' | 'bodyMedium' | 'mono' | 'monoSemiBold';

// Hiragana, katakana, CJK ideographs and full-width forms.
const japanese = /[\u3000-\u30ff\u3400-\u4dbf\u4e00-\u9fff\uff00-\uffef]/;

/**
 * The font for text of the given kind. React Native draws a Text in one font with no fallback
 * list, so text with Japanese glyphs is drawn in Zen Kaku Gothic New (DESIGN: the fallback after
 * Geist and Geist Mono), bold where the Latin kind is medium or semibold.
 */
export function fontFor(text: string, kind: FontKind): string {
  if (japanese.test(text)) {
    return kind === 'bodyMedium' || kind === 'monoSemiBold' ? fonts.jaBold : fonts.ja;
  }
  return fonts[kind];
}
