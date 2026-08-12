package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/arham/ai-second-brain/internal/config"
	"github.com/arham/ai-second-brain/internal/service"
)

type OpenAIAnalyzer struct {
	baseURL string
	apiKey  string
	model   string
	detail  string
	client  *http.Client
}

func NewOpenAI(cfg config.VisionConfig) (*OpenAIAnalyzer, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" ||
		strings.TrimSpace(cfg.Model) == "" || cfg.Timeout <= 0 {
		return nil, fmt.Errorf("OpenAI vision base URL, API key, model, and timeout are required")
	}
	return &OpenAIAnalyzer{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		detail:  cfg.Detail,
		client:  &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (a *OpenAIAnalyzer) Provider() string { return "openai" }
func (a *OpenAIAnalyzer) Model() string    { return a.model }

func (a *OpenAIAnalyzer) Analyze(
	ctx context.Context,
	input service.VisualAnalysisInput,
) (service.VisualAnalysis, error) {
	if len(input.Frames) == 0 {
		return service.VisualAnalysis{}, fmt.Errorf("at least one video frame is required")
	}
	content := make([]map[string]any, 0, len(input.Frames)+1)
	timestamps := make([]string, 0, len(input.Frames))
	for _, frame := range input.Frames {
		timestamps = append(timestamps, fmt.Sprintf(
			"%s at %ss (%s)", frame.FrameID,
			strconv.FormatFloat(frame.Timestamp, 'f', 2, 64), frame.SelectionReason,
		))
	}
	content = append(content, map[string]any{
		"type": "input_text",
		"text": "Analyze the following ordered video frames at timestamps " +
			strings.Join(timestamps, ", ") + `. Return exactly one observation for each
		supplied frame, in the same order.

		Analyze each frame carefully and produce a dense, self-contained, visually
		grounded description.

		For every frame:

		1. Identify the primary subject and all important visible people, animals,
		vehicles, objects, structures, surfaces, documents, screens, signs, products,
		and environmental elements.

		2. Describe relevant visible attributes, including type, color, material, size,
		quantity, condition, state, orientation, position, and distinguishing
		features.

		3. Describe the foreground, middle ground, and background. Include important
		spatial relationships between visible entities, such as on, inside, beside,
		behind, in front of, attached to, held by, worn by, displayed on, or near.

		4. Transcribe all readable text exactly as it appears, including text on signs,
		screens, documents, labels, packaging, clothing, vehicles, buildings,
		posters, handwritten notes, and other visible surfaces. Do not omit text
		merely because it is small or not central to the scene.

		5. For every item returned in text_detected, identify the object, surface, or
		visual region on which the text appears. Connect the text to that object in
		the summary using a natural-language relationship sentence.

		Use the general pattern:
		"A/An <visible object attributes> <object or surface> <contains, shows,
		bears, reads, or displays> the exact text '<detected text>' on <specific
		region, if known>."

		Choose the relationship verb that most accurately describes the visual
		evidence.

		6. The summary must include every item returned in text_detected. Never report
		readable text only as an isolated value when its containing object or surface
		can be identified.

		7. If the containing object cannot be identified, write:
		"An unidentified surface in the <position> of the frame displays the exact
		text '<detected text>'."

		8. If text is only partially readable, preserve the visible characters exactly,
		replace each unreadable character with "?", and explicitly state that the
		reading is partial or uncertain. Do not correct spelling, complete missing
		text, or guess hidden characters.

		9. Identify the visible activity, action, event, or scenario. Describe who or
		what is performing the action and which visible entities are involved.

		10. Make a conservative location guess using only visible evidence. Distinguish
			between directly observed environmental details and inferred location.
			Describe visible lighting, indoor or outdoor setting, terrain, road
			conditions, and visual weather cues when present, but do not claim external
			facts that cannot be seen.

		11. Write the summary as graph-friendly factual sentences. Use explicit entity
			names and relationships instead of ambiguous pronouns. Include:
			- The principal subject and its distinguishing features.
			- Relevant surrounding objects and environmental details.
			- Foreground, middle-ground, and background context.
			- Important spatial and interaction relationships.
			- Every detected text item connected to its containing object or surface.
			- The visible activity or event.

		12. Do not infer ownership, identity, intent, registration information, exact
			address, or other facts that are not visually supported. When uncertain,
			describe the uncertainty rather than inventing information.

		13. Do not omit a visible detail merely because it appears unimportant. However,
			do not repeat identical facts unnecessarily within the same observation.

		14. Label people anonymously as person-1, person-2, and so on. Never infer a
			person's identity from appearance. Preserve OCR exactly, use ? for each
			unreadable character, and store uncertainty instead of guessing.

		Use each supplied timestamp as start_time and the next supplied timestamp as
		end_time. For the final frame, use the configured window duration.

		All confidence values must be between 0 and 1 and should reflect visibility,
		image quality, occlusion, blur, and ambiguity.`,
	})
	for _, frame := range input.Frames {
		mediaType := strings.TrimSpace(frame.MediaType)
		if mediaType == "" {
			mediaType = "image/jpeg"
		}
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(frame.Image),
			"detail":    a.detail,
		})
	}

	payload := map[string]any{
		"model": a.model,
		"input": []map[string]any{{
			"role":    "user",
			"content": content,
		}},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "video_visual_analysis",
				"strict": true,
				"schema": visualAnalysisSchema(),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("encode vision request: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, a.baseURL+"/responses", bytes.NewReader(body),
	)
	if err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("create vision request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := a.client.Do(request)
	if err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("call vision API: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("read vision response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return service.VisualAnalysis{}, fmt.Errorf(
			"vision API returned %d: %s", response.StatusCode, boundedMessage(responseBody),
		)
	}
	var result struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("decode vision response: %w", err)
	}
	var outputText string
	for _, output := range result.Output {
		for _, item := range output.Content {
			if item.Type == "output_text" && strings.TrimSpace(item.Text) != "" {
				outputText = item.Text
				break
			}
		}
	}
	if outputText == "" {
		return service.VisualAnalysis{}, fmt.Errorf("vision response contained no structured output")
	}
	var analysis service.VisualAnalysis
	if err := json.Unmarshal([]byte(outputText), &analysis); err != nil {
		return service.VisualAnalysis{}, fmt.Errorf("decode structured visual analysis: %w", err)
	}
	if len(analysis.Observations) != len(input.Frames) {
		return service.VisualAnalysis{}, fmt.Errorf(
			"vision analysis returned %d observations for %d frames",
			len(analysis.Observations), len(input.Frames),
		)
	}
	normalizeObservations(&analysis, input)
	return analysis, nil
}

func normalizeObservations(
	analysis *service.VisualAnalysis,
	input service.VisualAnalysisInput,
) {
	for index := range analysis.Observations {
		observation := &analysis.Observations[index]
		if index < len(input.Frames) {
			observation.StartTime = input.Frames[index].Timestamp
			observation.FrameID = input.Frames[index].FrameID
			observation.SelectionReason = input.Frames[index].SelectionReason
		}
		if index+1 < len(input.Frames) {
			observation.EndTime = input.Frames[index+1].Timestamp
		} else {
			window := input.WindowDuration
			if window <= 0 {
				window = 5
			}
			observation.EndTime = observation.StartTime + window
		}
		if observation.Confidence != nil {
			if *observation.Confidence < 0 {
				*observation.Confidence = 0
			}
			if *observation.Confidence > 1 {
				*observation.Confidence = 1
			}
		}
	}
}

func visualAnalysisSchema() map[string]any {
	confidence := map[string]any{"type": []string{"number", "null"}, "minimum": 0, "maximum": 1}
	objectSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"object_id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
			"attributes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"position":   map[string]any{"type": "string"}, "state": map[string]any{"type": "string"},
			"confidence": confidence,
		},
		"required": []string{"object_id", "name", "attributes", "position", "state", "confidence"},
	}
	textSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"text": map[string]any{"type": "string"}, "surface": map[string]any{"type": "string"},
			"region": map[string]any{"type": "string"}, "reading_status": map[string]any{"type": "string", "enum": []string{"complete", "partial", "unreadable"}},
			"confidence": confidence,
		},
		"required": []string{"text", "surface", "region", "reading_status", "confidence"},
	}
	sceneSchema := strictObject(map[string]any{
		"setting": map[string]any{"type": "string"}, "lighting": map[string]any{"type": "string"},
		"foreground": map[string]any{"type": "string"}, "background": map[string]any{"type": "string"},
	}, "setting", "lighting", "foreground", "background")
	personSchema := strictObject(map[string]any{
		"visual_label": map[string]any{"type": "string"}, "appearance": map[string]any{"type": "string"},
		"position": map[string]any{"type": "string"}, "action": map[string]any{"type": "string"}, "confidence": confidence,
	}, "visual_label", "appearance", "position", "action", "confidence")
	relationSchema := strictObject(map[string]any{
		"source": map[string]any{"type": "string"}, "predicate": map[string]any{"type": "string"},
		"target": map[string]any{"type": "string"}, "confidence": confidence,
	}, "source", "predicate", "target", "confidence")
	observationSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"observation_id":   map[string]any{"type": "string"},
			"frame_id":         map[string]any{"type": "string"},
			"start_time":       map[string]any{"type": "number"},
			"end_time":         map[string]any{"type": "number"},
			"selection_reason": map[string]any{"type": "string", "enum": []string{"periodic", "first", "final", "scene_change", "motion_start", "motion_peak", "motion_end", "text_change"}},
			"scene":            sceneSchema,
			"people":           map[string]any{"type": "array", "items": personSchema},
			"objects":          map[string]any{"type": "array", "items": objectSchema},
			"text_detected":    map[string]any{"type": "array", "items": textSchema},
			"relations":        map[string]any{"type": "array", "items": relationSchema},
			"activity":         map[string]any{"type": "string"},
			"location_guess":   map[string]any{"type": "string"},
			"summary":          map[string]any{"type": "string"},
			"confidence":       confidence,
		},
		"required": []string{
			"observation_id", "frame_id", "start_time", "end_time", "selection_reason", "scene", "people", "objects", "text_detected", "relations", "activity",
			"location_guess", "summary", "confidence",
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"observations": map[string]any{
				"type": "array", "items": observationSchema,
			},
		},
		"required": []string{"observations"},
	}
}

func strictObject(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}

func boundedMessage(body []byte) string {
	message := strings.TrimSpace(string(body))
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}

var _ service.VisualAnalyzer = (*OpenAIAnalyzer)(nil)
