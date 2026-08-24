package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerVideo(router *gin.RouterGroup, video handler.VideoHandler) {
	videoRoutes := router.Group("/video")

	// POST /api/v1/video/recordings
	// Authenticated multipart/form-data upload. Fields: file, session_id and
	// memory_id are required; group_id, device_id, location, start_time and
	// confidence are optional. Returns 202 with service.VideoRecording.
	videoRoutes.POST("/recordings", video.Ingest)

	// GET /api/v1/video/recordings/:recording_id
	// Returns 200 with service.VideoRecordingDetail, including audio/visual
	// progress, transcript, visual analysis and memory episodes when available.
	videoRoutes.GET("/recordings/:recording_id", video.GetRecording)
	videoRoutes.POST("/recordings/:recording_id/reprocess", video.Reprocess)
	videoRoutes.GET("/recordings/:recording_id/evidence-url", video.GetEvidenceURL)

	// POST /api/v1/video/realtime/sessions
	// JSON: service.StartVideoRealtimeSessionInput. memory_id is required;
	// metadata, chunk duration and frame interval are optional. Returns 201 with
	// service.RealtimeVideoSession.
	videoRoutes.POST("/realtime/sessions", video.StartRealtimeSession)

	// POST /api/v1/video/realtime/sessions/:session_id/chunks
	// Authenticated multipart upload. file and UUID chunk_id are required;
	// is_final and confidence are optional. Returns 202 with
	// service.VideoRecording. The server assigns chunk_index.
	videoRoutes.POST("/realtime/sessions/:session_id/chunks", video.IngestRealtimeChunk)

	// GET /api/v1/video/realtime/sessions/:session_id
	// Returns 200 with service.RealtimeVideoSessionDetail: session metadata,
	// aggregate progress counters and the server-ordered chunk list.
	videoRoutes.GET("/realtime/sessions/:session_id", video.GetRealtimeSession)

	// POST /api/v1/video/realtime/sessions/:session_id/stop
	// Takes no body and idempotently marks the owned session stopped. Returns
	// 200 with service.RealtimeVideoSession.
	videoRoutes.POST("/realtime/sessions/:session_id/stop", video.StopRealtimeSession)
}
