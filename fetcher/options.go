package fetcher

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Option is used to change fetcher initialization.
type Option func(f *fetcher)

// WithTxDecoder sets decoder to use instead of Scorum decoder.
func WithTxDecoder(d sdk.TxDecoder) Option {
	return func(f *fetcher) {
		f.d = d
	}
}

// FetchBlocksOption is used to tune FetchBlocks method.
type FetchBlocksOption func(f *FetchBlocksOptions)

// FetchBlocksOptions is config for FetchBlocks method.
type FetchBlocksOptions struct {
	// How long should fetcher wait if fetcher got ErrTooHighBlockRequested.
	retryLastBlockInterval time.Duration
	// How long should fetcher wait after error.
	retryInterval time.Duration
	// errHandler will be called when fetcher will get an error.
	errHandler func(height uint64, err error)
	// skipError disable retries of block handling with handleFunc.
	skipError bool
	// skipUnknownTransactions used to set skipping transactions fetcher failed to decode.
	skipUnknownTransactions bool
}

var defaultFetchBlockOptions = FetchBlocksOptions{
	retryLastBlockInterval: time.Second,
	retryInterval:          time.Second,
	errHandler:             func(height uint64, err error) {},
	skipError:              false,
}

// WithRetryLastBlockInterval sets how long should fetcher wait if fetcher got ErrTooHighBlockRequested.
func WithRetryLastBlockInterval(d time.Duration) FetchBlocksOption {
	return func(opts *FetchBlocksOptions) {
		opts.retryLastBlockInterval = d
	}
}

// WithRetryInterval sets pause duration after error.
func WithRetryInterval(d time.Duration) FetchBlocksOption {
	return func(opts *FetchBlocksOptions) {
		opts.retryInterval = d
	}
}

// WithErrHandler sets function to process errors.
func WithErrHandler(f func(height uint64, err error)) FetchBlocksOption {
	return func(opts *FetchBlocksOptions) {
		opts.errHandler = f
	}
}

// WithSkipError disable retries of block handling with handleFunc.
func WithSkipError(b bool) FetchBlocksOption {
	return func(f *FetchBlocksOptions) {
		f.skipError = b
	}
}

// WithSkipUnknownTransactions used to set skipping transactions fetcher failed to decode.
func WithSkipUnknownTransactions(b bool) FetchBlocksOption {
	return func(f *FetchBlocksOptions) {
		f.skipUnknownTransactions = b
	}
}
