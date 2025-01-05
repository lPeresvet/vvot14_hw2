package main

import (
	"context"
	"encoding/json"
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

const inputDir = "/function/storage/images"

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
			return nil, err
		}

		bounds := task.Bounds

		rectcropimg := imaging.Crop(img, image.Rect(
			bounds.X, bounds.Y,
			bounds.X+bounds.Width,
			bounds.Y+bounds.Height))

		if err := imaging.Save(rectcropimg, path.Join(inputDir, uuid.New().String())); err != nil {
			return nil, err
		}
	}

	return &Response{
		StatusCode: 200,
	}, nil
}
