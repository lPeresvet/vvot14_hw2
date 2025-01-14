package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"
	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
	yc "github.com/ydb-platform/ydb-go-yc-metadata"
	"io"
	"log"
	"net/http"
	"os"
	"path"
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
	UpdateID int64   `json:"update_id"`
	Message  Message `json:"message"`
}

type Message struct {
	ID   int64 `json:"message_id"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	Text    string   `json:"text"`
	Photo   []Photo  `json:"photo,omitempty"`
	ReplyTo *Message `json:"reply_to_message,omitempty"`
}

type SendMsgReq struct {
	ChatId           int64  `json:"chat_id"`
	Text             string `json:"text"`
	ReplyToMessageId int64  `json:"reply_to_message_id"`
	ParseMode        string `json:"parse_mode,omitempty"`
}

type SendPhotoReq struct {
	ChatId int64  `json:"chat_id"`
	Photo  string `json:"photo"`
}

const (
	getFilePathURLPattern  = "https://api.telegram.org/bot%s/getFile?file_id=%s"
	sendMsgURLPattern      = "https://api.telegram.org/bot%s/sendMessage"
	sendPhotoURLPattern    = "https://api.telegram.org/bot%s/sendPhoto"
	downloadFileURLPattern = "https://api.telegram.org/file/bot%s"
	localPath              = "/function/storage/images"
	ocrURL                 = "https://ocr.api.cloud.yandex.net/ocr/v1/recognizeText"
	catalog                = "b1g163vdicpkeevao9ga"
	yaGPTURL               = "https://llm.api.cloud.yandex.net/foundationModels/v1/completion"
	maxMessageLen          = 4096
	gwPattern              = "https://%s/?face=%s"
	gwImagePattern         = "https://%s/?image=%s"
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
	if req.Message.ReplyTo != nil {
		faceID, err := read(ctx, db.Query())
		if err != nil {
			return nil, fmt.Errorf("failed to read face id: %v", err)
		}

		namesPath := "names"
		namesPath = path.Join(db.Name(), namesPath)

		err = db.Table().BulkUpsert(ctx,
			namesPath,
			table.BulkUpsertDataRows(
				types.ListValue(
					types.StructValue(
						types.StructFieldValue("FaceID", types.StringValueFromString(faceID)),
						types.StructFieldValue("FaceName", types.StringValueFromString(req.Message.Text)),
					))),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to BulkInsert to names: %v", err)
		}

		answer := fmt.Sprintf("Лицу с ID: `%s` присвоено имя `%s`", faceID, req.Message.Text)

		if err := sendReply(req.Message.Chat.ID, answer, req.Message.ID); err != nil {
			return nil, fmt.Errorf("failed to send reply: %w", err)
		}

		return &APIGatewayResponse{
			StatusCode: 200,
		}, nil
	}

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
		if err := handleGetFace(ctx, db, req.Message.Chat.ID); err != nil {
			if err := sendReply(req.Message.Chat.ID, "Не удалось найти фото без имени", req.Message.ID); err != nil {
				return nil, fmt.Errorf("failed to send reply: %w", err)
			}
		}
	case "/find":
		if len(cmds) < 2 {
			if err := sendReply(req.Message.Chat.ID, "Ошибка", req.Message.ID); err != nil {
				return nil, fmt.Errorf("failed to send reply: %w", err)
			}

			return &APIGatewayResponse{
				StatusCode: 200,
			}, nil
		}
		if err := handleFindName(ctx, db, cmds[1], req.Message.Chat.ID, req.Message.ID); err != nil {
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

func handleGetFace(ctx context.Context, db *ydb.Driver, chatID int64) error {
	faceID, err := read(ctx, db.Query())
	if err != nil {
		return fmt.Errorf("failed to sread face id: %v", err)
	}

	domain := os.Getenv("API_GW_URL")
	url := fmt.Sprintf(gwPattern, domain, faceID)
	if err := sendPhoto(chatID, url); err != nil {
		return fmt.Errorf("failed to send photo: %v", err)
	}

	return nil
}

func read(ctx context.Context, c query.Client) (string, error) {
	faceID := ""

	err := c.Do(ctx,
		func(ctx context.Context, s query.Session) (err error) {
			result, err := s.Query(ctx, `
					SELECT FaceID
					FROM names
					WHERE FaceName is NULL
					LIMIT 1
				`,
				query.WithTxControl(query.TxControl(query.BeginTx(query.WithSnapshotReadOnly()))),
			)
			if err != nil {
				return err
			}

			defer func() {
				_ = result.Close(ctx)
			}()

			for {
				set, err := result.NextResultSet(ctx)
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}

					return err
				}

				row, err := set.NextRow(ctx)
				if err != nil {
					return err
				}

				if err := row.Scan(&faceID); err != nil {
					return err
				}
			}

			return nil
		},
	)

	return faceID, err
}

func handleFindName(ctx context.Context, db *ydb.Driver, name string, chatID int64, replyTo int64) error {
	images, err := findByName(ctx, db.Query(), name)
	if err != nil {
		return fmt.Errorf("failed to retrieve images %v", err)
	}

	if len(images) == 0 {
		return sendReply(chatID, fmt.Sprintf("Фотографии с %s не найдены", name), replyTo)
	}

	domain := os.Getenv("API_GW_URL")
	for _, imageName := range images {
		if err := sendPhoto(chatID, fmt.Sprintf(gwImagePattern, domain, imageName)); err != nil {
			return fmt.Errorf("failed to send photo: %v", err)
		}
	}

	return nil
}

var (
	selectByName = `SELECT    r.ImageID as image
				 FROM      (SELECT FaceID FROM names WHERE FaceName = '%s') AS n
				 INNER  JOIN relations AS r ON n.FaceID = r.FaceID;`
)

func findByName(ctx context.Context, c query.Client, name string) ([]string, error) {
	var images []string

	err := c.Do(ctx,
		func(ctx context.Context, s query.Session) (err error) {
			result, err := s.Query(ctx, fmt.Sprintf(selectByName, name),
				query.WithTxControl(query.TxControl(query.BeginTx(query.WithSnapshotReadOnly()))),
			)
			if err != nil {
				return fmt.Errorf("failed to exec query %v", err)
			}

			defer func() {
				_ = result.Close(ctx)
			}()

			img := ""
			for {
				set, err := result.NextResultSet(ctx)
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}

					return err
				}

				for {
					row, err := set.NextRow(ctx)
					if err != nil {
						if errors.Is(err, io.EOF) {
							break
						}

						return fmt.Errorf("read rows error: %v", err)
					}

					if err := row.Scan(&img); err != nil {
						return err
					}

					images = append(images, img)
				}
			}

			return nil
		},
	)

	return images, err
}

func sendPhoto(chatID int64, photoURL string) error {
	token := os.Getenv("TG_API_KEY")

	sendPhotoBody := &SendPhotoReq{
		ChatId: chatID,
		Photo:  photoURL,
	}

	sendPhotoBodyBytes, err := json.Marshal(sendPhotoBody)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		fmt.Sprintf(sendPhotoURLPattern, token),
		"application/json",
		bytes.NewReader(sendPhotoBodyBytes))
	if err != nil {
		return err
	}

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return errors.New("failed to send photo: " + resp.Status + " " + string(body))
	}

	return nil
}
