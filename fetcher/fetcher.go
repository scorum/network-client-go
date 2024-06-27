// Package fetcher is a library for fetching blocks from cosmos based blockchain node.
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/scorum/cosmos-network/app"
)

// ErrTooHighBlockRequested returned when blockchain's height is less than requested.
var ErrTooHighBlockRequested = errors.New("too high block requested")

// Block presents transactions and height.
// If you need have more information open new issue on github or DIY and send pull request.
type Block struct {
	Height uint64
	Time   time.Time
	Txs    []Tx
}

type Tx struct {
	Messages []sdk.Msg
	Hash     string
	Memo     string
}

// Fetcher interface for fetching.
type Fetcher interface {
	// FetchBlocks starts fetching routine and runs handleFunc for every block.
	FetchBlocks(ctx context.Context, from uint64, handleFunc func(b Block) error, opts ...FetchBlocksOption) error
	// FetchBlock fetches block from blockchain.
	// If height is zero then the highest block will be requested.
	FetchBlock(ctx context.Context, height uint64) (*Block, error)
	// PingContext checks if possible to get latest block.
	PingContext(ctx context.Context) error
}

type fetcher struct {
	txc tx.ServiceClient
	tmc tmservice.ServiceClient

	d       sdk.TxDecoder
	timeout time.Duration
}

// New returns new instance of fetcher.
func New(conn *grpc.ClientConn, timeout time.Duration) Fetcher {
	return fetcher{
		txc: tx.NewServiceClient(conn),
		tmc: tmservice.NewServiceClient(conn),

		d:       app.MakeEncodingConfig().TxConfig.TxDecoder(),
		timeout: timeout,
	}
}

// PingContext checks if possible to get latest block.
func (f fetcher) PingContext(ctx context.Context) error {
	if _, err := f.tmc.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{}); err != nil {
		return fmt.Errorf("failed to get blockchain latest block")
	}

	return nil
}

// FetchBlocks starts fetching routine and runs handleFunc for every block.
func (f fetcher) FetchBlocks(ctx context.Context, height uint64, handleFunc func(b Block) error, opts ...FetchBlocksOption) error {
	cfg := defaultFetchBlockOptions
	for _, v := range opts {
		v(&cfg)
	}

	var (
		b   *Block
		err error
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if b == nil {
				if b, err = f.FetchBlock(ctx, height); err != nil {
					if errors.Is(err, ErrTooHighBlockRequested) {
						time.Sleep(cfg.retryLastBlockInterval)
						continue
					}

					cfg.errHandler(height, fmt.Errorf("failed to get block: %w", err))
					time.Sleep(cfg.retryInterval)
					continue
				}
			}

			if err := handleFunc(*b); err != nil {
				cfg.errHandler(b.Height, err)
				if !cfg.skipError {
					time.Sleep(cfg.retryInterval)
					continue
				}
			}

			height = b.Height + 1
			b = nil
		}
	}
}

// FetchBlock fetches block from blockchain.
// If height is zero then the highest block will be requested.
func (f fetcher) FetchBlock(ctx context.Context, height uint64) (*Block, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	if height == 0 {
		res, err := f.tmc.GetLatestBlock(ctx, &tmservice.GetLatestBlockRequest{})
		if err != nil {
			return nil, fmt.Errorf("failed to get latest block: %w", err)
		}
		height = uint64(res.SdkBlock.Header.Height)
	}

	blockResp, err := f.tmc.GetBlockByHeight(ctx, &tmservice.GetBlockByHeightRequest{Height: int64(height)})
	if err != nil {
		if err, ok := status.FromError(err); ok {
			if strings.Contains(err.Message(), "requested block height is bigger then the chain length") {
				return nil, ErrTooHighBlockRequested
			}
		}
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	block := Block{
		Height: uint64(blockResp.SdkBlock.Header.Height),
		Time:   blockResp.SdkBlock.Header.Time,
		Txs:    []Tx{},
	}

	if len(blockResp.SdkBlock.Data.Txs) > 0 {
		txResp, err := f.txc.GetTxsEvent(context.Background(), &tx.GetTxsEventRequest{
			Events:  []string{fmt.Sprintf("tx.height=%d", height)},
			OrderBy: 0,
			Page:    0,
			Limit:   uint64(len(blockResp.SdkBlock.Data.Txs)),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get transactions: %w", err)
		}

		for i, v := range txResp.TxResponses {
			if v.Code != 0 {
				continue
			}

			stdTx, err := f.d(v.Tx.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to decode tx: %w", err)
			}

			block.Txs = append(block.Txs, Tx{
				Messages: stdTx.GetMsgs(),
				Hash:     v.TxHash,
				Memo:     txResp.Txs[i].Body.Memo,
			})
		}
	}

	return &block, nil
}

// Messages returns all messages in all transactions.
func (b Block) Messages() []sdk.Msg {
	msgs := make([]sdk.Msg, 0, len(b.Txs))
	for _, tx := range b.Txs {
		msgs = append(msgs, tx.Messages...)
	}

	return msgs
}
