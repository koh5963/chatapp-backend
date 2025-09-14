package main

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"

	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	tableName string
	ddbCli    *ddb.Client
)

func init() {
	tableName = os.Getenv("CONNECTIONS_TABLE")
	cfg, _ := config.LoadDefaultConfig(context.TODO())
	ddbCli = ddb.NewFromConfig(cfg)
}

func handler(ctx context.Context, req events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connID := req.RequestContext.ConnectionID

	_, err := ddbCli.DeleteItem(ctx, &ddb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"connectionId": &ddbtypes.AttributeValueMemberS{Value: connID},
		},
	})
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: 500, Body: err.Error()}, nil
	}
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func main() { lambda.Start(handler) }
