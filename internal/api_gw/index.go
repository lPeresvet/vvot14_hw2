package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
)

// Структура запроса API Gateway v1
type APIGatewayRequest struct {
	OperationID string `json:"operationId"`
	Resource    string `json:"resource"`

	HTTPMethod string `json:"httpMethod"`

	Path           string            `json:"path"`
	PathParameters map[string]string `json:"pathParameters"`

	Headers           map[string]string   `json:"headers"`
	MultiValueHeaders map[string][]string `json:"multiValueHeaders"`

	QueryStringParameters           map[string]string   `json:"queryStringParameters"`
	MultiValueQueryStringParameters map[string][]string `json:"multiValueQueryStringParameters"`

	Parameters           map[string]string   `json:"parameters"`
	MultiValueParameters map[string][]string `json:"multiValueParameters"`

	Body            string `json:"body"`
	IsBase64Encoded bool   `json:"isBase64Encoded,omitempty"`

	RequestContext interface{} `json:"requestContext"`
}

// Структура ответа API Gateway v1
type APIGatewayResponse struct {
	StatusCode        int                 `json:"statusCode"`
	Headers           map[string]string   `json:"headers"`
	MultiValueHeaders map[string][]string `json:"multiValueHeaders"`
	Body              []byte              `json:"body"`
	IsBase64Encoded   bool                `json:"isBase64Encoded,omitempty"`
}

const (
	facesDir  = "/function/storage/faces"
	imagesDir = "/function/storage/images"
)

func Handler(ctx context.Context, event *APIGatewayRequest) (*APIGatewayResponse, error) {
	name := event.QueryStringParameters["face"]
	dir := facesDir

	if name == "" {
		name = event.QueryStringParameters["image"]
		dir = imagesDir
	}

	if name == "" {
		return &APIGatewayResponse{
			StatusCode: http.StatusNotFound,
		}, nil
	}

	// В журнале будет напечатано название HTTP-метода, с помощью которого осуществлен запрос, а также путь
	fmt.Println(event.HTTPMethod, name)

	fileBytes, err := ioutil.ReadFile(path.Join(dir, name))
	if err != nil {
		return &APIGatewayResponse{
			StatusCode: http.StatusNotFound,
		}, nil
	}

	// Тело ответа.
	return &APIGatewayResponse{
		StatusCode:      200,
		Body:            fileBytes,
		Headers:         map[string]string{"Content-type": "image/jpeg"},
		IsBase64Encoded: true,
	}, nil
}
