package decision

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	mockInputTokens  = 10
	mockOutputTokens = 2

	noulProbability  = 0.75
	choiceConfidence = 0.9
	scoreConfidence  = 0.8
	boxConfidence    = 0.9
)

type question struct {
	Type     string          `json:"type"`
	Name     string          `json:"name"`
	Criteria json.RawMessage `json:"criteria"`
	Choices  []choice        `json:"choices"`
	Levels   []level         `json:"levels"`
}

type choice struct {
	Value any `json:"value"`
}

type level struct {
	Label string `json:"label"`
}

type systemOneRequest struct {
	Model     string              `json:"model"`
	State     json.RawMessage     `json:"state"`
	Questions map[string]question `json:"questions"`
}

type decisionsRequest struct {
	Model     string          `json:"model"`
	Input     json.RawMessage `json:"input"`
	Questions []question      `json:"questions"`
}

// SetupRoutes registers the System One and OpenAI Decisions mock endpoints.
// Both handlers return deterministic values so e2e assertions can compare the
// response shape without a live model.
func SetupRoutes(server *gin.Engine) {
	server.POST("/v1/systemone", HandleSystemOne)
	server.POST("/v1/decisions", HandleDecisions)
}

func HandleSystemOne(ctx *gin.Context) {
	var request systemOneRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid System One request"})
		return
	}
	if len(request.Questions) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "at least one question is required"})
		return
	}
	answers := make(map[string]any, len(request.Questions))
	for name, question := range request.Questions {
		answer, err := systemOneAnswer(question)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		answers[name] = answer
	}
	ctx.JSON(http.StatusOK, gin.H{
		"model":   modelOrMock(request.Model),
		"answers": answers,
		"usage":   gin.H{"input_tokens": mockInputTokens, "output_tokens": mockOutputTokens},
	})
}

func HandleDecisions(ctx *gin.Context) {
	var request decisionsRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid Decisions request"})
		return
	}
	if len(request.Questions) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "at least one question is required"})
		return
	}
	answers := make([]any, 0, len(request.Questions))
	for _, question := range request.Questions {
		answer, err := decisionsAnswer(question)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		answers = append(answers, answer)
	}
	ctx.JSON(http.StatusOK, gin.H{
		"model":   modelOrMock(request.Model),
		"answers": answers,
		"usage": gin.H{
			"input_tokens":          mockInputTokens,
			"input_tokens_details":  gin.H{"cached_tokens": 0, "cache_write_tokens": 0},
			"output_tokens":         mockOutputTokens,
			"output_tokens_details": gin.H{"reasoning_tokens": 0},
			"total_tokens":          mockInputTokens + mockOutputTokens,
		},
	})
}

func systemOneAnswer(question question) (map[string]any, error) {
	switch question.Type {
	case "noul":
		return map[string]any{"type": "noul", "noul": noulProbability}, nil
	case "choice":
		criteria, err := criteriaKeys(question.Criteria)
		if err != nil || len(criteria) == 0 {
			return nil, err
		}
		return map[string]any{
			"type":          "choice",
			"choice":        criteria[0],
			"confidence":    choiceConfidence,
			"probabilities": choiceProbabilities(criteria),
		}, nil
	case "score":
		levels, err := scoreLevels(question.Criteria)
		if err != nil || len(levels) == 0 {
			return nil, err
		}
		return map[string]any{
			"type":          "score",
			"score":         0.5,
			"confidence":    scoreConfidence,
			"legend":        scoreLegend(levels),
			"probabilities": scoreProbabilities(len(levels)),
		}, nil
	case "box":
		return map[string]any{"type": "box", "box": mockBox(), "confidence": boxConfidence}, nil
	default:
		return nil, errUnsupported(question.Type)
	}
}

func decisionsAnswer(question question) (map[string]any, error) {
	answer := map[string]any{"type": question.Type}
	if question.Name != "" {
		answer["name"] = question.Name
	}
	switch question.Type {
	case "predicate":
		answer["probability"] = noulProbability
	case "choice":
		if len(question.Choices) == 0 {
			return nil, errUnsupported("choice without choices")
		}
		probabilities := make([]any, 0, len(question.Choices))
		for i, choice := range question.Choices {
			probability := 0.1 / float64(max(1, len(question.Choices)-1))
			if i == 0 {
				probability = choiceConfidence
			}
			probabilities = append(probabilities, gin.H{"value": choice.Value, "probability": probability})
		}
		answer["choice"] = question.Choices[0].Value
		answer["probabilities"] = probabilities
		answer["confidence"] = choiceConfidence
	case "score":
		if len(question.Levels) == 0 {
			return nil, errUnsupported("score without levels")
		}
		probabilities := make([]any, 0, len(question.Levels))
		probability := 1 / float64(len(question.Levels))
		for i, level := range question.Levels {
			probabilities = append(probabilities, gin.H{"value": i, "label": level.Label, "probability": probability})
		}
		answer["score"] = 0.5
		answer["probabilities"] = probabilities
		answer["confidence"] = scoreConfidence
	case "box":
		answer["box"] = mockBox()
		answer["confidence"] = boxConfidence
	default:
		return nil, errUnsupported(question.Type)
	}
	return answer, nil
}

func criteriaKeys(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, errUnsupported("choice without criteria")
	}
	var criteria map[string]json.RawMessage
	if err := json.Unmarshal(raw, &criteria); err != nil {
		return nil, errUnsupported("choice criteria must be an object")
	}
	keys := make([]string, 0, len(criteria))
	for key := range criteria {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func scoreLevels(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, errUnsupported("score without criteria")
	}
	var criteria []json.RawMessage
	if err := json.Unmarshal(raw, &criteria); err != nil {
		return nil, errUnsupported("score criteria must be an array")
	}
	levels := make([]string, 0, len(criteria))
	for _, criterion := range criteria {
		var label string
		if err := json.Unmarshal(criterion, &label); err != nil {
			label = string(criterion)
		}
		levels = append(levels, label)
	}
	return levels, nil
}

func choiceProbabilities(keys []string) map[string]float64 {
	probabilities := make(map[string]float64, len(keys))
	rest := 0.1 / float64(max(1, len(keys)-1))
	for i, key := range keys {
		if i == 0 {
			probabilities[key] = choiceConfidence
		} else {
			probabilities[key] = rest
		}
	}
	return probabilities
}

func scoreLegend(levels []string) map[string]string {
	legend := make(map[string]string, len(levels))
	for i, level := range levels {
		legend[strconv.Itoa(i)] = level
	}
	return legend
}

func scoreProbabilities(levelCount int) map[string]float64 {
	probabilities := make(map[string]float64, levelCount)
	probability := 1 / float64(levelCount)
	for i := 0; i < levelCount; i++ {
		probabilities[strconv.Itoa(i)] = probability
	}
	return probabilities
}

func mockBox() gin.H {
	return gin.H{"top": 0.1, "bottom": 0.2, "left": 0.3, "right": 0.4}
}

func modelOrMock(model string) string {
	if model == "" {
		return "jev-mock"
	}
	return model
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type unsupportedError string

func (e unsupportedError) Error() string {
	return "unsupported question: " + string(e)
}

func errUnsupported(detail string) error {
	return unsupportedError(detail)
}
