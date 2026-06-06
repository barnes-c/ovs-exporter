package otel

import (
	"reflect"
	"testing"
)

func TestParsePushExporters(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{"otlp only", "otlp", []string{"otlp"}, false},
		{"otlp and console", "otlp,console", []string{"otlp", "console"}, false},
		{"whitespace tolerated", "otlp, console", []string{"otlp", "console"}, false},
		{"dedup", "otlp,otlp", []string{"otlp"}, false},
		{"empty entries skipped", "otlp,,console", []string{"otlp", "console"}, false},
		{"prometheus ignored (always-on)", "prometheus,otlp", []string{"otlp"}, false},
		{"prometheus only yields no push", "prometheus", nil, false},
		{"none disables all push", "none", nil, false},
		{"none with whitespace", "  none  ", nil, false},
		{"empty input yields no push", "", nil, false},
		{"only commas yields no push", ",,,", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePushExporters(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
