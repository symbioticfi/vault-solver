package uniswapx

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

type rawDexConfig struct {
	InputToken          string         `yaml:"inputToken"`
	BridgeToken         string         `yaml:"bridgeToken"`
	Router              string         `yaml:"router"`
	Quoter              string         `yaml:"quoter"`
	Factory             string         `yaml:"factory"`
	Treasury            string         `yaml:"treasury"`
	Outputs             []rawDexOutput `yaml:"outputs"`
	MaxBridgeAmount     string         `yaml:"maxBridgeAmount"`
	MinProfitBridge     string         `yaml:"minProfitBridge"`
	SlippageBps         int            `yaml:"slippageBps"`
	GasUnitsBudget      uint64         `yaml:"gasUnitsBudget"`
	PlanValidity        string         `yaml:"planValidity"`
	RPCTimeout          string         `yaml:"rpcTimeout"`
	MaxConcurrentQuotes int            `yaml:"maxConcurrentQuotes"`
}

type rawDexOutput struct {
	Token string `yaml:"token"`
	Fee   uint32 `yaml:"fee"`
}

// DexConfig is an explicitly enabled, bounded single-hop prototype. Bridge amounts are atomic units.
type DexConfig struct {
	InputToken, BridgeToken, Router, Quoter, Factory, Treasury common.Address
	Outputs                                                    [2]DexOutput
	MaxBridgeAmount, MinProfitBridge                           *big.Int
	SlippageBps                                                int
	GasUnitsBudget                                             uint64
	PlanValidity, RPCTimeout                                   time.Duration
	MaxConcurrentQuotes                                        int
}

type DexOutput struct {
	Token common.Address
	Fee   uint32
}

func parseDexConfig(raw *rawDexConfig) (*DexConfig, error) {
	if raw == nil {
		return nil, nil
	}
	c := &DexConfig{SlippageBps: raw.SlippageBps, GasUnitsBudget: raw.GasUnitsBudget, MaxConcurrentQuotes: raw.MaxConcurrentQuotes}
	for _, field := range []struct {
		name, raw string
		to        *common.Address
	}{
		{"inputToken", raw.InputToken, &c.InputToken}, {"bridgeToken", raw.BridgeToken, &c.BridgeToken},
		{"router", raw.Router, &c.Router}, {"quoter", raw.Quoter, &c.Quoter},
		{"factory", raw.Factory, &c.Factory}, {"treasury", raw.Treasury, &c.Treasury},
	} {
		address, err := parse.NonZeroAddress(field.raw, "dex."+field.name)
		if err != nil {
			return nil, err
		}
		*field.to = address
	}
	if len(raw.Outputs) != 2 || c.InputToken == c.BridgeToken || c.SlippageBps < 0 || c.SlippageBps > 1_000 ||
		c.GasUnitsBudget < 100_000 || c.GasUnitsBudget > 10_000_000 || c.MaxConcurrentQuotes < 1 || c.MaxConcurrentQuotes > 16 {
		return nil, errors.New("dex requires two outputs, distinct input/bridge, slippage [0,1000], gas budget [100000,10000000] and concurrency [1,16]")
	}
	for i, output := range raw.Outputs {
		token, err := parse.NonZeroAddress(output.Token, "dex.outputs.token")
		if err != nil {
			return nil, err
		}
		if token == c.InputToken || token == c.BridgeToken || output.Fee == 0 || output.Fee > 0xffffff ||
			i == 1 && token == c.Outputs[0].Token {
			return nil, errors.New("dex output tokens must be distinct and pool fee must be a positive uint24")
		}
		c.Outputs[i] = DexOutput{Token: token, Fee: output.Fee}
	}
	var ok bool
	if c.MaxBridgeAmount, ok = new(big.Int).SetString(raw.MaxBridgeAmount, 10); !ok || c.MaxBridgeAmount.Sign() <= 0 || c.MaxBridgeAmount.BitLen() > 256 {
		return nil, errors.New("dex.maxBridgeAmount must be positive atomic units")
	}
	if c.MinProfitBridge, ok = new(big.Int).SetString(raw.MinProfitBridge, 10); !ok || c.MinProfitBridge.Sign() < 0 || c.MinProfitBridge.BitLen() > 256 {
		return nil, errors.New("dex.minProfitBridge must be non-negative atomic units")
	}
	if c.MinProfitBridge.Cmp(c.MaxBridgeAmount) >= 0 {
		return nil, errors.New("dex profit floor must be below bridge cap")
	}
	var err error
	if c.PlanValidity, err = time.ParseDuration(raw.PlanValidity); err != nil || c.PlanValidity <= 0 || c.PlanValidity > 30*time.Second {
		return nil, errors.New("dex.planValidity must be in (0,30s]")
	}
	if c.RPCTimeout, err = time.ParseDuration(raw.RPCTimeout); err != nil || c.RPCTimeout <= 0 || c.RPCTimeout > 400*time.Millisecond {
		return nil, errors.New("dex.rpcTimeout must be in (0,400ms]")
	}
	return c, nil
}

func (c *DexConfig) matches(in, out common.Address) bool {
	return c != nil && in == c.InputToken && (out == c.Outputs[0].Token || out == c.Outputs[1].Token)
}

func (c *DexConfig) output(token common.Address) (DexOutput, bool) {
	if c != nil {
		for _, out := range c.Outputs {
			if out.Token == token {
				return out, true
			}
		}
	}
	return DexOutput{}, false
}
