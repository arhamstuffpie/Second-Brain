import { useState } from 'react';
import { RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import {
  Body,
  Button,
  Card,
  ErrorNotice,
  Metric,
  PageHeader,
  SectionLabel,
  StatusPill,
} from '@/components/ui';
import { MaxContentWidth, Spacing } from '@/constants/theme';
import { getReadableError } from '@/lib/readable-error';
import { useApp } from '@/state/app-provider';
import type { QueuedVideoChunk, UploadState } from '@/types/app';
import type { VideoRecordingDetail } from '@/types/api';
import { useTheme } from '@/hooks/use-theme';

function stateTone(state: UploadState) {
  if (state === 'uploaded') return 'success' as const;
  if (state === 'failed') return 'danger' as const;
  if (state === 'uploading') return 'live' as const;
  return 'warning' as const;
}

function formatBytes(value?: number) {
  if (!value) return '—';
  if (value < 1024 * 1024) return `${Math.round(value / 1024)} KB`;
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

export function ActivityScreen() {
  const theme = useTheme();
  const {
    queue,
    network,
    capture,
    api,
    retryUpload,
    discardUpload,
    clearCompletedUploads,
    refreshRealtimeSession,
  } = useApp();
  const [refreshing, setRefreshing] = useState(false);
  const [detail, setDetail] = useState<VideoRecordingDetail | null>(null);
  const [detailError, setDetailError] = useState('');
  const pending = queue.filter((item) => item.state !== 'uploaded').length;
  const uploaded = queue.filter((item) => item.state === 'uploaded').length;
  const failed = queue.filter((item) => item.state === 'failed').length;

  async function refresh() {
    setRefreshing(true);
    setDetailError('');
    try {
      await refreshRealtimeSession();
      if (detail) {
        setDetail(await api.video.getRecording(detail.id));
      }
    } catch (error) {
      setDetailError(getReadableError(error, 'backend'));
    } finally {
      setRefreshing(false);
    }
  }

  async function showDetail(item: QueuedVideoChunk) {
    if (!item.recording?.id) return;
    setDetailError('');
    try {
      setDetail(await api.video.getRecording(item.recording.id));
    } catch (error) {
      setDetailError(getReadableError(error, 'backend'));
    }
  }

  return (
    <SafeAreaView style={[styles.safe, { backgroundColor: theme.background }]} edges={['top']}>
      <ScrollView
        contentContainerStyle={styles.scroll}
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={() => void refresh()} tintColor={theme.accent} />
        }>
        <View style={styles.inner}>
          <PageHeader
            eyebrow="DELIVERY"
            title="Capture activity"
            subtitle="Local buffer and backend processing, in one place."
            action={
              <StatusPill
                label={network.uploadAllowed ? 'syncing' : network.online ? 'waiting for Wi-Fi' : 'offline'}
                tone={network.uploadAllowed ? 'success' : 'warning'}
              />
            }
          />

          <Card>
            <View style={styles.metrics}>
              <Metric value={pending} label="to upload" />
              <Metric value={uploaded} label="delivered" />
              <Metric value={failed} label="needs action" />
              <Metric value={capture.remote?.progress.completed ?? 0} label="processed" />
            </View>
            {capture.sessionId && (
              <Body muted>
                Session {capture.sessionId.slice(0, 8)} · backend status{' '}
                {capture.remote?.status ?? 'tap refresh'}
              </Body>
            )}
          </Card>

          {detail && (
            <Card>
              <View style={styles.rowBetween}>
                <View style={styles.flex}>
                  <SectionLabel>Processing detail</SectionLabel>
                  <Text style={[styles.detailTitle, { color: theme.text }]}>
                    Chunk {(detail.chunk_index ?? 0) + 1}
                  </Text>
                </View>
                <StatusPill
                  label={detail.status}
                  tone={
                    detail.status === 'completed'
                      ? 'success'
                      : detail.status === 'failed'
                        ? 'danger'
                        : 'warning'
                  }
                />
              </View>
              <View style={styles.pipeline}>
                <Body muted>Audio · {detail.audio_status}</Body>
                <Body muted>Vision · {detail.visual_status}</Body>
                <Body muted>Merge · {detail.merge_status}</Body>
              </View>
              {detail.last_error ? (
                <ErrorNotice
                  title="Processing failed"
                  message={getReadableError(detail.last_error, 'upload')}
                />
              ) : null}
              {detail.transcript?.text ? <Body>{detail.transcript.text}</Body> : null}
              {detail.episodes.map((episode) => (
                <View
                  key={episode.id}
                  style={[styles.episode, { borderTopColor: theme.border }]}>
                  <Body>{episode.description}</Body>
                  <StatusPill
                    label={episode.status}
                    tone={episode.status === 'completed' ? 'success' : 'warning'}
                  />
                  {episode.last_error ? (
                    <ErrorNotice
                      title="Memory sync failed"
                      message={getReadableError(episode.last_error, 'memograph')}
                    />
                  ) : null}
                </View>
              ))}
              <Button label="Close detail" variant="ghost" compact onPress={() => setDetail(null)} />
            </Card>
          )}

          {detailError ? (
            <ErrorNotice title="Could not load processing details" message={detailError} />
          ) : null}

          <View style={styles.rowBetween}>
            <SectionLabel>Recent chunks</SectionLabel>
            {uploaded > 0 && (
              <Button label="Clear delivered" variant="ghost" compact onPress={clearCompletedUploads} />
            )}
          </View>

          {queue.length === 0 ? (
            <Card>
              <Text style={[styles.emptyTitle, { color: theme.text }]}>No captures yet</Text>
              <Body muted>Your first chunk will appear here as soon as recording begins.</Body>
            </Card>
          ) : (
            queue.map((item) => (
              <Card key={item.id}>
                <View style={styles.rowBetween}>
                  <View style={styles.flex}>
                    <Text style={[styles.itemTitle, { color: theme.text }]}>
                      {item.isFinal ? 'Final chunk' : 'Video + audio chunk'}
                    </Text>
                    <Body muted>
                      {new Date(item.createdAt).toLocaleTimeString([], {
                        hour: '2-digit',
                        minute: '2-digit',
                      })}
                      {' · '}
                      {formatBytes(item.recording?.size_bytes)}
                    </Body>
                  </View>
                  <StatusPill label={item.state} tone={stateTone(item.state)} />
                </View>
                {item.lastError ? (
                  <ErrorNotice title="Upload failed" message={item.lastError} />
                ) : null}
                <View style={styles.actions}>
                  {item.state === 'uploaded' && item.recording && (
                    <Button
                      label="Processing detail"
                      variant="secondary"
                      compact
                      onPress={() => void showDetail(item)}
                    />
                  )}
                  {item.state === 'failed' && (
                    <>
                      <Button label="Retry" compact onPress={() => retryUpload(item.id)} />
                      <Button
                        label="Discard"
                        compact
                        variant="danger"
                        onPress={() => discardUpload(item.id)}
                      />
                    </>
                  )}
                </View>
              </Card>
            ))
          )}
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1 },
  scroll: { paddingHorizontal: Spacing.lg, paddingTop: Spacing.xl, paddingBottom: 118 },
  inner: { width: '100%', maxWidth: MaxContentWidth, alignSelf: 'center', gap: Spacing.xl },
  metrics: { flexDirection: 'row', justifyContent: 'space-between', flexWrap: 'wrap', gap: Spacing.lg },
  rowBetween: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', gap: Spacing.lg },
  flex: { flex: 1, gap: Spacing.xs },
  itemTitle: { fontSize: 16, fontWeight: '800' },
  detailTitle: { fontSize: 22, fontWeight: '800' },
  pipeline: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.lg },
  episode: { borderTopWidth: StyleSheet.hairlineWidth, paddingTop: Spacing.lg, gap: Spacing.md },
  actions: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.sm },
  emptyTitle: { fontSize: 20, fontWeight: '800' },
});
