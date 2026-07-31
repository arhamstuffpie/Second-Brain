import { useEffect, useRef, useState } from 'react';
import { Alert, Animated, StyleSheet, Switch, Text, View } from 'react-native';

import {
  Body,
  Button,
  Card,
  ChoiceRow,
  ErrorNotice,
  Field,
  PageHeader,
  Screen,
  SectionLabel,
  StatusPill,
} from '@/components/ui';
import { Radius, Spacing } from '@/constants/theme';
import { useReducedMotion } from '@/hooks/use-reduced-motion';
import { getReadableError } from '@/lib/readable-error';
import { useApp } from '@/state/app-provider';
import type { AppSettings } from '@/types/app';
import { useTheme } from '@/hooks/use-theme';

function ToggleRow({
  label,
  description,
  value,
  onChange,
}: {
  label: string;
  description: string;
  value: boolean;
  onChange: (value: boolean) => void;
}) {
  const theme = useTheme();
  return (
    <View style={styles.toggleRow}>
      <View style={styles.toggleCopy}>
        <Text style={[styles.toggleLabel, { color: theme.text }]}>{label}</Text>
        <Body muted>{description}</Body>
      </View>
      <Switch
        value={value}
        onValueChange={onChange}
        trackColor={{ false: theme.backgroundElement, true: theme.accentSoft }}
        thumbColor={value ? theme.accent : theme.textSecondary}
      />
    </View>
  );
}

