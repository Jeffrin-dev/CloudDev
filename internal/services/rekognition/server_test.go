package rekognition

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetectLabels(t *testing.T) {
	rec := performRequest(t, "RekognitionService.DetectLabels")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != jsonContentType {
		t.Fatalf("expected content type %s, got %s", jsonContentType, got)
	}

	var resp struct {
		Labels []struct {
			Name       string  `json:"Name"`
			Confidence float64 `json:"Confidence"`
		} `json:"Labels"`
	}
	decodeBody(t, rec, &resp)

	if len(resp.Labels) != 3 {
		t.Fatalf("expected 3 labels, got %d", len(resp.Labels))
	}
	if resp.Labels[0].Name != "Person" || resp.Labels[0].Confidence != 99.1 {
		t.Fatalf("unexpected first label: %+v", resp.Labels[0])
	}
}

func TestDetectFaces(t *testing.T) {
	rec := performRequest(t, "RekognitionService.DetectFaces")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp struct {
		FaceDetails []struct {
			Confidence  float64 `json:"Confidence"`
			BoundingBox struct {
				Width float64 `json:"Width"`
			} `json:"BoundingBox"`
		} `json:"FaceDetails"`
	}
	decodeBody(t, rec, &resp)

	if len(resp.FaceDetails) != 1 {
		t.Fatalf("expected 1 face detail, got %d", len(resp.FaceDetails))
	}
	if resp.FaceDetails[0].Confidence != 99.5 {
		t.Fatalf("expected confidence 99.5, got %v", resp.FaceDetails[0].Confidence)
	}
	if resp.FaceDetails[0].BoundingBox.Width == 0 {
		t.Fatalf("expected non-zero bounding box width")
	}
}

func TestDetectText(t *testing.T) {
	rec := performRequest(t, "RekognitionService.DetectText")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp struct {
		TextDetections []struct {
			DetectedText string  `json:"DetectedText"`
			Confidence   float64 `json:"Confidence"`
			Type         string  `json:"Type"`
		} `json:"TextDetections"`
	}
	decodeBody(t, rec, &resp)

	if len(resp.TextDetections) != 1 {
		t.Fatalf("expected 1 text detection, got %d", len(resp.TextDetections))
	}
	if resp.TextDetections[0].DetectedText != "Hello World" || resp.TextDetections[0].Type != "LINE" {
		t.Fatalf("unexpected text detection: %+v", resp.TextDetections[0])
	}
}

func performRequest(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"Image":{"Bytes":""}}`))
	req.Header.Set("X-Amz-Target", target)
	req.Header.Set("Content-Type", jsonContentType)

	rec := httptest.NewRecorder()
	(&server{}).ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
