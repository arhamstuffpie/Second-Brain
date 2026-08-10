package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/arham/ai-second-brain/internal/config"
)

type videoService struct {
	repository        VideoRepository
	enrollments       VoiceRepository
	transcriber       Transcriber
	attributor        SpeakerAttributor
	speakerProfiles   SpeakerProfileRepository
	speakerIdentifier SpeakerIdentifier
	enrollmentStore   AudioStore
	store             VideoStore
	extractor         MediaExtractor
	analyzer          VisualAnalyzer
	memograph         MemographClient
	episodeDuration   time.Duration
	frameInterval     time.Duration
	maxFrames         int
	maxAttempts       int
}

func newVideoService(
	repository VideoRepository,
	enrollments VoiceRepository,
	transcriber Transcriber,
	attributor SpeakerAttributor,
	enrollmentStore AudioStore,
	store VideoStore,
	extractor MediaExtractor,
	analyzer VisualAnalyzer,
	memograph MemographClient,
	videoConfig config.VideoConfig,
	workerConfig config.WorkerConfig,
) *videoService {
	return &videoService{
		repository: repository, enrollments: enrollments, transcriber: transcriber,
		attributor: attributor, enrollmentStore: enrollmentStore, store: store,
		extractor: extractor, analyzer: analyzer, memograph: memograph,
		episodeDuration: videoConfig.EpisodeDuration,
		frameInterval:   videoConfig.FrameInterval,
		maxFrames:       videoConfig.MaxFrames,
		maxAttempts:     workerConfig.MaxAttempts,
	}
}