export function SettingsScreen() {
  const theme = useTheme();
  const { settings, auth, network, power, updateSettings, checkHealth, logout, showError } = useApp();
  const [draft, setDraft] = useState<AppSettings>(settings);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [health, setHealth] = useState<'idle' | 'checking' | 'healthy' | 'failed'>('idle');
  const [healthError, setHealthError] = useState('');
  const [snackbarVisible, setSnackbarVisible] = useState(false);
  const reducedMotion = useReducedMotion();
  const snackbarOpacity = useRef(new Animated.Value(0)).current;
  const snackbarOffset = useRef(new Animated.Value(12)).current;
  const snackbarTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => setDraft(settings), [settings]);
  useEffect(
    () => () => {
      if (snackbarTimer.current) clearTimeout(snackbarTimer.current);
    },
    [],
  );

  function patch<T extends keyof AppSettings>(key: T, value: AppSettings[T]) {
    setDraft((current) => ({ ...current, [key]: value }));
  }

  function showSavedSnackbar() {
    if (snackbarTimer.current) clearTimeout(snackbarTimer.current);
    setSnackbarVisible(true);
    if (reducedMotion) {
      snackbarOpacity.setValue(1);
      snackbarOffset.setValue(0);
      snackbarTimer.current = setTimeout(() => setSnackbarVisible(false), 2400);
      return;
    }
    snackbarOpacity.setValue(0);
    snackbarOffset.setValue(12);
    Animated.parallel([
      Animated.timing(snackbarOpacity, { toValue: 1, duration: 180, useNativeDriver: true }),
      Animated.timing(snackbarOffset, { toValue: 0, duration: 180, useNativeDriver: true }),
    ]).start();
    snackbarTimer.current = setTimeout(() => {
      Animated.parallel([
        Animated.timing(snackbarOpacity, { toValue: 0, duration: 180, useNativeDriver: true }),
        Animated.timing(snackbarOffset, { toValue: 8, duration: 180, useNativeDriver: true }),
      ]).start(({ finished }) => {
        if (finished) setSnackbarVisible(false);
      });
    }, 2400);
  }

  async function save() {
    setSaveError('');
    const base = draft.apiBaseUrl.trim().replace(/\/+$/, '');
    if (!/^https?:\/\//.test(base)) {
      Alert.alert('Invalid backend URL', 'Enter a complete http:// or https:// address.');
      return;
    }
    setSaving(true);
    try {
      await updateSettings({
        ...draft,
        apiBaseUrl: base,
        memographApiKey: draft.memographApiKey.trim(),
        projectId: draft.projectId.trim(),
        memoryId: draft.memoryId.trim(),
        groupId: draft.groupId.trim(),
        location: draft.location.trim(),
      });
      showSavedSnackbar();
    } catch (error) {
      const message = getReadableError(error, 'backend');
      setSaveError(message);
      showError(message);
    } finally {
      setSaving(false);
    }
  }

  async function testBackend() {
    setHealth('checking');
    setHealthError('');
    try {
      await checkHealth();
      setHealth('healthy');
    } catch (error) {
      setHealth('failed');
      const message = getReadableError(error, 'backend');
      setHealthError(message);
      showError(message);
    }
  }

  return (
    <View style={styles.root}>
      <Screen contentStyle={styles.screen}>
      <PageHeader
        eyebrow="PREFERENCES"
        title="Settings"
        subtitle="Manage capture quality, network use, and your Memograph destination."
        action={
          <StatusPill
            label={network.online ? network.type.toLowerCase() : 'offline'}
            tone={network.online ? 'success' : 'warning'}
          />
        }
      />

      <Card>
        <SectionLabel>Backend</SectionLabel>
        <Field
          label="API base URL"
          value={draft.apiBaseUrl}
          onChangeText={(value) => patch('apiBaseUrl', value)}
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="url"
          hint="Production builds require HTTPS. Use a LAN IP for local-device testing."
        />
        <View style={styles.inline}>
          <Button
            label={health === 'checking' ? 'Checking…' : 'Test connection'}
            variant="secondary"
            compact
            disabled={health === 'checking'}
            onPress={() => void testBackend()}
          />
          {health !== 'idle' && health !== 'checking' && (
            <StatusPill
              label={health === 'healthy' ? 'database ready' : 'unreachable'}
              tone={health === 'healthy' ? 'success' : 'danger'}
            />
          )}
        </View>
        {healthError ? <ErrorNotice title="Backend connection failed" message={healthError} /> : null}
      </Card>

      <Card>
        <SectionLabel>Memograph sync</SectionLabel>
        <Field
          label="Memograph API key (optional)"
          value={draft.memographApiKey}
          onChangeText={(value) => patch('memographApiKey', value)}
          placeholder="mg_live_…"
          autoCapitalize="none"
          autoCorrect={false}
          secureTextEntry
          hint="Overrides the server Memograph credential for your requests. Your app login is still required."
        />
        <Field
          label="Project ID"
          value={draft.projectId}
          onChangeText={(value) => patch('projectId', value)}
          placeholder="Used when creating a memory"
        />
        <Field
          label="Memory ID"
          value={draft.memoryId}
          onChangeText={(value) => patch('memoryId', value)}
          placeholder="Required before capture"
        />
        <Field
          label="Group ID"
          value={draft.groupId}
          onChangeText={(value) => patch('groupId', value)}
          placeholder="Optional shared web-app group"
          hint="Leave blank to let each realtime session create its own group."
        />
        <Field
          label="Location label"
          value={draft.location}
          onChangeText={(value) => patch('location', value)}
          placeholder="Studio, office, home…"
        />
      </Card>

      <Card>
        <SectionLabel>Capture quality</SectionLabel>
        <Body muted>Video quality</Body>
        <ChoiceRow
          slidable
          value={draft.videoQuality}
          onChange={(value) => patch('videoQuality', value)}
          options={[
            { label: 'Data saver · 480p', value: '480p' },
            { label: 'Balanced · 720p', value: '720p' },
            { label: 'Detail · 1080p', value: '1080p' },
          ]}
        />
        <Body muted>Upload chunk interval</Body>
        <ChoiceRow
          slidable
          value={draft.chunkDurationSeconds}
          onChange={(value) => patch('chunkDurationSeconds', value)}
          options={[
            { label: '10 sec', value: 10 },
            { label: '30 sec', value: 30 },
            { label: '60 sec', value: 60 },
          ]}
        />
        <Body muted>Visual sampling interval</Body>
        <ChoiceRow
          slidable
          value={draft.frameIntervalSeconds}
          onChange={(value) => patch('frameIntervalSeconds', value)}
          options={[
            { label: '3 sec', value: 3 },
            { label: '5 sec', value: 5 },
            { label: '10 sec', value: 10 },
          ]}
        />
      </Card>

      <Card>
        <SectionLabel>Efficiency</SectionLabel>
        <ToggleRow
          label="Upload on Wi-Fi only"
          description="Continue capturing offline or on cellular, but defer large uploads."
          value={draft.wifiOnly}
          onChange={(value) => patch('wifiOnly', value)}
        />
        <ToggleRow
          label="Respect low-power mode"
          description="Prevent new capture below 15% battery or while system low-power mode is enabled."
          value={draft.pauseOnLowBattery}
          onChange={(value) => patch('pauseOnLowBattery', value)}
        />
        <View style={[styles.powerRow, { backgroundColor: theme.backgroundElement }]}>
          <Body muted>
            Battery {power.batteryLevel >= 0 ? `${Math.round(power.batteryLevel * 100)}%` : 'unknown'}
            {power.lowPowerMode ? ' · low-power mode' : ''}
          </Body>
        </View>
      </Card>

      {saveError ? <ErrorNotice title="Preferences were not saved" message={saveError} /> : null}
      <Button label="Save preferences" onPress={() => void save()} loading={saving} />

      <Card>
        <SectionLabel>Account & privacy</SectionLabel>
        <Body>{auth?.user.email}</Body>
        <Body muted>
          Access tokens are stored in the native secure store. Buffered media is kept in this
          app&apos;s private document directory and deleted after successful upload.
        </Body>
        <Button
          label="Sign out"
          variant="danger"
          onPress={() =>
            Alert.alert('Sign out?', 'Buffered uploads remain on this device.', [
              { text: 'Cancel', style: 'cancel' },
              { text: 'Sign out', style: 'destructive', onPress: () => void logout() },
            ])
          }
        />
      </Card>
      </Screen>
      {snackbarVisible && (
        <Animated.View
          accessibilityLiveRegion="polite"
          style={[
            styles.snackbar,
            {
              backgroundColor: theme.text,
              opacity: snackbarOpacity,
              transform: [{ translateY: snackbarOffset }],
            },
          ]}>
          <Text style={[styles.snackbarText, { color: theme.background }]}>
            Your preferences have been saved.
          </Text>
        </Animated.View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1 },
  screen: { paddingBottom: 96 },
  toggleRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.lg },
  toggleCopy: { flex: 1, gap: Spacing.xs },
  toggleLabel: { fontSize: 15, fontWeight: '800' },
  inline: { flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', gap: Spacing.md },
  powerRow: { borderRadius: Radius.md, padding: Spacing.md },
  snackbar: {
    position: 'absolute',
    left: Spacing.lg,
    right: Spacing.lg,
    bottom: Spacing.lg,
    minHeight: 50,
    borderRadius: Radius.md,
    paddingHorizontal: Spacing.lg,
    justifyContent: 'center',
    shadowColor: '#000000',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.18,
    shadowRadius: 10,
    elevation: 6,
  },
  snackbarText: { fontSize: 14, fontWeight: '800', textAlign: 'center' },
});
