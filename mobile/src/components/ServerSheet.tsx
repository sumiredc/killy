import { useEffect, useRef, useState } from 'react';
import { KeyboardAvoidingView, Modal, Pressable, StyleSheet, Switch, TextInput, View } from 'react-native';

import type { Key } from '@/i18n';
import { useI18n } from '@/i18n/context';
import { ServerError } from '@/killy/client';
import { useKilly } from '@/killy/context';
import type { ServerState } from '@/killy/types';
import { KText } from '@/theme/KText';
import { colors, fonts, radii } from '@/theme/tokens';

function failure(e: unknown): Key {
  return e instanceof ServerError && e.kind === 'unreachable' ? 'pairUnreachable' : 'serverSaveFailed';
}

/**
 * Rename, mute or remove one server, for this phone only (DESIGN: Server sheet). The switch is
 * sent when it is flipped and the name when Done is pressed; a failure stays in the sheet, under
 * the control that failed. What the sheet shows comes from the state, which the Killy server sends
 * again after each change.
 */
export function ServerSheet({ server, onClose }: { server: ServerState; onClose: () => void }) {
  const { t } = useI18n();
  const { client } = useKilly();
  const [name, setName] = useState(server.name);
  const [saving, setSaving] = useState(false);
  const [nameError, setNameError] = useState<Key | null>(null);
  // The value sent, shown until a state with another value for the switch arrives; null shows the
  // state's value.
  const [notifySent, setNotifySent] = useState<boolean | null>(null);
  const [stateNotify, setStateNotify] = useState(server.notify);
  if (server.notify !== stateNotify) {
    setStateNotify(server.notify);
    setNotifySent(null);
  }
  const [notifySending, setNotifySending] = useState(false);
  const [notifyError, setNotifyError] = useState<Key | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeError, setRemoveError] = useState<Key | null>(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  async function toggleNotify(notify: boolean) {
    setNotifySent(notify);
    setNotifySending(true);
    setNotifyError(null);
    try {
      await client.updateServer(server.id, { notify });
    } catch (e) {
      if (!mounted.current) return;
      setNotifySent(null);
      setNotifyError(failure(e));
    } finally {
      if (mounted.current) setNotifySending(false);
    }
  }

  async function done() {
    const trimmed = name.trim();
    // An empty name keeps the previous one on the Killy server too, so there is nothing to send.
    if (trimmed === '' || trimmed === server.name) {
      onClose();
      return;
    }
    setSaving(true);
    setNameError(null);
    try {
      await client.updateServer(server.id, { name: trimmed });
      if (mounted.current) onClose();
    } catch (e) {
      if (!mounted.current) return;
      setNameError(failure(e));
      setSaving(false);
    }
  }

  async function remove() {
    if (!confirming) {
      setConfirming(true);
      return;
    }
    setRemoving(true);
    setRemoveError(null);
    try {
      await client.removeServer(server.id);
      if (mounted.current) onClose();
    } catch (e) {
      if (!mounted.current) return;
      setRemoveError(failure(e));
      setRemoving(false);
    }
  }

  return (
    <Modal transparent animationType="slide" onRequestClose={onClose} visible>
      <KeyboardAvoidingView style={styles.backdrop} behavior="padding">
        <View style={styles.sheet} testID="server-sheet">
          <View style={styles.grabber} />
          <KText kind="monoSemiBold" style={styles.title}>
            {t('detailTitle')}
          </KText>

          <KText style={styles.label}>{t('nameLabelEdit')}</KText>
          <TextInput
            accessibilityLabel={t('nameLabelEdit')}
            style={styles.input}
            value={name}
            onChangeText={(text) => {
              setName(text);
              setNameError(null);
            }}
            maxLength={100}
            autoCapitalize="none"
            autoCorrect={false}
            spellCheck={false}
            placeholderTextColor={colors.muted}
          />
          {nameError !== null && <ErrorLine testID="name-error" text={t(nameError)} />}

          <View style={styles.group}>
            <View style={styles.row}>
              <KText style={[styles.rowTitle, styles.rowMain]}>{t('hostLabel')}</KText>
              <KText kind="mono" style={styles.rowValue} numberOfLines={1} selectable testID="server-hostname">
                {server.hostname}
              </KText>
            </View>
            <View style={[styles.row, styles.rowDivider]}>
              <KText style={[styles.rowTitle, styles.rowMain]}>{t('notifyServer')}</KText>
              <Switch
                accessibilityLabel={t('notifyServer')}
                value={notifySent ?? server.notify}
                onValueChange={(v) => void toggleNotify(v)}
                disabled={notifySending}
                accessibilityState={{ disabled: notifySending }}
                trackColor={{ false: colors.elev3, true: colors.purple }}
                thumbColor="#FFFFFF"
                ios_backgroundColor={colors.elev3}
              />
            </View>
          </View>
          {notifyError !== null && <ErrorLine testID="notify-error" text={t(notifyError)} />}

          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t(confirming ? 'removeConfirm' : 'remove')}
            disabled={removing}
            onPress={() => void remove()}
            style={[styles.danger, confirming && styles.dangerConfirm]}>
            <KText style={[styles.dangerLabel, confirming && styles.dangerConfirmLabel]}>{t(confirming ? 'removeConfirm' : 'remove')}</KText>
          </Pressable>
          {removeError !== null && <ErrorLine testID="remove-error" text={t(removeError)} />}
          <KText style={styles.hint}>{t('removeHint')}</KText>

          <View style={styles.actions}>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('done')}
              disabled={saving}
              onPress={() => void done()}
              style={[styles.primary, saving && styles.primaryDisabled]}>
              <KText kind="monoSemiBold" style={styles.primaryLabel}>
                {t('done')}
              </KText>
            </Pressable>
          </View>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

