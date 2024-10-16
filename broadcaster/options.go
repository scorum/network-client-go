package broadcaster

import (
	"time"

	"github.com/scorum/cosmos-network/app"
	"github.com/scorum/cosmos-network/app/params"
)

var defaultOpts = Options{
	encodingConfig: app.MakeEncodingConfig(),
	sdkConfig: SDKConfig{
		AccountAddressPrefix:   app.AccountAddressPrefix,
		AccountPubKeyPrefix:    app.AccountAddressPrefix + "pub",
		ValidatorAddressPrefix: app.AccountAddressPrefix + "valoper",
		ValidatorPubKeyPrefix:  app.AccountAddressPrefix + "valoperpub",
		ConsNodeAddressPrefix:  app.AccountAddressPrefix + "valcons",
		ConsNodePubKeyPrefix:   app.AccountAddressPrefix + "valconspub",
	},

	waitBlock: false,
}

type SDKConfig struct {
	AccountAddressPrefix   string
	AccountPubKeyPrefix    string
	ValidatorAddressPrefix string
	ValidatorPubKeyPrefix  string
	ConsNodeAddressPrefix  string
	ConsNodePubKeyPrefix   string
}

type Options struct {
	encodingConfig params.EncodingConfig
	sdkConfig      SDKConfig

	waitBlock         bool
	waitRetries       int
	waitRetryInterval time.Duration
}

// Option is used to change broadcaster initialization.
type Option func(f *Options)

// WithEncodingConfig sets encoding to use instead of Scorum's one.
func WithEncodingConfig(config params.EncodingConfig) Option {
	return func(o *Options) {
		o.encodingConfig = config
	}
}

// WithSDKConfig sets sdk config to use instead of Scorum's one.
func WithSDKConfig(config SDKConfig) Option {
	return func(o *Options) {
		o.sdkConfig = config
	}
}

// WithWaitBlock enables block waiting and sets its parameters.
func WithWaitBlock(retryCount int, retryInterval time.Duration) Option {
	return func(o *Options) {
		o.waitBlock = true
		o.waitRetries = retryCount
		o.waitRetryInterval = retryInterval
	}
}
