package liquidlane

import (
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestUnauthorizedAdapters(t *testing.T) {
	adapterA := common.HexToAddress("0x000000000000000000000000000000000000000a")
	adapterB := common.HexToAddress("0x000000000000000000000000000000000000000b")
	adapterC := common.HexToAddress("0x000000000000000000000000000000000000000c")
	routes := []Route{
		{Adapter: adapterA},
		{Adapter: adapterB},
		{Adapter: adapterB},
		{Adapter: adapterC},
	}

	got := UnauthorizedAdapters(routes, []Route{{Adapter: adapterB}})
	want := []common.Address{adapterA, adapterC}
	if !slices.Equal(got, want) {
		t.Fatalf("UnauthorizedAdapters() = %v, want %v", got, want)
	}
}
