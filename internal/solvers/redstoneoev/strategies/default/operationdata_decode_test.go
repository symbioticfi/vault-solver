package defaultstrategy

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/go-errors/errors"
)

func decodeOperationData(data []byte) (operationData, error) {
	vals, err := operationDataArgs.Unpack(data)
	if err != nil {
		return operationData{}, errors.Errorf("decode operationData: %w", err)
	}
	return *abi.ConvertType(vals[0], new(operationData)).(*operationData), nil
}
