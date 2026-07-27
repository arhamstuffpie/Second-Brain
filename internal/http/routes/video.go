package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerVideo(router *gin.RouterGroup, video handler.VideoHandler) {
	videoRoutes := router.Group("/video")
	videoRoutes.POST("/recordings", video.Ingest)
	videoRoutes.GET("/recordings/:recording_id", video.GetRecording)
	videoRoutes.POST("/realtime/sessions", video.StartRealtimeSession)
	videoRoutes.POST("/realtime/sessions/:session_id/chunks", video.IngestRealtimeChunk)
	videoRoutes.GET("/realtime/sessions/:session_id", video.GetRealtimeSession)
	videoRoutes.POST("/realtime/sessions/:session_id/stop", video.StopRealtimeSession)
}
