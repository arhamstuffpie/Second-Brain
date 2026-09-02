package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerPipelineDebug(router *gin.RouterGroup, debug handler.PipelineDebugHandler) {
	routes := router.Group("/debug/pipeline")
	routes.GET("/providers", debug.Providers)
	routes.GET("/owners", debug.Owners)
	routes.GET("/overview", debug.AnalysisOverview)
	routes.GET("/dense", debug.DenseOverview)
	routes.GET("/dense/recordings/:recordingID", debug.DenseRecording)
	routes.GET("/dense/recordings/:recordingID/tracks/:trackID/observations/:observationID/face", debug.DenseFace)
	routes.POST("/face", debug.AnalyzeFace)
	routes.POST("/speaker", debug.EmbedSpeaker)
	routes.POST("/active-speaker", debug.DetectActiveSpeaker)
}
