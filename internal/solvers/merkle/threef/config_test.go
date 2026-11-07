package threef

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const validConfig = `mode: simulate
executor: "0x0000000000000000000000000000000000000010"
policy: "0x0000000000000000000000000000000000000050"
curator:
  url: http://127.0.0.1:1/api/v1
url: https://bf.3f.xyz
apiKeyEnv: THREEF_CURATOR_API_KEY
broker: "0x0000000000000000000000000000000000000040"
`

func configNode(t *testing.T, text string) yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(text), &n); err != nil {
		t.Fatal(err)
	}
	return *n.Content[0]
}
func TestConfigValidation(t *testing.T) {
	for name, raw := range map[string]string{
		"default simulate": strings.Replace(validConfig, "mode: simulate\n", "", 1),
		"send":             strings.Replace(validConfig, "mode: simulate", "mode: send", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(configNode(t, raw)); err != nil {
				t.Fatal(err)
			}
		})
	}
	for name, raw := range map[string]string{
		"mode":           strings.Replace(validConfig, "mode: simulate", "mode: unsafe", 1),
		"removed state":  validConfig + "stateDir: state/threef-merkle\n",
		"zero executor":  strings.Replace(validConfig, "0000000000000000000000000000000000000010", "0000000000000000000000000000000000000000", 1),
		"missing policy": strings.Replace(validConfig, `policy: "0x0000000000000000000000000000000000000050"`, "", 1),
		"zero policy":    strings.Replace(validConfig, "0000000000000000000000000000000000000050", "0000000000000000000000000000000000000000", 1),
		"invalid policy": strings.Replace(validConfig, "0000000000000000000000000000000000000050", "bad", 1),
		"zero broker":    strings.Replace(validConfig, "0000000000000000000000000000000000000040", "0000000000000000000000000000000000000000", 1),
		"curator":        strings.Replace(validConfig, "http://127.0.0.1:1/api/v1", "", 1),
		"url":            strings.Replace(validConfig, "https://bf.3f.xyz", "https://bf.3f.xyz?key=private", 1),
		"key":            strings.Replace(validConfig, "THREEF_CURATOR_API_KEY", "invalid-env-name", 1),
		"inline key":     validConfig + "apiKey: secret\n",
		"manual amount":  validConfig + "principal: '100'\n",
		"manual account": validConfig + "account: '0x20'\n",
		"strategies":     validConfig + "strategies: []\n",
		"nested threeF":  validConfig + "threeF: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(configNode(t, raw)); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestConfigRequiresOneExecutorAndRejectsArray(t *testing.T) {
	const entry = `executor: "0x0000000000000000000000000000000000000010"`
	for name, tc := range map[string]struct{ value, want string }{
		"missing": {"", "executor"}, "zero": {`executor: "0x0000000000000000000000000000000000000000"`, "executor"},
		"malformed": {`executor: "invalid"`, "executor"}, "array": {`executors: ["0x0000000000000000000000000000000000000010"]`, "field executors not found"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseConfig(configNode(t, strings.Replace(validConfig, entry, tc.value, 1)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q got %v", tc.want, err)
			}
		})
	}
}
