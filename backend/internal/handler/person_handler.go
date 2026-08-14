package handler

import (
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/utils"
	"github.com/gin-gonic/gin"
)

type PersonHandler interface {
	EnrollFace(c *gin.Context)
	RecognizeFace(c *gin.Context)
	List(c *gin.Context)
	Update(c *gin.Context)
	Delete(c *gin.Context)
}

type personHandler struct {
	service        service.PersonService
	maxUploadBytes int64
}

func newPersonHandler(personService service.PersonService, maxUploadBytes int64) *personHandler {
	if maxUploadBytes < 1 {
		maxUploadBytes = 10 << 20
	}
	return &personHandler{service: personService, maxUploadBytes: maxUploadBytes}
}

func (h *personHandler) EnrollFace(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	header, file, ok := h.openImage(c)
	if !ok {
		return
	}
	defer file.Close()
	consent, err := strconv.ParseBool(c.PostForm("consent_confirmed"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "consent_confirmed must be true or false")
		return
	}
	result, err := h.service.EnrollFace(c.Request.Context(), service.FaceEnrollmentInput{
		OwnerUserID: principal.Subject, PersonProfileID: c.PostForm("person_profile_id"),
		DisplayName: c.PostForm("display_name"), RelationshipCategory: c.PostForm("relationship_category"),
		RelationshipLabel: c.PostForm("relationship_label"), ConsentConfirmed: consent,
		FileName: header.Filename, MediaType: header.Header.Get("Content-Type"), Content: file,
	})
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, result, "face enrollment stored")
}

func (h *personHandler) RecognizeFace(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	header, file, ok := h.openImage(c)
	if !ok {
		return
	}
	defer file.Close()
	result, err := h.service.RecognizeFace(c.Request.Context(), service.FaceRecognitionRequest{
		OwnerUserID: principal.Subject, FileName: header.Filename,
		MediaType: header.Header.Get("Content-Type"), Content: file,
	})
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "face recognition completed")
}

func (h *personHandler) List(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	result, err := h.service.ListPeople(c.Request.Context(), principal.Subject)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "person profiles")
}

func (h *personHandler) Update(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	var request service.UpdatePersonInput
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid person profile request")
		return
	}
	request.ID, request.OwnerUserID = c.Param("person_profile_id"), principal.Subject
	result, err := h.service.UpdatePerson(c.Request.Context(), request)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "person profile updated")
}

func (h *personHandler) Delete(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	err := h.service.DeletePerson(c.Request.Context(), c.Param("person_profile_id"), principal.Subject)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, map[string]bool{"deleted": true}, "person and face biometrics deleted")
}

func (h *personHandler) openImage(c *gin.Context) (*multipart.FileHeader, multipart.File, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+(1<<20))
	header, err := c.FormFile("file")
	if err != nil {
		code, message := http.StatusBadRequest, "multipart field file is required"
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			code, message = http.StatusRequestEntityTooLarge, "face image exceeds the configured limit"
		}
		response.Error(c, code, "VALIDATION_ERROR", message)
		return nil, nil, false
	}
	file, err := header.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "uploaded image could not be opened")
		return nil, nil, false
	}
	return header, file, true
}

var _ PersonHandler = (*personHandler)(nil)
