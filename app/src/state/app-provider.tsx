import { BatteryState, usePowerState } from 'expo-battery';
import { randomUUID } from 'expo-crypto';
import { NetworkStateType, useNetworkState } from 'expo-network';
import {
  createContext,
  type PropsWithChildren,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { StyleSheet, View } from 'react-native';

import { ErrorSnackbar } from '@/components/ui';
import { ApiClient, ApiError } from '@/lib/api-client';
import { getReadableError } from '@/lib/readable-error';
import {
  deleteQueuedFile,
  loadAuthSession,
  loadSettings,
  loadUploadQueue,
  saveAuthSession,
  saveSettings,
  saveUploadQueue,
} from '@/lib/storage';
import type { AppSettings, AuthSession, CaptureSnapshot, QueuedVideoChunk } from '@/types/app';
import type { Credentials, Health, RealtimeVideoSessionDetail } from '@/types/api';

const MAX_QUEUE_HISTORY = 60;
const MAX_UPLOAD_ATTEMPTS = 8;

function trimQueue(queue: QueuedVideoChunk[]) {
  const active = queue.filter((item) => item.state !== 'uploaded');
  const completed = queue.filter((item) => item.state === 'uploaded');
  return [...active, ...completed.slice(0, Math.max(0, MAX_QUEUE_HISTORY - active.length))];
}

type AppContextValue = {
  ready: boolean;
  auth: AuthSession | null;
  settings: AppSettings;
  queue: QueuedVideoChunk[];
  network: {
    online: boolean;
    type: NetworkStateType;
    uploadAllowed: boolean;
  };
  power: {
    batteryLevel: number;
    lowPowerMode: boolean;
    captureAllowed: boolean;
  };
  api: ApiClient;
  capture: CaptureSnapshot;
  setCapture: (snapshot: CaptureSnapshot) => void;
  login: (credentials: Credentials, baseUrl?: string) => Promise<void>;
  signup: (credentials: Credentials, baseUrl?: string) => Promise<void>;
  logout: () => Promise<void>;
  updateSettings: (patch: Partial<AppSettings>) => Promise<void>;
  enqueueVideoChunk: (chunk: QueuedVideoChunk) => Promise<void>;
  retryUpload: (id: string) => void;
  discardUpload: (id: string) => void;
  clearCompletedUploads: () => void;
  refreshRealtimeSession: (sessionId?: string) => Promise<RealtimeVideoSessionDetail | undefined>;
  checkHealth: () => Promise<Health>;
  showError: (message: string) => void;
};

const AppContext = createContext<AppContextValue | null>(null);

export function AppProvider({ children }: PropsWithChildren) {
  const [ready, setReady] = useState(false);
  const [auth, setAuth] = useState<AuthSession | null>(null);
  const [settings, setSettings] = useState<AppSettings | null>(null);
  const [queue, setQueue] = useState<QueuedVideoChunk[]>([]);
  const [capture, setCapture] = useState<CaptureSnapshot>({ phase: 'idle' });
  const [errorSnackbar, setErrorSnackbar] = useState<{ id: number; message: string } | null>(null);
  const queueRef = useRef(queue);
  const uploadLock = useRef(false);
  const queueWrite = useRef(Promise.resolve());
  const validatedToken = useRef<string | undefined>(undefined);
  const networkState = useNetworkState();
  const powerState = usePowerState();

  useEffect(() => {
    queueRef.current = queue;
  }, [queue]);

  useEffect(() => {
    void Promise.all([loadAuthSession(), loadSettings(), loadUploadQueue()]).then(
      async ([storedAuth, storedSettings, storedQueue]) => {
        if (!storedSettings.deviceId) {
          storedSettings.deviceId = randomUUID();
          await saveSettings(storedSettings);
        }
        if (storedAuth && Date.parse(storedAuth.expires_at) <= Date.now()) {
          storedAuth = null;
          await saveAuthSession(null);
          setErrorSnackbar({
            id: Date.now(),
            message: 'Your session expired. Sign in again to continue.',
          });
        }
        setAuth(storedAuth);
        setSettings(storedSettings);
        setQueue(storedQueue);
        setReady(true);
      },
    );
  }, []);

  const logout = useCallback(async () => {
    setAuth(null);
    setCapture({ phase: 'idle' });
    validatedToken.current = undefined;
    await saveAuthSession(null);
  }, []);

  const showError = useCallback((message: string) => {
    if (!message.trim()) return;
    setErrorSnackbar({ id: Date.now(), message });
  }, []);

  const activeSettings = settings ?? {
    apiBaseUrl: 'http://localhost:8181',
    memographApiKey: '',
    projectId: '',
    memoryId: '',
    groupId: '',
    deviceId: '',
    location: '',
    chunkDurationSeconds: 30 as const,
    frameIntervalSeconds: 5 as const,
    videoQuality: '720p' as const,
    wifiOnly: true,
    pauseOnLowBattery: true,
    lowBatteryThreshold: 0.15,
  };

  const api = useMemo(
    () =>
      new ApiClient(
        activeSettings.apiBaseUrl,
        () => auth?.access_token,
        () => activeSettings.memographApiKey,
        () => {
          showError('Your session expired. Sign in again to continue.');
          void logout();
        },
      ),
    [
      activeSettings.apiBaseUrl,
      activeSettings.memographApiKey,
      auth?.access_token,
      logout,
      showError,
    ],
  );

  const online =
    networkState.isConnected !== false &&
    networkState.isInternetReachable !== false &&
    networkState.type !== NetworkStateType.NONE;
  const onUnmeteredNetwork =
    networkState.type === NetworkStateType.WIFI ||
    networkState.type === NetworkStateType.ETHERNET;
  const uploadAllowed = online && (!activeSettings.wifiOnly || onUnmeteredNetwork);
  const isCharging =
    powerState.batteryState === BatteryState.CHARGING ||
    powerState.batteryState === BatteryState.FULL;
  const batteryIsLow =
    powerState.batteryLevel >= 0 &&
    powerState.batteryLevel <= activeSettings.lowBatteryThreshold &&
    !isCharging;
  const captureAllowed =
    !activeSettings.pauseOnLowBattery || (!batteryIsLow && !powerState.lowPowerMode);

  const replaceQueue = useCallback(
    (updater: (current: QueuedVideoChunk[]) => QueuedVideoChunk[]) => {
      setQueue((current) => {
        const next = updater(current);
        queueRef.current = next;
        // Serialize writes so a slower stale write cannot overwrite a newer
        // upload/retry state after an app restart.
        queueWrite.current = queueWrite.current
          .catch(() => undefined)
          .then(() => saveUploadQueue(next));
        return next;
      });
    },
    [],
  );

  const authenticate = useCallback(
    async (mode: 'login' | 'signup', credentials: Credentials, baseUrl?: string) => {
      const authClient = new ApiClient(baseUrl ?? activeSettings.apiBaseUrl, () => undefined);
      const result =
        mode === 'login' ? await authClient.login(credentials) : await authClient.signup(credentials);
      await saveAuthSession(result);
      setAuth(result);
    },
    [activeSettings.apiBaseUrl],
  );

  const login = useCallback(
    (credentials: Credentials, baseUrl?: string) => authenticate('login', credentials, baseUrl),
    [authenticate],
  );
  const signup = useCallback(
    (credentials: Credentials, baseUrl?: string) => authenticate('signup', credentials, baseUrl),
    [authenticate],
  );

  useEffect(() => {
    if (!ready || !auth || !online || validatedToken.current === auth.access_token) return;
    validatedToken.current = auth.access_token;
    void api.secure().catch((error) => {
      if (error instanceof ApiError && error.status === 401) {
        void logout();
      } else {
        // A reachable-network signal does not guarantee the backend is
        // reachable. Allow validation to retry without discarding offline auth.
        validatedToken.current = undefined;
      }
    });
  }, [api, auth, logout, online, ready]);

  useEffect(() => {
    if (!auth) return;
    let timeout: ReturnType<typeof setTimeout>;
    const checkExpiration = () => {
      const expiresIn = Date.parse(auth.expires_at) - Date.now();
      if (!Number.isFinite(expiresIn) || expiresIn <= 0) {
        showError('Your session expired. Sign in again to continue.');
        void logout();
        return;
      }
      timeout = setTimeout(checkExpiration, Math.min(expiresIn, 60_000));
    };
    checkExpiration();
    return () => clearTimeout(timeout);
  }, [auth, logout, showError]);

  const updateSettings = useCallback(
    async (patch: Partial<AppSettings>) => {
      const next = { ...activeSettings, ...patch };
      setSettings(next);
      await saveSettings(next);
    },
    [activeSettings],
  );

  const enqueueVideoChunk = useCallback(
    async (chunk: QueuedVideoChunk) => {
      const activeCount = queueRef.current.filter(
        (item) => item.state !== 'uploaded' && item.state !== 'failed',
      ).length;
      if (activeCount >= 100) {
        throw new Error('Offline buffer is full. Reconnect and retry uploads before capturing more.');
      }
      replaceQueue((current) => trimQueue([chunk, ...current]));
    },
    [replaceQueue],
  );

  const flushQueue = useCallback(async () => {
    if (!ready || !auth || !uploadAllowed || uploadLock.current) return;
    const item = [...queueRef.current]
      .reverse()
      .find(
        (candidate) =>
          (candidate.state === 'pending' || candidate.state === 'retrying') &&
          candidate.nextAttemptAt <= Date.now(),
      );
    if (!item) return;

    uploadLock.current = true;
    replaceQueue((current) =>
      current.map((candidate) =>
        candidate.id === item.id ? { ...candidate, state: 'uploading' } : candidate,
      ),
    );

    try {
      const recording = await api.video.ingestRealtimeChunk(item.sessionId, {
        file: { uri: item.fileUri, name: item.fileName, type: item.mediaType },
        chunkId: item.chunkId,
        isFinal: item.isFinal,
      });
      deleteQueuedFile(item.fileUri);
      replaceQueue((current) =>
        trimQueue(
          current.map((candidate) =>
            candidate.id === item.id
              ? {
                  ...candidate,
                  fileUri: '',
                  state: 'uploaded' as const,
                  recording,
                  lastError: undefined,
                }
              : candidate,
          ),
        ),
      );
    } catch (error) {
      const attempts = item.attempts + 1;
      const retryable = !(error instanceof ApiError) || error.retryable;
      const state = retryable && attempts < MAX_UPLOAD_ATTEMPTS ? 'retrying' : 'failed';
      const delay = Math.min(5 * 2 ** Math.max(attempts - 1, 0), 300) * 1000;
      const readableError = getReadableError(error, 'upload');
      replaceQueue((current) =>
        current.map((candidate) =>
          candidate.id === item.id
            ? {
                ...candidate,
                attempts,
                state,
                nextAttemptAt: Date.now() + delay,
                lastError: readableError,
              }
            : candidate,
        ),
      );
      if (state === 'failed') {
        showError(readableError);
      }
      if (error instanceof ApiError && error.status === 401) {
        await logout();
      }
    } finally {
      uploadLock.current = false;
    }
  }, [api, auth, logout, ready, replaceQueue, showError, uploadAllowed]);

  useEffect(() => {
    void flushQueue();
    const interval = setInterval(() => void flushQueue(), 5000);
    return () => clearInterval(interval);
  }, [flushQueue, queue]);

  const retryUpload = useCallback(
    (id: string) => {
      replaceQueue((current) =>
        current.map((item) =>
          item.id === id
            ? { ...item, state: 'retrying', attempts: 0, nextAttemptAt: Date.now(), lastError: undefined }
            : item,
        ),
      );
    },
    [replaceQueue],
  );

  const discardUpload = useCallback(
    (id: string) => {
      const target = queueRef.current.find((item) => item.id === id);
      if (target?.fileUri) deleteQueuedFile(target.fileUri);
      replaceQueue((current) => current.filter((item) => item.id !== id));
    },
    [replaceQueue],
  );

  const clearCompletedUploads = useCallback(() => {
    replaceQueue((current) => current.filter((item) => item.state !== 'uploaded'));
  }, [replaceQueue]);

  const refreshRealtimeSession = useCallback(
    async (sessionId = capture.sessionId) => {
      if (!sessionId || !auth || !online) return undefined;
      const remote = await api.video.getRealtimeSession(sessionId);
      setCapture((current) => ({ ...current, remote }));
      return remote;
    },
    [api, auth, capture.sessionId, online],
  );

  const value = useMemo<AppContextValue>(
    () => ({
      ready,
      auth,
      settings: activeSettings,
      queue,
      network: {
        online,
        type: networkState.type ?? NetworkStateType.UNKNOWN,
        uploadAllowed,
      },
      power: {
        batteryLevel: powerState.batteryLevel,
        lowPowerMode: powerState.lowPowerMode,
        captureAllowed,
      },
      api,
      capture,
      setCapture,
      login,
      signup,
      logout,
      updateSettings,
      enqueueVideoChunk,
      retryUpload,
      discardUpload,
      clearCompletedUploads,
      refreshRealtimeSession,
      checkHealth: () => api.health(),
      showError,
    }),
    [
      activeSettings,
      api,
      auth,
      capture,
      captureAllowed,
      clearCompletedUploads,
      discardUpload,
      enqueueVideoChunk,
      login,
      logout,
      networkState.type,
      online,
      powerState.batteryLevel,
      powerState.lowPowerMode,
      queue,
      ready,
      refreshRealtimeSession,
      retryUpload,
      signup,
      showError,
      updateSettings,
      uploadAllowed,
    ],
  );

  return (
    <AppContext.Provider value={value}>
      <View style={styles.provider}>
        {children}
        {errorSnackbar ? (
          <ErrorSnackbar
            key={errorSnackbar.id}
            message={errorSnackbar.message}
            onDismiss={() => setErrorSnackbar(null)}
          />
        ) : null}
      </View>
    </AppContext.Provider>
  );
}

export function useApp() {
  const context = useContext(AppContext);
  if (!context) {
    throw new Error('useApp must be used inside AppProvider.');
  }
  return context;
}

const styles = StyleSheet.create({
  provider: { flex: 1 },
});
