import { CameraView, useCameraPermissions } from 'expo-camera';
import { useEffect, useRef, useState } from 'react';
import { ActivityIndicator, KeyboardAvoidingView, Modal, Platform, Pressable, StyleSheet, TextInput, View } from 'react-native';

import { buildDemoEnabled } from '@/config';
import { useServerURL } from '@/config-context';
import type { Key } from '@/i18n';
import { useI18n } from '@/i18n/context';
import { ClaimError } from '@/killy/client';
import { claimMessages } from '@/killy/claim-messages';
import { useKilly } from '@/killy/context';
import type { PairLink } from '@/killy/pair-link';
import { type PairCheck, parsePairURL } from '@/killy/pair-url';
import { KText } from '@/theme/KText';
import { colors, fonts, radii } from '@/theme/tokens';

type Phase = { kind: 'form'; error: Key | null } | { kind: 'adding' } | { kind: 'added'; name: string };

/** How long "Added ‹name›" stays before the sheet closes (DESIGN: Add server sheet). */
export const addedFor = 1300;

/** The message for a pairing link from a QR code or a deep link that was not accepted. */
export function pairLinkError(reason: Extract<PairCheck, { ok: false }>['reason']): Key {
  return reason === 'other_server' ? 'qrOtherServer' : 'qrInvalid';
}

/**
 * Upper-case letters and digits only, at most 6: the form of a pairing code. Full-width letters
 * and digits, which a Japanese keyboard types, count as their ASCII forms.
 */
export function cleanCode(text: string): string {
  return text
    .normalize('NFKC')
    .toUpperCase()
    .replace(/[^A-Z0-9]/g, '')
    .slice(0, 6);
}

/**
 * Adding a server is one path whatever the code came from (typed, a scanned QR code, a killy://pair
 * link): it goes into the code field and Add sends the claim, so a display name can still be given.
 */
