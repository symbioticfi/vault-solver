package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestConfigCompositionCharacterization(t *testing.T) {
	t.Run("sorted solver catalog", func(t *testing.T) {
		want := []string{
			"3f-bridge-facilitator",
			"lifi-samechain",
			"redstone-oev",
			"rfq-filler",
			"uniswapx-filler",
		}
		if got := configuredSolverNames(); !slices.Equal(got, want) {
			t.Fatalf("solver catalog = %v, want %v", got, want)
		}
	})

	t.Run("committed example basenames", func(t *testing.T) {
		paths, err := filepath.Glob(filepath.Join("..", "..", "config", "*.example.yaml"))
		if err != nil {
			t.Fatalf("glob examples: %v", err)
		}
		got := make([]string, len(paths))
		for i, path := range paths {
			got[i] = filepath.Base(path)
		}
		want := []string{
			"3f.example.yaml",
			"lifi.example.yaml",
			"redstone-oev.example.yaml",
			"rfq.example.yaml",
			"uniswapx.example.yaml",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("committed example basenames = %v, want %v", got, want)
		}
	})

	t.Run("first invalid solver follows YAML order", func(t *testing.T) {
		tests := []struct {
			name string
			body string
			want func(string) string
		}{
			{
				name: "3F before RFQ",
				body: configCharacterizationPrefix + `
solvers:
  - name: 3f-bridge-facilitator
    config:
      apiBaseUrl: https://3f.example
      adapterFactory: "0x1111111111111111111111111111111111111111"
      strategy: {name: first, config: {}}
  - name: rfq-filler
    config:
      backendUrl: https://rfq.example
      backendSharedSecretEnv: RFQ_SECRET
      executor: "0x2222222222222222222222222222222222222222"
      solverMode: internal
      strategy: {name: second, config: {}}
`,
				want: func(path string) string {
					return `invalid config "` + path + `": solver "3f-bridge-facilitator" config: strategy: unknown 3F strategy "first" (registered: [default webhook])`
				},
			},
			{
				name: "RFQ before 3F",
				body: configCharacterizationPrefix + `
solvers:
  - name: rfq-filler
    config:
      backendUrl: https://rfq.example
      backendSharedSecretEnv: RFQ_SECRET
      executor: "0x2222222222222222222222222222222222222222"
      solverMode: internal
      strategy: {name: first, config: {}}
  - name: 3f-bridge-facilitator
    config:
      apiBaseUrl: https://3f.example
      adapterFactory: "0x1111111111111111111111111111111111111111"
      strategy: {name: second, config: {}}
`,
				want: func(path string) string {
					return `invalid config "` + path + `": solver "rfq-filler" config: strategy: unknown RFQ strategy "first" (registered: [default webhook])`
				},
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				path := writeConfigFile(t, test.body)
				err := validateConfigFile(path)
				if err == nil || err.Error() != test.want(path) {
					t.Fatalf("validateConfigFile error = %v, want %q", err, test.want(path))
				}
			})
		}
	})

	t.Run("OEV-only config does not require max fee", func(t *testing.T) {
		path := writeConfigFile(t, configCharacterizationPrefix+oevConfigCharacterization)
		if err := validateConfigFile(path); err != nil {
			t.Fatalf("validateConfigFile: %v", err)
		}
	})

	t.Run("mixed OEV and transaction solver requires max fee", func(t *testing.T) {
		path := writeConfigFile(t, configCharacterizationPrefix+oevConfigCharacterization+`
  - name: 3f-bridge-facilitator
    config:
      apiBaseUrl: https://3f.example
      adapterFactory: "0x1111111111111111111111111111111111111111"
`)
		err := validateConfigFile(path)
		want := `invalid config "` + path + `": txManager.maxFeeGwei must be finite and positive`
		if err == nil || err.Error() != want {
			t.Fatalf("validateConfigFile error = %v, want %q", err, want)
		}
	})

	t.Run("strategy validation precedes conditional transaction manager validation", func(t *testing.T) {
		path := writeConfigFile(t, configCharacterizationPrefix+`
solvers:
  - name: 3f-bridge-facilitator
    config:
      apiBaseUrl: https://3f.example
      adapterFactory: "0x1111111111111111111111111111111111111111"
      strategy: {name: missing, config: {}}
`)
		err := validateConfigFile(path)
		want := `invalid config "` + path + `": solver "3f-bridge-facilitator" config: strategy: unknown 3F strategy "missing" (registered: [default webhook])`
		if err == nil || err.Error() != want {
			t.Fatalf("validateConfigFile error = %v, want %q", err, want)
		}
	})

	t.Run("offline validation does not resolve secrets", func(t *testing.T) {
		for _, name := range []string{
			"OFFLINE_SIGNER_KEY",
			"OFFLINE_WEBHOOK_HEADER",
			"OFFLINE_RFQ_SECRET",
			"OFFLINE_OEV_API_KEY",
			"OFFLINE_LIFI_API_KEY",
			"OFFLINE_UNISWAPX_API_KEY",
		} {
			t.Setenv(name, "")
		}
		path := writeConfigFile(t, `
chain: {rpcUrl: https://rpc.example, chainId: 1}
signer: {keyEnv: OFFLINE_SIGNER_KEY}
txManager: {maxFeeGwei: 50}
solvers:
  - name: 3f-bridge-facilitator
    config:
      apiBaseUrl: https://3f.example
      adapterFactory: "0x1111111111111111111111111111111111111111"
      strategy:
        name: webhook
        config:
          url: https://strategy.example
          headers:
            Authorization: {env: OFFLINE_WEBHOOK_HEADER}
  - name: rfq-filler
    config:
      backendUrl: https://rfq.example
      backendSharedSecretEnv: OFFLINE_RFQ_SECRET
      executor: "0x2222222222222222222222222222222222222222"
      solverMode: internal
  - name: redstone-oev
    config:
      ws: {url: wss://oev.example, apiKeyEnv: OFFLINE_OEV_API_KEY}
      executor: "0x3333333333333333333333333333333333333333"
      adapter: "0x4444444444444444444444444444444444444444"
      callback: "0x5555555555555555555555555555555555555555"
      strategy:
        name: default
        config:
          morphoApiUrl: https://api.morpho.example/graphql
          bid: {bidEth: "0.0001"}
  - name: lifi-samechain
    config:
      orderServer:
        baseUrl: https://order.example
        wsUrl: wss://order.example
        apiKeyEnv: OFFLINE_LIFI_API_KEY
      inputSettler: "0x6666666666666666666666666666666666666666"
      outputSettler: "0x7777777777777777777777777777777777777777"
      executor: "0x8888888888888888888888888888888888888888"
      adapters: ["0x9999999999999999999999999999999999999999"]
  - name: uniswapx-filler
    config:
      reactor: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
      executor: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
      adapters: ["0xcccccccccccccccccccccccccccccccccccccccc"]
      quoteServer: {}
      orderServer:
        baseUrl: https://api.uniswap.org/v2
        apiKeyEnv: OFFLINE_UNISWAPX_API_KEY
        sources: {exclusiveV2: true}
`)
		if err := validateConfigFile(path); err != nil {
			t.Fatalf("validateConfigFile resolved an offline secret: %v", err)
		}
	})
}

const configCharacterizationPrefix = `
chain: {rpcUrl: https://rpc.example, chainId: 1}
signer: {keyEnv: OFFLINE_SIGNER_KEY}
`

const oevConfigCharacterization = `
solvers:
  - name: redstone-oev
    config:
      ws: {url: wss://oev.example, apiKeyEnv: OFFLINE_OEV_API_KEY}
      executor: "0x3333333333333333333333333333333333333333"
      adapter: "0x4444444444444444444444444444444444444444"
      callback: "0x5555555555555555555555555555555555555555"
      strategy:
        name: default
        config:
          morphoApiUrl: https://api.morpho.example/graphql
          bid: {bidEth: "0.0001"}
`
