package bedrock

import (
	"context"

	awsruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	awstypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// ConverseEventStream is the narrow event-stream seam used by the protocol
// driver. AWS SDK values remain confined to this internal package.
type ConverseEventStream interface {
	Events() <-chan awstypes.ConverseStreamOutput
	Err() error
	Close() error
}

type InvokeEventStream interface {
	Events() <-chan awstypes.ResponseStream
	Err() error
	Close() error
}

// Client is the testable subset of the AWS Bedrock Runtime SDK.
type Client interface {
	Converse(context.Context, *awsruntime.ConverseInput) (*awsruntime.ConverseOutput, error)
	ConverseStream(context.Context, *awsruntime.ConverseStreamInput) (ConverseEventStream, error)
	InvokeModel(context.Context, *awsruntime.InvokeModelInput) (*awsruntime.InvokeModelOutput, error)
	InvokeModelWithResponseStream(context.Context, *awsruntime.InvokeModelWithResponseStreamInput) (InvokeEventStream, error)
}

type sdkClient struct{ native *awsruntime.Client }

func NewSDKClient(native *awsruntime.Client) Client {
	if native == nil {
		return nil
	}
	return &sdkClient{native: native}
}

func (c *sdkClient) Converse(ctx context.Context, input *awsruntime.ConverseInput) (*awsruntime.ConverseOutput, error) {
	return c.native.Converse(ctx, input)
}

func (c *sdkClient) ConverseStream(ctx context.Context, input *awsruntime.ConverseStreamInput) (ConverseEventStream, error) {
	output, err := c.native.ConverseStream(ctx, input)
	if err != nil || output == nil {
		return nil, err
	}
	return output.GetStream(), nil
}

func (c *sdkClient) InvokeModel(ctx context.Context, input *awsruntime.InvokeModelInput) (*awsruntime.InvokeModelOutput, error) {
	return c.native.InvokeModel(ctx, input)
}

func (c *sdkClient) InvokeModelWithResponseStream(ctx context.Context, input *awsruntime.InvokeModelWithResponseStreamInput) (InvokeEventStream, error) {
	output, err := c.native.InvokeModelWithResponseStream(ctx, input)
	if err != nil || output == nil {
		return nil, err
	}
	return output.GetStream(), nil
}

var _ Client = (*sdkClient)(nil)