export function AddServerSheet({ onClose, link }: { onClose: () => void; link?: PairLink | null }) {
  const { t } = useI18n();
  const { client, registration } = useKilly();
  const appServer = useServerURL();
  const showDemo = buildDemoEnabled();
  const [code, setCode] = useState(link?.ok ? link.code : '');
  const [name, setName] = useState('');
  const [phase, setPhase] = useState<Phase>({ kind: 'form', error: link && !link.ok ? pairLinkError(link.reason) : null });
  const [scanning, setScanning] = useState(false);
  const [taken, setTaken] = useState<number | null>(link?.id ?? null);
  // A link that comes while the sheet is open is taken in place, once the form is back if a claim
  // is running: a second sheet over this one is refused by iOS. A refused link keeps the field.
  if (link && link.id !== taken && phase.kind === 'form') {
    setTaken(link.id);
    setScanning(false);
    if (link.ok) {
      setCode(link.code);
      setPhase({ kind: 'form', error: null });
    } else {
      setPhase({ kind: 'form', error: pairLinkError(link.reason) });
    }
  }
  const [, requestCameraPermission] = useCameraPermissions();
  // onBarcodeScanned fires for every frame the code is in view; only the first read counts.
  const scanned = useRef(false);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      if (closeTimer.current !== null) clearTimeout(closeTimer.current);
    };
  }, []);

  // Not while registration is still in progress: it settles within the request timeout.
  const ready = code.length === 6 && registration !== 'pending';

  async function startScan() {
    setPhase({ kind: 'form', error: null });
    const permission = await requestCameraPermission();
    if (!mounted.current) return;
    if (!permission.granted) {
      setPhase({ kind: 'form', error: 'cameraDenied' });
      return;
    }
    scanned.current = false;
    setScanning(true);
  }

  // Nothing is sent from here: a link for another Killy server must not reach any server.
  function onScanned({ data }: { data: string }) {
    if (scanned.current) return;
    scanned.current = true;
    setScanning(false);
    const link = parsePairURL(data, appServer);
    if (link.ok) {
      setCode(link.code);
      setPhase({ kind: 'form', error: null });
    } else {
      setPhase({ kind: 'form', error: pairLinkError(link.reason) });
    }
  }

  // A claim with the code in the field, or the public demo without one: the same states either way.
  async function add(send: () => Promise<{ name: string }>) {
    if (phase.kind !== 'form' || registration === 'pending') {
      return;
    }
    // Without a device token nothing is sent; the phone says why instead.
    if (registration !== 'registered') {
      setPhase({ kind: 'form', error: registration === 'rate_limited' ? 'deviceRateLimited' : 'deviceNotRegistered' });
      return;
    }
    setPhase({ kind: 'adding' });
    try {
      const server = await send();
      if (!mounted.current) return;
      // The server is added without the agent; its state arrives later on the event stream.
      setPhase({ kind: 'added', name: server.name });
      closeTimer.current = setTimeout(onClose, addedFor);
    } catch (e) {
      if (!mounted.current) return;
      setPhase({ kind: 'form', error: claimMessages[e instanceof ClaimError ? e.kind : 'other'] });
    }
  }

  async function submit() {
    if (ready) {
      await add(() => client.claim(code, name));
    }
  }

  return (
    <Modal transparent animationType="slide" onRequestClose={onClose} visible>
      {/* padding on both: the Modal's window is not resized on Android, so the keyboard would cover Add. */}
      <KeyboardAvoidingView style={styles.backdrop} behavior="padding">
        <View style={styles.sheet} testID="add-server-sheet">
          <View style={styles.grabber} />
          <KText kind="monoSemiBold" style={styles.title}>
            {t('addTitle')}
          </KText>

          {phase.kind === 'form' && scanning ? (
            <View style={styles.scanner}>
              <CameraView
                style={styles.camera}
                facing="back"
                barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
                onBarcodeScanned={onScanned}
              />
              <Pressable accessibilityRole="button" accessibilityLabel={t('cameraClose')} onPress={() => setScanning(false)} style={styles.textButton}>
                <KText style={styles.textButtonLabel}>{t('cameraClose')}</KText>
              </Pressable>
            </View>
          ) : phase.kind === 'form' ? (
            <>
              <View style={styles.step}>
                <KText kind="mono" style={styles.stepNumber}>
                  1
                </KText>
                <View style={styles.stepBody}>
                  <KText style={styles.stepText}>{t('addStep1')}</KText>
                  <View style={styles.cmd}>
                    <KText kind="mono" style={styles.cmdText} selectable>
                      killy agent pair
                    </KText>
                  </View>
                </View>
              </View>
              <View style={styles.step}>
                <KText kind="mono" style={styles.stepNumber}>
                  2
                </KText>
                <KText style={[styles.stepText, styles.stepBody]}>{t('addStep2')}</KText>
              </View>

              <KText style={styles.label}>{t('codeLabel')}</KText>
              <TextInput
                accessibilityLabel={t('codeLabel')}
                style={[styles.input, styles.codeInput]}
                value={code}
                onChangeText={(text) => {
                  setCode(cleanCode(text));
                  setPhase({ kind: 'form', error: null });
                }}
                autoCapitalize="characters"
                autoCorrect={false}
                autoComplete="off"
                spellCheck={false}
                // A code is ASCII: no input method composing on top of it, no suggestions.
                keyboardType={Platform.OS === 'ios' ? 'ascii-capable' : 'visible-password'}
                placeholder="K7Q4XP"
                placeholderTextColor={colors.elev3}
              />
              {/* The Web version reads no QR code: the browser's camera is not what the code is for. */}
              {Platform.OS !== 'web' && (
                <Pressable accessibilityRole="button" accessibilityLabel={t('scanQr')} onPress={() => void startScan()} style={styles.ghost}>
                  <KText style={styles.ghostLabel}>{t('scanQr')}</KText>
                </Pressable>
              )}

              <KText style={styles.label}>{t('nameLabel')}</KText>
              <TextInput
                accessibilityLabel={t('nameLabel')}
                style={[styles.input, { fontFamily: fonts.mono }]}
                value={name}
                onChangeText={setName}
                maxLength={100}
                autoCapitalize="none"
                autoCorrect={false}
                spellCheck={false}
                placeholder={t('namePlaceholder')}
                placeholderTextColor={colors.muted}
              />

              {phase.error !== null && (
                <View style={styles.error}>
                  <KText style={styles.errorText}>{t(phase.error, { server: appServer })}</KText>
                </View>
              )}

              <View style={styles.actions}>
                <Pressable accessibilityRole="button" onPress={onClose} style={styles.textButton}>
                  <KText style={styles.textButtonLabel}>{t('cancel')}</KText>
                </Pressable>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('addSubmit')}
                  disabled={!ready}
                  onPress={() => void submit()}
                  style={[styles.primary, !ready && styles.primaryDisabled]}>
                  <KText kind="monoSemiBold" style={styles.primaryLabel}>
                    {t('addSubmit')}
                  </KText>
                </Pressable>
              </View>

              {/* DESIGN Add server sheet 5: below the form, for anyone without a server of their own,
                  in the builds that offer the public demo only. */}
              {showDemo && (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('tryDemo')}
                  disabled={registration === 'pending'}
                  onPress={() => void add(() => client.addDemoServer())}
                  style={[styles.ghost, styles.demo]}>
                  <KText style={styles.ghostLabel}>{t('tryDemo')}</KText>
                </Pressable>
              )}
            </>
          ) : (
            <View style={styles.status}>
              {phase.kind === 'adding' ? (
                <>
                  <ActivityIndicator color={colors.purple300} size="large" />
                  <KText style={styles.statusText}>{t('pairing')}</KText>
                </>
              ) : (
                <KText style={styles.statusText}>{t('paired', { name: phase.name })}</KText>
              )}
            </View>
          )}
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: 'flex-end', backgroundColor: 'rgba(0,0,0,0.6)' },
  sheet: {
    backgroundColor: colors.elev1,
    borderTopLeftRadius: radii.card,
    borderTopRightRadius: radii.card,
    borderTopWidth: 1,
    borderColor: colors.line,
    paddingHorizontal: 16,
    paddingTop: 10,
    paddingBottom: 34,
  },
  grabber: { alignSelf: 'center', width: 40, height: 5, borderRadius: radii.pill, backgroundColor: colors.line, marginBottom: 14 },
  title: { color: colors.text, fontSize: 19, marginBottom: 14 },
  step: { flexDirection: 'row', gap: 10, marginBottom: 12 },
  stepNumber: { color: colors.muted, fontSize: 14, lineHeight: 22, width: 12 },
  stepBody: { flex: 1 },
  stepText: { color: '#CFCAD6', fontSize: 14, lineHeight: 22 },
  cmd: { marginTop: 6, paddingVertical: 10, paddingHorizontal: 12, borderRadius: 10, backgroundColor: '#000', borderWidth: 1, borderColor: colors.line },
  cmdText: { color: colors.purple100, fontSize: 13 },
  label: { color: colors.muted, fontSize: 13, marginTop: 4, marginBottom: 6 },
  input: {
    minHeight: 48,
    paddingHorizontal: 14,
    borderRadius: radii.input,
    borderWidth: 1,
    borderColor: colors.line,
    backgroundColor: colors.elev2,
    color: colors.text,
    fontSize: 16,
    marginBottom: 12,
  },
  codeInput: { fontFamily: fonts.mono, fontSize: 24, letterSpacing: 10, textAlign: 'center' },
  // The secondary button of the sheet (`.ghost-btn` in the mock).
  ghost: { minHeight: 44, marginTop: -2, marginBottom: 14, borderRadius: radii.input, borderWidth: 1, borderColor: colors.line, alignItems: 'center', justifyContent: 'center' },
  demo: { marginTop: 16, marginBottom: 0 },
  ghostLabel: { color: colors.text, fontSize: 14 },
  scanner: { alignItems: 'center', gap: 6, marginBottom: 6 },
  camera: { width: '100%', aspectRatio: 1, borderRadius: radii.input, overflow: 'hidden' },
  error: { padding: 12, borderRadius: radii.input, backgroundColor: 'rgba(247,80,106,0.12)', marginBottom: 12 },
  errorText: { color: '#FF9AAA', fontSize: 13, lineHeight: 20 },
  actions: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginTop: 6 },
  textButton: { paddingVertical: 12, paddingHorizontal: 4 },
  textButtonLabel: { color: colors.muted, fontSize: 15 },
  primary: { minHeight: 48, paddingHorizontal: 22, borderRadius: radii.button, backgroundColor: colors.purple, justifyContent: 'center' },
  primaryDisabled: { backgroundColor: colors.elev3 },
  primaryLabel: { color: '#FFFFFF', fontSize: 15 },
  status: { alignItems: 'center', gap: 14, paddingTop: 26, paddingBottom: 16 },
  statusText: { color: '#CFCAD6', fontSize: 14, textAlign: 'center' },
});
