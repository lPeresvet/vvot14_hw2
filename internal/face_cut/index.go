package main

import (
	"context"
	"encoding/json"
	"log"
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

const inputDir = "images"

func Handler(ctx context.Context, request []byte) (*Response, error) {
	messages := &Messages{}

	if err := json.Unmarshal(request, messages); err != nil {
		return nil, err
	}

	//for _, msg := range messages.Messages {
	//	img, err := imaging.Open(path.Join(inputDir, msg.Details.Message.Body.ObjectID))
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	bounds := msg.Details.Message.Body.Bounds
	//
	//	rectcropimg := imaging.Crop(img, image.Rect(
	//		bounds.X, bounds.Y,
	//		bounds.X+bounds.Width,
	//		bounds.Y+bounds.Height))
	//
	//	if err := imaging.Save(rectcropimg, path.Join(inputDir, uuid.New().String())); err != nil {
	//		return nil, err
	//	}
	//}

	log.Println(messages)

	return &Response{
		StatusCode: 200,
	}, nil
}
