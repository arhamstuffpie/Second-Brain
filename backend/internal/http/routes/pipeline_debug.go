package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerPipelineDebug(router *gin.RouterGroup, debug handler.PipelineDebugHandler) {
	routes := router.Group("/debug/pipeline")
	routes.GET("/providers", debug.Providers)
	routes.POST("/face", debug.AnalyzeFace)
	routes.POST("/speaker", debug.EmbedSpeaker)
	routes.POST("/active-speaker", debug.DetectActiveSpeaker)
}
