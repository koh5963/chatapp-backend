package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"

	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	apimgmt "github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	apimgmttypes "github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi/types"

	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type In struct {
	Action string `json:"action"`
	RoomID string `json:"roomId"`
	Text   string `json:"text"`
	UserID string `json:"userId"`
}

type Conn struct {
	ConnectionId string `dynamodbav:"connectionId"`
	RoomId       string `dynamodbav:"roomId"`
}

var (
	tableName string
	ddbCli    *ddb.Client
	mgmtCli   *apimgmt.Client
)

// API Gateway Management API クライアント（署名/エンドポイントを確実に固定）
type legacyResolver struct {
	endpoint string
	region   string
}

// ※ 旧インターフェース（V1）のResolverを実装します。
//
//	これだと戻り値に aws.Endpoint を使えるので SigningName/SigningRegion を明示できます。
func (r legacyResolver) ResolveEndpoint(_ string, _ apimgmt.EndpointResolverOptions) (aws.Endpoint, error) {
	return aws.Endpoint{
		URL:               r.endpoint, // 例: https://<api-id>.execute-api.ap-northeast-1.amazonaws.com/dev
		PartitionID:       "aws",
		SigningRegion:     r.region,      // 例: "ap-northeast-1"
		SigningName:       "execute-api", // ★これが肝（apigateway ではなく execute-api）
		HostnameImmutable: true,
	}, nil
}

func newMgmtClient(ctx context.Context) (*apimgmt.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := os.Getenv("APIGW_MGMT_ENDPOINT")
	if endpoint == "" {
		return nil, fmt.Errorf("APIGW_MGMT_ENDPOINT is empty")
	}

	// ★ NewFromConfig を使いつつ、旧Resolverを差し込んで SigningName を強制
	client := apimgmt.NewFromConfig(cfg, func(o *apimgmt.Options) {
		o.Region = "ap-northeast-1" // 明示固定（cfg.RegionでもOK）
		o.EndpointResolver = legacyResolver{
			endpoint: endpoint,
			region:   "ap-northeast-1",
		}
		// ※ WithEndpointResolverV2 は使いません（型が合わないため）
	})

	fmt.Println("[mgmt] region = ap-northeast-1 endpoint =", endpoint)
	return client, nil
}

func handler(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	var in In
	_ = json.Unmarshal([]byte(req.Body), &in)
	if in.RoomID == "" {
		in.RoomID = "lobby"
	}

	// roomId の接続一覧を取得（GSI: roomId-index）
	out, err := ddbCli.Query(ctx, &ddb.QueryInput{
		TableName:              aws.String(tableName),
		IndexName:              aws.String("roomId-index"),
		KeyConditionExpression: aws.String("roomId = :r"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":r": &ddbtypes.AttributeValueMemberS{Value: in.RoomID},
		},
	})
	if err != nil {
		fmt.Println("[send] Query error:", err)
		return resp(500, err.Error()), nil
	}

	var cons []Conn
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &cons); err != nil {
		fmt.Println("[send] Unmarshal error:", err)
		return resp(500, err.Error()), nil
	}
	fmt.Printf("[send] room=%s items=%d\n", in.RoomID, len(cons))

	payload, _ := json.Marshal(map[string]any{
		"type":   "message",
		"roomId": in.RoomID,
		"userId": in.UserID,
		"text":   in.Text,
	})

	for _, c := range cons {
		fmt.Printf("[send] post to %s\n", c.ConnectionId)
		_, postErr := mgmtCli.PostToConnection(ctx, &apimgmt.PostToConnectionInput{
			ConnectionId: aws.String(c.ConnectionId),
			Data:         payload,
		})
		if postErr != nil {
			// Gone(410) は掃除、それ以外はログ
			if isGone(postErr) {
				fmt.Printf("[send] gone; delete %s\n", c.ConnectionId)
				_, _ = ddbCli.DeleteItem(ctx, &ddb.DeleteItemInput{
					TableName: aws.String(tableName),
					Key: map[string]ddbtypes.AttributeValue{
						"connectionId": &ddbtypes.AttributeValueMemberS{Value: c.ConnectionId},
					},
				})
			} else {
				fmt.Println("PostToConnection error:", postErr.Error())
			}
		}
	}

	return resp(200, "ok"), nil
}

func isGone(err error) bool {
	var ge *apimgmttypes.GoneException
	if errors.As(err, &ge) {
		return true
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.Response != nil && respErr.Response.StatusCode == 410 {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "GoneException"
}

func resp(code int, body string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{StatusCode: code, Body: body}
}

func main() {
	ctx := context.Background()

	// 共通設定読み込み
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic(err)
	}

	// DynamoDB クライアント
	ddbCli = ddb.NewFromConfig(cfg)

	// 環境変数
	tableName = os.Getenv("CONNECTIONS_TABLE")
	if tableName == "" {
		tableName = "connections" // 念のためのデフォルト
	}

	// API Gateway Management API クライアント
	mgmtCli, err = newMgmtClient(ctx)
	if err != nil {
		panic(err)
	}

	lambda.Start(handler)
}