func (s *videoService) IngestVideo(
	ctx context.Context,
	input VideoIngestInput,
) (VideoRecording, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.GroupID = strings.TrimSpace(input.GroupID)
	input.MemoryID = strings.TrimSpace(input.MemoryID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.Location = strings.TrimSpace(input.Location)
	input.FileName = filepath.Base(strings.TrimSpace(input.FileName))
	input.MediaType = strings.TrimSpace(input.MediaType)
	if input.OwnerUserID == "" {
		return VideoRecording{}, validation("owner_user_id", "is required")
	}
	if input.SessionID == "" {
		return VideoRecording{}, validation("session_id", "is required")
	}
	if input.GroupID == "" {
		input.GroupID = defaultMemoryGroupID(input.OwnerUserID)
	}
	if input.MemoryID == "" {
		return VideoRecording{}, validation("memory_id", "is required")
	}
	if input.FileName == "" || input.FileName == "." || input.Content == nil {
		return VideoRecording{}, validation("file", "a named video file is required")
	}
	if input.StartOffset < 0 {
		return VideoRecording{}, validation("start_time", "must not be negative")
	}
	if input.DefaultConfidence != nil &&
		(*input.DefaultConfidence < 0 || *input.DefaultConfidence > 1) {
		return VideoRecording{}, validation("confidence", "must be between 0 and 1")
	}
	if !supportedVideo(input.FileName, input.MediaType) {
		return VideoRecording{}, validation(
			"file", "must be an mp4, webm, mov, m4v, or mkv video",
		)
	}

	stored, err := s.store.Save(ctx, input.FileName, input.Content)
	if err != nil {
		return VideoRecording{}, validation("file", err.Error())
	}
	result, err := s.repository.CreateVideoRecording(ctx, CreateVideoRecordingInput{
		OwnerUserID: input.OwnerUserID, SessionID: input.SessionID,
		GroupID: input.GroupID, MemoryID: input.MemoryID, DeviceID: input.DeviceID,
		Location: input.Location, FileName: input.FileName, FilePath: stored.Path,
		MediaType: input.MediaType, SizeBytes: stored.SizeBytes,
		StartOffset: input.StartOffset, FrameInterval: s.frameInterval.Seconds(),
		DefaultConfidence: input.DefaultConfidence,
	}, s.maxAttempts)
	if err != nil {
		_ = s.store.Delete(context.Background(), stored.Path)
		return VideoRecording{}, fmt.Errorf("persist video ingestion: %w", err)
	}
	return result, nil
}

func (s *videoService) GetVideoRecording(
	ctx context.Context,
	id, ownerUserID string,
) (VideoRecordingDetail, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(ownerUserID) == "" {
		return VideoRecordingDetail{}, validation("recording_id", "is required")
	}
	detail, err := s.repository.GetVideoRecording(ctx, id, ownerUserID)
	if err != nil || detail.Transcript == nil {
		return detail, err
	}
	profiles, err := s.speakerProfiles.ListSpeakerProfiles(ctx, ownerUserID)
	if err != nil {
		return VideoRecordingDetail{}, err
	}
	enrichTranscriptSpeakerProfiles(detail.Transcript, profiles)
	return detail, nil
}

func (s *videoService) StartVideoRealtimeSession(
	ctx context.Context,
	input StartVideoRealtimeSessionInput,
) (RealtimeVideoSession, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.MemoryID = strings.TrimSpace(input.MemoryID)
	input.GroupID = strings.TrimSpace(input.GroupID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.Location = strings.TrimSpace(input.Location)
	if input.OwnerUserID == "" {
		return RealtimeVideoSession{}, validation("owner_user_id", "is required")
	}
	if input.MemoryID == "" {
		return RealtimeVideoSession{}, validation("memory_id", "is required")
	}
	if input.GroupID == "" {
		input.GroupID = defaultMemoryGroupID(input.OwnerUserID)
	}
	if input.ChunkDurationSeconds == 0 {
		input.ChunkDurationSeconds = 30
	}
	if input.ChunkDurationSeconds < 5 || input.ChunkDurationSeconds > 300 {
		return RealtimeVideoSession{}, validation(
			"chunk_duration_seconds", "must be between 5 and 300",
		)
	}
	if input.FrameIntervalSeconds == 0 {
		input.FrameIntervalSeconds = int(math.Round(s.frameInterval.Seconds()))
	}
	if input.FrameIntervalSeconds < 1 || input.FrameIntervalSeconds > 60 ||
		input.FrameIntervalSeconds > input.ChunkDurationSeconds {
		return RealtimeVideoSession{}, validation(
			"frame_interval_seconds",
			"must be between 1 and 60 and no greater than chunk_duration_seconds",
		)
	}
	return s.repository.CreateVideoRealtimeSession(ctx, input)
}

func (s *videoService) IngestVideoRealtimeChunk(
	ctx context.Context,
	input RealtimeVideoChunkInput,
) (VideoRecording, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.ClientChunkID = strings.ToLower(strings.TrimSpace(input.ClientChunkID))
	input.FileName = filepath.Base(strings.TrimSpace(input.FileName))
	input.MediaType = strings.TrimSpace(input.MediaType)
	if input.OwnerUserID == "" || input.SessionID == "" {
		return VideoRecording{}, validation(
			"session_id", "session_id and authenticated owner are required",
		)
	}
	if !uuidPattern.MatchString(input.ClientChunkID) {
		return VideoRecording{}, validation("chunk_id", "must be a UUID")
	}
	if existing, found, err := s.repository.FindVideoRecordingByClientChunk(
		ctx, input.OwnerUserID, input.SessionID, input.ClientChunkID,
	); err != nil {
		return VideoRecording{}, err
	} else if found {
		if input.IsFinal {
			if _, stopErr := s.repository.StopVideoRealtimeSession(
				ctx, input.SessionID, input.OwnerUserID,
			); stopErr != nil {
				return VideoRecording{}, stopErr
			}
		}
		return existing, nil
	}
	session, err := s.repository.GetVideoRealtimeSession(
		ctx, input.SessionID, input.OwnerUserID,
	)
	if err != nil {
		return VideoRecording{}, err
	}
	if session.Status != "active" {
		return VideoRecording{}, fmt.Errorf("%w: realtime video session is stopped", ErrConflict)
	}
	if input.FileName == "" || input.FileName == "." || input.Content == nil {
		return VideoRecording{}, validation("file", "a named video file is required")
	}
	if !supportedVideo(input.FileName, input.MediaType) {
		return VideoRecording{}, validation(
			"file", "must be an mp4, webm, mov, m4v, or mkv video",
		)
	}
	if input.DefaultConfidence != nil &&
		(*input.DefaultConfidence < 0 || *input.DefaultConfidence > 1) {
		return VideoRecording{}, validation("confidence", "must be between 0 and 1")
	}

	stored, err := s.store.Save(ctx, input.FileName, input.Content)
	if err != nil {
		return VideoRecording{}, validation("file", err.Error())
	}
	result, err := s.repository.CreateRealtimeVideoChunk(
		ctx,
		CreateRealtimeVideoChunkInput{
			OwnerUserID: input.OwnerUserID, RealtimeSessionID: input.SessionID,
			ClientChunkID: input.ClientChunkID, FileName: input.FileName,
			FilePath: stored.Path, MediaType: input.MediaType, SizeBytes: stored.SizeBytes,
			IsFinal: input.IsFinal, DefaultConfidence: input.DefaultConfidence,
		},
		s.maxAttempts,
	)
	if err != nil {
		existing, found, findErr := s.repository.FindVideoRecordingByClientChunk(
			ctx, input.OwnerUserID, input.SessionID, input.ClientChunkID,
		)
		if findErr == nil && found {
			_ = s.store.Delete(context.Background(), stored.Path)
			return existing, nil
		}
		_ = s.store.Delete(context.Background(), stored.Path)
		return VideoRecording{}, fmt.Errorf("persist realtime video chunk: %w", err)
	}
	if input.IsFinal {
		if _, stopErr := s.repository.StopVideoRealtimeSession(
			ctx, input.SessionID, input.OwnerUserID,
		); stopErr != nil {
			return VideoRecording{}, stopErr
		}
	}
	return result, nil
}

func (s *videoService) GetVideoRealtimeSession(
	ctx context.Context,
	id, ownerUserID string,
) (RealtimeVideoSessionDetail, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(ownerUserID) == "" {
		return RealtimeVideoSessionDetail{}, validation("session_id", "is required")
	}
	return s.repository.GetVideoRealtimeSession(ctx, id, ownerUserID)
}

func (s *videoService) StopVideoRealtimeSession(
	ctx context.Context,
	id, ownerUserID string,
) (RealtimeVideoSession, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(ownerUserID) == "" {
		return RealtimeVideoSession{}, validation("session_id", "is required")
	}
	return s.repository.StopVideoRealtimeSession(ctx, id, ownerUserID)
}

func (s *videoService) ProcessNextVideoJob(ctx context.Context) (bool, error) {
	job, found, err := s.repository.ClaimVideoJob(ctx)
	if err != nil || !found {
		return found, err
	}
	switch job.Kind {
	case "audio":
		err = s.processVideoAudio(ctx, job)
	case "visual":
		err = s.processVideoVisual(ctx, job)
	case "merge":
		err = s.processVideoMerge(ctx, job)
	case "memograph":
		err = s.processVideoMemograph(ctx, job)
	default:
		err = fmt.Errorf("unsupported video job kind %q", job.Kind)
	}
	if err == nil {
		return true, nil
	}
	dead := job.Attempts >= job.MaxAttempts
	retryCtx := ctx
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		retryCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	if retryErr := s.repository.RetryVideoJob(
		retryCtx, job, err.Error(),
		time.Now().UTC().Add(retryDelayForError(err, job.Attempts)), dead,
	); retryErr != nil {
		return true, fmt.Errorf("%v; persist video retry: %w", err, retryErr)
	}
	return true, err
}

func (s *videoService) processVideoAudio(ctx context.Context, job VideoJob) error {
	extracted, err := s.extractor.ExtractAudio(ctx, job.FilePath)
	if errors.Is(err, ErrNoAudioTrack) {
		audioTrackPresent := false
		return s.repository.SaveVideoTranscript(
			ctx, job, Transcript{
				Segments:          []TranscriptSegment{},
				AudioTrackPresent: &audioTrackPresent,
				Warning:           "video contains no audio track",
			},
			[]string{}, s.transcriber.Provider(), s.transcriber.Model(), s.maxAttempts,
		)
	}
	if err != nil {
		return err
	}
	defer extracted.Audio.Close()
	references, err := loadOwnerSpeakerReferences(
		ctx, s.enrollments, s.enrollmentStore, job.OwnerUserID,
	)
	if err != nil {
		return err
	}
	transcript, err := s.transcriber.Transcribe(ctx, TranscriptionInput{
		OwnerUserID: job.OwnerUserID,
		FileName:    extracted.FileName, MediaType: extracted.MediaType, Audio: extracted.Audio,
		KnownSpeakers: references,
	})
	if err != nil {
		return err
	}
	transcript, err = s.attributor.Attribute(ctx, SpeakerAttributionInput{
		Transcript: transcript, References: references,
	})
	if err != nil {
		return fmt.Errorf("attribute video transcript speakers: %w", err)
	}
	if s.speakerIdentifier != nil {
		identified, identifyErr := s.speakerIdentifier.Identify(ctx, SpeakerIdentificationInput{
			OwnerUserID: job.OwnerUserID, SourceKind: "video",
			SourceRecordingID: job.RecordingID, AudioPath: extracted.Path,
			Transcript: transcript,
		})
		transcript = identified
		if identifyErr != nil {
			transcript.Warning = appendTranscriptWarning(transcript.Warning, "persistent speaker identification was unavailable")
		}
	}
	if transcript.Segments == nil {
		transcript.Segments = []TranscriptSegment{}
	}
	audioTrackPresent := true
	transcript.AudioTrackPresent = &audioTrackPresent
	if strings.TrimSpace(transcript.Text) == "" && len(transcript.Segments) == 0 {
		transcript.Warning = "audio track was present, but no speech was detected"
	}
	return s.repository.SaveVideoTranscript(
		ctx, job, transcript, speakerReferenceIDs(references),
		transcriptionProvider(transcript, s.transcriber),
		transcriptionModel(transcript, s.transcriber), s.maxAttempts,
	)
}

func (s *videoService) processVideoVisual(ctx context.Context, job VideoJob) error {
	interval := time.Duration(job.FrameInterval * float64(time.Second))
	if interval <= 0 {
		interval = s.frameInterval
	}
	frames, err := s.extractor.ExtractFrames(ctx, job.FilePath, interval, s.maxFrames)
	if err != nil {
		return err
	}
	analysis, err := s.analyzer.Analyze(ctx, VisualAnalysisInput{
		Frames: frames, WindowDuration: interval.Seconds(),
	})
	if err != nil {
		return err
	}
	return s.repository.SaveVideoAnalysis(
		ctx, job, analysis, s.analyzer.Provider(), s.analyzer.Model(), s.maxAttempts,
	)
}

func (s *videoService) processVideoMerge(ctx context.Context, job VideoJob) error {
	episodes := BuildVideoEpisodes(
		job.Transcript, job.VisualAnalysis, s.episodeDuration,
		job.StartOffset, job.SessionID, job.Location,
	)
	if len(episodes) == 0 {
		return fmt.Errorf("audio and visual processing produced no episodes")
	}
	if err := s.repository.SaveVideoEpisodes(ctx, job, episodes, s.maxAttempts); err != nil {
		return err
	}
	_ = s.store.Delete(context.Background(), job.FilePath)
	return nil
}

func (s *videoService) processVideoMemograph(ctx context.Context, job VideoJob) error {
	if s.speakerProfiles != nil && (job.MemographSource == "speech" || job.MemographSource == "legacy") {
		profiles, err := s.speakerProfiles.ListSpeakerProfiles(ctx, job.OwnerUserID)
		if err != nil {
			return fmt.Errorf("refresh speaker labels for graph write: %w", err)
		}
		enrichTranscriptSpeakerProfiles(&job.Transcript, profiles)
		job.SpeechDescription = videoSpeechDescription(
			job.Transcript, job.StartOffset, job.EpisodeStart, job.EpisodeEnd,
		)
	}
	baseMeta := map[string]any{
		"session_id": job.SessionID, "group_id": job.GroupID,
		"episode_id": job.EpisodeID,
		"start_time": job.EpisodeStart, "end_time": job.EpisodeEnd,
	}
	addOwnerIdentityMeta(baseMeta, job.OwnerUserID)
	if job.ClientChunkID != "" {
		baseMeta["chunk_id"] = job.ClientChunkID
	}
	if job.Location != "" {
		baseMeta["location"] = job.Location
	}
	if job.DeviceID != "" {
		baseMeta["device_id"] = job.DeviceID
	}
	custom := make(map[string]any)
	if job.Confidence != nil {
		custom["confidence"] = *job.Confidence
	}

	visualDescription := strings.TrimSpace(job.VisualDescription)
	speechDescription := strings.TrimSpace(job.SpeechDescription)
	if visualDescription == "" && speechDescription == "" {
		visualDescription, speechDescription = legacyVideoMemographData(job.Description)
	}
	source := strings.TrimSpace(job.MemographSource)
	data := ""
	var structuredGraph *StructuredGraph
	switch source {
	case "visual":
		data = visualDescription
	case "speech":
		data = speechDescription
		structuredGraph = structuredVideoConversation(job)
		if structuredGraph == nil {
			return fmt.Errorf(
				"video speech episode %s has no grounded transcript segments",
				job.EpisodeID,
			)
		}
	case "legacy":
		data = strings.TrimSpace(job.Description)
		structuredGraph = structuredVideoConversation(job)
	default:
		return fmt.Errorf("unsupported video Memograph source %q", source)
	}
	if strings.TrimSpace(data) == "" {
		return fmt.Errorf("video Memograph %s branch contains no data", source)
	}
	baseMeta["source"] = source
	baseMeta["source_description"] = source + " from video recording"
	idempotencyKey := memographIdempotencyKey(
		job.MemoryID, job.EpisodeID, source, job.GraphRevision,
	)
	baseMeta["idempotency_key"] = idempotencyKey
	response, err := s.memograph.InsertEpisode(ctx, job.MemoryID, EpisodeInsertRequest{
		Data: data, Meta: baseMeta, StructuredGraph: structuredGraph,
		CustomFields:   custom,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return fmt.Errorf("write video %s episode: %w", source, err)
	}
	return s.repository.CompleteVideoMemographBranch(ctx, job, response)
}

func legacyVideoMemographData(description string) (string, string) {
	visual := videoDescriptionSection(
		description,
		"Visible context:",
		[]string{" Activities:", " Objects visible:", " Readable text:", " Speech:", " No speech"},
	)
	speech := videoDescriptionSection(description, "Speech:", nil)
	speech = strings.ReplaceAll(speech, ` said: "`, " Said : ")
	speech = strings.ReplaceAll(speech, `"`, "")
	return strings.TrimSpace(visual), strings.TrimSpace(speech)
}

func videoDescriptionSection(description, label string, endLabels []string) string {
	index := strings.Index(description, label)
	if index < 0 {
		return ""
	}
	value := description[index+len(label):]
	end := len(value)
	for _, endLabel := range endLabels {
		if marker := strings.Index(value, endLabel); marker >= 0 && marker < end {
			end = marker
		}
	}
	return strings.TrimSpace(value[:end])
}

type videoBucket struct {
	segments     []TranscriptSegment
	observations []VideoObservation
}

func BuildVideoEpisodes(
	transcript Transcript,
	analysis VisualAnalysis,
	duration time.Duration,
	offset float64,
	sessionID, fallbackLocation string,
) []VideoEpisodeDraft {
	bucketSeconds := duration.Seconds()
	if bucketSeconds <= 0 {
		bucketSeconds = 30
	}
	buckets := make(map[int]*videoBucket)
	getBucket := func(index int) *videoBucket {
		if buckets[index] == nil {
			buckets[index] = &videoBucket{}
		}
		return buckets[index]
	}
	for _, segment := range transcript.Segments {
		if strings.TrimSpace(segment.Text) == "" {
			continue
		}
		index := int(math.Floor(math.Max(segment.StartTime, 0) / bucketSeconds))
		getBucket(index).segments = append(getBucket(index).segments, segment)
	}
	for _, observation := range analysis.Observations {
		index := int(math.Floor(math.Max(observation.StartTime, 0) / bucketSeconds))
		getBucket(index).observations = append(getBucket(index).observations, observation)
	}
	indices := make([]int, 0, len(buckets))
	for index := range buckets {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	result := make([]VideoEpisodeDraft, 0, len(indices))
	for _, index := range indices {
		bucket := buckets[index]
		start := offset + float64(index)*bucketSeconds
		end := start
		location := strings.TrimSpace(fallbackLocation)
		speech := make([]string, 0, len(bucket.segments))
		visualSummaries := make([]string, 0, len(bucket.observations))
		activities := make([]string, 0, len(bucket.observations))
		objects := make([]string, 0)
		visibleText := make([]string, 0)
		confidenceTotal := 0.0
		confidenceCount := 0
		absoluteObservations := make([]VideoObservation, 0, len(bucket.observations))

		for _, segment := range bucket.segments {
			speaker := videoEpisodeSpeaker(segment)
			speech = append(
				speech,
				fmt.Sprintf(
					"[%.2fs-%.2fs] %s: %s",
					offset+segment.StartTime, offset+segment.EndTime,
					speaker, strings.TrimSpace(segment.Text),
				),
			)
			if segmentEnd := offset + segment.EndTime; segmentEnd > end {
				end = segmentEnd
			}
			addConfidence(segment.Confidence, &confidenceTotal, &confidenceCount)
		}
		for _, observation := range bucket.observations {
			absolute := observation
			absolute.StartTime += offset
			absolute.EndTime += offset
			absoluteObservations = append(absoluteObservations, absolute)
			if absolute.EndTime > end {
				end = absolute.EndTime
			}
			if location == "" && strings.TrimSpace(observation.LocationGuess) != "" {
				location = strings.TrimSpace(observation.LocationGuess)
			}
			appendUnique(&visualSummaries, observation.Summary)
			appendUnique(&activities, observation.Activity)
			for _, object := range observation.Objects {
				appendUnique(&objects, object.Name)
				addConfidence(object.Confidence, &confidenceTotal, &confidenceCount)
			}
			for _, detected := range observation.TextDetected {
				appendUnique(&visibleText, detected.Text)
				addConfidence(detected.Confidence, &confidenceTotal, &confidenceCount)
			}
			addConfidence(observation.Confidence, &confidenceTotal, &confidenceCount)
		}
		maximum := start + bucketSeconds
		if end <= start || end > maximum {
			end = maximum
		}
		parts := []string{
			fmt.Sprintf(
				"Audio and video memory from session %s between %.2fs and %.2fs.",
				sessionID, start, end,
			),
		}
		if location != "" {
			parts = append(parts, "The scene was at or appeared to be "+location+".")
		}
		if len(visualSummaries) > 0 {
			parts = append(parts, "Visible context: "+strings.Join(visualSummaries, " "))
		}
		if len(activities) > 0 {
			parts = append(parts, "Activities: "+strings.Join(activities, "; ")+".")
		}
		if len(objects) > 0 {
			parts = append(parts, "Objects visible: "+strings.Join(objects, ", ")+".")
		}
		if len(visibleText) > 0 {
			parts = append(parts, "Readable text: "+strings.Join(visibleText, " | ")+".")
		}
		if len(speech) > 0 {
			parts = append(parts, "Speech: "+strings.Join(speech, " "))
		} else {
			parts = append(parts, "No speech was detected in this interval.")
		}
		var confidence *float64
		if confidenceCount > 0 {
			value := confidenceTotal / float64(confidenceCount)
			confidence = &value
		}
		result = append(result, VideoEpisodeDraft{
			BucketIndex: index, StartTime: start, EndTime: end,
			Description:       strings.Join(parts, " "),
			VisualDescription: strings.Join(visualSummaries, " "),
			SpeechDescription: strings.Join(speech, " "),
			Location:          location,
			Confidence:        confidence, Visual: absoluteObservations,
		})
	}
	return result
}

func videoEpisodeSpeaker(segment TranscriptSegment) string {
	if name := strings.TrimSpace(segment.SpeakerName); name != "" {
		if relationship := strings.TrimSpace(segment.SpeakerRelationship); relationship != "" {
			return name + " (" + relationship + ")"
		}
		return name
	}
	switch segment.SpeakerRole {
	case "owner":
		return "Owner"
	case "other":
		if label := strings.TrimSpace(segment.Speaker); label != "" {
			return "Other speaker " + label
		}
		return "Other speaker"
	default:
		if label := strings.TrimSpace(segment.Speaker); label != "" && label != "speaker" {
			return "Unknown speaker " + label
		}
		return "Unknown speaker"
	}
}

func videoSpeechDescription(
	transcript Transcript, offset, episodeStart, episodeEnd float64,
) string {
	lines := make([]string, 0, len(transcript.Segments))
	for _, segment := range transcript.Segments {
		start := offset + segment.StartTime
		if start < episodeStart || start >= episodeEnd || strings.TrimSpace(segment.Text) == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"[%.2fs-%.2fs] %s: %s",
			start, offset+segment.EndTime, videoEpisodeSpeaker(segment),
			strings.TrimSpace(segment.Text),
		))
	}
	return strings.Join(lines, " ")
}

func supportedVideo(filename, mediaType string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4", ".webm", ".mov", ".m4v", ".mkv":
		return true
	}
	return strings.HasPrefix(strings.ToLower(mediaType), "video/")
}

func appendUnique(values *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range *values {
		if strings.EqualFold(existing, value) {
			return
		}
	}
	*values = append(*values, value)
}

func addConfidence(value *float64, total *float64, count *int) {
	if value == nil {
		return
	}
	*total += *value
	*count++
}

var uuidPattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

var _ VideoService = (*videoService)(nil)
