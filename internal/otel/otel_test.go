package otel

import (
	"reflect"
	"testing"
)

func TestParseMetricsExporters(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{"prometheus only", "prometheus", []string{"prometheus"}, false},
		{"prometheus and otlp", "prometheus,otlp", []string{"prometheus", "otlp"}, false},
		{"whitespace tolerated", "prometheus, otlp", []string{"prometheus", "otlp"}, false},
		{"dedup", "prometheus,prometheus", []string{"prometheus"}, false},
		{"empty entries skipped", "prometheus,,otlp", []string{"prometheus", "otlp"}, false},
		{"none disables all", "none", nil, false},
		{"none with whitespace", "  none  ", nil, false},
		{"empty input errors", "", nil, true},
		{"only commas errors", ",,,", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMetricsExporters(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
