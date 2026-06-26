package redstoneoev

import (
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

func decodeOperationData(data []byte) (operationData, error) {
	vals, err := operationDataArgs.Unpack(data)
	if err != nil {
		return operationData{}, errors.Errorf("decode operationData: %w", err)
	}
	if len(vals) != 1 {
		return operationData{}, errors.Errorf("decode operationData: got %d values, want 1", len(vals))
	}
	if out, ok := vals[0].(operationData); ok {
		return out, nil
	}
	return decodeOperationDataValue(reflect.ValueOf(vals[0]))
}

func decodeOperationDataValue(v reflect.Value) (operationData, error) {
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return operationData{}, errors.Errorf("decode operationData: got %s, want struct", v.Kind())
	}
	auth, err := decodeOperationAuthValue(v.FieldByName("Auth"))
	if err != nil {
		return operationData{}, err
	}
	legs, err := decodeOperationLegsValue(v.FieldByName("Legs"))
	if err != nil {
		return operationData{}, err
	}
	sigV := v.FieldByName("AuthSig")
	sig, ok := sigV.Interface().([]byte)
	if !ok {
		return operationData{}, errors.Errorf("decode operationData: authSig has type %s", sigV.Type())
	}
	return operationData{Auth: auth, Legs: legs, AuthSig: append([]byte(nil), sig...)}, nil
}

func decodeOperationAuthValue(v reflect.Value) (operationAuth, error) {
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return operationAuth{}, errors.Errorf("decode operationData auth: got %s, want struct", v.Kind())
	}
	key, ok := hashValue(v.FieldByName("AuctionKey"))
	if !ok {
		return operationAuth{}, errors.New("decode operationData auth: bad auctionKey")
	}
	bid, ok := bigValue(v.FieldByName("BidAmount"))
	if !ok {
		return operationAuth{}, errors.New("decode operationData auth: bad bidAmount")
	}
	minBundleProfit, ok := bigValue(v.FieldByName("MinBundleProfit"))
	if !ok {
		return operationAuth{}, errors.New("decode operationData auth: bad minBundleProfit")
	}
	return operationAuth{AuctionKey: key, BidAmount: bid, MinBundleProfit: minBundleProfit}, nil
}

func decodeOperationLegsValue(v reflect.Value) ([]callbackLeg, error) {
	if v.Kind() != reflect.Slice {
		return nil, errors.Errorf("decode operationData legs: got %s, want slice", v.Kind())
	}
	out := make([]callbackLeg, v.Len())
	for i := 0; i < v.Len(); i++ {
		legV := v.Index(i)
		if legV.Kind() == reflect.Pointer {
			legV = legV.Elem()
		}
		if legV.Kind() != reflect.Struct {
			return nil, errors.Errorf("decode operationData leg %d: got %s, want struct", i, legV.Kind())
		}
		id, ok := hashValue(legV.FieldByName("MarketId"))
		if !ok {
			return nil, errors.Errorf("decode operationData leg %d: bad marketId", i)
		}
		borrower, ok := addressValue(legV.FieldByName("Borrower"))
		if !ok {
			return nil, errors.Errorf("decode operationData leg %d: bad borrower", i)
		}
		maxSeize, ok := bigValue(legV.FieldByName("MaxSeizeAssets"))
		if !ok {
			return nil, errors.Errorf("decode operationData leg %d: bad maxSeizeAssets", i)
		}
		minProfit, ok := bigValue(legV.FieldByName("MinProfit"))
		if !ok {
			return nil, errors.Errorf("decode operationData leg %d: bad minProfit", i)
		}
		out[i] = callbackLeg{MarketId: id, Borrower: borrower, MaxSeizeAssets: maxSeize, MinProfit: minProfit}
	}
	return out, nil
}

func hashValue(v reflect.Value) (common.Hash, bool) {
	if !v.IsValid() {
		return common.Hash{}, false
	}
	if h, ok := v.Interface().(common.Hash); ok {
		return h, true
	}
	if v.Kind() != reflect.Array || v.Len() != common.HashLength {
		return common.Hash{}, false
	}
	var h common.Hash
	for i := 0; i < common.HashLength; i++ {
		h[i] = byte(v.Index(i).Uint())
	}
	return h, true
}

func addressValue(v reflect.Value) (common.Address, bool) {
	if !v.IsValid() {
		return common.Address{}, false
	}
	if a, ok := v.Interface().(common.Address); ok {
		return a, true
	}
	if v.Kind() != reflect.Array || v.Len() != common.AddressLength {
		return common.Address{}, false
	}
	var a common.Address
	for i := 0; i < common.AddressLength; i++ {
		a[i] = byte(v.Index(i).Uint())
	}
	return a, true
}

func bigValue(v reflect.Value) (*big.Int, bool) {
	if !v.IsValid() {
		return nil, false
	}
	b, ok := v.Interface().(*big.Int)
	if !ok || b == nil {
		return nil, false
	}
	return new(big.Int).Set(b), true
}
