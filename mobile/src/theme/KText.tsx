import { Children, isValidElement } from 'react';
import { StyleSheet, Text, type TextProps } from 'react-native';

import { type FontKind, fontFor } from './tokens';

/** Text in the font DESIGN.md gives its kind, switching to the Japanese font for Japanese text. */
export function KText({ kind = 'body', style, children, ...rest }: TextProps & { kind?: FontKind }) {
  return (
    <Text style={StyleSheet.compose({ fontFamily: fontFor(plain(children), kind) }, style)} {...rest}>
      {children}
    </Text>
  );
}

function plain(children: TextProps['children']): string {
  return Children.toArray(children)
    .map((c) => (typeof c === 'string' || typeof c === 'number' ? String(c) : isValidElement(c) ? '' : ''))
    .join('');
}
