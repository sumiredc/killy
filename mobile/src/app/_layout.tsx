import { Geist_400Regular, Geist_500Medium } from '@expo-google-fonts/geist';
import { GeistMono_400Regular, GeistMono_600SemiBold } from '@expo-google-fonts/geist-mono';
import { ZenKakuGothicNew_400Regular, ZenKakuGothicNew_700Bold } from '@expo-google-fonts/zen-kaku-gothic-new';
import { useFonts } from 'expo-font';
import { Stack } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useMemo } from 'react';
import { StyleSheet, Text, View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { buildServerURL, ConfigError } from '@/config';
import { ServerURLProvider } from '@/config-context';
import { I18nProvider } from '@/i18n/context';
import { BattleProvider } from '@/killy/battle-context';
import { KillyProvider } from '@/killy/context';
import { colors, fonts } from '@/theme/tokens';

SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const [loaded, fontError] = useFonts({
    Geist_400Regular,
    Geist_500Medium,
    GeistMono_400Regular,
    GeistMono_600SemiBold,
    ZenKakuGothicNew_400Regular,
    ZenKakuGothicNew_700Bold,
  });
  const config = useMemo(() => readConfig(), []);
  // A font that fails to load falls back to the system font; the app still opens.
  const ready = loaded || fontError !== null;
  useEffect(() => {
    if (ready) {
      SplashScreen.hideAsync();
    }
  }, [ready]);
  if (!ready) {
    return null;
  }
  return (
    <SafeAreaProvider>
      <StatusBar style="light" />
      {'error' in config ? (
        <ConfigErrorScreen message={config.error} />
      ) : (
        <ServerURLProvider url={config.url}>
          <I18nProvider>
            <KillyProvider>
              <BattleProvider>
                <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.bg } }} />
              </BattleProvider>
            </KillyProvider>
          </I18nProvider>
        </ServerURLProvider>
      )}
    </SafeAreaProvider>
  );
}

function readConfig(): { url: string } | { error: string } {
  try {
    return { url: buildServerURL() };
  } catch (e) {
    if (e instanceof ConfigError) {
      return { error: e.message };
    }
    throw e;
  }
}

// Shown instead of the app when the build has no Killy server; nothing is sent anywhere.
function ConfigErrorScreen({ message }: { message: string }) {
  return (
    <View style={styles.error} testID="config-error">
      <Text style={styles.errorTitle}>Killy</Text>
      <Text style={styles.errorText}>{message}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  error: { flex: 1, backgroundColor: colors.bg, justifyContent: 'center', padding: 24 },
  errorTitle: { color: colors.text, fontFamily: fonts.mono, fontSize: 22, marginBottom: 12 },
  errorText: { color: colors.red, fontFamily: fonts.mono, fontSize: 14 },
});
