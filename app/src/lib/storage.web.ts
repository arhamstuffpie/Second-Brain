import AsyncStorage from '@react-native-async-storage/async-storage';

import type { AppSettings, AuthSession, QueuedVideoChunk } from '@/types/app';

const AUTH_KEY = 'second-brain.auth.v1';
const SETTINGS_KEY = 'second-brain.settings.v1';
const DEVICE_SETTINGS_KEY = 'second-brain.device-settings.v1';
const QUEUE_KEY = 'second-brain.upload-queue.v1';
const VOICE_ONBOARDING_KEY = 'second-brain.voice-onboarding.v1';
const sessionMemographAPIKeys: Record<string, string> = {};

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
      const migrated = {
        ...defaultSettings,
        ...deviceSettings(device),
        ...parseJSON<Partial<AppSettings>>(legacyValue, {}),
        memographApiKey: sessionMemographAPIKeys.legacy ?? '',
      };
      await saveSettings(userId, migrated);
      await AsyncStorage.removeItem(SETTINGS_KEY);
      delete sessionMemographAPIKeys.legacy;
      return migrated;
    }
  }

  const stored = parseJSON<Partial<AppSettings>>(scopedValue, {});
  return {
    ...defaultSettings,
    ...deviceSettings(device),
    ...stored,
    memographApiKey: sessionMemographAPIKeys[userId] ?? '',
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
  delete sessionMemographAPIKeys.legacy;
}

export async function saveSettings(userId: string | null, settings: AppSettings) {
  const { memographApiKey, ...nonSensitiveSettings } = settings;
  await AsyncStorage.setItem(DEVICE_SETTINGS_KEY, JSON.stringify(deviceSettings(settings)));
  if (!userId) return;

  sessionMemographAPIKeys[userId] = memographApiKey;
  await AsyncStorage.setItem(userSettingsKey(userId), JSON.stringify(nonSensitiveSettings));
}

function userQueueKey(userId: string) {
  return `${QUEUE_KEY}:${userId}`;
}

export async function loadUploadQueue(userId: string, migrateLegacy = false) {
  const scopedKey = userQueueKey(userId);
  const scoped = await AsyncStorage.getItem(scopedKey);
  if (scoped) {
    return parseJSON<QueuedVideoChunk[]>(scoped, []).filter(
      (item) => item.ownerUserId === userId && item.state === 'uploaded',
    );
  }
  if (!migrateLegacy) return [];
  const legacy = await AsyncStorage.getItem(QUEUE_KEY);
  if (!legacy) return [];
  const queue = parseJSON<QueuedVideoChunk[]>(legacy, [])
    .filter((item) => item.state === 'uploaded')
    .map((item) => ({ ...item, ownerUserId: userId }));
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

export function persistCapturedVideo() {
  throw new Error('Durable media capture is available only on iOS and Android.');
}

export function deleteQueuedFile() {
  // Web capture is disabled, so there is no local media file to remove.
}
