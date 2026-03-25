package rekognition

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const jsonContentType = "application/x-amz-json-1.1"

func Start(port int) error {
	return http.ListenAndServe(fmt.Sprintf(":%d", port), &server{})
}

type server struct{}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "InvalidParameterException", "Only POST is supported")
		return
	}

	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Missing X-Amz-Target header")
		return
	}

	if err := consumeJSON(r.Body); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterException", "Invalid JSON body")
		return
	}

	switch target {
	case "RekognitionService.DetectLabels":
		writeJSON(w, http.StatusOK, map[string]any{
			"Labels": []map[string]any{
				{"Name": "Person", "Confidence": 99.1},
				{"Name": "Outdoors", "Confidence": 87.3},
				{"Name": "Nature", "Confidence": 76.5},
			},
		})
	case "RekognitionService.DetectFaces":
		writeJSON(w, http.StatusOK, map[string]any{
			"FaceDetails": []map[string]any{
				{
					"BoundingBox": map[string]any{
						"Width":  0.24,
						"Height": 0.35,
						"Left":   0.41,
						"Top":    0.18,
					},
					"Confidence": 99.5,
				},
			},
		})
	case "RekognitionService.DetectText":
		writeJSON(w, http.StatusOK, map[string]any{
			"TextDetections": []map[string]any{
				{"DetectedText": "Hello World", "Confidence": 98.2, "Type": "LINE"},
			},
		})
	case "RekognitionService.CompareFaces":
		writeJSON(w, http.StatusOK, map[string]any{
			"FaceMatches": []map[string]any{
				{"Similarity": 95.5},
			},
		})
	case "RekognitionService.IndexFaces":
		faceID, err := randomUUID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "InternalFailure", "Failed to generate FaceId")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"FaceRecords": []map[string]any{
				{
					"Face": map[string]any{
						"FaceId":     faceID,
						"Confidence": 99.5,
					},
				},
			},
		})
	case "RekognitionService.SearchFacesByImage":
		writeJSON(w, http.StatusOK, map[string]any{
			"FaceMatches": []map[string]any{
				{
					"Similarity": 92.3,
					"Face": map[string]any{
						"FaceId": "mock-face-1",
					},
				},
			},
		})
	default:
		writeError(w, http.StatusBadRequest, "UnknownOperationException", "Unknown X-Amz-Target operation")
	}
}

func consumeJSON(body io.ReadCloser) error {
	defer body.Close()
	var payload map[string]any
	dec := json.NewDecoder(body)
	if err := dec.Decode(&payload); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"__type":  code,
		"message": message,
	})
}

func randomUUID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16],
	), nil
}
