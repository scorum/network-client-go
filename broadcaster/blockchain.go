// Package broadcaster contains code for interacting with the scorum blockchain.
package broadcaster

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/scorum/cosmos-network/app"
	"github.com/spf13/pflag"
)

const defaultWaitRetries = 5
const defaultWaitRetryInterval = 2 * time.Second

func init() {
	app.InitSDKConfig()
}

// ErrTxInMempoolCache is returned when tx is already broadcast and exists in mempool cache.
var ErrTxInMempoolCache = errors.New("tx is already in mempool cache")

//go:generate mockgen -destination=./mock/broadcaster.go -package=mock -source=blockchain.go

// Broadcaster provides functionality to broadcast messages to cosmos based blockchain node.
type Broadcaster interface {
	// From returns address of broadcaster.
	From() sdk.AccAddress
	// GetHeight returns current height.
	GetHeight(ctx context.Context) (uint64, error)
	// BroadcastMsg broadcasts alone message.
	BroadcastMsg(ctx context.Context, msg sdk.Msg, memo string) (*sdk.TxResponse, error)
	// Broadcast broadcasts messages.
	Broadcast(ctx context.Context, msgs []sdk.Msg, memo string) (*sdk.TxResponse, error)

	// PingContext pings node.
	PingContext(ctx context.Context) error
}

var _ Broadcaster = &broadcaster{}

var accountSequenceMismatchErrorRegExp = regexp.MustCompile(`.+account sequence mismatch, expected (\d+), got \d+:.+`)

type broadcaster struct {
	ctx client.Context
	txf tx.Factory

	waitBlock         bool
	waitRetries       int
	waitRetryInterval time.Duration

	mu sync.Mutex
}

// Config ...
type Config struct {
	Client *http.Client

	KeyringRootDir     string
	KeyringBackend     string
	KeyringPromptInput string

	NodeURI       string
	BroadcastMode string

	WaitBlock              bool
	WaitBlockRetriesCount  int
	WaitBlockRetryInterval time.Duration

	From    string
	ChainID string

	Fees      sdk.Coins
	Gas       uint64
	GasAdjust float64
}

