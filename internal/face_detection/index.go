package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

	queueName := "mq_example_golang_sdk"

	queue, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: &queueName,
	})
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Queue created, URL: " + *queue.QueueUrl)

	msg := "test message"

	send, err := client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    queue.QueueUrl,
		MessageBody: &msg,
	})
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Message sent, ID: " + *send.MessageId)

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
