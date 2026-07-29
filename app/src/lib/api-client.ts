import type {
  ApiEnvelope,
  AuthResult,
  Credentials,
  Health,
  MemoryAnswerRequest,
  MemoryCreateRequest,
  MemorySearchRequest,
  RealtimeVideoSession,
  RealtimeVideoSessionDetail,
  RealtimeVoiceSession,
  RealtimeVoiceSessionDetail,
  RecordingUploadInput,
  StartVideoSessionInput,
  StartVoiceSessionInput,
  UploadFile,
  VideoChunkUploadInput,
  VideoRecording,
  VideoRecordingDetail,
  VoiceChunkUploadInput,
  VoiceRecording,
  VoiceRecordingDetail,
} from '@/types/api';
import { randomUUID } from 'expo-crypto';

type RequestOptions = {
  method?: 'GET' | 'POST';
  body?: unknown;
  form?: FormData;
  authenticated?: boolean;
  timeoutMs?: number;
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }

  get retryable() {
    return this.status === 0 || this.status === 408 || this.status === 429 || this.status >= 500;
  }
}

function normalizeBaseUrl(value: string) {
  return value.trim().replace(/\/+$/, '');
}

function appendFile(form: FormData, file: UploadFile) {
  // React Native's FormData implementation accepts this file descriptor.
  form.append('file', file as unknown as Blob);
}

function appendOptional(form: FormData, key: string, value: string | number | boolean | undefined) {
  if (value !== undefined && value !== '') {
    form.append(key, String(value));
  }
}

function recordingForm(input: RecordingUploadInput) {
  const form = new FormData();
  appendFile(form, input.file);
  form.append('session_id', input.sessionId);
  form.append('memory_id', input.memoryId);
  appendOptional(form, 'group_id', input.groupId);
  appendOptional(form, 'device_id', input.deviceId);
  appendOptional(form, 'location', input.location);
  appendOptional(form, 'start_time', input.startTime);
  appendOptional(form, 'confidence', input.confidence);
  return form;
}

export class ApiClient {
  readonly baseUrl: string;

  constructor(
    baseUrl: string,
    private readonly getAccessToken: () => string | undefined,
    private readonly getMemographAPIKey: () => string | undefined = () => undefined,
    private readonly onUnauthorized?: () => void,
  ) {
    this.baseUrl = normalizeBaseUrl(baseUrl);
    if (!this.baseUrl) {
      throw new ApiError('Backend URL is required.', 0, 'CLIENT_CONFIG');
    }
  }

  private async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    if (!__DEV__ && !this.baseUrl.startsWith('https://')) {
      throw new ApiError('Production backend URL must use HTTPS.', 0, 'INSECURE_BACKEND_URL');
    }
    const headers: Record<string, string> = {
      Accept: 'application/json',
      'X-Request-ID': randomUUID(),
    };
    if (options.authenticated !== false) {
      const token = this.getAccessToken();
      if (!token) {
        throw new ApiError('Please sign in again.', 401, 'UNAUTHORIZED');
      }
      headers.Authorization = `Bearer ${token}`;
      const memographAPIKey = this.getMemographAPIKey()?.trim();
      if (memographAPIKey) {
        headers['X-Memograph-Api-Key'] = memographAPIKey;
      }
    }
    if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    const controller = new AbortController();
    const timeout = setTimeout(
      () => controller.abort(),
      options.timeoutMs ?? (options.form ? 180_000 : 30_000),
    );
    let response: Response;
    try {
      response = await fetch(`${this.baseUrl}${path}`, {
        method: options.method ?? 'GET',
        headers,
        body: options.form ?? (options.body === undefined ? undefined : JSON.stringify(options.body)),
        signal: controller.signal,
      });
    } catch (error) {
      const timedOut = error instanceof Error && error.name === 'AbortError';
      throw new ApiError(
        timedOut ? 'The backend request timed out.' : 'Unable to reach the backend.',
        0,
        timedOut ? 'REQUEST_TIMEOUT' : 'NETWORK_ERROR',
      );
    } finally {
      clearTimeout(timeout);
    }

