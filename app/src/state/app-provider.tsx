import { BatteryState } from 'expo-battery';
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
import { usePowerState } from '@/hooks/use-power-state';
import {
  defaultSettings,
  deleteQueuedFile,
  loadAuthSession,
  loadSettings,
  loadUploadQueue,
  loadVoiceOnboardingRequired,
  quarantineLegacySettings,
  quarantineLegacyUploadQueue,
  saveAuthSession,
  saveSettings,
  saveUploadQueue,
  saveVoiceOnboardingRequired,
} from '@/lib/storage';
import type { AppSettings, AuthSession, CaptureSnapshot, QueuedVideoChunk } from '@/types/app';
import type {
  Credentials,
  Health,
  RealtimeVideoSessionDetail,
  UploadFile,
  VoiceEnrollmentSample,
} from '@/types/api';

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
  voiceEnrollment: {
    status: 'idle' | 'checking' | 'required' | 'enrolled' | 'error';
    samples: VoiceEnrollmentSample[];
    onboardingRequired: boolean;
    error?: string;
  };
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
  enrollOwnerVoice: (file: UploadFile) => Promise<void>;
  replaceOwnerVoice: (file: UploadFile) => Promise<void>;
  deleteOwnerVoice: (sampleId: string) => Promise<void>;
  refreshVoiceEnrollment: () => Promise<void>;
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
  const [voiceEnrollment, setVoiceEnrollment] = useState<AppContextValue['voiceEnrollment']>({
    status: 'idle',
    samples: [],
    onboardingRequired: false,
  });
  const [capture, setCapture] = useState<CaptureSnapshot>({ phase: 'idle' });
  const [errorSnackbar, setErrorSnackbar] = useState<{ id: number; message: string } | null>(null);
  const queueRef = useRef(queue);
  const uploadLock = useRef(false);
  const queueWrite = useRef(Promise.resolve());
  const authWrite = useRef(Promise.resolve());
  const queueOwner = useRef<string | null>(null);
  const uploadGeneration = useRef(0);
  const validatedToken = useRef<string | undefined>(undefined);
  const networkState = useNetworkState();
  const powerState = usePowerState();

  useEffect(() => {
    queueRef.current = queue;
  }, [queue]);

  useEffect(() => {
    void (async () => {
      let storedAuth = await loadAuthSession();
      const sessionOwnerId = storedAuth?.user.id;
      if (storedAuth && Date.parse(storedAuth.expires_at) <= Date.now()) {
        storedAuth = null;
        await saveAuthSession(null);
        setErrorSnackbar({
          id: Date.now(),
          message: 'Your session expired. Sign in again to continue.',
        });
      }

      let storedQueue: QueuedVideoChunk[] = [];
      let storedSettings: AppSettings;
      if (storedAuth) {
        [storedQueue, storedSettings] = await Promise.all([
          loadUploadQueue(storedAuth.user.id, true),
          loadSettings(storedAuth.user.id, true),
        ]);
      } else if (sessionOwnerId) {
        // A just-expired session still gives us a trustworthy owner for
        // migrating legacy local data, but none of it is rendered signed out.
        await Promise.all([
          loadUploadQueue(sessionOwnerId, true),
          loadSettings(sessionOwnerId, true),
        ]);
        storedSettings = await loadSettings();
      } else {
        // Legacy global data has no trustworthy owner while signed out. Keep
        // it quarantined rather than assigning it to the next account.
        await Promise.all([quarantineLegacyUploadQueue(), quarantineLegacySettings()]);
        storedSettings = await loadSettings();
      }

      if (!storedSettings.deviceId) {
        storedSettings = { ...storedSettings, deviceId: randomUUID() };
        await saveSettings(storedAuth?.user.id ?? null, storedSettings);
      }
      const onboardingRequired = storedAuth
        ? await loadVoiceOnboardingRequired(storedAuth.user.id)
        : false;
      queueOwner.current = storedAuth?.user.id ?? null;
      setVoiceEnrollment({
        status: storedAuth ? 'checking' : 'idle',
        samples: [],
        onboardingRequired,
      });
      setAuth(storedAuth);
      setSettings(storedSettings);
      setQueue(storedQueue);
      setReady(true);
    })();
  }, []);

  const logout = useCallback(async () => {
    uploadGeneration.current += 1;
    uploadLock.current = false;
    queueOwner.current = null;
    queueRef.current = [];
    setQueue([]);
    setAuth(null);
    setSettings((current) => ({
      ...defaultSettings,
      apiBaseUrl: current?.apiBaseUrl ?? defaultSettings.apiBaseUrl,
      deviceId: current?.deviceId ?? defaultSettings.deviceId,
    }));
    setVoiceEnrollment({ status: 'idle', samples: [], onboardingRequired: false });
    setCapture({ phase: 'idle' });
    setErrorSnackbar(null);
    validatedToken.current = undefined;
    authWrite.current = authWrite.current.catch(() => undefined).then(() => saveAuthSession(null));
    await authWrite.current;
  }, []);

  const showError = useCallback((message: string) => {
    if (!message.trim()) return;
    setErrorSnackbar({ id: Date.now(), message });
  }, []);

  const accountUserId = auth?.user.id ?? null;
  const showAccountError = useCallback(
    (message: string) => {
      if (queueOwner.current === accountUserId) showError(message);
    },
    [accountUserId, showError],
  );
  const setAccountCapture = useCallback(
    (snapshot: CaptureSnapshot) => {
      if (accountUserId && queueOwner.current === accountUserId) setCapture(snapshot);
    },
    [accountUserId],
  );

  const activeSettings = settings ?? defaultSettings;

  const api = useMemo(() => {
    const apiOwnerUserId = auth?.user.id ?? null;
    return new ApiClient(
      activeSettings.apiBaseUrl,
      () => auth?.access_token,
      () => activeSettings.memographApiKey,
      () => {
        if (queueOwner.current !== apiOwnerUserId) return;
        showError('Your session expired. Sign in again to continue.');
        void logout();
      },
    );
  },
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
      const ownerUserId = queueOwner.current;
      if (!ownerUserId) return;
      setQueue((current) => {
        if (queueOwner.current !== ownerUserId) return current;
        const next = updater(current).filter((item) => item.ownerUserId === ownerUserId);
        queueRef.current = next;
        // Serialize writes so a slower stale write cannot overwrite a newer
        // upload/retry state after an app restart.
        queueWrite.current = queueWrite.current
          .catch(() => undefined)
          .then(() => saveUploadQueue(ownerUserId, next));
        return next;
      });
    },
    [],
  );

  const authenticate = useCallback(
    async (mode: 'login' | 'signup', credentials: Credentials, baseUrl?: string) => {
      const authBaseUrl = (baseUrl ?? activeSettings.apiBaseUrl).trim().replace(/\/+$/, '');
      const authClient = new ApiClient(authBaseUrl, () => undefined);
      const result =
        mode === 'login' ? await authClient.login(credentials) : await authClient.signup(credentials);
      const [nextQueue, storedSettings, storedOnboardingRequired] = await Promise.all([
        loadUploadQueue(result.user.id),
        loadSettings(result.user.id),
        loadVoiceOnboardingRequired(result.user.id),
      ]);
      const nextSettings = { ...storedSettings, apiBaseUrl: authBaseUrl };
      const onboardingRequired = mode === 'signup' ? true : storedOnboardingRequired;
      if (mode === 'signup') {
        await saveVoiceOnboardingRequired(result.user.id, true);
      }
      authWrite.current = authWrite.current
        .catch(() => undefined)
        .then(() => saveAuthSession(result));
      await Promise.all([authWrite.current, saveSettings(result.user.id, nextSettings)]);

      uploadGeneration.current += 1;
      uploadLock.current = false;
      queueOwner.current = result.user.id;
      validatedToken.current = undefined;
      queueRef.current = nextQueue;
      setQueue(nextQueue);
      setSettings(nextSettings);
      setCapture({ phase: 'idle' });
      setVoiceEnrollment({ status: 'checking', samples: [], onboardingRequired });
      setErrorSnackbar(null);
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

  const refreshVoiceEnrollment = useCallback(async () => {
    if (!auth) return;
    const ownerUserId = auth.user.id;
    setVoiceEnrollment((current) => ({ ...current, status: 'checking', error: undefined }));
    try {
      const samples = await api.voice.listEnrollmentSamples();
      if (queueOwner.current !== ownerUserId) return;
      if (samples.length > 0) await saveVoiceOnboardingRequired(ownerUserId, false);
      setVoiceEnrollment((current) => ({
        ...current,
        status: samples.length > 0 ? 'enrolled' : 'required',
        samples,
        onboardingRequired: samples.length > 0 ? false : current.onboardingRequired,
        error: undefined,
      }));
    } catch (error) {
      if (queueOwner.current !== ownerUserId) return;
      setVoiceEnrollment((current) => ({
        ...current,
        status: 'error',
        error: getReadableError(error, 'backend'),
      }));
    }
  }, [api, auth]);

  useEffect(() => {
    if (!auth) return;
    void refreshVoiceEnrollment();
  }, [auth?.user.id, refreshVoiceEnrollment]);

  const enrollOwnerVoice = useCallback(
    async (file: UploadFile) => {
      const ownerUserId = auth?.user.id;
      if (!ownerUserId) throw new Error('Please sign in again.');
      const sample = await api.voice.enrollSample(file);
      if (queueOwner.current !== ownerUserId) return;
      await saveVoiceOnboardingRequired(ownerUserId, false);
      setVoiceEnrollment((current) => ({
        status: 'enrolled',
        samples: [sample, ...current.samples.filter((item) => item.id !== sample.id)],
        onboardingRequired: false,
      }));
    },
    [api, auth?.user.id],
  );

  const replaceOwnerVoice = useCallback(
    async (file: UploadFile) => {
      const ownerUserId = auth?.user.id;
      if (!ownerUserId) throw new Error('Please sign in again.');
      let previous = voiceEnrollment.samples;
      if (previous.length >= 4) {
        await api.voice.deleteEnrollmentSample(previous[0].id);
        previous = previous.slice(1);
      }
      const sample = await api.voice.enrollSample(file);
      if (queueOwner.current !== ownerUserId) return;
      let cleanupFailed = false;
      for (const oldSample of previous) {
        try {
          await api.voice.deleteEnrollmentSample(oldSample.id);
        } catch {
          cleanupFailed = true;
        }
      }
      let samples = [sample];
      if (cleanupFailed) {
        showError('The new voice sample is active, but an older sample could not be removed.');
        try {
          samples = await api.voice.listEnrollmentSamples();
        } catch {
          // Keep the confirmed new sample visible if refreshing metadata fails.
        }
      }
      if (queueOwner.current !== ownerUserId) return;
      await saveVoiceOnboardingRequired(ownerUserId, false);
      setVoiceEnrollment({ status: 'enrolled', samples, onboardingRequired: false });
    },
    [api, auth?.user.id, showError, voiceEnrollment.samples],
  );

  const deleteOwnerVoice = useCallback(
    async (sampleId: string) => {
      const ownerUserId = auth?.user.id;
      if (!ownerUserId) throw new Error('Please sign in again.');
      await api.voice.deleteEnrollmentSample(sampleId);
      if (queueOwner.current !== ownerUserId) return;
      setVoiceEnrollment((current) => {
        const samples = current.samples.filter((sample) => sample.id !== sampleId);
        return {
          ...current,
          status: samples.length > 0 ? 'enrolled' : 'required',
          samples,
          error: undefined,
        };
      });
    },
    [api, auth?.user.id],
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
      await saveSettings(auth?.user.id ?? null, next);
      if (queueOwner.current === (auth?.user.id ?? null)) setSettings(next);
    },
    [activeSettings, auth?.user.id],
  );

  const enqueueVideoChunk = useCallback(
    async (chunk: QueuedVideoChunk) => {
      if (chunk.ownerUserId !== queueOwner.current) {
        throw new Error('The active account changed before this recording could be queued.');
      }
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

    const generation = uploadGeneration.current;
    const ownerUserId = auth.user.id;
    if (item.ownerUserId !== ownerUserId) return;
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
      if (uploadGeneration.current !== generation || queueOwner.current !== ownerUserId) return;
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
      if (uploadGeneration.current !== generation || queueOwner.current !== ownerUserId) return;
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
      if (uploadGeneration.current === generation) uploadLock.current = false;
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
      const generation = uploadGeneration.current;
      const ownerUserId = auth.user.id;
      const remote = await api.video.getRealtimeSession(sessionId);
      if (uploadGeneration.current !== generation || queueOwner.current !== ownerUserId) {
        return undefined;
      }
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
      voiceEnrollment,
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
      setCapture: setAccountCapture,
      login,
      signup,
      logout,
      enrollOwnerVoice,
      replaceOwnerVoice,
      deleteOwnerVoice,
      refreshVoiceEnrollment,
      updateSettings,
      enqueueVideoChunk,
      retryUpload,
      discardUpload,
      clearCompletedUploads,
      refreshRealtimeSession,
      checkHealth: () => api.health(),
      showError: showAccountError,
    }),
    [
      activeSettings,
      api,
      auth,
      capture,
      captureAllowed,
      clearCompletedUploads,
      discardUpload,
      deleteOwnerVoice,
      enqueueVideoChunk,
      enrollOwnerVoice,
      login,
      logout,
      networkState.type,
      online,
      powerState.batteryLevel,
      powerState.lowPowerMode,
      queue,
      ready,
      refreshRealtimeSession,
      refreshVoiceEnrollment,
      replaceOwnerVoice,
      retryUpload,
      signup,
      setAccountCapture,
      showAccountError,
      updateSettings,
      voiceEnrollment,
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
