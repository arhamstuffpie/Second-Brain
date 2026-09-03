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

export type PipelineDebugProvider = {
	stage: 'face' | 'dense_person_tracking' | 'speaker' | 'active_speaker' | 'stt' | 'vision' | 'memograph';
	enabled: boolean;
	provider: string;
	model: string;
	cost_profile: 'local' | 'paid';
};

export type PipelineDebugStatus = {
	memograph_called: false;
	providers: PipelineDebugProvider[];
};

export type PipelineDebugRun = {
	run_id: string;
	stage: string;
	started_at: string;
	duration_ms: number;
	memograph_called: false;
	request: unknown;
	response: unknown;
};

export type PipelineDebugOwner = {
  id: string;
  email: string;
  recording_count: number;
  run_count: number;
  last_activity_at?: string;
};

export type PipelineDebugAnalysisStage = {
  stage: string;
  required: boolean;
  status: string;
  attempts: number;
  max_attempts: number;
  depends_on: string[];
  checkpoint: Record<string, unknown>;
  result_provenance: Record<string, unknown>;
  last_error: string;
  run_at: string;
  updated_at: string;
};

export type PipelineDebugAnalysisRun = {
  id: string;
  recording_id: string;
  file_name: string;
  processing_version: number;
  status: string;
  active: boolean;
  configuration_profile: string;
  last_error: string;
  created_at: string;
  updated_at: string;
  stages: PipelineDebugAnalysisStage[];
};

export type PipelineDebugAnalysisOverview = {
  owner_id: string;
  runs: PipelineDebugAnalysisRun[];
};

export type PipelineDebugDenseWorker = {
  enabled: boolean;
  provider: string;
  detector_model: string;
  embedding_model: string;
  profile: {
    fps: number;
    confirmation_detections: number;
    confirmation_window_frames: number;
    lost_timeout_seconds: number;
    reidentification_window_seconds: number;
    high_confidence_threshold: number;
    low_confidence_threshold: number;
    iou_threshold: number;
    appearance_threshold: number;
    max_gallery_samples: number;
  };
  jobs: Record<'queued' | 'processing' | 'completed' | 'retryable_failed' | 'dead', number>;
  oldest_queued_at?: string;
  last_completed_at?: string;
};

export type PipelineDebugDenseRecording = {
  recording_id: string;
  file_name: string;
  processing_version: number;
  run_status: string;
  stage_status: string;
  attempts: number;
  max_attempts: number;
  last_error: string;
  checkpoint: Record<string, unknown>;
  result_provenance: Record<string, unknown>;
  track_count: number;
  observation_count: number;
  gallery_count: number;
  embedding_count: number;
  created_at: string;
  updated_at: string;
};

export type PipelineDebugDenseOverview = {
  worker: PipelineDebugDenseWorker;
  recordings: PipelineDebugDenseRecording[];
};

export type PipelineDebugDenseObservation = {
  observation_id: string;
  frame_index: number;
  timestamp: number;
  box: { x: number; y: number; width: number; height: number };
  landmarks: number[][];
  detection_score: number;
  quality: { usable: boolean; reasons: string[]; score: number };
  pose: { yaw: number; pitch: number; roll: number; bucket: string };
  embedding_reference?: string;
  embedding_model: string;
  embedding_dimensions: number;
  embedding: number[];
  mouth_visible: boolean;
  mouth_activity: number;
  gallery_selected: boolean;
  created_at: string;
};

export type PipelineDebugDenseTrack = {
  id: string;
  provider_track_reference: string;
  temporary_visual_label: string;
  resolved_person_profile_id?: string;
  resolved_person_name?: string;
  resolved_person_status?: string;
  lifecycle_status: string;
  first_frame: number;
  last_frame: number;
  start_time: number;
  end_time: number;
  observation_count: number;
  tracking_confidence: number;
  quality: { mean: number; maximum: number; usable_observations: number };
  evidence_frame_ids: string[];
  model_provenance: Record<string, unknown>;
  metrics: {
    duration_seconds: number;
    observations_per_second: number;
    detection_minimum: number;
    detection_mean: number;
    detection_maximum: number;
    gallery_coverage: number;
    mouth_visible_coverage: number;
    mouth_activity_mean: number;
    maximum_observation_gap_seconds: number;
    mean_consecutive_box_iou: number;
    embedding_count: number;
    embedding_dimensions: number;
    embedding_norm_mean: number;
    embedding_cosine_minimum?: number;
    embedding_cosine_mean?: number;
    pose_buckets: Record<string, number>;
  };
  observations: PipelineDebugDenseObservation[];
  created_at: string;
  updated_at: string;
};