    const requestId = response.headers.get('X-Request-ID') ?? undefined;
    if (response.status === 401 && options.authenticated !== false) {
      this.onUnauthorized?.();
    }
    let envelope: ApiEnvelope<T>;
    try {
      envelope = (await response.json()) as ApiEnvelope<T>;
    } catch {
      throw new ApiError('Backend returned an invalid response.', response.status, 'INVALID_RESPONSE', requestId);
    }
    if (!response.ok) {
      throw new ApiError(
        envelope.message || envelope.error || 'Request failed.',
        response.status,
        envelope.code || 'REQUEST_FAILED',
        requestId,
      );
    }
    return envelope.data as T;
  }

  health() {
    return this.request<Health>('/health', { authenticated: false });
  }

  signup(credentials: Credentials) {
    return this.request<AuthResult>('/api/v1/auth/signup', {
      method: 'POST',
      body: credentials,
      authenticated: false,
    });
  }

  login(credentials: Credentials) {
    return this.request<AuthResult>('/api/v1/auth/login', {
      method: 'POST',
      body: credentials,
      authenticated: false,
    });
  }

  secure() {
    return this.request<{ user_id: string }>('/api/v1/secure');
  }

  voice = {
    ingestRecording: (input: RecordingUploadInput) =>
      this.request<VoiceRecording>('/api/v1/voice/recordings', {
        method: 'POST',
        form: recordingForm(input),
      }),

    ingestChunkAlias: (input: RecordingUploadInput) =>
      this.request<VoiceRecording>('/api/v1/voice/chunks', {
        method: 'POST',
        form: recordingForm(input),
      }),

    getRecording: (recordingId: string) =>
      this.request<VoiceRecordingDetail>(
        `/api/v1/voice/recordings/${encodeURIComponent(recordingId)}`,
      ),

    startRealtimeSession: (input: StartVoiceSessionInput) =>
      this.request<RealtimeVoiceSession>('/api/v1/voice/realtime/sessions', {
        method: 'POST',
        body: input,
      }),

    ingestRealtimeChunk: (sessionId: string, input: VoiceChunkUploadInput) => {
      const form = new FormData();
      appendFile(form, input.file);
      form.append('chunk_index', String(input.chunkIndex));
      appendOptional(form, 'is_final', input.isFinal);
      appendOptional(form, 'confidence', input.confidence);
      return this.request<VoiceRecording>(
        `/api/v1/voice/realtime/sessions/${encodeURIComponent(sessionId)}/chunks`,
        { method: 'POST', form },
      );
    },

    getRealtimeSession: (sessionId: string) =>
      this.request<RealtimeVoiceSessionDetail>(
        `/api/v1/voice/realtime/sessions/${encodeURIComponent(sessionId)}`,
      ),

    stopRealtimeSession: (sessionId: string) =>
      this.request<RealtimeVoiceSession>(
        `/api/v1/voice/realtime/sessions/${encodeURIComponent(sessionId)}/stop`,
        { method: 'POST' },
      ),
  };

  video = {
    ingestRecording: (input: RecordingUploadInput) =>
      this.request<VideoRecording>('/api/v1/video/recordings', {
        method: 'POST',
        form: recordingForm(input),
      }),

    getRecording: (recordingId: string) =>
      this.request<VideoRecordingDetail>(
        `/api/v1/video/recordings/${encodeURIComponent(recordingId)}`,
      ),

    startRealtimeSession: (input: StartVideoSessionInput) =>
      this.request<RealtimeVideoSession>('/api/v1/video/realtime/sessions', {
        method: 'POST',
        body: input,
      }),

    ingestRealtimeChunk: (sessionId: string, input: VideoChunkUploadInput) => {
      const form = new FormData();
      appendFile(form, input.file);
      form.append('chunk_id', input.chunkId);
      appendOptional(form, 'is_final', input.isFinal);
      appendOptional(form, 'confidence', input.confidence);
      return this.request<VideoRecording>(
        `/api/v1/video/realtime/sessions/${encodeURIComponent(sessionId)}/chunks`,
        { method: 'POST', form },
      );
    },

    getRealtimeSession: (sessionId: string) =>
      this.request<RealtimeVideoSessionDetail>(
        `/api/v1/video/realtime/sessions/${encodeURIComponent(sessionId)}`,
      ),

    stopRealtimeSession: (sessionId: string) =>
      this.request<RealtimeVideoSession>(
        `/api/v1/video/realtime/sessions/${encodeURIComponent(sessionId)}/stop`,
        { method: 'POST' },
      ),
  };

  memory = {
    create: (projectId: string, input: MemoryCreateRequest) =>
      this.request<unknown>(
        `/api/v1/voice/projects/${encodeURIComponent(projectId)}/memories`,
        { method: 'POST', body: input },
      ),

    search: (memoryId: string, input: MemorySearchRequest) =>
      this.request<unknown>(`/api/v1/voice/memories/${encodeURIComponent(memoryId)}/search`, {
        method: 'POST',
        body: input,
      }),

    answer: (memoryId: string, input: MemoryAnswerRequest) =>
      this.request<unknown>(`/api/v1/voice/memories/${encodeURIComponent(memoryId)}/answer`, {
        method: 'POST',
        body: input,
      }),

    graph: (memoryId: string, groupId?: string) => {
      const query = groupId ? `?group_id=${encodeURIComponent(groupId)}` : '';
      return this.request<unknown>(
        `/api/v1/voice/memories/${encodeURIComponent(memoryId)}/graph${query}`,
      );
    },
  };
}
