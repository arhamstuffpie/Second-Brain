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

type VideoHandler interface {
	Ingest(c *gin.Context)
	GetRecording(c *gin.Context)
	Reprocess(c *gin.Context)
	GetEvidenceURL(c *gin.Context)
	StartRealtimeSession(c *gin.Context)
	IngestRealtimeChunk(c *gin.Context)
	GetRealtimeSession(c *gin.Context)
	StopRealtimeSession(c *gin.Context)
}

type videoHandler struct {
	service        service.VideoService
	maxUploadBytes int64
}

func newVideoHandler(videoService service.VideoService, maxUploadBytes int64) *videoHandler {
	if maxUploadBytes < 1 {
		maxUploadBytes = 250 << 20
	}
	return &videoHandler{service: videoService, maxUploadBytes: maxUploadBytes}
}

func (h *videoHandler) Ingest(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	fileHeader, file, ok := h.openVideo(c)
	if !ok {
		return
	}
	defer file.Close()

	startOffset, ok := parseOptionalFloat(c, "start_time")
	if !ok {
		return
	}
	confidence, ok := parseOptionalConfidence(c)
	if !ok {
		return
	}
	result, err := h.service.IngestVideo(c.Request.Context(), service.VideoIngestInput{
		OwnerUserID: principal.Subject, SessionID: c.PostForm("session_id"),
		GroupID: c.PostForm("group_id"), MemoryID: c.PostForm("memory_id"),
		DeviceID: c.PostForm("device_id"), Location: c.PostForm("location"),
		FileName: fileHeader.Filename, MediaType: fileHeader.Header.Get("Content-Type"),
		StartOffset: startOffset, DefaultConfidence: confidence, Content: file,
	})
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusAccepted, result, "video accepted for processing")
}

func (h *videoHandler) GetRecording(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	result, err := h.service.GetVideoRecording(
		c.Request.Context(), c.Param("recording_id"), principal.Subject,
	)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "video recording status")
}

func (h *videoHandler) Reprocess(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	result, err := h.service.ReprocessVideo(
		c.Request.Context(), c.Param("recording_id"), principal.Subject,
	)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusAccepted, result, "video reprocessing queued")
}

func (h *videoHandler) GetEvidenceURL(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	timestamp, err := strconv.ParseFloat(c.DefaultQuery("timestamp", "0"), 64)
	if err != nil || timestamp < 0 {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "timestamp must be non-negative seconds")
		return
	}
	result, err := h.service.GetVideoEvidenceURL(
		c.Request.Context(), c.Param("recording_id"), principal.Subject, timestamp,
	)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "short-lived evidence URL created")
}

func (h *videoHandler) StartRealtimeSession(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	var request service.StartVideoRealtimeSessionInput
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid realtime video session request")
		return
	}
	request.OwnerUserID = principal.Subject
	result, err := h.service.StartVideoRealtimeSession(c.Request.Context(), request)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, result, "realtime video session started")
}

func (h *videoHandler) IngestRealtimeChunk(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	fileHeader, file, ok := h.openVideo(c)
	if !ok {
		return
	}
	defer file.Close()

	isFinal := false
	var err error
	if raw := strings.TrimSpace(c.PostForm("is_final")); raw != "" {
		isFinal, err = strconv.ParseBool(raw)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "is_final must be true or false")
			return
		}
	}
	confidence, ok := parseOptionalConfidence(c)
	if !ok {
		return
	}
	result, err := h.service.IngestVideoRealtimeChunk(
		c.Request.Context(),
		service.RealtimeVideoChunkInput{
			OwnerUserID: principal.Subject, SessionID: c.Param("session_id"),
			ClientChunkID: c.PostForm("chunk_id"), IsFinal: isFinal,
			FileName: fileHeader.Filename, MediaType: fileHeader.Header.Get("Content-Type"),
			DefaultConfidence: confidence, Content: file,
		},
	)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusAccepted, result, "realtime video chunk accepted")
}

func (h *videoHandler) GetRealtimeSession(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	result, err := h.service.GetVideoRealtimeSession(
		c.Request.Context(), c.Param("session_id"), principal.Subject,
	)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "realtime video session status")
}

func (h *videoHandler) StopRealtimeSession(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	result, err := h.service.StopVideoRealtimeSession(
		c.Request.Context(), c.Param("session_id"), principal.Subject,
	)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "realtime video session stopped")
}

func (h *videoHandler) openVideo(
	c *gin.Context,
) (*multipart.FileHeader, multipart.File, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+(1<<20))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			response.Error(c, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "uploaded video exceeds the configured limit")
			return nil, nil, false
		}
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "multipart field file is required")
		return nil, nil, false
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "uploaded video could not be opened")
		return nil, nil, false
	}
	return fileHeader, file, true
}

func parseOptionalFloat(c *gin.Context, field string) (float64, bool) {
	raw := strings.TrimSpace(c.PostForm(field))
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", field+" must be a number of seconds")
		return 0, false
	}
	return value, true
}

func parseOptionalConfidence(c *gin.Context) (*float64, bool) {
	raw := strings.TrimSpace(c.PostForm("confidence"))
	if raw == "" {
		return nil, true
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "confidence must be a number")
		return nil, false
	}
	return &value, true
}

var _ VideoHandler = (*videoHandler)(nil)
