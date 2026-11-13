import { render, screen } from '@testing-library/react-native';

import { KText } from '../theme/KText';
import { fontFor } from '../theme/tokens';

describe('fontFor', () => {
  test('Latin text uses Geist and Geist Mono', () => {
    expect(fontFor('Battle', 'mono')).toBe('GeistMono_400Regular');
    expect(fontFor('No servers yet', 'monoSemiBold')).toBe('GeistMono_600SemiBold');
    expect(fontFor('Add a server in Settings', 'body')).toBe('Geist_400Regular');
  });

  test('text with Japanese glyphs uses Zen Kaku Gothic New at a matching weight', () => {
    expect(fontFor('撃退', 'mono')).toBe('ZenKakuGothicNew_400Regular');
    expect(fontFor('サーバーがありません', 'monoSemiBold')).toBe('ZenKakuGothicNew_700Bold');
    expect(fontFor('親 PID 3801', 'body')).toBe('ZenKakuGothicNew_400Regular');
  });
});

test('KText picks the font from its text', async () => {
  await render(
    <>
      <KText kind="mono">設定</KText>
      <KText kind="mono">Settings</KText>
    </>,
  );
  expect(screen.getByText('設定')).toHaveStyle({ fontFamily: 'ZenKakuGothicNew_400Regular' });
  expect(screen.getByText('Settings')).toHaveStyle({ fontFamily: 'GeistMono_400Regular' });
});
