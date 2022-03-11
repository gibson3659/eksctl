package eks

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/aws/aws-sdk-go-v2/aws"
	middlewarev2 "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/smithy-go/middleware"

	"github.com/kris-nova/logger"

	api "github.com/weaveworks/eksctl/pkg/apis/eksctl.io/v1alpha5"
	"github.com/weaveworks/eksctl/pkg/version"
)

func newV2Config(pc *api.ProviderConfig, region string) (aws.Config, error) {
	var options []func(options *config.LoadOptions) error

	// TODO default region
	if region != "" {
		options = append(options, config.WithRegion(region))
	}
	clientLogMode := aws.LogRetries

	if logger.Level >= api.AWSDebugLevel {
		clientLogMode = clientLogMode | aws.LogRequestWithBody | aws.LogRequestEventMessage | aws.LogResponseWithBody
	}
	options = append(options, config.WithClientLogMode(clientLogMode))

	// TODO configure file-based credentials cache
	return config.LoadDefaultConfig(context.TODO(), append(options,
		config.WithSharedConfigProfile(pc.Profile),
		config.WithRetryer(func() aws.Retryer {
			return NewRetryerV2()
		}),
		config.WithAssumeRoleCredentialOptions(func(o *stscreds.AssumeRoleOptions) {
			o.TokenProvider = stscreds.StdinTokenProvider
			o.Duration = 30 * time.Minute
		}),
		config.WithAPIOptions([]func(stack *middleware.Stack) error{
			middlewarev2.AddUserAgentKeyValue("eksctl", version.String()),
		}),
		config.WithEndpointResolverWithOptions(newEndpointResolver()),
	)...)
}

func newEndpointResolver() aws.EndpointResolverWithOptionsFunc {
	serviceIDEnvMap := map[string]string{
		cloudformation.ServiceID:         "AWS_CLOUDFORMATION_ENDPOINT",
		eks.ServiceID:                    "AWS_EKS_ENDPOINT",
		ec2.ServiceID:                    "AWS_EC2_ENDPOINT",
		elasticloadbalancing.ServiceID:   "AWS_ELB_ENDPOINT",
		elasticloadbalancingv2.ServiceID: "AWS_ELBV2_ENDPOINT",
		sts.ServiceID:                    "AWS_STS_ENDPOINT",
		iam.ServiceID:                    "AWS_IAM_ENDPOINT",
		cloudtrail.ServiceID:             "AWS_CLOUDTRAIL_ENDPOINT",
	}

	for service, envName := range serviceIDEnvMap {
		if endpoint, ok := os.LookupEnv(envName); ok {
			logger.Debug("Setting %s endpoint to %s", service, endpoint)
		}
	}

	return func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		envName, ok := serviceIDEnvMap[service]
		if ok {
			if endpoint, ok := os.LookupEnv(envName); ok {
				return aws.Endpoint{
					URL:           endpoint,
					SigningRegion: region,
				}, nil
			}
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	}
}