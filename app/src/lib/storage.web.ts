import AsyncStorage from '@react-native-async-storage/async-storage';

import type { AppSettings, AuthSession, QueuedVideoChunk } from '@/types/app';

const AUTH_KEY = 'second-brain.auth.v1';
const SETTINGS_KEY = 'second-brain.settings.v1';
const QUEUE_KEY = 'second-brain.upload-queue.v1';
let sessionMemographAPIKey = '';

export const defaultSettings: AppSettings = {
  apiBaseUrl: process.env.EXPO_PUBLIC_API_BASE_URL ?? 'http://localhost:8181',
  memographApiKey: '',
  projectId: '',
  memoryId: '',
  groupId: '',
  deviceId: '',
  location: '',
  chunkDurationSeconds: 30,
  frameIntervalSeconds: 5,
  videoQuality: '720p',
  wifiOnly: true,
  pauseOnLowBattery: true,
  lowBatteryThreshold: 0.15,
};

function parseJSON<T>(value: string | null, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

export async function loadAuthSession() {
  return parseJSON<AuthSession | null>(await AsyncStorage.getItem(AUTH_KEY), null);
}

export async function saveAuthSession(session: AuthSession | null) {
  if (session) {
    await AsyncStorage.setItem(AUTH_KEY, JSON.stringify(session));
  } else {
    await AsyncStorage.removeItem(AUTH_KEY);
  }
}

export async function loadSettings() {
  const stored = parseJSON<Partial<AppSettings>>(await AsyncStorage.getItem(SETTINGS_KEY), {});
  return { ...defaultSettings, ...stored, memographApiKey: sessionMemographAPIKey };
}

export async function saveSettings(settings: AppSettings) {
  const { memographApiKey, ...nonSensitiveSettings } = settings;
  sessionMemographAPIKey = memographApiKey;
  await AsyncStorage.setItem(SETTINGS_KEY, JSON.stringify(nonSensitiveSettings));
}

export async function loadUploadQueue() {
  return parseJSON<QueuedVideoChunk[]>(await AsyncStorage.getItem(QUEUE_KEY), []).filter(
    (item) => item.state === 'uploaded',
  );
}

export async function saveUploadQueue(queue: QueuedVideoChunk[]) {
  await AsyncStorage.setItem(QUEUE_KEY, JSON.stringify(queue));
}

export function persistCapturedVideo() {
  throw new Error('Durable media capture is available only on iOS and Android.');
}

export function deleteQueuedFile() {
  // Web capture is disabled, so there is no local media file to remove.
}
