package service

import (
	"reflect"
	"testing"
)

func TestEnableUnitsIncludesWifibroadcastMainService(t *testing.T) {
	tests := []struct {
		unit string
		want []string
	}{
		{
			unit: "wifibroadcast@gs",
			want: []string{"wifibroadcast.service", "wifibroadcast@gs.service"},
		},
		{
			unit: "wifibroadcast@drone.service",
			want: []string{"wifibroadcast.service", "wifibroadcast@drone.service"},
		},
		{
			unit: "rtsp@h265",
			want: []string{"rtsp@h265"},
		},
	}

	for _, tt := range tests {
		if got := enableUnits(tt.unit); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("enableUnits(%q) = %v, want %v", tt.unit, got, tt.want)
		}
	}
}
