package main

import (
	"context"
	"log"
)

type Response struct {
	StatusCode int         `json:"statusCode"`
	Body       interface{} `json:"body"`
}

func Handler(ctx context.Context, request []byte) (*Response, error) {
	log.Println(string(request))

	return &Response{
		StatusCode: 200,
	}, nil
}