function ErrorLine({ text, testID }: { text: string; testID: string }) {
  return (
    <KText style={styles.errorLine} testID={testID} accessibilityLiveRegion="polite">
      {text}
    </KText>
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
  label: { color: colors.muted, fontSize: 13, marginBottom: 6 },
  input: {
    minHeight: 48,
    paddingHorizontal: 14,
    borderRadius: radii.input,
    borderWidth: 1,
    borderColor: colors.line,
    backgroundColor: colors.elev2,
    color: colors.text,
    fontFamily: fonts.mono,
    fontSize: 16,
    marginBottom: 12,
  },
  group: { borderRadius: radii.group, borderWidth: 1, borderColor: colors.line, backgroundColor: colors.elev1, overflow: 'hidden' },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: 56, paddingVertical: 10, paddingHorizontal: 14 },
  rowDivider: { borderTopWidth: 1, borderTopColor: colors.line },
  rowMain: { flex: 1, minWidth: 0 },
  rowTitle: { color: colors.text, fontSize: 15 },
  rowValue: { color: colors.muted, fontSize: 13, flexShrink: 1 },
  errorLine: { color: colors.red, fontSize: 13, lineHeight: 20, marginTop: 6, marginHorizontal: 4 },
  danger: {
    minHeight: 48,
    marginTop: 16,
    borderRadius: radii.input,
    borderWidth: 1,
    borderColor: 'rgba(247,80,106,0.5)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  dangerConfirm: { backgroundColor: colors.red, borderColor: colors.red },
  dangerLabel: { color: colors.red, fontSize: 15 },
  dangerConfirmLabel: { color: '#FFFFFF' },
  hint: { color: colors.muted, fontSize: 12.5, lineHeight: 20, marginTop: 8, marginHorizontal: 4, marginBottom: 12 },
  actions: { flexDirection: 'row', justifyContent: 'flex-end', marginTop: 6 },
  primary: { minHeight: 48, paddingHorizontal: 22, borderRadius: radii.button, backgroundColor: colors.purple, justifyContent: 'center' },
  primaryDisabled: { backgroundColor: colors.elev3 },
  primaryLabel: { color: '#FFFFFF', fontSize: 15 },
});
