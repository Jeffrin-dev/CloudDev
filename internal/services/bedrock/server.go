package bedrock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const jsonContentType = "application/json"

type foundationModel struct {
	ModelID          string   `json:"modelId"`
	ModelName        string   `json:"modelName"`
	ProviderName     string   `json:"providerName"`
	InputModalities  []string `json:"inputModalities"`
	OutputModalities []string `json:"outputModalities"`
}

var mockModels = []foundationModel{
	{ModelID: "anthropic.claude-3-sonnet-20240229-v1:0", ModelName: "Claude 3 Sonnet", ProviderName: "Anthropic", InputModalities: []string{"TEXT"}, OutputModalities: []string{"TEXT"}},
	{ModelID: "anthropic.claude-instant-v1", ModelName: "Claude Instant", ProviderName: "Anthropic", InputModalities: []string{"TEXT"}, OutputModalities: []string{"TEXT"}},
	{ModelID: "amazon.titan-text-express-v1", ModelName: "Titan Text Express", ProviderName: "Amazon", InputModalities: []string{"TEXT"}, OutputModalities: []string{"TEXT"}},
	{ModelID: "meta.llama2-13b-chat-v1", ModelName: "Llama 2 13B Chat", ProviderName: "Meta", InputModalities: []string{"TEXT"}, OutputModalities: []string{"TEXT"}},
}

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), newServer())
}

func newServer() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/foundation-models":
			listFoundationModels(w)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/foundation-models/"):
			getFoundationModel(w, r)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/model/"):
			invokeModelRoute(w, r)
		default:
			writeError(w, http.StatusNotFound, "ResourceNotFoundException", "Not found")
		}
	})
}

func listFoundationModels(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"modelSummaries": mockModels})
}

func getFoundationModel(w http.ResponseWriter, r *http.Request) {
	modelID := strings.TrimPrefix(r.URL.Path, "/foundation-models/")
	for _, model := range mockModels {
		if model.ModelID == modelID {
			writeJSON(w, http.StatusOK, model)
			return
		}
	}
	writeError(w, http.StatusNotFound, "ResourceNotFoundException", "Model not found")
}

func invokeModelRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/model/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "ResourceNotFoundException", "Not found")
		return
	}
	modelID, action := parts[0], parts[1]
	if action != "invoke" && action != "invoke-with-response-stream" {
		writeError(w, http.StatusNotFound, "ResourceNotFoundException", "Not found")
		return
	}
	invokeModel(w, r, modelID)
}

func invokeModel(w http.ResponseWriter, r *http.Request, modelID string) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "ValidationException", "Invalid JSON body")
		return
	}

	switch {
	case strings.HasPrefix(modelID, "anthropic."):
		writeJSON(w, http.StatusOK, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "Mock response from Claude: "}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 20},
		})
	case strings.HasPrefix(modelID, "amazon.titan-"):
		writeJSON(w, http.StatusOK, map[string]any{
			"results": []map[string]any{{"outputText": "Mock response from Titan: ", "tokenCount": 20, "completionReason": "FINISH"}},
		})
	case strings.HasPrefix(modelID, "meta.llama"):
		writeJSON(w, http.StatusOK, map[string]any{
			"generation":             "Mock response from Llama: ",
			"prompt_token_count":     10,
			"generation_token_count": 20,
			"stop_reason":            "stop",
		})
	default:
		writeError(w, http.StatusNotFound, "ResourceNotFoundException", "Model not found")
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"__type": code, "message": message})
}
