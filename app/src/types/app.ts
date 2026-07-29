import type { VideoQuality } from 'expo-camera';

import type { AuthResult, RealtimeVideoSessionDetail, VideoRecording } from '@/types/api';

export type AuthSession = AuthResult;

export type AppSettings = {
  apiBaseUrl: string;
  memographApiKey: string;
  projectId: string;
  memoryId: string;
  groupId: string;
  deviceId: string;
  location: string;
  chunkDurationSeconds: 10 | 30 | 60;
  frameIntervalSeconds: 3 | 5 | 10;
  videoQuality: Extract<VideoQuality, '480p' | '720p' | '1080p'>;
  wifiOnly: boolean;
  pauseOnLowBattery: boolean;
  lowBatteryThreshold: number;
};

export type UploadState = 'pending' | 'uploading' | 'retrying' | 'uploaded' | 'failed';

export type QueuedVideoChunk = {
  id: string;
  sessionId: string;
  chunkId: string;
  fileUri: string;
  fileName: string;
  mediaType: 'video/mp4';
  isFinal: boolean;
  createdAt: string;
  state: UploadState;
  attempts: number;
  nextAttemptAt: number;
  recording?: VideoRecording;
  lastError?: string;
};

export type CapturePhase = 'idle' | 'starting' | 'capturing' | 'stopping' | 'error';

export type CaptureSnapshot = {
  phase: CapturePhase;
  sessionId?: string;
  startedAt?: number;
  error?: string;
  remote?: RealtimeVideoSessionDetail;
};
