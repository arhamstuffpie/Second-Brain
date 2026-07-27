package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerVoice(router *gin.RouterGroup, voice handler.VoiceHandler) {
	voiceRoutes := router.Group("/voice")
	voiceRoutes.POST("/recordings", voice.Ingest)
	voiceRoutes.POST("/chunks", voice.IngestChunk)
	voiceRoutes.GET("/recordings/:recording_id", voice.GetRecording)
	voiceRoutes.POST("/realtime/sessions", voice.StartRealtimeSession)
	voiceRoutes.POST("/realtime/sessions/:session_id/chunks", voice.IngestRealtimeChunk)
	voiceRoutes.GET("/realtime/sessions/:session_id", voice.GetRealtimeSession)
	voiceRoutes.POST("/realtime/sessions/:session_id/stop", voice.StopRealtimeSession)
	voiceRoutes.POST("/projects/:project_id/memories", voice.CreateMemory)
	voiceRoutes.POST("/memories/:memory_id/search", voice.Search)
	voiceRoutes.POST("/memories/:memory_id/answer", voice.Answer)
	voiceRoutes.GET("/memories/:memory_id/graph", voice.GetGraph)
}
