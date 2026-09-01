package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerModelProfiles(router *gin.RouterGroup, models handler.ModelProfileHandler) {
	profiles := router.Group("/model-profiles")
	profiles.GET("/transcription", models.GetTranscription)
	profiles.PUT("/transcription", models.SaveTranscription)
	profiles.DELETE("/transcription", models.ResetTranscription)
}
