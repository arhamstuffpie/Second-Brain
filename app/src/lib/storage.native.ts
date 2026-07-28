import AsyncStorage from '@react-native-async-storage/async-storage';
import { Directory, File, Paths } from 'expo-file-system';
import * as SecureStore from 'expo-secure-store';
import { Platform } from 'react-native';

// Native-only durable storage. The web fallback deliberately never touches
// expo-file-system because it is unavailable during static rendering.

import type { AppSettings, AuthSession, QueuedVideoChunk } from '@/types/app';

const AUTH_KEY = 'second-brain.auth.v1';
const SETTINGS_KEY = 'second-brain.settings.v1';
const QUEUE_KEY = 'second-brain.upload-queue.v1';
const QUEUE_DIRECTORY = new Directory(Paths.document, 'second-brain-upload-queue');

export const defaultSettings: AppSettings = {
  apiBaseUrl: process.env.EXPO_PUBLIC_API_BASE_URL ?? 'http://localhost:8181',
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

async function secureStoreAvailable() {
  return Platform.OS !== 'web' && SecureStore.isAvailableAsync();
}

export async function loadAuthSession() {
  const value = (await secureStoreAvailable())
    ? await SecureStore.getItemAsync(AUTH_KEY)
    : await AsyncStorage.getItem(AUTH_KEY);
  return parseJSON<AuthSession | null>(value, null);
}

export async function saveAuthSession(session: AuthSession | null) {
  if (await secureStoreAvailable()) {
    if (session) {
      await SecureStore.setItemAsync(AUTH_KEY, JSON.stringify(session), {
        keychainAccessible: SecureStore.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
      });
    } else {
      await SecureStore.deleteItemAsync(AUTH_KEY);
    }
    return;
  }
  if (session) {
    await AsyncStorage.setItem(AUTH_KEY, JSON.stringify(session));
  } else {
    await AsyncStorage.removeItem(AUTH_KEY);
  }
}

export async function loadSettings() {
  const stored = parseJSON<Partial<AppSettings>>(await AsyncStorage.getItem(SETTINGS_KEY), {});
  return { ...defaultSettings, ...stored };
}

export async function saveSettings(settings: AppSettings) {
  await AsyncStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
}

export async function loadUploadQueue() {
  const queue = parseJSON<QueuedVideoChunk[]>(await AsyncStorage.getItem(QUEUE_KEY), []);
  return queue
    .filter((item) => item.state === 'uploaded' || new File(item.fileUri).exists)
    .map((item) => (item.state === 'uploading' ? { ...item, state: 'retrying' as const } : item));
}

export async function saveUploadQueue(queue: QueuedVideoChunk[]) {
  await AsyncStorage.setItem(QUEUE_KEY, JSON.stringify(queue));
}

export function persistCapturedVideo(cacheUri: string, fileName: string) {
  QUEUE_DIRECTORY.create({ idempotent: true, intermediates: true });
  const source = new File(cacheUri);
  const destination = new File(QUEUE_DIRECTORY, fileName);
  source.move(destination);
  return destination.uri;
}

export function deleteQueuedFile(fileUri: string) {
  if (!fileUri) return;
  const file = new File(fileUri);
  if (file.exists) {
    file.delete();
  }
}
