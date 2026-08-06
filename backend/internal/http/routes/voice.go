package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerVoice(router *gin.RouterGroup, voice handler.VoiceHandler) {
	voiceRoutes := router.Group("/voice")

	voiceRoutes.POST("/enrollments/samples", voice.EnrollVoice)
	voiceRoutes.GET("/enrollments/samples", voice.ListVoiceEnrollments)
	voiceRoutes.DELETE("/enrollments/samples/:sample_id", voice.DeleteVoiceEnrollment)

	// POST /api/v1/voice/recordings
	// Authenticated multipart/form-data upload. Fields: file, session_id and
	// memory_id are required; group_id, device_id, location, start_time and
	// confidence are optional. Returns 202 with service.VoiceRecording.
	voiceRoutes.POST("/recordings", voice.Ingest)

	// POST /api/v1/voice/chunks
	// Uses the same multipart contract and processing path as /recordings. This
	// legacy non-realtime route creates a closed standalone episode batch; it
	// does not accept chunk_index, provide idempotency, or assemble across calls.
	voiceRoutes.POST("/chunks", voice.IngestChunk)

	// GET /api/v1/voice/recordings/:recording_id
	// Returns 200 with service.VoiceRecordingDetail, including processing
	// status, transcript and generated memory episodes when available.
	voiceRoutes.GET("/recordings/:recording_id", voice.GetRecording)

	// POST /api/v1/voice/realtime/sessions
	// JSON: service.StartRealtimeSessionInput. memory_id is required; group_id,
	// device_id, location and chunk_duration_seconds are optional. Returns 201
	// with service.RealtimeVoiceSession.
	voiceRoutes.POST("/realtime/sessions", voice.StartRealtimeSession)

	// POST /api/v1/voice/realtime/sessions/:session_id/chunks
	// Authenticated multipart upload. file and non-negative chunk_index are
	// required; is_final and confidence are optional. Returns 202 with
	// service.VoiceRecording. A final chunk also stops the session.
	voiceRoutes.POST("/realtime/sessions/:session_id/chunks", voice.IngestRealtimeChunk)

	// GET /api/v1/voice/realtime/sessions/:session_id
	// Returns 200 with service.RealtimeVoiceSessionDetail: session metadata,
	// aggregate progress counters and the ordered chunk list.
	voiceRoutes.GET("/realtime/sessions/:session_id", voice.GetRealtimeSession)

	// POST /api/v1/voice/realtime/sessions/:session_id/stop
	// Takes no body and idempotently marks the owned session stopped. Returns
	// 200 with service.RealtimeVoiceSession.
	voiceRoutes.POST("/realtime/sessions/:session_id/stop", voice.StopRealtimeSession)

	// POST /api/v1/voice/projects/:project_id/memories
	// JSON: service.MemoryCreateRequest. Creates a graph memory through
	// Memograph and returns 201 with Memograph's JSON response as data.
	voiceRoutes.POST("/projects/:project_id/memories", voice.CreateMemory)

	// POST /api/v1/voice/memories/:memory_id/search
	// JSON: service.MemorySearchRequest. query is required; limit, group_id and
	// filters are optional. Returns 200 with Memograph's search JSON as data.
	voiceRoutes.POST("/memories/:memory_id/search", voice.Search)

	// POST /api/v1/voice/memories/:memory_id/answer
	// JSON: service.MemoryAnswerRequest. Requires query or messages; limit,
	// model, group_id and filters are optional. Returns the upstream answer JSON.
	voiceRoutes.POST("/memories/:memory_id/answer", voice.Answer)

	// POST /api/v1/memory/:memory_id/answer
	// JSON: service.MemoryAnswerRequest. Forces stream=true upstream and proxies
	// Memograph's meta, token, usage, error and done SSE events without buffering.
	router.POST("/memory/:memory_id/answer", voice.AnswerStream)

	// GET /api/v1/voice/memories/:memory_id/graph?group_id=<optional>
	// Loads the memory graph, optionally restricted to group_id, and returns 200
	// with Memograph's graph JSON as data.
	voiceRoutes.GET("/memories/:memory_id/graph", voice.GetGraph)
}
