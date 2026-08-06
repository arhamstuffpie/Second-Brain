import AsyncStorage from '@react-native-async-storage/async-storage';
import { Directory, File, Paths } from 'expo-file-system';
import * as SecureStore from 'expo-secure-store';
import { Platform } from 'react-native';

// Native-only durable storage. The web fallback deliberately never touches
// expo-file-system because it is unavailable during static rendering.

import type { AppSettings, AuthSession, QueuedVideoChunk } from '@/types/app';

const AUTH_KEY = 'second-brain.auth.v1';
const MEMOGRAPH_API_KEY = 'second-brain.memograph-api-key.v1';
const SETTINGS_KEY = 'second-brain.settings.v1';
const DEVICE_SETTINGS_KEY = 'second-brain.device-settings.v1';
const QUEUE_KEY = 'second-brain.upload-queue.v1';
const VOICE_ONBOARDING_KEY = 'second-brain.voice-onboarding.v1';
const QUEUE_DIRECTORY = new Directory(Paths.document, 'second-brain-upload-queue');

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

export async function loadVoiceOnboardingRequired(userId: string) {
  return (await AsyncStorage.getItem(`${VOICE_ONBOARDING_KEY}:${userId}`)) === 'true';
}

export async function saveVoiceOnboardingRequired(userId: string, required: boolean) {
  const key = `${VOICE_ONBOARDING_KEY}:${userId}`;
  if (required) {
    await AsyncStorage.setItem(key, 'true');
  } else {
    await AsyncStorage.removeItem(key);
  }
}

function userSettingsKey(userId: string) {
  return `${SETTINGS_KEY}:${userId}`;
}

function userMemographAPIKey(userId: string) {
  return `${MEMOGRAPH_API_KEY}.${userId.replace(/[^a-zA-Z0-9._-]/g, '_')}`;
}

function deviceSettings(settings: Partial<AppSettings>) {
  return {
    apiBaseUrl: settings.apiBaseUrl ?? defaultSettings.apiBaseUrl,
    deviceId: settings.deviceId ?? defaultSettings.deviceId,
  };
}

export async function loadSettings(userId?: string, migrateLegacy = false) {
  const device = parseJSON<Partial<AppSettings>>(
    await AsyncStorage.getItem(DEVICE_SETTINGS_KEY),
    {},
  );
  if (!userId) return { ...defaultSettings, ...deviceSettings(device) };

  const scopedKey = userSettingsKey(userId);
  let scopedValue = await AsyncStorage.getItem(scopedKey);
  if (!scopedValue && migrateLegacy) {
    const legacyValue = await AsyncStorage.getItem(SETTINGS_KEY);
    if (legacyValue) {
      const legacy = parseJSON<Partial<AppSettings>>(legacyValue, {});
      const legacyApiKey = (await secureStoreAvailable())
        ? await SecureStore.getItemAsync(MEMOGRAPH_API_KEY)
        : undefined;
      const migrated = {
        ...defaultSettings,
        ...deviceSettings(device),
        ...legacy,
        memographApiKey: legacyApiKey ?? '',
      };
      await saveSettings(userId, migrated);
      await AsyncStorage.removeItem(SETTINGS_KEY);
      if (await secureStoreAvailable()) await SecureStore.deleteItemAsync(MEMOGRAPH_API_KEY);
      return migrated;
    }
  }

  const stored = parseJSON<Partial<AppSettings>>(scopedValue, {});
  const securedApiKey = (await secureStoreAvailable())
    ? await SecureStore.getItemAsync(userMemographAPIKey(userId))
    : undefined;
  return {
    ...defaultSettings,
    ...deviceSettings(device),
    ...stored,
    memographApiKey: securedApiKey ?? '',
  };
}

export async function quarantineLegacySettings() {
  const legacyValue = await AsyncStorage.getItem(SETTINGS_KEY);
  if (!legacyValue) return;
  const legacy = parseJSON<Partial<AppSettings>>(legacyValue, {});
  const currentDevice = parseJSON<Partial<AppSettings>>(
    await AsyncStorage.getItem(DEVICE_SETTINGS_KEY),
    {},
  );
  await AsyncStorage.setItem(
    DEVICE_SETTINGS_KEY,
    JSON.stringify(deviceSettings({ ...legacy, ...currentDevice })),
  );
  await AsyncStorage.setItem(`${SETTINGS_KEY}:quarantined`, legacyValue);
  await AsyncStorage.removeItem(SETTINGS_KEY);
}

export async function saveSettings(userId: string | null, settings: AppSettings) {
  const { memographApiKey, ...nonSensitiveSettings } = settings;
  await AsyncStorage.setItem(DEVICE_SETTINGS_KEY, JSON.stringify(deviceSettings(settings)));
  if (!userId) return;

  await AsyncStorage.setItem(userSettingsKey(userId), JSON.stringify(nonSensitiveSettings));
  if (await secureStoreAvailable()) {
    if (memographApiKey) {
      await SecureStore.setItemAsync(userMemographAPIKey(userId), memographApiKey, {
        keychainAccessible: SecureStore.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
      });
    } else {
      await SecureStore.deleteItemAsync(userMemographAPIKey(userId));
    }
  }
}

function userQueueKey(userId: string) {
  return `${QUEUE_KEY}:${userId}`;
}

function validQueue(queue: QueuedVideoChunk[], userId: string) {
  return queue
    .filter(
      (item) =>
        (!item.ownerUserId || item.ownerUserId === userId) &&
        (item.state === 'uploaded' || new File(item.fileUri).exists),
    )
    .map((item) => ({ ...item, ownerUserId: userId }))
    .map((item) => (item.state === 'uploading' ? { ...item, state: 'retrying' as const } : item));
}

export async function loadUploadQueue(userId: string, migrateLegacy = false) {
  const scopedKey = userQueueKey(userId);
  const scoped = await AsyncStorage.getItem(scopedKey);
  if (scoped) return validQueue(parseJSON<QueuedVideoChunk[]>(scoped, []), userId);

  if (!migrateLegacy) return [];
  const legacy = await AsyncStorage.getItem(QUEUE_KEY);
  if (!legacy) return [];
  const queue = validQueue(parseJSON<QueuedVideoChunk[]>(legacy, []), userId);
  await AsyncStorage.setItem(scopedKey, JSON.stringify(queue));
  await AsyncStorage.removeItem(QUEUE_KEY);
  return queue;
}

export async function quarantineLegacyUploadQueue() {
  const legacy = await AsyncStorage.getItem(QUEUE_KEY);
  if (legacy) await AsyncStorage.setItem(`${QUEUE_KEY}:quarantined`, legacy);
  await AsyncStorage.removeItem(QUEUE_KEY);
}

export async function saveUploadQueue(userId: string, queue: QueuedVideoChunk[]) {
  const owned = queue.filter((item) => item.ownerUserId === userId);
  await AsyncStorage.setItem(userQueueKey(userId), JSON.stringify(owned));
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
