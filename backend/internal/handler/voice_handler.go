package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/utils"
	"github.com/gin-gonic/gin"
)

type VoiceHandler interface {
	Ingest(c *gin.Context)
	IngestChunk(c *gin.Context)
	GetRecording(c *gin.Context)
	StartRealtimeSession(c *gin.Context)
	IngestRealtimeChunk(c *gin.Context)
	GetRealtimeSession(c *gin.Context)
	StopRealtimeSession(c *gin.Context)
	CreateMemory(c *gin.Context)
	Search(c *gin.Context)
	Answer(c *gin.Context)
	AnswerStream(c *gin.Context)
	GetGraph(c *gin.Context)
}

type voiceHandler struct {
	service        service.VoiceService
	maxUploadBytes int64
}

func newVoiceHandler(voiceService service.VoiceService, maxUploadBytes int64) *voiceHandler {
	if maxUploadBytes < 1 {
		maxUploadBytes = 25 << 20
	}
	return &voiceHandler{service: voiceService, maxUploadBytes: maxUploadBytes}
}

func (h *voiceHandler) Ingest(c *gin.Context) {
	h.ingest(c, false)
}

func (h *voiceHandler) IngestChunk(c *gin.Context) {
	h.ingest(c, true)
}

func (h *voiceHandler) ingest(c *gin.Context, chunk bool) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	// Allow a small envelope above the configured file limit for multipart
	// headers and form fields, while preventing large bodies from being spooled
	// to disk before the storage layer applies its own exact byte limit.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+(1<<20))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			response.Error(c, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "uploaded audio exceeds the configured limit")
			return
		}
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "multipart field file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "uploaded audio could not be opened")
		return
	}
	defer file.Close()

	startOffset := 0.0
	if raw := strings.TrimSpace(c.PostForm("start_time")); raw != "" {
		startOffset, err = strconv.ParseFloat(raw, 64)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "start_time must be a number of seconds")
			return
		}
	}
	var confidence *float64
	if raw := strings.TrimSpace(c.PostForm("confidence")); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "confidence must be a number")
			return
		}
		confidence = &value
	}
	result, err := h.service.Ingest(c.Request.Context(), service.VoiceIngestInput{
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
	message := "audio accepted for processing"
	if chunk {
		message = "audio chunk accepted for processing"
	}
	response.Success(c, http.StatusAccepted, result, message)
}

func (h *voiceHandler) GetRecording(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	result, err := h.service.GetRecording(c.Request.Context(), c.Param("recording_id"), principal.Subject)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "voice recording status")
}

func (h *voiceHandler) StartRealtimeSession(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	var request service.StartRealtimeSessionInput
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid realtime session request")
		return
	}
	request.OwnerUserID = principal.Subject
	result, err := h.service.StartRealtimeSession(c.Request.Context(), request)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, result, "realtime voice session started")
}

func (h *voiceHandler) IngestRealtimeChunk(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+(1<<20))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			response.Error(c, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "uploaded audio exceeds the configured limit")
			return
		}
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "multipart field file is required")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "uploaded audio could not be opened")
		return
	}
	defer file.Close()

	chunkIndex, err := strconv.Atoi(strings.TrimSpace(c.PostForm("chunk_index")))
	if err != nil || chunkIndex < 0 {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "chunk_index must be a non-negative integer")
		return
	}
	isFinal := false
	if raw := strings.TrimSpace(c.PostForm("is_final")); raw != "" {
		isFinal, err = strconv.ParseBool(raw)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "is_final must be true or false")
			return
		}
	}
	var confidence *float64
	if raw := strings.TrimSpace(c.PostForm("confidence")); raw != "" {
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "confidence must be a number")
			return
		}
		confidence = &value
	}
	result, err := h.service.IngestRealtimeChunk(c.Request.Context(), service.RealtimeChunkInput{
		OwnerUserID:       principal.Subject,
		SessionID:         c.Param("session_id"),
		ChunkIndex:        chunkIndex,
		IsFinal:           isFinal,
		FileName:          fileHeader.Filename,
		MediaType:         fileHeader.Header.Get("Content-Type"),
		DefaultConfidence: confidence,
		Content:           file,
	})
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusAccepted, result, "realtime audio chunk accepted")
}

func (h *voiceHandler) GetRealtimeSession(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	result, err := h.service.GetRealtimeSession(
		c.Request.Context(), c.Param("session_id"), principal.Subject,
	)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "realtime voice session status")
}

func (h *voiceHandler) StopRealtimeSession(c *gin.Context) {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return
	}
	result, err := h.service.StopRealtimeSession(
		c.Request.Context(), c.Param("session_id"), principal.Subject,
	)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "realtime voice session stopped")
}

func (h *voiceHandler) CreateMemory(c *gin.Context) {
	var request service.MemoryCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid memory configuration")
		return
	}
	result, err := h.service.CreateMemory(c.Request.Context(), c.Param("project_id"), request)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, result, "graph memory created")
}

func (h *voiceHandler) Search(c *gin.Context) {
	var request service.MemorySearchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid search request")
		return
	}
	result, err := h.service.Search(c.Request.Context(), c.Param("memory_id"), request)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "memory search complete")
}

func (h *voiceHandler) Answer(c *gin.Context) {
	var request service.MemoryAnswerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid answer request")
		return
	}
	result, err := h.service.Answer(c.Request.Context(), c.Param("memory_id"), request)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "memory answer complete")
}

func (h *voiceHandler) AnswerStream(c *gin.Context) {
	var request service.MemoryAnswerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid answer request")
		return
	}
	stream, err := h.service.AnswerStream(c.Request.Context(), c.Param("memory_id"), request)
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	defer stream.Body.Close()

	contentType := stream.ContentType
	if contentType == "" {
		contentType = "text/event-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	buffer := make([]byte, 4096)
	for {
		read, readErr := stream.Body.Read(buffer)
		if read > 0 {
			if _, writeErr := c.Writer.Write(buffer[:read]); writeErr != nil {
				return
			}
			c.Writer.Flush()
		}
		if readErr != nil {
			if readErr != io.EOF {
				_ = c.Error(readErr)
			}
			return
		}
	}
}

func (h *voiceHandler) GetGraph(c *gin.Context) {
	result, err := h.service.GetGraph(c.Request.Context(), c.Param("memory_id"), c.Query("group_id"))
	if err != nil {
		_ = c.Error(err)
		response.ServiceError(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "memory graph loaded")
}

var _ VoiceHandler = (*voiceHandler)(nil)
