import {
  RecordingPresets,
  requestRecordingPermissionsAsync,
  setAudioModeAsync,
  useAudioRecorder,
  useAudioRecorderState,
} from 'expo-audio';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Linking, Platform, StyleSheet, Text, View } from 'react-native';

import { Body, Button, ErrorNotice } from '@/components/ui';
import { Fonts, Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';
import { getReadableError } from '@/lib/readable-error';
import { useApp } from '@/state/app-provider';
import type { UploadFile } from '@/types/api';

const MINIMUM_DURATION_MS = 2_000;
const AUTOMATIC_STOP_MS = 9_000;
const VOICE_PROMPT = [
  'Memory One, learn my voice.',
  'Separate my words from other speakers.',
  'Keep the context that matters.',
  'Help me remember my thoughts later.',
].join('\n');
const recordingOptions = {
  ...RecordingPresets.HIGH_QUALITY,
  numberOfChannels: 1,
  bitRate: 96_000,
};

type SampleState = 'idle' | 'recording' | 'ready' | 'uploading';

type VoiceSampleRecorderProps = {
  onSubmit: (file: UploadFile) => Promise<void>;
  submitLabel?: string;
  onComplete?: () => void;
  onCancel?: () => void;
};

function seconds(durationMillis: number) {
  return (durationMillis / 1000).toFixed(1);
}

function resetRecordingAudioMode() {
  return setAudioModeAsync({ allowsRecording: false }).catch(() => undefined);
}

export function VoiceSampleRecorder({
  onSubmit,
  submitLabel = 'Use this sample',
  onComplete,
  onCancel,
}: VoiceSampleRecorderProps) {
  const theme = useTheme();
  const { showError } = useApp();
  const recorder = useAudioRecorder(recordingOptions);
  const recorderState = useAudioRecorderState(recorder, 100);
  const [sampleState, setSampleState] = useState<SampleState>('idle');
  const [sampleUri, setSampleUri] = useState<string | null>(null);
  const [sampleDuration, setSampleDuration] = useState(0);
  const [permissionBlocked, setPermissionBlocked] = useState(false);
  const [error, setError] = useState('');
  const stopping = useRef(false);

  const stopRecording = useCallback(async () => {
    if (stopping.current || sampleState !== 'recording') return;
    stopping.current = true;
    try {
      const duration = recorder.getStatus().durationMillis;
      await recorder.stop();
      const uri = recorder.uri;
      void resetRecordingAudioMode();
      if (!uri || duration < MINIMUM_DURATION_MS) {
        setSampleState('idle');
        setSampleUri(null);
        setError('That sample was too short. Please speak for at least 2 seconds.');
        return;
      }
      setSampleDuration(duration);
      setSampleUri(uri);
      setSampleState('ready');
    } catch (cause) {
      const message = getReadableError(cause, 'capture');
      setError(message);
      showError(message);
      setSampleState('idle');
      void resetRecordingAudioMode();
    } finally {
      stopping.current = false;
    }
  }, [recorder, sampleState, showError]);

  useEffect(() => {
    if (sampleState === 'recording' && recorderState.durationMillis >= AUTOMATIC_STOP_MS) {
      void stopRecording();
    }
  }, [recorderState.durationMillis, sampleState, stopRecording]);

  useEffect(
    () => () => {
      // useAudioRecorder owns and releases its native shared object during
      // unmount. Touching the recorder here can race that release on Android.
      void resetRecordingAudioMode();
    },
    [],
  );

  async function startRecording() {
    setError('');
    setPermissionBlocked(false);
    setSampleUri(null);
    try {
      const permission = await requestRecordingPermissionsAsync();
      if (!permission.granted) {
        setPermissionBlocked(!permission.canAskAgain);
        setError('Microphone access is required to create your owner voice sample.');
        return;
      }
      await setAudioModeAsync({ allowsRecording: true, playsInSilentMode: true });
      await recorder.prepareToRecordAsync();
      recorder.record();
      setSampleState('recording');
    } catch (cause) {
      const message = getReadableError(cause, 'capture');
      setError(message);
      showError(message);
    }
  }

  async function uploadSample() {
    if (!sampleUri) return;
    setError('');
    setSampleState('uploading');
    const web = Platform.OS === 'web';
    try {
      await onSubmit({
        uri: sampleUri,
        name: web ? 'owner-voice.webm' : 'owner-voice.m4a',
        type: web ? 'audio/webm' : 'audio/mp4',
      });
      onComplete?.();
    } catch (cause) {
      const message = getReadableError(cause, 'upload');
      setError(message);
      showError(message);
      setSampleState('ready');
    }
  }

  const displayedDuration =
    sampleState === 'recording' ? recorderState.durationMillis : sampleDuration;

  return (
    <View style={styles.root}>
      <View style={styles.prompt}>
        <Text style={[styles.promptLabel, { color: theme.textSecondary }]}>READ THIS ALOUD</Text>
        <Text style={[styles.phrase, { color: theme.text }]}>{VOICE_PROMPT}</Text>
      </View>

      <View
        accessibilityLabel={
          sampleState === 'recording'
            ? `Recording, ${seconds(displayedDuration)} seconds`
            : 'Voice sample recorder'
        }
        accessibilityLiveRegion="polite"
        style={styles.recorderRow}>
        <View
          style={[
            styles.micOrb,
            {
              backgroundColor:
                sampleState === 'recording' ? theme.dangerSoft : theme.backgroundElement,
              borderColor: sampleState === 'recording' ? theme.danger : theme.border,
            },
          ]}>
          <View
            style={[
              styles.micCore,
              {
                backgroundColor:
                  sampleState === 'recording' ? theme.danger : theme.textSecondary,
              },
            ]}
          />
        </View>
        <View style={styles.recorderCopy}>
          <Text style={[styles.timer, { color: theme.text }]}>{seconds(displayedDuration)}s</Text>
          <Body muted>
            {sampleState === 'recording'
              ? 'Speak naturally in a quiet place'
              : sampleState === 'ready' || sampleState === 'uploading'
                ? 'Sample ready to save'
                : 'Aim for 6–9 seconds of clear speech'}
          </Body>
        </View>
      </View>

      {error ? <ErrorNotice title="Voice sample not ready" message={error} /> : null}
      {permissionBlocked ? (
        <Button
          label="Open device settings"
          variant="secondary"
          onPress={() => void Linking.openSettings()}
        />
      ) : null}

      {sampleState === 'idle' ? (
        <View style={styles.actions}>
          <Button label="Record my voice" onPress={() => void startRecording()} />
          {onCancel ? <Button label="Cancel" variant="ghost" onPress={onCancel} /> : null}
        </View>
      ) : sampleState === 'recording' ? (
        <Button
          label={displayedDuration < MINIMUM_DURATION_MS ? 'Keep speaking…' : 'Stop recording'}
          disabled={displayedDuration < MINIMUM_DURATION_MS}
          variant="danger"
          onPress={() => void stopRecording()}
        />
      ) : (
        <View style={styles.actions}>
          <Button
            label={submitLabel}
            loading={sampleState === 'uploading'}
            onPress={() => void uploadSample()}
          />
          <Button
            label="Record again"
            disabled={sampleState === 'uploading'}
            variant="secondary"
            onPress={() => {
              setSampleState('idle');
              setSampleUri(null);
              setSampleDuration(0);
            }}
          />
          {onCancel ? (
            <Button
              label="Cancel"
              disabled={sampleState === 'uploading'}
              variant="ghost"
              onPress={onCancel}
            />
          ) : null}
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { gap: Spacing.lg },
  prompt: { gap: Spacing.sm },
  promptLabel: { fontFamily: Fonts.mono, fontSize: 10, fontWeight: '900', letterSpacing: 1.4 },
  phrase: { fontSize: 17, lineHeight: 26, fontWeight: '700' },
  recorderRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.lg },
  micOrb: {
    width: 64,
    height: 64,
    borderRadius: Radius.pill,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  micCore: { width: 18, height: 28, borderRadius: 9 },
  recorderCopy: { flex: 1, gap: 2 },
  timer: { fontFamily: Fonts.mono, fontSize: 24, fontWeight: '900' },
  actions: { gap: Spacing.sm },
});
