package rfq

import "testing"

func TestDirectQuoteParsingRejectsMalformedDiscountID(t *testing.T) {
	for _, id := range []string{"0x01", "not-a-hash", "0xgg00000000000000000000000000000000000000000000000000000000000000"} {
		t.Run(id, func(t *testing.T) {
			request := validQuoteBody()
			request.Adapters[0].DiscountID = &id
			if _, err := request.toStrategy(1); err == nil {
				t.Fatal("malformed id passed without HTTP schema validation")
			}
		})
	}
}
