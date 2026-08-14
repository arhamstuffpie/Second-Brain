package routes

import (
	"github.com/arham/ai-second-brain/internal/handler"
	"github.com/gin-gonic/gin"
)

func registerPeople(router *gin.RouterGroup, people handler.PersonHandler) {
	routes := router.Group("/people")
	routes.GET("", people.List)
	routes.POST("/face-enrollments", people.EnrollFace)
	routes.POST("/face-recognition", people.RecognizeFace)
	routes.PATCH("/:person_profile_id", people.Update)
	routes.DELETE("/:person_profile_id", people.Delete)
}
