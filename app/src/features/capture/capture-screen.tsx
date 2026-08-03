import {
  CameraView,
  type CameraType,
  useCameraPermissions,
  useMicrophonePermissions,
} from 'expo-camera';
import { randomUUID } from 'expo-crypto';
import { activateKeepAwakeAsync, deactivateKeepAwake } from 'expo-keep-awake';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Animated,
  AppState,
  Platform,
  Pressable,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Body, BrandMark, Button, Card, ErrorNotice, Metric, StatusPill } from '@/components/ui';
import { Fonts, Radius, Spacing } from '@/constants/theme';
import { deleteQueuedFile, persistCapturedVideo } from '@/lib/storage';
import { getReadableError } from '@/lib/readable-error';
import { useRecordingSoundCues } from '@/lib/recording-cues';
import { useReducedMotion } from '@/hooks/use-reduced-motion';
import { useApp } from '@/state/app-provider';
import type { CapturePhase, QueuedVideoChunk } from '@/types/app';
import { useTheme } from '@/hooks/use-theme';

const KEEP_AWAKE_TAG = 'second-brain-foreground-capture';

function formatElapsed(seconds: number) {
  const minutes = Math.floor(seconds / 60);
  return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`;
}

export function CaptureScreen() {
  const theme = useTheme();
  const insets = useSafeAreaInsets();
  const reducedMotion = useReducedMotion();
  const {
    auth,
    api,
    settings,
    network,
    power,
    queue,
    capture,
    setCapture,
    enqueueVideoChunk,
    showError,
  } = useApp();
  const [cameraPermission, requestCameraPermission] = useCameraPermissions();
  const [microphonePermission, requestMicrophonePermission] = useMicrophonePermissions();
  const [cameraReady, setCameraReady] = useState(false);
  const [facing, setFacing] = useState<CameraType>('back');
  const [elapsed, setElapsed] = useState(0);
  const { playStarted, playStopped } = useRecordingSoundCues();
  const cameraRef = useRef<CameraView>(null);
  const shouldCapture = useRef(false);
  const stopRequested = useRef(false);
  const runningLoop = useRef(false);
  const pulse = useRef(new Animated.Value(0.45)).current;

  const capturing = capture.phase === 'capturing';
  const busy = capture.phase === 'starting' || capture.phase === 'stopping';
  const pendingCount = queue.filter((item) => item.state !== 'uploaded').length;

  useEffect(() => {
    if (capture.error) showError(capture.error);
  }, [capture.error, showError]);

  useEffect(() => {
    if (!capturing) {
      pulse.setValue(0.45);
      return;
    }
    if (reducedMotion) {
      pulse.setValue(1);
      return;
    }
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, { toValue: 1, duration: 900, useNativeDriver: true }),
        Animated.timing(pulse, { toValue: 0.45, duration: 900, useNativeDriver: true }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [capturing, pulse, reducedMotion]);

  useEffect(() => {
    if (!capture.startedAt || capture.phase === 'idle') {
      setElapsed(0);
      return;
    }
    const update = () => setElapsed(Math.floor((Date.now() - capture.startedAt!) / 1000));
    update();
    const interval = setInterval(update, 1000);
    return () => clearInterval(interval);
  }, [capture.phase, capture.startedAt]);

  const stopCapture = useCallback(() => {
    if (!shouldCapture.current || capture.phase === 'stopping') return;
    stopRequested.current = true;
    shouldCapture.current = false;
    setCapture({ ...capture, phase: 'stopping' });
    cameraRef.current?.stopRecording();
  }, [capture, setCapture]);

  useEffect(() => {
    const subscription = AppState.addEventListener('change', (state) => {
      if (state !== 'active' && shouldCapture.current) {
        stopCapture();
      }
    });
    return () => subscription.remove();
  }, [stopCapture]);

  useEffect(
    () => () => {
      if (shouldCapture.current) {
        shouldCapture.current = false;
        stopRequested.current = true;
        cameraRef.current?.stopRecording();
      }
      void deactivateKeepAwake(KEEP_AWAKE_TAG);
    },
    [],
  );

  async function requestPermissions() {
    await Promise.all([requestCameraPermission(), requestMicrophonePermission()]);
  }

  async function runCaptureLoop(sessionId: string) {
    if (runningLoop.current) return;
    runningLoop.current = true;
    let finalChunkQueued = false;
    let loopError: string | undefined;
    try {
      while (shouldCapture.current) {
        const result = await cameraRef.current?.recordAsync({
          maxDuration: settings.chunkDurationSeconds,
        });
        if (!result?.uri) break;

        const isFinal = stopRequested.current || !shouldCapture.current;
        const chunkId = randomUUID();
        const fileName = `${sessionId}-${chunkId}.mp4`;
        const fileUri = persistCapturedVideo(result.uri, fileName);
        const item: QueuedVideoChunk = {
          id: chunkId,
          ownerUserId: auth!.user.id,
          sessionId,
          chunkId,
          fileUri,
          fileName,
          mediaType: 'video/mp4',
          isFinal,
          createdAt: new Date().toISOString(),
          state: 'pending',
          attempts: 0,
          nextAttemptAt: Date.now(),
        };
        try {
          await enqueueVideoChunk(item);
        } catch (error) {
          deleteQueuedFile(fileUri);
          throw error;
        }
        if (isFinal) {
          finalChunkQueued = true;
          break;
        }
      }
    } catch (error) {
      shouldCapture.current = false;
      loopError = getReadableError(error, 'capture');
      setCapture({ phase: 'error', sessionId, error: loopError });
    } finally {
      runningLoop.current = false;
      shouldCapture.current = false;
      await deactivateKeepAwake(KEEP_AWAKE_TAG);
      if (!finalChunkQueued) {
        try {
          await api.video.stopRealtimeSession(sessionId);
        } catch {
          // A session stop is best-effort here; the backend may already be offline.
        }
      }
      if (!loopError) {
        if (stopRequested.current && AppState.currentState === 'active') {
          playStopped();
        }
        setCapture({ phase: 'idle', sessionId });
      }
    }
  }

  async function startCapture() {
    if (!settings.memoryId.trim()) {
      setCapture({ phase: 'error', error: 'Add a Memograph memory ID in Settings first.' });
      return;
    }
    if (!network.online) {
      setCapture({ phase: 'error', error: 'Connect once to create a capture session.' });
      return;
    }
    if (!power.captureAllowed) {
      setCapture({
        phase: 'error',
        error: 'Capture is paused by the low-battery preference. Charge the device or change Settings.',
      });
      return;
    }
    if (!cameraReady) {
      setCapture({ phase: 'error', error: 'Camera is still preparing. Try again in a moment.' });
      return;
    }

    setCapture({ phase: 'starting' });
    try {
      const session = await api.video.startRealtimeSession({
        memory_id: settings.memoryId.trim(),
        group_id: settings.groupId.trim() || undefined,
        device_id: settings.deviceId.trim() || undefined,
        location: settings.location.trim() || undefined,
        chunk_duration_seconds: settings.chunkDurationSeconds,
        frame_interval_seconds: settings.frameIntervalSeconds,
      });
      stopRequested.current = false;
      shouldCapture.current = true;
      const startedAt = Date.now();
      setCapture({ phase: 'capturing', sessionId: session.id, startedAt });
      await activateKeepAwakeAsync(KEEP_AWAKE_TAG);
      playStarted();
      void runCaptureLoop(session.id);
    } catch (error) {
      setCapture({
        phase: 'error',
        error: getReadableError(error, 'capture'),
      });
    }
  }

  const hasPermissions = cameraPermission?.granted && microphonePermission?.granted;

  if (Platform.OS === 'web') {
    return (
      <View style={styles.fallback}>
        <BrandMark />
        <Card>
          <StatusPill label="Mobile only" tone="warning" />
          <Text style={[styles.fallbackTitle, { color: theme.text }]}>Open on iOS or Android</Text>
          <Body muted>
            Foreground chunk recording and durable offline buffering are intentionally enabled only
            in the native mobile build.
          </Body>
        </Card>
      </View>
    );
  }

  if (!hasPermissions) {
    return (
      <View style={styles.fallback}>
        <BrandMark />
        <Card>
          <StatusPill label="Permission needed" tone="warning" />
          <Text style={[styles.fallbackTitle, { color: theme.text }]}>Camera + microphone</Text>
          <Body muted>
            Both permissions are required because each foreground video chunk contains its matching
            audio track. Nothing is captured until you press Start.
          </Body>
          <Button label="Allow capture access" onPress={() => void requestPermissions()} />
          {(cameraPermission?.canAskAgain === false ||
            microphonePermission?.canAskAgain === false) && (
            <Body muted>Permission is blocked. Enable it from the device Settings app.</Body>
          )}
        </Card>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.background }]}>
      <CameraView
        ref={cameraRef}
        style={StyleSheet.absoluteFill}
        facing={facing}
        mode="video"
        mute={false}
        videoQuality={settings.videoQuality}
        videoBitrate={{ '480p': 800_000, '720p': 1_800_000, '1080p': 3_500_000 }[settings.videoQuality]}
        onCameraReady={() => setCameraReady(true)}
        onMountError={({ message }) => setCapture({ phase: 'error', error: message })}
      />

      <View style={[styles.topBar, { top: insets.top + Spacing.md }]}>
        <BrandMark compact />
        <View style={styles.topPills}>
          <StatusPill
            label={network.uploadAllowed ? 'uplink ready' : network.online ? 'buffering' : 'offline'}
            tone={network.uploadAllowed ? 'success' : 'warning'}
          />
          {capturing && (
            <Animated.View style={{ opacity: pulse }}>
              <StatusPill label="live" tone="live" />
            </Animated.View>
          )}
        </View>
      </View>

      <View style={[styles.previewActions, { top: insets.top + 68 }]}>
        <Pressable
          accessibilityLabel="Switch camera"
          accessibilityRole="button"
          disabled={capturing || busy}
          onPress={() => setFacing((current) => (current === 'back' ? 'front' : 'back'))}
          style={({ pressed }) => [
            styles.roundAction,
            { backgroundColor: 'rgba(15,16,14,0.62)' },
            pressed && styles.pressed,
            (capturing || busy) && styles.disabled,
          ]}>
          <Text style={styles.roundActionText}>↻</Text>
        </Pressable>
      </View>

      <View
        style={[
          styles.controlDock,
          { backgroundColor: theme.surface, borderColor: theme.border },
        ]}>
        <View style={styles.metrics}>
          <Metric value={formatElapsed(elapsed)} label="elapsed" />
          <Metric value={`${settings.chunkDurationSeconds}s`} label="chunks" />
          <Metric value={pendingCount} label="buffered" />
        </View>

        {capture.error ? <ErrorNotice title="Capture unavailable" message={capture.error} /> : null}

        <View style={styles.captureRow}>
          <View style={styles.captureCopy}>
            <Text style={[styles.captureTitle, { color: theme.text }]}>
              {capturing ? 'Capturing your timeline' : busy ? 'Finalizing safely' : 'Ready to remember'}
            </Text>
            <Text style={[styles.captureSub, { color: theme.textSecondary }]}>
              {capturing
                ? network.uploadAllowed
                  ? 'Uploading while the next chunk records'
                  : 'Saved locally until your preferred network returns'
                : `${settings.videoQuality} · audio on · foreground only`}
            </Text>
          </View>
          <Pressable
            accessibilityLabel={capturing ? 'Stop capture' : 'Start capture'}
            accessibilityRole="button"
            disabled={busy}
            onPress={capturing ? stopCapture : () => void startCapture()}
            style={({ pressed }) => [
              styles.captureButton,
              {
                backgroundColor: capturing ? theme.surface : theme.accent,
                borderColor: capturing ? theme.accent : theme.accent,
              },
              pressed && styles.pressed,
              busy && styles.disabled,
            ]}>
            <View
              style={[
                capturing ? styles.stopIcon : styles.recordIcon,
                { backgroundColor: capturing ? theme.accent : theme.onAccent },
              ]}
            />
          </Pressable>
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, overflow: 'hidden' },
  fallback: { flex: 1, justifyContent: 'center', padding: Spacing.xl, gap: Spacing.xl },
  fallbackTitle: { fontFamily: Fonts.rounded, fontSize: 28, fontWeight: '800' },
  topBar: {
    position: 'absolute',
    left: Spacing.lg,
    right: Spacing.lg,
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  topPills: { flexDirection: 'row', gap: Spacing.sm },
  previewActions: { position: 'absolute', right: Spacing.lg },
  roundAction: {
    width: 48,
    height: 48,
    borderRadius: Radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
  },
  roundActionText: { color: '#FFFFFF', fontSize: 23, lineHeight: 25, fontWeight: '500' },
  controlDock: {
    position: 'absolute',
    left: Spacing.md,
    right: Spacing.md,
    bottom: Spacing.md,
    borderRadius: Radius.lg,
    borderWidth: StyleSheet.hairlineWidth,
    padding: Spacing.lg,
    gap: Spacing.lg,
  },
  metrics: { flexDirection: 'row', justifyContent: 'space-between', gap: Spacing.lg },
  captureRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.lg },
  captureCopy: { flex: 1, gap: 4 },
  captureTitle: { fontFamily: Fonts.rounded, fontSize: 18, fontWeight: '800' },
  captureSub: { fontSize: 12, lineHeight: 17 },
  captureButton: {
    width: 64,
    height: 64,
    borderRadius: 32,
    borderWidth: 3,
    alignItems: 'center',
    justifyContent: 'center',
  },
  recordIcon: { width: 24, height: 24, borderRadius: 12 },
  stopIcon: { width: 21, height: 21, borderRadius: 5 },
  pressed: { opacity: 0.72, transform: [{ scale: 0.97 }] },
  disabled: { opacity: 0.45 },
});
