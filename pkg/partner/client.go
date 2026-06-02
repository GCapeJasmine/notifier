package partner

import (
	"context"
	"fmt"

	"github.com/gleo/subscribers/pkg/http/client"
	"github.com/go-resty/resty/v2"
)

type Config struct {
	BaseUrl      string             `yaml:"base_url"     mapstructure:"base_url"`
	Key          string             `yaml:"key"          mapstructure:"key"`
	ClientConfig client.RestyConfig `yaml:"client_config" mapstructure:"client_config"`
}

//go:generate mockery --name Client --output ./mock --filename client.go --with-expecter
type Client interface {
	Notify(ctx context.Context, payload any) error
}

type PartnerService struct {
	client *resty.Client
}

func NewPartnerClient(config Config) Client {
	c := client.NewRestyClient(config.ClientConfig)
	c.SetBaseURL(config.BaseUrl)
	c.SetHeader("x-api-key", config.Key)
	return &PartnerService{client: c}
}

func (p *PartnerService) Notify(ctx context.Context, payload any) error {
	resp, err := p.client.R().
		SetContext(ctx).
		SetBody(payload).
		Post("")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("partner: unexpected status %d", resp.StatusCode())
	}
	return nil
}
