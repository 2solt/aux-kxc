package main

import (
	"context"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/gin-gonic/gin"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	VERSION string `envconfig:"VERSION" required:"true"`
}

type response struct {
	Version string `json:"version"`
	Data    any    `json:"data"`
}

type awsClients struct {
	s3  *s3.Client
	ssm *ssm.Client
}

func newAWSClients(ctx context.Context) (*awsClients, error) {
	cfg, err := config.LoadDefaultConfig(ctx) // reads env vars automatically
	if err != nil {
		return nil, err
	}

	// validate credentials with a cheap sts call
	stsClient := sts.NewFromConfig(cfg)
	if _, err = stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}); err != nil {
		return nil, err
	}

	return &awsClients{
		s3:  s3.NewFromConfig(cfg),
		ssm: ssm.NewFromConfig(cfg),
	}, nil
}

func listBucketsHandler(cl *awsClients, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := cl.s3.ListBuckets(c.Request.Context(), &s3.ListBucketsInput{})
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		var names []string
		for _, b := range out.Buckets {
			names = append(names, *b.Name)
		}
		c.JSON(http.StatusOK, response{
			Version: version,
			Data:    names,
		})
	}
}

func listParametersHandler(cl *awsClients, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := cl.ssm.DescribeParameters(c.Request.Context(), &ssm.DescribeParametersInput{})
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		var names []string
		for _, p := range out.Parameters {
			names = append(names, *p.Name)
		}
		c.JSON(http.StatusOK, response{
			Version: version,
			Data:    names,
		})
	}
}

func getParameterHandler(cl *awsClients, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		out, err := cl.ssm.GetParameter(c.Request.Context(), &ssm.GetParameterInput{
			Name: &name,
		})
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.JSON(http.StatusOK, response{
			Version: version,
			Data:    *out.Parameter.Value,
		})
	}
}

func livenessHandler(c *gin.Context) {
	c.Status(http.StatusOK)
}

func main() {
	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	clients, err := newAWSClients(ctx)
	if err != nil {
		panic("AWS init failed: " + err.Error())
	}

	r := gin.Default()
	r.GET("/buckets", listBucketsHandler(clients, cfg.VERSION))
	r.GET("/parameters", listParametersHandler(clients, cfg.VERSION))
	r.GET("/parameters/:name", getParameterHandler(clients, cfg.VERSION))

	// Health entpoint
	r.GET("/livez", livenessHandler)

	addr := ":8081"
	log.Printf("Service listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("router error: %v", err)
	}
}
