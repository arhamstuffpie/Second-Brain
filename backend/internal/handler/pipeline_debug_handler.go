package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/arham/ai-second-brain/internal/http/response"
	"github.com/arham/ai-second-brain/internal/service"
	"github.com/arham/ai-second-brain/internal/utils"
	"github.com/gin-gonic/gin"
)

type PipelineDebugHandler interface {
	Providers(*gin.Context)
	Owners(*gin.Context)
	AnalysisOverview(*gin.Context)
	AnalyzeFace(*gin.Context)
	EmbedSpeaker(*gin.Context)
	DetectActiveSpeaker(*gin.Context)
	DenseOverview(*gin.Context)
	DenseRecording(*gin.Context)
	DenseFace(*gin.Context)
}

type pipelineDebugHandler struct {
	service       service.PipelineDebugService
	adminUserID   string
	maxImageBytes int64
	maxAudioBytes int64
	maxVideoBytes int64
}

func newPipelineDebugHandler(
	debug service.PipelineDebugService,
	adminUserID string,
	maxImageBytes, maxAudioBytes, maxVideoBytes int64,
) *pipelineDebugHandler {
	return &pipelineDebugHandler{
		service: debug, adminUserID: adminUserID,
		maxImageBytes: maxImageBytes, maxAudioBytes: maxAudioBytes, maxVideoBytes: maxVideoBytes,
	}
}

func (h *pipelineDebugHandler) Providers(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	response.Success(c, http.StatusOK, h.service.Providers(), "pipeline debug providers")
}

func (h *pipelineDebugHandler) Owners(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	owners, err := h.service.Owners(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, gin.H{"owners": owners}, "pipeline debug owners")
}

func (h *pipelineDebugHandler) AnalysisOverview(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	result, err := h.service.AnalysisOverview(c.Request.Context(), h.requestedOwner(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "pipeline analysis debug overview")
}

func (h *pipelineDebugHandler) AnalyzeFace(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	file, ok := readDebugUpload(c, h.maxImageBytes)
	if !ok {
		return
	}
	h.run(c, func() (service.PipelineDebugRun, error) {
		return h.service.AnalyzeFace(c.Request.Context(), file)
	})
}

func (h *pipelineDebugHandler) EmbedSpeaker(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	file, ok := readDebugUpload(c, h.maxAudioBytes)
	if !ok {
		return
	}
	h.run(c, func() (service.PipelineDebugRun, error) {
		return h.service.EmbedSpeaker(c.Request.Context(), file)
	})
}

func (h *pipelineDebugHandler) DetectActiveSpeaker(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	file, ok := readDebugUpload(c, h.maxVideoBytes)
	if !ok {
		return
	}
	var metadata struct {
		RecordingID  string                        `json:"recording_id"`
		PersonTracks []service.TemporalPersonTrack `json:"person_tracks"`
		Segments     []service.TranscriptSegment   `json:"segments"`
	}
	if err := json.Unmarshal([]byte(c.PostForm("metadata")), &metadata); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "metadata must be valid JSON")
		return
	}
	h.run(c, func() (service.PipelineDebugRun, error) {
		return h.service.DetectActiveSpeaker(c.Request.Context(), service.PipelineDebugActiveSpeakerInput{
			File: file, RecordingID: metadata.RecordingID,
			PersonTracks: metadata.PersonTracks, Segments: metadata.Segments,
		})
	})
}

func (h *pipelineDebugHandler) DenseOverview(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	result, err := h.service.DenseOverview(c.Request.Context(), h.requestedOwner(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "dense pipeline debug overview")
}

func (h *pipelineDebugHandler) DenseRecording(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	version, ok := debugProcessingVersion(c)
	if !ok {
		return
	}
	result, err := h.service.DenseRecording(
		c.Request.Context(), h.requestedOwner(c), c.Param("recordingID"), version,
	)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, result, "dense recording debug detail")
}

func (h *pipelineDebugHandler) DenseFace(c *gin.Context) {
	if !h.authorized(c) {
		return
	}
	version, ok := debugProcessingVersion(c)
	if !ok {
		return
	}
	image, err := h.service.DenseFace(
		c.Request.Context(), h.requestedOwner(c), c.Param("recordingID"),
		c.Param("trackID"), c.Param("observationID"), version,
	)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Data(http.StatusOK, image.MediaType, image.Content)
}

func (h *pipelineDebugHandler) requestedOwner(c *gin.Context) string {
	if ownerUserID := strings.TrimSpace(c.Query("owner_user_id")); ownerUserID != "" {
		return ownerUserID
	}
	return h.adminUserID
}

func debugProcessingVersion(c *gin.Context) (int, bool) {
	value := strings.TrimSpace(c.Query("processing_version"))
	if value == "" {
		return 0, true
	}
	version, err := strconv.Atoi(value)
	if err != nil || version < 1 {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "processing_version must be a positive integer")
		return 0, false
	}
	return version, true
}

func (h *pipelineDebugHandler) authorized(c *gin.Context) bool {
	principal, ok := utils.PrincipalFromContext(c.Request.Context())
	if !ok || principal.Subject != h.adminUserID {
		response.Error(c, http.StatusForbidden, "DEBUG_ADMIN_REQUIRED", "pipeline debug access requires the configured admin account")
		return false
	}
	return true
}

func (h *pipelineDebugHandler) run(c *gin.Context, call func() (service.PipelineDebugRun, error)) {
	result, err := call()
	if err != nil {
		_ = c.Error(err)
		response.Error(c, http.StatusBadGateway, "PIPELINE_DEBUG_ERROR", err.Error())
		return
	}
	response.Success(c, http.StatusOK, result, "pipeline debug run completed")
}

func (h *pipelineDebugHandler) fail(c *gin.Context, err error) {
	_ = c.Error(err)
	status := http.StatusBadGateway
	if errors.Is(err, service.ErrNotFound) {
		status = http.StatusNotFound
	}
	response.Error(c, status, "PIPELINE_DEBUG_ERROR", err.Error())
}

func readDebugUpload(c *gin.Context, maxBytes int64) (service.PipelineDebugFile, bool) {
	if maxBytes < 1 {
		maxBytes = 10 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+(1<<20))
	header, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "multipart field file is required")
		return service.PipelineDebugFile{}, false
	}
	file, err := header.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "uploaded file could not be opened")
		return service.PipelineDebugFile{}, false
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(content)) > maxBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "VALIDATION_ERROR", fmt.Sprintf("file exceeds the %d MiB debug limit", maxBytes>>20))
		return service.PipelineDebugFile{}, false
	}
	return service.PipelineDebugFile{
		FileName: safeUploadName(header), MediaType: uploadMediaType(header), Content: content,
	}, true
}

func safeUploadName(header *multipart.FileHeader) string {
	name := strings.TrimSpace(header.Filename)
	if name == "" {
		return "debug-upload"
	}
	return name
}

func uploadMediaType(header *multipart.FileHeader) string {
	if mediaType := strings.TrimSpace(header.Header.Get("Content-Type")); mediaType != "" {
		return mediaType
	}
	return "application/octet-stream"
}

var _ PipelineDebugHandler = (*pipelineDebugHandler)(nil)
