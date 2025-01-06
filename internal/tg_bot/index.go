package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	yc "github.com/ydb-platform/ydb-go-yc-metadata"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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
	Body              string              `json:"body"`
	IsBase64Encoded   bool                `json:"isBase64Encoded,omitempty"`
}

type Photo struct {
	ID       string `json:"file_id"`
	UniqueID string `json:"file_unique_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type Request struct {
	UpdateID int64 `json:"update_id"`
	Message  struct {
		ID   int64 `json:"message_id"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text  string  `json:"text"`
		Photo []Photo `json:"photo,omitempty"`
	} `json:"message"`
}

type GetFilePathResp struct {
	Result struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}

type OCRRequest struct {
	MimeType      string   `json:"mimeType"`
	LanguageCodes []string `json:"languageCodes"`
	Model         string   `json:"model"`
	Content       string   `json:"content"`
}

type OCRResp struct {
	Result struct {
		TextAnnotation struct {
			FullText string `json:"fullText"`
		} `json:"textAnnotation"`
	} `json:"result"`
}

type SendMsgReq struct {
	ChatId           int64  `json:"chat_id"`
	Text             string `json:"text"`
	ReplyToMessageId int64  `json:"reply_to_message_id"`
	ParseMode        string `json:"parse_mode,omitempty"`
}

type YaGPTRequest struct {
	ModelUri          string                 `json:"modelUri"`
	CompletionOptions YaGPTRequestOptions    `json:"completionOptions"`
	Messages          []YaGPTRequestMessages `json:"messages"`
}

type YaGPTRequestOptions struct {
	Stream      bool    `json:"stream"`
	Temperature float64 `json:"temperature"`
	MaxTokens   string  `json:"maxTokens"`
}

type YaGPTRequestMessages struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type YaGPTResponse struct {
	Result struct {
		Alternatives []struct {
			Message struct {
				Role string `json:"role"`
				Text string `json:"text"`
			} `json:"message"`
			Status string `json:"status"`
		} `json:"alternatives"`
		Usage struct {
			InputTextTokens  string `json:"inputTextTokens"`
			CompletionTokens string `json:"completionTokens"`
			TotalTokens      string `json:"totalTokens"`
		} `json:"usage"`
		ModelVersion string `json:"modelVersion"`
	} `json:"result"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

const (
	getFilePathURLPattern  = "https://api.telegram.org/bot%s/getFile?file_id=%s"
	sendMsgURLPattern      = "https://api.telegram.org/bot%s/sendMessage"
	downloadFileURLPattern = "https://api.telegram.org/file/bot%s"
	localPath              = "/function/storage/images"
	ocrURL                 = "https://ocr.api.cloud.yandex.net/ocr/v1/recognizeText"
	catalog                = "b1g163vdicpkeevao9ga"
	yaGPTURL               = "https://llm.api.cloud.yandex.net/foundationModels/v1/completion"
	maxMessageLen          = 4096
)

func Handler(ctx context.Context, event *APIGatewayRequest) (*APIGatewayResponse, error) {
	log.Print("received message")

	ydbURL := os.Getenv("YDB_URL")
	db, err := ydb.Open(ctx,
		ydbURL,
		yc.WithInternalCA(),
		yc.WithCredentials(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to init db connection: %v", err)
	}
	defer func() {
		_ = db.Close(ctx)
	}()

	req := &Request{}

	if err := json.Unmarshal([]byte(event.Body), &req); err != nil {
		return nil, fmt.Errorf("an error has occurred when parsing body: %w", err)
	}

	//log.Println(event)

	if req.Message.Text == "" {
		if err := sendReply(req.Message.Chat.ID, "Ошибка", req.Message.ID); err != nil {
			return nil, fmt.Errorf("failed to send reply: %w", err)
		}

		return &APIGatewayResponse{
			StatusCode: 200,
		}, nil
	}

	cmds := strings.Split(req.Message.Text, " ")

	switch cmds[0] {
	case "/getface":
		if err := handleGetFace(ctx, db); err != nil {
			return nil, fmt.Errorf("failed to handle /getface: %v", err)
		}
	case "/find":
		if len(cmds) < 2 {
			if err := sendReply(req.Message.Chat.ID, "Incorrect command", req.Message.ID); err != nil {
				return nil, fmt.Errorf("failed to send reply: %w", err)
			}
		}
		if err := handleFindName(ctx, db, cmds[1]); err != nil {
			return nil, fmt.Errorf("failed to handle /findname: %v", err)
		}
	default:
		if err := sendReply(req.Message.Chat.ID, "Ошибка", req.Message.ID); err != nil {
			return nil, fmt.Errorf("failed to send reply: %w", err)
		}
	}

	return &APIGatewayResponse{
		StatusCode: 200,
	}, nil
}

func sendReply(chatID int64, text string, replyToMsgID int64) error {
	token := os.Getenv("TG_API_KEY")

	texts := make([]string, 0)
	if len(text) > maxMessageLen {
		texts = append(texts, text[:maxMessageLen])
		texts = append(texts, text[maxMessageLen:])
	} else {
		texts = append(texts, text)
	}

	for i := 0; i < len(texts); i++ {
		sendReplyBody := &SendMsgReq{
			ChatId:           chatID,
			Text:             texts[i],
			ReplyToMessageId: replyToMsgID,
			ParseMode:        "Markdown",
		}

		sendReplyBodyBytes, err := json.Marshal(sendReplyBody)
		if err != nil {
			return err
		}

		resp, err := http.Post(
			fmt.Sprintf(sendMsgURLPattern, token),
			"application/json",
			bytes.NewReader(sendReplyBodyBytes))
		if err != nil {
			return err
		}

		if resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return errors.New("failed to send reply tg message: " + resp.Status + " " + string(body))
		}

	}

	return nil
}

func handleGetFace(ctx context.Context, db *ydb.Driver) error {
	return nil
}

func handleFindName(ctx context.Context, db *ydb.Driver, name string) error {
	return nil
}
