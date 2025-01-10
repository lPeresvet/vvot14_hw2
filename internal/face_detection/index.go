package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
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

type CutterTask struct {
	Bounds   FaceBounds `json:"bounds"`
	ObjectID string     `json:"objectID"`
}

type FaceBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type APIRequest struct {
	Providers string `json:"providers"`
	FileUrl   string `json:"file_url"`
}

type APIResponse struct {
	Amazon struct {
		Items []struct {
			BoundingBox struct {
				XMin float64 `json:"x_min"`
				XMax float64 `json:"x_max"`
				YMin float64 `json:"y_min"`
				YMax float64 `json:"y_max"`
			} `json:"bounding_box"`
		} `json:"items"`
	} `json:"amazon"`
}

const (
	apiURL         = "https://api.edenai.run/v2/image/face_detection"
	gwImagePattern = "https://%s/?image=%s"
	imgDir         = "/function/storage/images"
)

func Handler(ctx context.Context, request []byte) (*Response, error) {
	messages := &Messages{}

	if err := json.Unmarshal(request, messages); err != nil {
		return nil, err
	}

	log.Println(messages)

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:           "https://message-queue.api.cloud.yandex.net",
			SigningRegion: "ru-central1",
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		log.Fatalln(err)
	}

	client := sqs.NewFromConfig(cfg)

	domain := os.Getenv("API_GW_URL")

	for _, message := range messages.Messages {
		apiReq := &APIRequest{
			Providers: "amazon",
			FileUrl:   fmt.Sprintf(gwImagePattern, domain, message.Details.ObjectId),
		}
		jsonStr, err := json.Marshal(apiReq)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonStr))
		req.Header.Set("authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiMzJkZjFhZjUtZTgwOS00MmU4LWIwOWItZTU4M2Y5MzRkZTI3IiwidHlwZSI6ImFwaV90b2tlbiJ9.UMGUBzGKBl7gLdZJ5JIsp_MUI-BHTH4LweO7aqBu7R4")
		req.Header.Set("Content-Type", "application/json")
		httpClient := &http.Client{}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		fmt.Println("response Status:", resp.Status)
		fmt.Println("response Headers:", resp.Header)
		body, _ := io.ReadAll(resp.Body)

		apiResp := &APIResponse{}

		if err := json.Unmarshal(body, apiResp); err != nil {
			return nil, err
		}
		fmt.Println(apiResp)

		maxX, maxY, err := getImageDimensions(path.Join(imgDir, message.Details.ObjectId))
		if err != nil {
			return nil, fmt.Errorf("failed to get image size: %w", err)
		}
		fmt.Println(maxX, maxY)
		maxXF := float64(maxX)
		maxYF := float64(maxY)
		for _, bound := range apiResp.Amazon.Items {
			x := int(math.Round(maxXF * bound.BoundingBox.XMin))
			y := int(math.Round(maxYF * bound.BoundingBox.YMin))
			task := CutterTask{
				Bounds: FaceBounds{
					X:      x,
					Y:      y,
					Width:  int(math.Round(maxXF*bound.BoundingBox.XMax)) - x,
					Height: int(math.Round(maxYF*bound.BoundingBox.YMax)) - y,
				},
				ObjectID: messages.Messages[0].Details.ObjectId,
			}

			msgBytes, err := json.Marshal(task)
			if err != nil {
				return nil, err
			}

			msg := string(msgBytes)
			queueURL := os.Getenv("QUEUE_URL")

			send, err := client.SendMessage(ctx, &sqs.SendMessageInput{
				QueueUrl:    &queueURL,
				MessageBody: &msg,
			})
			if err != nil {
				log.Fatalln(err)
			}

			fmt.Printf("Message %s sent, ID: %v", msg, *send.MessageId)
		}
	}

	return &Response{
		StatusCode: 200,
	}, nil
}

func getImageDimensions(imagePath string) (int, int, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return 0, 0, fmt.Errorf("не удалось открыть файл: %w", err)
	}
	defer file.Close()

	img, err := jpeg.Decode(file)
	if err != nil {
		return 0, 0, fmt.Errorf("не удалось декодировать изображение: %w", err)
	}

	bounds := img.Bounds()

	width := bounds.Max.X
	height := bounds.Max.Y

	return width, height, nil
}
