package client

import (
	"github.com/google/wire"
)

var (
	HttpClientProviderSet = wire.NewSet(
		NewRestyClient,
	)
)
