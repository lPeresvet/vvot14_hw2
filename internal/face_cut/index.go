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

func Handler(ctx context.Context, request []byte) (*Response, error) {
	messages := &Messages{}

	if err := json.Unmarshal(request, messages); err != nil {
		return nil, err
	}

	log.Println(messages)

	return &Response{
		StatusCode: 200,
	}, nil
}