export type PipelineDebugFusionEvidence = {
  id: string;
  segment_id: string;
  segment_start_time: number;
  segment_end_time: number;
  known_voice_name: string;
  voice_speaker_profile_id: string;
  canonical_person_profile_id: string;
  person_track_id?: string;
  voice_confidence: number;
  active_speaker_score: number;
  runner_up_score: number;
  decision_margin: number;
  temporal_coverage: number;
  mouth_visible_coverage: number;
  mouth_activity: number;
  combined_score: number;
  supporting_segment_count: number;
  decision: string;
  conflict_reasons: string[];
  model_provenance: Record<string, unknown>;
  raw_evidence: Record<string, unknown>;
  created_at: string;
};

export type PipelineDebugDenseRecordingDetail = {
  recording: PipelineDebugDenseRecording;
  visual_analysis: {
    observations?: Array<{
      observation_id: string;
      frame_id: string;
      start_time: number;
      end_time: number;
      people: Array<{
        visual_label: string;
        person_track_id?: string;
        appearance: string;
        position: string;
        action: string;
        physical_presence: boolean;
        face_visible: boolean;
        person_profile_id?: string;
        person_name?: string;
        face_match_confidence?: number;
      }>;
    }>;
  };
  tracks: PipelineDebugDenseTrack[];
  fusion_evidence: PipelineDebugFusionEvidence[];
};

export type ModelProfile = {
  task: 'transcription';
  provider: string;
  base_url: string;
  model: string;
  api_key_configured: boolean;
  source: 'server' | 'account';
  updated_at?: string;
};

export type ModelProfileInput = {
  provider: string;
  base_url: string;
  model: string;
  api_key?: string;
  clear_api_key?: boolean;
};

export type Credentials = {
  email: string;
  password: string;
};

export type VoiceEnrollmentSample = {
  id: string;
  file_name: string;
  media_type: string;
  size_bytes: number;
  duration_seconds: number;
  created_at: string;
};

export type TranscriptSegment = {
  id?: string;
  start_time: number;
  end_time: number;
  speaker: string;
  speaker_role: 'owner' | 'other' | 'unknown';
  speaker_profile_id?: string;
  speaker_name?: string;
  speaker_relationship?: string;
  speaker_identity_status?: 'provisional' | 'confirmed';
  text: string;
  confidence?: number;
};

export type SpeakerRelationshipCategory =
  | ''
  | 'family'
  | 'friend'
  | 'colleague'
  | 'professional'
  | 'acquaintance'
  | 'other';

export type SpeakerSample = {
  id: string;
  profile_id: string;
  file_name: string;
  media_type: string;
  size_bytes: number;
  duration_seconds: number;
  created_at: string;
};

export type SpeakerProfile = {
  id: string;
  status: 'provisional' | 'confirmed';
  display_name: string;
  relationship_category: SpeakerRelationshipCategory;
  relationship_label: string;
  sample_count: number;
  first_seen_at: string;
  last_seen_at: string;
  expires_at?: string;
  samples: SpeakerSample[];
};

export type SpeakerProfileInput = {
  display_name: string;
  relationship_category: SpeakerRelationshipCategory;
  relationship_label: string;
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
  | 'assembling'
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
  stt_provider?: string;
  stt_model?: string;
  speaker_reference_ids: string[];
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
  speaker_reference_ids: string[];
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
  stream?: boolean;
  limit?: number;
  model?: string;
  group_id?: string;
  filters?: Record<string, unknown>;
};

export type MemoryAnswerStreamMeta = {
  memory_id: string;
  memory_name?: string;
};

export type MemoryAnswerStreamUsage = {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
};

export type MemoryAnswerStreamHandlers = {
  onMeta?: (meta: MemoryAnswerStreamMeta) => void;
  onToken: (content: string) => void;
  onUsage?: (usage: MemoryAnswerStreamUsage) => void;
  onDone?: () => void;
};
