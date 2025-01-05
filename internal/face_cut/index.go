package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"image"
	"log"
	"path"
)

type Response struct {
	StatusCode int         `json:"statusCode"`
	Body       interface{} `json:"body"`
}

type Messages struct {
	Messages []struct {
		Details struct {
			Message struct {
				Body string `json:"body"`
			} `json:"message"`
		} `json:"details"`
	} `json:"messages"`
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

const (
	inputDir  = "/function/storage/images"
	outputDir = "/function/storage/faces"
)

func Handler(ctx context.Context, request []byte) (*Response, error) {
	messages := &Messages{}

	if err := json.Unmarshal(request, messages); err != nil {
		return nil, err
	}

	log.Println(messages)

	for _, msg := range messages.Messages {
		rowTask := msg.Details.Message.Body

		task := &CutterTask{}

		if err := json.Unmarshal([]byte(rowTask), task); err != nil {
			return nil, err
		}

		log.Println(task)

		img, err := imaging.Open(path.Join(inputDir, task.ObjectID))
		if err != nil {
			return nil, fmt.Errorf("failed to open input img: %s", err)
		}

		bounds := task.Bounds

		rectcropimg := imaging.Crop(img, image.Rect(
			bounds.X, bounds.Y,
			bounds.X+bounds.Width,
			bounds.Y+bounds.Height))

		if err := imaging.Save(rectcropimg, path.Join(outputDir, uuid.New().String()+".jpg")); err != nil {
			return nil, fmt.Errorf("failed to save img: %v", err)
		}
	}

	return &Response{
		StatusCode: 200,
	}, nil
}
