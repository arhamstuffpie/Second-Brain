import { useEffect, useState } from 'react';
import { Alert, StyleSheet, Switch, Text, View } from 'react-native';

import {
  Body,
  Button,
  Card,
  ChoiceRow,
  Field,
  PageHeader,
  Screen,
  SectionLabel,
  StatusPill,
} from '@/components/ui';
import { Radius, Spacing } from '@/constants/theme';
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
  const { settings, auth, network, power, updateSettings, checkHealth, logout } = useApp();
  const [draft, setDraft] = useState<AppSettings>(settings);
  const [saving, setSaving] = useState(false);
  const [health, setHealth] = useState<'idle' | 'checking' | 'healthy' | 'failed'>('idle');

  useEffect(() => setDraft(settings), [settings]);

  function patch<T extends keyof AppSettings>(key: T, value: AppSettings[T]) {
    setDraft((current) => ({ ...current, [key]: value }));
  }

  async function save() {
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
        projectId: draft.projectId.trim(),
        memoryId: draft.memoryId.trim(),
        groupId: draft.groupId.trim(),
        location: draft.location.trim(),
      });
    } finally {
      setSaving(false);
    }
  }

  async function testBackend() {
    setHealth('checking');
    try {
      await checkHealth();
      setHealth('healthy');
    } catch {
      setHealth('failed');
    }
  }

  return (
    <Screen contentStyle={styles.screen}>
      <PageHeader
        eyebrow="PREFERENCES"
        title="Collection controls"
        subtitle="Tune capture quality, delivery cost, and the Memograph destination."
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
      </Card>

      <Card>
        <SectionLabel>Memograph sync</SectionLabel>
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
  );
}

const styles = StyleSheet.create({
  screen: { paddingBottom: 96 },
  toggleRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.lg },
  toggleCopy: { flex: 1, gap: Spacing.xs },
  toggleLabel: { fontSize: 15, fontWeight: '800' },
  inline: { flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', gap: Spacing.md },
  powerRow: { borderRadius: Radius.md, padding: Spacing.md },
});
