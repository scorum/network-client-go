# network-client-go

Both fetcher and broadcaster client  
This library is always up-to-date with `main` branch of [https://github.com/scorum/cosmos-network](Scorum Cosmos Network)

## Broadcaster

This package is used to broadcast messages from go code.

Broadcaster instance requires keyring provided

### Usage
```
b, err := broadcaster.New(Config{
    KeyringRootDir:     "test",
    KeyringBackend:     "file",
    KeyringPromptInput: "1qaz2wsX",
    NodeURI:            "localhost:26657",
    BroadcastMode:      "block",
    From:               "keyringKey",
    ChainID:            "test",
})
if err != nil {
    return fmt.Errorf("failed to create broadcaster: %w", err)
}

msg := NewSomeScorumMsg(from)
tx, err := b.BroadcastMsg(msg, "memo")
if err != nil {
    return fmt.Errorf("failed to broadcast: %w", err)
}

log.Info(tx.TxHash)
```

## Fetcher

This package is used to listen blockchain. Note: fetcher's FetchBlocks call is blocking.

### Usage
```
f, err := fetcher.New(ctx, "localhost:9090", time.Second)
if err != nil {
    return fmt.Errorf("failed to create fetcher: %w", err)
}

f.FetchBlocks(
    ctx,
    from,
    func(b Block) error {
        fmt.Printf("%+v", b)

        return nil
    },
    WithErrHandler(func(height uint64, err error) {
        log.Error("failed to fetch", height, err)
    }),
    WithRetryInterval(time.Minute),
    WithSkipError(false),
    WithRetryLastBlockInterval(time.Second*5),
)
```
