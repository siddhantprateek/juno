package node

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/NethermindEth/juno/blockchain"
	"github.com/NethermindEth/juno/clients/feeder"
	"github.com/NethermindEth/juno/db"
	"github.com/NethermindEth/juno/db/pebble"
	"github.com/NethermindEth/juno/jsonrpc"
	"github.com/NethermindEth/juno/pprof"
	"github.com/NethermindEth/juno/rpc"
	"github.com/NethermindEth/juno/service"
	adaptfeeder "github.com/NethermindEth/juno/starknetdata/feeder"
	"github.com/NethermindEth/juno/sync"
	"github.com/NethermindEth/juno/utils"
	"github.com/sourcegraph/conc"
)

const (
	defaultPprofPort = uint16(9080)
)

// Config is the top-level juno configuration.
type Config struct {
	LogLevel     utils.LogLevel `mapstructure:"log-level"`
	RPCPort      uint16         `mapstructure:"rpc-port"`
	DatabasePath string         `mapstructure:"db-path"`
	Network      utils.Network  `mapstructure:"network"`
	Pprof        bool           `mapstructure:"pprof"`
}

type Node struct {
	cfg        *Config
	db         db.DB
	blockchain *blockchain.Blockchain

	services []service.Service
	log      utils.Logger
}

// New sets the config and logger to the StarknetNode.
// Any errors while parsing the config on creating logger will be returned.
func New(cfg *Config) (*Node, error) {
	if cfg.DatabasePath == "" {
		dirPrefix, err := utils.DefaultDataDir()
		if err != nil {
			return nil, err
		}
		cfg.DatabasePath = filepath.Join(dirPrefix, cfg.Network.String())
	}
	log, err := utils.NewZapLogger(cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	return &Node{
		cfg: cfg,
		log: log,
	}, nil
}

func makeHTTP(port uint16, rpcHandler *rpc.Handler, log utils.SimpleLogger) *jsonrpc.HTTP {
	return jsonrpc.NewHTTP(port, []jsonrpc.Method{
		{
			Name:    "starknet_chainId",
			Handler: rpcHandler.ChainID,
		},
		{
			Name:    "starknet_blockNumber",
			Handler: rpcHandler.BlockNumber,
		},
		{
			Name:    "starknet_blockHashAndNumber",
			Handler: rpcHandler.BlockNumberAndHash,
		},
		{
			Name:    "starknet_getBlockWithTxHashes",
			Params:  []jsonrpc.Parameter{{Name: "block_id"}},
			Handler: rpcHandler.BlockWithTxHashes,
		},
		{
			Name:    "starknet_getBlockWithTxs",
			Params:  []jsonrpc.Parameter{{Name: "block_id"}},
			Handler: rpcHandler.BlockWithTxs,
		},
		{
			Name:    "starknet_getTransactionByHash",
			Params:  []jsonrpc.Parameter{{Name: "transaction_hash"}},
			Handler: rpcHandler.TransactionByHash,
		},
		{
			Name:    "starknet_getTransactionReceipt",
			Params:  []jsonrpc.Parameter{{Name: "transaction_hash"}},
			Handler: rpcHandler.TransactionReceiptByHash,
		},
		{
			Name:    "starknet_getBlockTransactionCount",
			Params:  []jsonrpc.Parameter{{Name: "block_id"}},
			Handler: rpcHandler.BlockTransactionCount,
		},
		{
			Name:    "starknet_getTransactionByBlockIdAndIndex",
			Params:  []jsonrpc.Parameter{{Name: "block_id"}, {Name: "index"}},
			Handler: rpcHandler.TransactionByBlockIDAndIndex,
		},
		{
			Name:    "starknet_getStateUpdate",
			Params:  []jsonrpc.Parameter{{Name: "block_id"}},
			Handler: rpcHandler.StateUpdate,
		},
	}, log)
}

// Run starts Juno node by opening the DB, initialising services.
// All the services blocking and any errors returned by service run function is logged.
// Run will wait for all services to return before exiting.
func (n *Node) Run(ctx context.Context) {
	n.log.Infow("Starting Juno...", "config", fmt.Sprintf("%+v", *n.cfg))

	dbLog, err := utils.NewZapLogger(utils.ERROR)
	if err != nil {
		n.log.Errorw("Error creating DB logger", "err", err)
		return
	}

	n.db, err = pebble.New(n.cfg.DatabasePath, dbLog)
	if err != nil {
		n.log.Errorw("Error opening DB", "err", err)
		return
	}

	defer func() {
		if closeErr := n.db.Close(); closeErr != nil {
			n.log.Errorw("Error while closing the DB", "err", closeErr)
		}
	}()

	n.blockchain = blockchain.New(n.db, n.cfg.Network)

	client := feeder.NewClient(n.cfg.Network.URL())
	synchronizer := sync.New(n.blockchain, adaptfeeder.New(client), n.log)

	http := makeHTTP(n.cfg.RPCPort, rpc.New(n.blockchain, n.cfg.Network), n.log)

	n.services = []service.Service{synchronizer, http}

	if n.cfg.Pprof {
		n.services = append(n.services, pprof.New(defaultPprofPort, n.log))
	}

	ctx, cancel := context.WithCancel(ctx)

	wg := conc.NewWaitGroup()
	for _, s := range n.services {
		s := s
		wg.Go(func() {
			if err = s.Run(ctx); err != nil {
				n.log.Errorw("Service error", "name", reflect.TypeOf(s), "err", err)
				cancel()
			}
		})
	}
	defer wg.Wait()

	<-ctx.Done()
	cancel()
	n.log.Infow("Shutting down Juno...")
}

func (n *Node) Config() Config {
	return *n.cfg
}
