export type Paging = {
  page: number;
  page_size: number;
  total_items: number;
  total_pages: number;
};

export type ApiEnvelope<T> = {
  data: T | null;
  error: string;
  code: string;
  message: string;
  paging: Paging | null;
};

export type User = {
  id: string;
  email: string;
  created_at: string;
  updated_at: string;
};

export type AuthResult = {
  user: User;
  access_token: string;
  token_type: 'Bearer';
  expires_at: string;
};

export type Health = {
  status: 'ok';
  database: 'up';
  checked_at: string;
};

export type Credentials = {
  email: string;
  password: string;
};

export type TranscriptSegment = {
  start_time: number;
  end_time: number;
  speaker: string;
  text: string;
  confidence?: number;
};

export type Transcript = {
  text: string;
  language?: string;
  duration: number;
  segments: TranscriptSegment[];
  audio_track_present?: boolean;
  warning?: string;
};

export type VoiceRecordingStatus =
  | 'queued'
  | 'transcribing'
  | 'memograph_pending'
  | 'completed'
  | 'failed';

export type VoiceRecording = {
  id: string;
  session_id: string;
  group_id: string;
  memory_id: string;
  status: VoiceRecordingStatus;
  file_name: string;
  media_type: string;
  size_bytes: number;
  chunk_index?: number;
  is_final?: boolean;
  created_at: string;
};

export type VoiceEpisode = {
  id: string;
  bucket_index: number;
  start_time: number;
  end_time: number;
  description: string;
  confidence?: number;
  status: 'queued' | 'writing' | 'completed' | 'failed';
  memograph_response?: unknown;
  last_error?: string;
};

export type VoiceRecordingDetail = VoiceRecording & {
  device_id?: string;
  location?: string;
  transcript?: Transcript;
  episodes: VoiceEpisode[];
  last_error?: string;
  updated_at: string;
};

export type RealtimeProgress = {
  total: number;
  queued: number;
  processing: number;
  completed: number;
  failed: number;
  latest_chunk_index: number;
};

export type StartVoiceSessionInput = {
  memory_id: string;
  group_id?: string;
  device_id?: string;
  location?: string;
  chunk_duration_seconds?: number;
};

export type RealtimeVoiceSession = {
  id: string;
  memory_id: string;
  group_id: string;
  device_id?: string;
  location?: string;
  chunk_duration_seconds: number;
  status: 'active' | 'stopped';
  created_at: string;
  updated_at: string;
  stopped_at?: string;
};

export type RealtimeVoiceSessionDetail = RealtimeVoiceSession & {
  progress: RealtimeProgress;
  chunks: VoiceRecording[];
};

export type VideoRecordingStatus =
  | 'queued'
  | 'processing'
  | 'merging'
  | 'memograph_pending'
  | 'completed'
  | 'failed';

export type PipelineStatus = 'queued' | 'processing' | 'completed' | 'failed';
export type MergeStatus = 'waiting' | PipelineStatus;

export type VideoRecording = {
  id: string;
  session_id: string;
  group_id: string;
  memory_id: string;
  status: VideoRecordingStatus;
  audio_status: PipelineStatus;
  visual_status: PipelineStatus;
  merge_status: MergeStatus;
  file_name: string;
  media_type: string;
  size_bytes: number;
  chunk_id?: string;
  chunk_index?: number;
  start_time: number;
  is_final?: boolean;
  created_at: string;
};

export type VideoObservation = {
  start_time: number;
  end_time: number;
  objects: Array<{ name: string; confidence?: number }>;
  text_detected: Array<{ text: string; confidence?: number }>;
  activity: string;
  location_guess: string;
  summary: string;
  confidence?: number;
};

export type VideoEpisode = {
  id: string;
  bucket_index: number;
  start_time: number;
  end_time: number;
  description: string;
  visual_description?: string;
  speech_description?: string;
  location?: string;
  confidence?: number;
  status: 'queued' | 'writing' | 'completed' | 'failed';
  memograph_response?: unknown;
  last_error?: string;
};

export type VideoRecordingDetail = VideoRecording & {
  device_id?: string;
  location?: string;
  stt_provider?: string;
  stt_model?: string;
  visual_provider?: string;
  visual_model?: string;
  transcript?: Transcript;
  visual_analysis?: { observations: VideoObservation[] };
  episodes: VideoEpisode[];
  last_error?: string;
  updated_at: string;
};

export type StartVideoSessionInput = {
  memory_id: string;
  group_id?: string;
  device_id?: string;
  location?: string;
  chunk_duration_seconds?: number;
  frame_interval_seconds?: number;
};

export type RealtimeVideoSession = {
  id: string;
  memory_id: string;
  group_id: string;
  device_id?: string;
  location?: string;
  chunk_duration_seconds: number;
  frame_interval_seconds: number;
  next_chunk_index: number;
  status: 'active' | 'stopped';
  created_at: string;
  updated_at: string;
  stopped_at?: string;
};

export type RealtimeVideoSessionDetail = RealtimeVideoSession & {
  progress: RealtimeProgress;
  chunks: VideoRecording[];
};

export type UploadFile = {
  uri: string;
  name: string;
  type: string;
};

export type RecordingUploadInput = {
  file: UploadFile;
  sessionId: string;
  memoryId: string;
  groupId?: string;
  deviceId?: string;
  location?: string;
  startTime?: number;
  confidence?: number;
};

export type VoiceChunkUploadInput = {
  file: UploadFile;
  chunkIndex: number;
  isFinal?: boolean;
  confidence?: number;
};

export type VideoChunkUploadInput = {
  file: UploadFile;
  chunkId: string;
  isFinal?: boolean;
  confidence?: number;
};

export type CustomField = {
  name: string;
  type: string;
  description?: string;
  required?: boolean;
};

export type GraphConfig =
  | { mode: 'template'; template: string }
  | { mode: 'instruction'; instruction: string }
  | {
      mode: 'custom';
      entity_types: Record<string, string>;
      edge_types: Record<string, string>;
      entity_type_colors?: Record<string, string>;
      edge_type_map?: Record<string, string[]>;
    };

export type MemoryCreateRequest = {
  name: string;
  memory_type?: string;
  embedding_model?: string;
  secret_id?: string;
  custom_fields?: CustomField[];
  graph_config: GraphConfig;
};

export type MemorySearchRequest = {
  query: string;
  limit?: number;
  group_id?: string;
  filters?: Record<string, unknown>;
};

export type MemoryAnswerRequest = {
  query?: string;
  messages?: Array<{ role: string; content: string }>;
  limit?: number;
  model?: string;
  group_id?: string;
  filters?: Record<string, unknown>;
};
