package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ydb-platform/ydb-go-sdk/v3/table"
	"github.com/ydb-platform/ydb-go-sdk/v3/table/types"
	"image"
	"log"
	"os"
	"path"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	yc "github.com/ydb-platform/ydb-go-yc-metadata"
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

	relationsPath := "relations"
	relationsPath = path.Join(db.Name(), relationsPath)

	namesPath := "names"
	namesPath = path.Join(db.Name(), namesPath)

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

		faceName := uuid.New().String() + ".jpg"
		if err := imaging.Save(rectcropimg, path.Join(outputDir, faceName)); err != nil {
			return nil, fmt.Errorf("failed to save img: %v", err)
		}

		err = db.Table().BulkUpsert(ctx,
			namesPath,
			table.BulkUpsertDataRows(
				types.ListValue(
					types.StructValue(
						types.StructFieldValue("FaceID", types.StringValueFromString(faceName)),
					))),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to BulkInsert to names: %v", err)
		}

		err = db.Table().BulkUpsert(ctx,
			relationsPath,
			table.BulkUpsertDataRows(
				types.ListValue(
					types.StructValue(
						types.StructFieldValue("ImageID", types.StringValueFromString(task.ObjectID)),
						types.StructFieldValue("FaceID", types.StringValueFromString(faceName)),
					))),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to BulkInsert to relations: %v", err)
		}
	}

	return &Response{
		StatusCode: 200,
	}, nil
}
