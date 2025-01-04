package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Kagami/go-face"
	"log"
	"path/filepath"
	"time"
)

const dataDir = "images"

type Messages struct {
	Messages []struct {
		EventMetadata struct {
			EventId        string    `json:"event_id"`
			EventType      string    `json:"event_type"`
			CreatedAt      time.Time `json:"created_at"`
			TracingContext struct {
				TraceId      string `json:"trace_id"`
				SpanId       string `json:"span_id"`
				ParentSpanId string `json:"parent_span_id"`
			} `json:"tracing_context"`
			CloudId  string `json:"cloud_id"`
			FolderId string `json:"folder_id"`
		} `json:"event_metadata"`
		Details struct {
			BucketId string `json:"bucket_id"`
			ObjectId string `json:"object_id"`
		} `json:"details"`
	} `json:"messages"`
}

type Response struct {
	StatusCode int         `json:"statusCode"`
	Body       interface{} `json:"body"`
}

func Handler(ctx context.Context, request []byte) (*Response, error) {

	messages := &Messages{}

	if err := json.Unmarshal(request, messages); err != nil {
		return nil, err
	}

	log.Println(messages)

	rec, err := face.NewRecognizer(filepath.Join(dataDir, "models"))
	if err != nil {
		log.Printf("Can't init face recognizer: %v", err)
	}
	// Free the resources when you're finished.
	defer rec.Close()

	for _, msg := range messages.Messages {
		faceBounds, err := detectFaceBounds(filepath.Join(dataDir, msg.Details.ObjectId), rec)
		if err != nil {
			log.Printf("Can't detect faces: %v", err)
		}

		log.Println(faceBounds)
	}

	return &Response{
		StatusCode: 200,
		Body:       fmt.Sprintf("Hello, %s", "rew"),
	}, nil
}

type FaceBounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

// detectFaceBounds - Функция для распознавания границ лица на изображении
func detectFaceBounds(imagePath string, recognizer *face.Recognizer) ([]FaceBounds, error) {

	faces, err := recognizer.RecognizeFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка распознавания лиц: %w", err)
	}

	var faceBounds []FaceBounds
	for _, face := range faces {
		faceBounds = append(faceBounds, FaceBounds{
			X:      face.Rectangle.Min.X,
			Y:      face.Rectangle.Min.Y,
			Width:  face.Rectangle.Max.X - face.Rectangle.Min.X,
			Height: face.Rectangle.Max.Y - face.Rectangle.Min.Y,
		})
	}
	return faceBounds, nil
}
