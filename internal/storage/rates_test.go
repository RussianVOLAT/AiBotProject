package storage

import (
	"math"
	"testing"
)

// Table-driven test — стандартный паттерн в Go: одна функция теста,
// таблица case'ов вместо копипасты одинаковых t.Run блоков.
func TestCalcPercentChange(t *testing.T) {
	tests := []struct {
		name     string
		oldPrice float64
		newPrice float64
		wantPct  float64
	}{
		{
			name:     "price grew by 10 percent",
			oldPrice: 100,
			newPrice: 110,
			wantPct:  10,
		},
		{
			name:     "price dropped by 10 percent",
			oldPrice: 100,
			newPrice: 90,
			wantPct:  -10,
		},
		{
			name:     "no change",
			oldPrice: 65000,
			newPrice: 65000,
			wantPct:  0,
		},
		{
			name:     "zero old price does not panic or divide by zero",
			oldPrice: 0,
			newPrice: 100,
			wantPct:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcPercentChange(tt.oldPrice, tt.newPrice)
			if math.Abs(got-tt.wantPct) > 0.0001 {
				t.Errorf("calcPercentChange(%v, %v) = %v, want %v",
					tt.oldPrice, tt.newPrice, got, tt.wantPct)
			}
		})
	}
}
