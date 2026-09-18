package bitcoin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestFeeEstimateResponse(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      FeeEstimate
	}{
		{"returned target", `{"feerate":0.0001,"blocks":2}`, FeeEstimate{Blocks: 2, SatPerVB: 10}},
		{"minimum", `{"feerate":0.000001,"blocks":6}`, FeeEstimate{Blocks: 6, SatPerVB: 1}},
		{"no data", `{"errors":["Insufficient data"],"blocks":0}`, FeeEstimate{}},
		{"error with rate", `{"feerate":0.0001,"blocks":2,"errors":["unavailable"]}`, FeeEstimate{}},
		{"missing target", `{"feerate":0.0001}`, FeeEstimate{}},
		{"invalid target", `{"feerate":0.0001,"blocks":1}`, FeeEstimate{}},
		{"missing rate", `{"blocks":2}`, FeeEstimate{}},
		{"invalid rate", `{"feerate":-1,"blocks":2}`, FeeEstimate{}},
		{"malformed", `{`, FeeEstimate{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readFeeEstimate(1, func(result any) error { return json.Unmarshal([]byte(tc.raw), result) })
			if got != tc.want || (err != nil) != (tc.want == FeeEstimate{}) {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
	got, err := readFeeEstimate(1, func(any) error { return context.Canceled })
	if !errors.Is(err, context.Canceled) || got != (FeeEstimate{}) {
		t.Fatal("transport cancellation lost at estimate boundary")
	}
}