// New returns new instance of broadcaster
func New(cfg Config) (*broadcaster, error) {
	encodingConfig := app.MakeEncodingConfig()
	kr, err := keyring.New(
		app.Name,
		cfg.KeyringBackend,
		cfg.KeyringRootDir,
		strings.NewReader(cfg.KeyringPromptInput),
		encodingConfig.Marshaler,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	acc, err := kr.Key(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	var c *rpchttp.HTTP
	if cfg.Client != nil {
		c, err = rpchttp.NewWithClient(cfg.NodeURI, "/websocket", cfg.Client)
	} else {
		c, err = rpchttp.New(cfg.NodeURI, "/websocket")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	addr, err := acc.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}
	ctx := client.Context{}.
		WithCodec(encodingConfig.Marshaler).
		WithChainID(cfg.ChainID).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithAccountRetriever(types.AccountRetriever{}).
		WithBroadcastMode(cfg.BroadcastMode).
		WithHomeDir(cfg.KeyringRootDir).
		WithKeyring(kr).
		WithFrom(acc.Name).
		WithFromName(acc.Name).
		WithFromAddress(addr).
		WithNodeURI(cfg.NodeURI).
		WithClient(c)

	if strings.EqualFold(cfg.BroadcastMode, "async") {
		ctx = ctx.WithBroadcastMode("async")
	}

	factory, err := tx.NewFactoryCLI(ctx, &pflag.FlagSet{})
	if err != nil {
		return nil, fmt.Errorf("failed to create factory: %w", err)
	}

	factory = factory.
		WithFees(cfg.Fees.String()).
		WithGas(cfg.Gas).
		WithGasAdjustment(cfg.GasAdjust)

	b := &broadcaster{
		ctx: ctx,
		txf: factory,

		waitBlock:         cfg.WaitBlock,
		waitRetries:       cfg.WaitBlockRetriesCount,
		waitRetryInterval: cfg.WaitBlockRetryInterval,

		mu: sync.Mutex{},
	}

	if b.waitRetries == 0 {
		b.waitRetries = defaultWaitRetries
	}

	if b.waitRetryInterval == 0 {
		b.waitRetryInterval = defaultWaitRetryInterval
	}

	if err := b.refreshSequence(); err != nil {
		return nil, fmt.Errorf("failed to refresh sequence: %w", err)
	}

	return b, nil
}

// From returns address of broadcaster.
func (b *broadcaster) From() sdk.AccAddress {
	return b.ctx.FromAddress
}

// GetHeight returns current height.
func (b *broadcaster) GetHeight(ctx context.Context) (uint64, error) {
	c, err := b.ctx.GetNode()
	if err != nil {
		return 0, fmt.Errorf("failed get node: %w", err)
	}

	i, err := c.ABCIInfo(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch ABCIInfo: %w", err)
	}

	return uint64(i.Response.LastBlockHeight), nil
}

// BroadcastMsg broadcasts alone message.
func (b *broadcaster) BroadcastMsg(ctx context.Context, msg sdk.Msg, memo string) (*sdk.TxResponse, error) {
	return b.Broadcast(ctx, []sdk.Msg{msg}, memo)
}

// Broadcast broadcasts messages.
func (b *broadcaster) Broadcast(ctx context.Context, msgs []sdk.Msg, memo string) (*sdk.TxResponse, error) {
	out, err := b.broadcast(ctx, msgs, memo, false)

	if err != nil {
		return nil, fmt.Errorf("failed to broadcast: %w", err)
	}

	return out, nil
}

// PingContext pings node.
func (b *broadcaster) PingContext(ctx context.Context) error {
	c, err := b.ctx.GetNode()
	if err != nil {
		return fmt.Errorf("failed to get rpc client: %w", err)
	}
	if _, err := c.ABCIInfo(ctx); err != nil {
		return fmt.Errorf("failed to check node status: %w", err)
	}

	return nil
}

func (b *broadcaster) broadcast(ctx context.Context, msgs []sdk.Msg, memo string, isRetry bool) (*sdk.TxResponse, error) {
	if !isRetry {
		b.mu.Lock()
		defer b.mu.Unlock()
	}

	txf := b.txf.WithMemo(memo)

	if txf.GasAdjustment() == 0 {
		txf = txf.WithGasAdjustment(1)
	}

	if txf.Gas() == 0 {
		_, gas, err := tx.CalculateGas(b.ctx, txf, msgs...)
		if err != nil {
			if !isRetry {
				if seq := getNextSequence(err.Error()); seq != 0 {
					b.txf = b.txf.WithSequence(seq)
				}

				return b.broadcast(ctx, msgs, memo, true)
			}

			return nil, fmt.Errorf("failed to calculate gas: %w", err)
		}
		txf = txf.WithGas(gas)
	}

	unsignedTx, err := txf.BuildUnsignedTx(msgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to build tx: %w", err)
	}

	if err := tx.Sign(ctx, txf, b.ctx.GetFromName(), unsignedTx, true); err != nil {
		return nil, fmt.Errorf("failed to sign tx: %w", err)
	}

	txBytes, err := b.ctx.TxConfig.TxEncoder()(unsignedTx.GetTx())
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx: %w", err)
	}

	// broadcast to node
	resp, err := b.ctx.BroadcastTx(txBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast tx: %w", err)
	}

	b.txf = b.txf.WithSequence(b.txf.Sequence() + 1)

	if resp.Code != 0 {
		if sdkerrors.ErrTxInMempoolCache.ABCICode() == resp.Code {
			return nil, ErrTxInMempoolCache
		}

		if !isRetry {
			if seq := getNextSequence(resp.RawLog); seq != 0 {
				b.txf = b.txf.WithSequence(seq)

				return b.broadcast(ctx, msgs, memo, true)
			}
		}

		return nil, fmt.Errorf("failed to broadcast tx: %s", resp.String())
	}

	if b.waitBlock {
		node, err := b.ctx.GetNode()
		if err != nil {
			return nil, fmt.Errorf("failed to get node: %w", err)
		}

		hash, err := hex.DecodeString(resp.TxHash)
		if err != nil {
			return nil, fmt.Errorf("failed to decode txHash: %w", err)
		}

		var confirmErr error
		for i := 0; i < b.waitRetries; i++ {
			if confirmErr != nil {
				time.Sleep(b.waitRetryInterval)
			}

			txResp, err := node.Tx(context.Background(), hash, true)
			if err != nil {
				confirmErr = err
				continue
			}

			if !txResp.TxResult.IsOK() {
				return nil, fmt.Errorf("failed to broadcast tx: %s", resp.String())
			}

			parsedLogs, _ := sdk.ParseABCILogs(txResp.TxResult.Log)

			resp.Height = txResp.Height
			resp.Codespace = txResp.TxResult.Codespace
			resp.Info = txResp.TxResult.Info
			resp.Data = strings.ToUpper(hex.EncodeToString(txResp.TxResult.Data))
			resp.Logs = parsedLogs
			resp.RawLog = txResp.TxResult.Log
			resp.Events = txResp.TxResult.Events
			resp.Code = txResp.TxResult.Code
			resp.GasUsed = txResp.TxResult.GasUsed
			resp.GasWanted = txResp.TxResult.GasWanted

			return resp, nil
		}

		if confirmErr != nil {
			return nil, fmt.Errorf("failed to check tx: %w", err)
		}
	}

	return resp, nil
}

func (b *broadcaster) refreshSequence() error {
	if err := b.txf.AccountRetriever().EnsureExists(b.ctx, b.From()); err != nil {
		return fmt.Errorf("failed to EnsureExists: %w", err)
	}

	num, seq, err := b.txf.AccountRetriever().GetAccountNumberSequence(b.ctx, b.From())
	if err != nil {
		return fmt.Errorf("failed to get GetAccountNumberSequence: %w", err)
	}

	b.txf = b.txf.WithAccountNumber(num).WithSequence(seq)

	return nil
}

func getNextSequence(m string) uint64 {
	s := accountSequenceMismatchErrorRegExp.FindStringSubmatch(m)

	if len(s) != 2 {
		return 0
	}

	seq, _ := strconv.ParseUint(s[1], 10, 64)

	return seq
}
